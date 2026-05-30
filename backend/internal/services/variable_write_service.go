package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
	"spindle-edge/backend/internal/protocol/kio"
)

type VariableWriteService struct {
	repo  *database.Repository
	tags  *pipeline.TagManager
	kio   *KIOWriteService
	flows *pipeline.TaskFlowExecutor
}

type VariableWriteInput struct {
	VarID          int64
	Value          any
	Quality        int
	Trigger        bool
	WaitAck        bool
	AckTimeoutSec  int
	OriginFlowID   uint64
	OriginRunID    uint64
	Depth          int
	MaxDepth       int
	AllowReentrant bool
	RequestID      string
}

type VariableWriteResult struct {
	VarID            int64           `json:"var_id"`
	VarName          string          `json:"var_name"`
	SourceType       string          `json:"source_type"`
	ProjectID        *uint           `json:"project_id,omitempty"`
	Value            any             `json:"value"`
	Quality          int             `json:"quality"`
	UpdatedAt        time.Time       `json:"updated_at,omitempty"`
	Triggered        int             `json:"triggered"`
	OriginFlowID     uint64          `json:"origin_flow_id,omitempty"`
	OriginRunID      uint64          `json:"origin_run_id,omitempty"`
	Depth            int             `json:"depth"`
	NextDepth        int             `json:"next_depth"`
	MaxDepth         int             `json:"max_depth"`
	AllowReentrant   bool            `json:"allow_reentrant"`
	RequestID        string          `json:"request_id,omitempty"`
	BrokerAccepted   bool            `json:"broker_accepted,omitempty"`
	ProjectConfirmed bool            `json:"Project_confirmed,omitempty"`
	KIO              *KIOWriteResult `json:"kio,omitempty"`
}

func NewVariableWriteService(repo *database.Repository, tags *pipeline.TagManager, kioService *KIOWriteService, flows *pipeline.TaskFlowExecutor) *VariableWriteService {
	return &VariableWriteService{repo: repo, tags: tags, kio: kioService, flows: flows}
}

