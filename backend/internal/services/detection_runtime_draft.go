package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
)

type DetectionRuntimeDraftResult struct {
	CustomItems   []models.DetectionStandardItem
	ProcessParams any
	Clear         func()
}

type detectionPreloadDraftData struct {
	StandardID    uint             `json:"standard_id"`
	ConfigHash    string           `json:"config_hash"`
	Items         []map[string]any `json:"items"`
	ProcessParams any              `json:"process_params"`
}

func ResolveDetectionRuntimeDraft(drafts *RuntimeDraftService, repo *database.Repository, projectID uint, ref database.RuntimeDraftReference, customItemsPresent bool) (DetectionRuntimeDraftResult, error) {
	if drafts == nil {
		return DetectionRuntimeDraftResult{}, fmt.Errorf("runtime draft service is not available")
	}
	namespace := strings.TrimSpace(ref.Namespace)
	if namespace == "" {
		namespace = RuntimeDraftNamespaceStationDetectionPreload
	}
	if namespace != RuntimeDraftNamespaceStationDetectionPreload {
		return DetectionRuntimeDraftResult{}, fmt.Errorf("unsupported runtime draft namespace")
	}
	if ref.Revision <= 0 {
		return DetectionRuntimeDraftResult{}, fmt.Errorf("runtime_draft.revision is required")
	}
	if customItemsPresent {
		return DetectionRuntimeDraftResult{}, fmt.Errorf("runtime_draft and custom_items cannot both be set")
	}
	draft, err := drafts.Get(namespace, RuntimeDraftScopeProject, strconv.FormatUint(uint64(projectID), 10))
	if err != nil {
		return DetectionRuntimeDraftResult{}, err
	}
	if draft.Revision != ref.Revision {
		return DetectionRuntimeDraftResult{}, ErrRuntimeDraftRevisionConflict
	}
	var data detectionPreloadDraftData
	if err := json.Unmarshal(draft.Data, &data); err != nil {
		return DetectionRuntimeDraftResult{}, fmt.Errorf("runtime draft data is invalid")
	}
	if data.StandardID == 0 {
		return DetectionRuntimeDraftResult{}, fmt.Errorf("runtime draft standard_id is required")
	}
	if repo != nil {
		standard, err := repo.GetDetectionStandard(data.StandardID)
		if err != nil {
			return DetectionRuntimeDraftResult{}, err
		}
		if strings.TrimSpace(data.ConfigHash) != "" && strings.TrimSpace(standard.ConfigHash) != strings.TrimSpace(data.ConfigHash) {
			return DetectionRuntimeDraftResult{}, fmt.Errorf("%w: config_hash", ErrRuntimeDraftStale)
		}
	}
	items, err := detectionRuntimeDraftItems(data.Items)
	if err != nil {
		return DetectionRuntimeDraftResult{}, err
	}
	if len(items) == 0 {
		return DetectionRuntimeDraftResult{}, fmt.Errorf("runtime draft items are required")
	}
	return DetectionRuntimeDraftResult{
		CustomItems:   items,
		ProcessParams: data.ProcessParams,
		Clear: func() {
			drafts.ClearIfRevision(namespace, RuntimeDraftScopeProject, strconv.FormatUint(uint64(projectID), 10), ref.Revision)
		},
	}, nil
}

func detectionRuntimeDraftItems(rawItems []map[string]any) ([]models.DetectionStandardItem, error) {
	items := make([]models.DetectionStandardItem, 0, len(rawItems))
	seen := make(map[int64]struct{}, len(rawItems))
	for _, itemMap := range rawItems {
		varID := int64FromDraftValue(firstNonNilDraftValue(itemMap["var_id_text"], itemMap["var_id"]))
		if varID == 0 {
			return nil, fmt.Errorf("custom_items.var_id is required")
		}
		if _, ok := seen[varID]; ok {
			return nil, fmt.Errorf("custom_items contains duplicate var_id %d", varID)
		}
		seen[varID] = struct{}{}
		item := models.DetectionStandardItem{
			VarID:           varID,
			VarName:         strings.TrimSpace(stringFromDraftValue(itemMap["var_name"])),
			DisplayName:     stringFromDraftValue(itemMap["display_name"]),
			DisplayNameEN:   stringFromDraftValue(itemMap["display_name_en"]),
			DisplayNameJA:   stringFromDraftValue(itemMap["display_name_ja"]),
			CheckEnabled:    boolFromDraftValue(itemMap["check_enabled"], true),
			AlarmEnabled:    boolFromDraftValue(itemMap["alarm_enabled"], true),
			StoreEnabled:    boolFromDraftValue(itemMap["store_enabled"], true),
			CheckCycleMS:    int(floatFromDraftValue(itemMap["check_cycle_ms"])),
			CheckOnStart:    boolFromDraftValue(itemMap["check_on_start"], true),
			Required:        boolFromDraftValue(itemMap["required"], false),
			CheckMethod:     firstNonEmpty(strings.TrimSpace(stringFromDraftValue(itemMap["check_method"])), models.CheckMethodNumericRange),
			TargetValue:     strings.TrimSpace(stringFromDraftValue(itemMap["target_value"])),
			LimitLL:         floatPointerFromDraftValue(itemMap["limit_ll"]),
			LimitL:          floatPointerFromDraftValue(itemMap["limit_l"]),
			LimitH:          floatPointerFromDraftValue(itemMap["limit_h"]),
			LimitHH:         floatPointerFromDraftValue(itemMap["limit_hh"]),
			LimitDeadband:   floatFromDraftValue(itemMap["limit_deadband"]),
			ViolationHoldMS: int(floatFromDraftValue(itemMap["violation_hold_ms"])),
			RecoverHoldMS:   int(floatFromDraftValue(itemMap["recover_hold_ms"])),
			QualityPolicy:   firstNonEmpty(strings.TrimSpace(stringFromDraftValue(itemMap["quality_policy"])), models.QualityPolicyIgnoreBad),
			Unit:            stringFromDraftValue(itemMap["unit"]),
			DecimalPlaces:   int(floatFromDraftValue(itemMap["decimal_places"])),
			SortOrder:       int(floatFromDraftValue(itemMap["sort_order"])),
		}
		if item.VarName == "" {
			return nil, fmt.Errorf("custom_items.var_name is required")
		}
		items = append(items, item)
	}
	return items, nil
}

func firstNonNilDraftValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
				continue
			}
			return value
		}
	}
	return nil
}

func int64FromDraftValue(value any) int64 {
	switch typed := value.(type) {
	case json.Number:
		out, _ := typed.Int64()
		return out
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case string:
		out, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return out
	default:
		return 0
	}
}

func floatFromDraftValue(value any) float64 {
	switch typed := value.(type) {
	case json.Number:
		out, _ := typed.Float64()
		return out
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		out, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return out
	default:
		return 0
	}
}

func floatPointerFromDraftValue(value any) *float64 {
	if value == nil {
		return nil
	}
	out := floatFromDraftValue(value)
	return &out
}

func boolFromDraftValue(value any, fallback bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		out, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err == nil {
			return out
		}
		return fallback
	default:
		return fallback
	}
}

func stringFromDraftValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}
