package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	ProjectID      uint
	ProjectCode    string
	VarName        string
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

type VariableWriteError struct {
	Code    string
	Message string
	Status  int
}

func (e *VariableWriteError) Error() string {
	return e.Message
}

type VariableWriteResult struct {
	VarID            int64           `json:"var_id"`
	VarIDText        string          `json:"var_id_text"`
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
	ProjectConfirmed bool            `json:"project_confirmed,omitempty"`
	KIO              *KIOWriteResult `json:"kio,omitempty"`
}

func NewVariableWriteService(repo *database.Repository, tags *pipeline.TagManager, kioService *KIOWriteService, flows *pipeline.TaskFlowExecutor) *VariableWriteService {
	return &VariableWriteService{repo: repo, tags: tags, kio: kioService, flows: flows}
}

func (s *VariableWriteService) Write(ctx context.Context, input VariableWriteInput) (VariableWriteResult, error) {
	varID, err := s.resolveVarID(input)
	if err != nil {
		return VariableWriteResult{}, err
	}
	tag, ok := s.tags.Get(varID)
	if !ok {
		return VariableWriteResult{}, variableWriteError("variable_not_found", fmt.Sprintf("variable %d not found", varID), http.StatusNotFound)
	}
	if _, ok := anyValuePresent(input.Value); !ok {
		return VariableWriteResult{}, variableWriteError("invalid_payload", "value is required", http.StatusBadRequest)
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
		return VariableWriteResult{}, variableWriteError("variable_not_writable", fmt.Sprintf("variable %d is not writable", varID), http.StatusBadRequest)
	}
	if err := validateWriteValue(tag.Config, input.Value); err != nil {
		return VariableWriteResult{}, err
	}
	gatewayID := tag.Config.WriteSourceID
	if gatewayID == 0 {
		gatewayID = tag.Config.GatewayID
	}
	if gatewayID == 0 {
		return VariableWriteResult{}, variableWriteError("write_config_missing", "write_source_id or gateway_id is required", http.StatusBadRequest)
	}
	if strings.TrimSpace(tag.Config.WritePath) == "" {
		return VariableWriteResult{}, variableWriteError("write_config_missing", "write_path is required", http.StatusBadRequest)
	}
	if s.kio == nil {
		return VariableWriteResult{}, variableWriteError("write_service_unavailable", "kio write service is not available", http.StatusBadRequest)
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
		VarIDText:        strconv.FormatInt(tag.Config.VarID, 10),
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

func (s *VariableWriteService) resolveVarID(input VariableWriteInput) (int64, error) {
	if input.VarID != 0 {
		return input.VarID, nil
	}
	varName := strings.TrimSpace(input.VarName)
	if varName == "" {
		return 0, variableWriteError("invalid_payload", "var_id or project_id/project_code + var_name is required", http.StatusBadRequest)
	}
	projectID := input.ProjectID
	projectCode := strings.TrimSpace(input.ProjectCode)
	if projectID == 0 && projectCode != "" {
		if s.repo == nil {
			return 0, variableWriteError("write_service_unavailable", "project resolver is not available", http.StatusBadRequest)
		}
		project, err := s.repo.GetProjectByCode(projectCode)
		if err != nil {
			return 0, variableWriteError("variable_not_found", fmt.Sprintf("project_code %q not found", projectCode), http.StatusNotFound)
		}
		projectID = project.ID
	}
	if projectID == 0 {
		return 0, variableWriteError("invalid_payload", "project_id or project_code is required when var_id is omitted", http.StatusBadRequest)
	}
	matches := make([]int64, 0, 1)
	for _, tag := range s.tags.ForProject(projectID) {
		if strings.TrimSpace(tag.Config.VarName) == varName {
			matches = append(matches, tag.Config.VarID)
		}
	}
	if len(matches) == 0 {
		return 0, variableWriteError("variable_not_found", fmt.Sprintf("variable %q not found in project %d", varName, projectID), http.StatusNotFound)
	}
	if len(matches) > 1 {
		return 0, variableWriteError("ambiguous_variable", fmt.Sprintf("variable name %q is duplicated in project %d", varName, projectID), http.StatusConflict)
	}
	return matches[0], nil
}

func variableWriteError(code string, message string, status int) error {
	return &VariableWriteError{Code: code, Message: message, Status: status}
}

func VariableWriteErrorStatus(err error) (int, bool) {
	var typed *VariableWriteError
	if errors.As(err, &typed) {
		return typed.Status, true
	}
	return 0, false
}

func VariableWriteErrorCode(err error) (string, bool) {
	var typed *VariableWriteError
	if errors.As(err, &typed) {
		return typed.Code, true
	}
	return "", false
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
		VarIDText:      strconv.FormatInt(tag.Config.VarID, 10),
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