func (s *VariableWriteService) Write(ctx context.Context, input VariableWriteInput) (VariableWriteResult, error) {
	if input.VarID == 0 {
		return VariableWriteResult{}, fmt.Errorf("var_id is required")
	}
	tag, ok := s.tags.Get(input.VarID)
	if !ok {
		return VariableWriteResult{}, fmt.Errorf("variable %d not found", input.VarID)
	}
	if _, ok := anyValuePresent(input.Value); !ok {
		return VariableWriteResult{}, fmt.Errorf("value is required")
	}
	quality := input.Quality
	if quality == 0 {
		quality = 1
	}
	if input.MaxDepth <= 0 {
		input.MaxDepth = 1
	}
	if tag.Config.SourceType == models.TagSourceVirtual {
		return s.writeVirtual(tag, input, quality), nil
	}
	if !canWritePhysicalVariable(tag.Config) {
		return VariableWriteResult{}, fmt.Errorf("variable %d is not writable", input.VarID)
	}
	if err := validateWriteValue(tag.Config, input.Value); err != nil {
		return VariableWriteResult{}, err
	}
	gatewayID := tag.Config.WriteSourceID
	if gatewayID == 0 {
		gatewayID = tag.Config.GatewayID
	}
	if gatewayID == 0 {
		return VariableWriteResult{}, fmt.Errorf("write_source_id or gateway_id is required")
	}
	if strings.TrimSpace(tag.Config.WritePath) == "" {
		return VariableWriteResult{}, fmt.Errorf("write_path is required")
	}
	if s.kio == nil {
		return VariableWriteResult{}, fmt.Errorf("kio write service is not available")
	}
	writeValue := normalizeWriteValue(tag.Config, input.Value)
	kioResult, err := s.kio.Write(ctx, KIOWriteInput{
		GatewayID:     gatewayID,
		Values:        []kio.WriteValue{{Name: tag.Config.WritePath, Value: writeValue}},
		WaitAck:       input.WaitAck,
		AckTimeoutSec: input.AckTimeoutSec,
	})
	result := VariableWriteResult{
		VarID:            tag.Config.VarID,
		VarName:          tag.Config.VarName,
		SourceType:       tag.Config.SourceType,
		ProjectID:        tag.Config.ProjectID,
		Value:            writeValue,
		Quality:          quality,
		RequestID:        input.RequestID,
		BrokerAccepted:   kioResult.BrokerAccepted,
		ProjectConfirmed: kioResult.ProjectConfirmed,
		KIO:              &kioResult,
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *VariableWriteService) writeVirtual(tag *models.Tag, input VariableWriteInput, quality int) VariableWriteResult {
	now := time.Now()
	if models.IsStringDataType(tag.Config.DataType) {
		tag.UpdateString(fmt.Sprint(input.Value), now, quality)
	} else {
		tag.UpdateNumeric(toFloat64(input.Value), now, quality)
	}
	triggered := 0
	nextDepth := input.Depth + 1
	if input.Trigger && s.flows != nil && nextDepth <= input.MaxDepth {
		projectID := uint(0)
		if tag.Config.ProjectID != nil {
			projectID = *tag.Config.ProjectID
		}
		eventValue := input.Value
		if !models.IsStringDataType(tag.Config.DataType) {
			eventValue = toFloat64(input.Value)
		}
		triggered = s.flows.Trigger(pipeline.TaskFlowEvent{
			TriggerType:    models.TaskFlowTriggerDataChange,
			ProjectID:      projectID,
			TriggerVarID:   tag.Config.VarID,
			TriggerValue:   eventValue,
			GatewayID:      tag.Config.GatewayID,
			Topic:          tag.Config.SourceTopic,
			At:             now,
			OriginFlowID:   input.OriginFlowID,
			OriginRunID:    input.OriginRunID,
			Depth:          nextDepth,
			MaxDepth:       input.MaxDepth,
			AllowReentrant: input.AllowReentrant,
			RequestID:      input.RequestID,
		})
	}
	return VariableWriteResult{
		VarID:          tag.Config.VarID,
		VarName:        tag.Config.VarName,
		SourceType:     tag.Config.SourceType,
		ProjectID:      tag.Config.ProjectID,
		Value:          input.Value,
		Quality:        quality,
		UpdatedAt:      now,
		Triggered:      triggered,
		OriginFlowID:   input.OriginFlowID,
		OriginRunID:    input.OriginRunID,
		Depth:          input.Depth,
		NextDepth:      nextDepth,
		MaxDepth:       input.MaxDepth,
		AllowReentrant: input.AllowReentrant,
		RequestID:      input.RequestID,
	}
}

func canWritePhysicalVariable(cfg models.TagConfig) bool {
	mode := strings.ToUpper(strings.TrimSpace(cfg.RWMode))
	return cfg.Writable && (mode == models.RWModeWrite || mode == models.RWModeReadWrite)
}

func validateWriteValue(cfg models.TagConfig, value any) error {
	if !models.IsStringDataType(cfg.WriteDataType) && !models.IsStringDataType(cfg.DataType) {
		numeric := toFloat64(value)
		if cfg.WriteMin != nil && numeric < *cfg.WriteMin {
			return fmt.Errorf("value %.6g is below write_min %.6g", numeric, *cfg.WriteMin)
		}
		if cfg.WriteMax != nil && numeric > *cfg.WriteMax {
			return fmt.Errorf("value %.6g is above write_max %.6g", numeric, *cfg.WriteMax)
		}
	}
	if strings.TrimSpace(cfg.WriteEnum) == "" {
		return nil
	}
	allowed := parseWriteEnum(cfg.WriteEnum)
	if len(allowed) == 0 {
		return nil
	}
	key := fmt.Sprint(value)
	if _, ok := allowed[key]; ok {
		return nil
	}
	return fmt.Errorf("value %q is not in write_enum", key)
}

func parseWriteEnum(raw string) map[string]struct{} {
	raw = strings.TrimSpace(raw)
	out := map[string]struct{}{}
	var items []any
	if strings.HasPrefix(raw, "[") && json.Unmarshal([]byte(raw), &items) == nil {
		for _, item := range items {
			out[fmt.Sprint(item)] = struct{}{}
		}
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = struct{}{}
		}
	}
	return out
}

func normalizeWriteValue(cfg models.TagConfig, value any) any {
	dataType := strings.ToUpper(strings.TrimSpace(firstNonEmpty(cfg.WriteDataType, cfg.DataType)))
	switch dataType {
	case "BOOL", "BOOLEAN":
		return toFloat64(value) != 0
	case "INT", "INT16", "INT32", "INT64", "INTEGER", "LONG":
		return int64(toFloat64(value))
	case "FLOAT", "DOUBLE", "REAL", "NUMBER":
		return toFloat64(value)
	default:
		return fmt.Sprint(value)
	}
}

func anyValuePresent(value any) (any, bool) {
	if value == nil {
		return nil, false
	}
	return value, true
}

func toFloat64(value any) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case uint:
		return float64(v)
	case uint64:
		return float64(v)
	case uint32:
		return float64(v)
	case float64:
		return v
	case float32:
		return float64(v)
	case bool:
		if v {
			return 1
		}
		return 0
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return parsed
		}
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "on":
			return 1
		case "false", "no", "off":
			return 0
		}
		return 0
	default:
		return 0
	}
}
