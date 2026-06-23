package query

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const mainServerDefaultNodeID uint64 = 9000
const defaultSyncIDBlockSize uint64 = 1000000000000

type SyncWriteMeta struct {
	UpdatedByUser  string
	UpdatedByNode  string
	EdgeInstanceID string
	SyncScope      string
}

func (q *StationViewQuery) ListStationViewItems(templateUID string) ([]StationViewItemDTO, error) {
	templateUID = strings.TrimSpace(templateUID)
	if templateUID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var items []StationViewItem
	if err := q.db.Where("template_uid = ?", templateUID).
		Order("layout_area asc, sort_order asc, id asc").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return stationViewItemsToDTO(items), nil
}

func (q *StationViewQuery) ReplaceStationViewItems(templateUID string, items []StationViewItemDTO, meta SyncWriteMeta) ([]StationViewItemDTO, error) {
	templateUID = strings.TrimSpace(templateUID)
	if templateUID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	now := time.Now()
	cleaned := make([]StationViewItem, 0, len(items))
	seen := map[string]bool{}
	for idx, item := range items {
		cleanedItem, err := stationViewItemFromDTO(templateUID, item, idx, now, meta)
		if err != nil {
			return nil, err
		}
		if seen[cleanedItem.ItemUID] {
			return nil, errors.New("duplicate station view item_uid")
		}
		seen[cleanedItem.ItemUID] = true
		cleaned = append(cleaned, cleanedItem)
	}
	err := q.db.Transaction(func(tx *gorm.DB) error {
		var template StationViewTemplate
		if err := tx.First(&template, "template_uid = ?", templateUID).Error; err != nil {
			return err
		}
		if err := tx.Delete(&StationViewItem{}, "template_uid = ?", templateUID).Error; err != nil {
			return err
		}
		ids, err := nextSyncIDs(tx, (StationViewItem{}).TableName(), len(cleaned))
		if err != nil {
			return err
		}
		for i := range cleaned {
			if len(ids) > 0 {
				cleaned[i].ID = uint(ids[i])
			}
		}
		if len(cleaned) > 0 {
			if err := tx.Create(&cleaned).Error; err != nil {
				return err
			}
		}
		return tx.Model(&StationViewTemplate{}).Where("template_uid = ?", templateUID).Updates(map[string]any{
			"version":         gorm.Expr("version + ?", 1),
			"updated_at":      now,
			"updated_by_node": normalizedUpdatedByNode(meta),
			"updated_by_user": strings.TrimSpace(meta.UpdatedByUser),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return q.ListStationViewItems(templateUID)
}

func (q *StationViewQuery) CreateDetectionStandard(standard *DetectionStandard, items []DetectionStandardItem, meta SyncWriteMeta) (DetectionStandard, error) {
	if standard == nil {
		return DetectionStandard{}, errors.New("standard is required")
	}
	now := time.Now()
	normalizeDetectionStandard(standard, meta, now)
	returned := DetectionStandard{}
	err := q.db.Transaction(func(tx *gorm.DB) error {
		if err := validateDetectionStandardDefinitionTx(tx, standard, items); err != nil {
			return err
		}
		id, err := nextSyncID(tx, standard.TableName())
		if err != nil {
			return err
		}
		standard.ID = uint(id)
		if err := tx.Create(standard).Error; err != nil {
			return err
		}
		ids, err := nextSyncIDs(tx, (DetectionStandardItem{}).TableName(), len(items))
		if err != nil {
			return err
		}
		for i := range items {
			normalizeDetectionStandardItem(&items[i], standard, meta, now)
			if len(ids) > 0 {
				items[i].ID = uint(ids[i])
			}
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}
		loaded := *standard
		loaded.Items = items
		hash, err := detectionStandardHash(loaded, items)
		if err != nil {
			return err
		}
		if err := tx.Model(&DetectionStandard{}).Where("id = ?", standard.ID).Updates(map[string]any{
			"version":     standard.Version,
			"config_hash": hash,
			"updated_at":  now,
		}).Error; err != nil {
			return err
		}
		returned, err = q.getDetectionStandardTx(tx, standard.ID)
		return err
	})
	return returned, err
}

func (q *StationViewQuery) UpdateDetectionStandard(id uint, updates map[string]any, items *[]DetectionStandardItem, meta SyncWriteMeta) (DetectionStandard, error) {
	var returned DetectionStandard
	err := q.db.Transaction(func(tx *gorm.DB) error {
		var current DetectionStandard
		if err := tx.First(&current, "id = ?", id).Error; err != nil {
			return err
		}
		now := time.Now()
		targetVersion := current.Version + 1
		delete(updates, "id")
		if rawProjectID, ok := updates["project_id"]; ok {
			projectID, err := detectionStandardProjectIDValue(rawProjectID)
			if err != nil {
				return err
			}
			projectGroupRaw, projectGroupSet := updates["project_group"]
			projectGroupEmpty := !projectGroupSet || projectGroupRaw == nil || strings.TrimSpace(fmt.Sprint(projectGroupRaw)) == ""
			if projectID != nil && projectGroupEmpty {
				var project Project
				if err := tx.Select("id", "project_group").First(&project, "id = ?", *projectID).Error; err != nil {
					return err
				}
				if strings.TrimSpace(project.ProjectGroup) != "" {
					updates["project_group"] = strings.TrimSpace(project.ProjectGroup)
				}
			}
		}
		if rawVersion, ok := updates["version"]; ok {
			parsed, err := detectionStandardVersionValue(rawVersion)
			if err != nil {
				return err
			}
			targetVersion = parsed
			delete(updates, "version")
		}
		delete(updates, "config_hash")
		updates["updated_at"] = now
		updates["updated_by_node"] = normalizedUpdatedByNode(meta)
		updates["updated_by_user"] = strings.TrimSpace(meta.UpdatedByUser)
		if err := tx.Model(&DetectionStandard{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if items != nil {
			if err := tx.Delete(&DetectionStandardItem{}, "standard_id = ?", id).Error; err != nil {
				return err
			}
			ids, err := nextSyncIDs(tx, (DetectionStandardItem{}).TableName(), len(*items))
			if err != nil {
				return err
			}
			for i := range *items {
				normalizeDetectionStandardItem(&(*items)[i], &current, meta, now)
				(*items)[i].StandardID = id
				if len(ids) > 0 {
					(*items)[i].ID = uint(ids[i])
				}
			}
			currentAfterUpdate, err := q.getDetectionStandardTx(tx, id)
			if err != nil {
				return err
			}
			if err := validateDetectionStandardDefinitionTx(tx, &currentAfterUpdate, *items); err != nil {
				return err
			}
			if len(*items) > 0 {
				if err := tx.Create(items).Error; err != nil {
					return err
				}
			}
		} else {
			currentAfterUpdate, err := q.getDetectionStandardTx(tx, id)
			if err != nil {
				return err
			}
			if err := validateDetectionStandardDefinitionTx(tx, &currentAfterUpdate, currentAfterUpdate.Items); err != nil {
				return err
			}
		}
		loaded, err := q.getDetectionStandardTx(tx, id)
		if err != nil {
			return err
		}
		loaded.Version = targetVersion
		hash, err := detectionStandardHash(loaded, loaded.Items)
		if err != nil {
			return err
		}
		if err := tx.Model(&DetectionStandard{}).Where("id = ?", id).Updates(map[string]any{
			"config_hash": hash,
			"version":     targetVersion,
			"updated_at":  now,
		}).Error; err != nil {
			return err
		}
		returned, err = q.getDetectionStandardTx(tx, id)
		return err
	})
	return returned, err
}

func (q *StationViewQuery) DeleteDetectionStandard(id uint) error {
	return q.db.Transaction(func(tx *gorm.DB) error {
		var current DetectionStandard
		if err := tx.First(&current, "id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&DetectionStandardItem{}, "standard_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&DetectionStandardFavorite{}, "standard_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&DetectionStandardRecent{}, "standard_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(&DetectionStandard{}, "id = ?", id).Error
	})
}

func (q *StationViewQuery) CreateTaskFlow(flow *TaskFlow, meta SyncWriteMeta) (TaskFlow, error) {
	if flow == nil {
		return TaskFlow{}, errors.New("task flow is required")
	}
	now := time.Now()
	normalizeTaskFlow(flow, meta, now)
	var returned TaskFlow
	err := q.db.Transaction(func(tx *gorm.DB) error {
		id, err := nextSyncID(tx, flow.TableName())
		if err != nil {
			return err
		}
		flow.ID = id
		ids, err := nextSyncIDs(tx, (TaskFlowVar{}).TableName(), len(flow.Vars))
		if err != nil {
			return err
		}
		for i := range flow.Vars {
			normalizeTaskFlowVar(&flow.Vars[i], flow, meta, now)
			if len(ids) > 0 {
				flow.Vars[i].ID = ids[i]
			}
		}
		if err := tx.Create(flow).Error; err != nil {
			return err
		}
		returned, err = q.getTaskFlowTx(tx, flow.ID)
		return err
	})
	return returned, err
}

func (q *StationViewQuery) UpdateTaskFlow(id uint64, updates map[string]any, vars *[]TaskFlowVar, meta SyncWriteMeta) (TaskFlow, error) {
	var returned TaskFlow
	err := q.db.Transaction(func(tx *gorm.DB) error {
		var current TaskFlow
		if err := tx.First(&current, "id = ?", id).Error; err != nil {
			return err
		}
		now := time.Now()
		delete(updates, "id")
		delete(updates, "version")
		updates["updated_at"] = now
		updates["updated_by_node"] = normalizedUpdatedByNode(meta)
		updates["updated_by_user"] = strings.TrimSpace(meta.UpdatedByUser)
		if err := tx.Model(&TaskFlow{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if vars != nil {
			if err := tx.Delete(&TaskFlowVar{}, "flow_id = ?", id).Error; err != nil {
				return err
			}
			ids, err := nextSyncIDs(tx, (TaskFlowVar{}).TableName(), len(*vars))
			if err != nil {
				return err
			}
			for i := range *vars {
				normalizeTaskFlowVar(&(*vars)[i], &current, meta, now)
				(*vars)[i].FlowID = id
				if len(ids) > 0 {
					(*vars)[i].ID = ids[i]
				}
			}
			if len(*vars) > 0 {
				if err := tx.Create(vars).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&TaskFlow{}).Where("id = ?", id).Updates(map[string]any{
			"version":    gorm.Expr("version + ?", 1),
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		var loadErr error
		returned, loadErr = q.getTaskFlowTx(tx, id)
		return loadErr
	})
	return returned, err
}

func nextSyncID(tx *gorm.DB, table string) (uint64, error) {
	nodeID := mainServerDefaultNodeID
	if raw := strings.TrimSpace(os.Getenv("MAIN_SYNC_NODE_ID")); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil && parsed > 0 {
			nodeID = parsed
		}
	}
	blockSize := defaultSyncIDBlockSize
	if raw := strings.TrimSpace(os.Getenv("MAIN_SYNC_ID_BLOCK_SIZE")); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil && parsed > 0 {
			blockSize = parsed
		}
	}
	base := nodeID * blockSize
	upper := base + blockSize
	var maxID uint64
	if err := tx.Table(table).Select("COALESCE(MAX(id), 0)").Where("id >= ? AND id < ?", base, upper).Scan(&maxID).Error; err != nil {
		return 0, err
	}
	next := base + 1
	if maxID >= base {
		next = maxID + 1
	}
	if next >= upper {
		return 0, fmt.Errorf("id block exhausted for main server table %s", table)
	}
	return next, nil
}

func nextSyncIDs(tx *gorm.DB, table string, count int) ([]uint64, error) {
	if count <= 0 {
		return nil, nil
	}
	first, err := nextSyncID(tx, table)
	if err != nil {
		return nil, err
	}
	ids := make([]uint64, count)
	for i := range ids {
		ids[i] = first + uint64(i)
	}
	return ids, nil
}

func stationViewItemsToDTO(items []StationViewItem) []StationViewItemDTO {
	result := make([]StationViewItemDTO, 0, len(items))
	for _, item := range items {
		result = append(result, StationViewItemDTO{
			ItemUID:     item.ItemUID,
			LayoutArea:  item.LayoutArea,
			ItemType:    item.ItemType,
			BindingType: item.BindingType,
			BindingKey:  item.BindingKey,
			BindingJSON: item.BindingJSON,
			DisplayJSON: item.DisplayJSON,
			SortOrder:   item.SortOrder,
			Pinned:      item.Pinned,
			Visible:     item.Visible,
		})
	}
	return result
}

func stationViewItemFromDTO(templateUID string, item StationViewItemDTO, index int, now time.Time, meta SyncWriteMeta) (StationViewItem, error) {
	itemUID := strings.TrimSpace(item.ItemUID)
	if itemUID == "" {
		itemUID = fmt.Sprintf("%s-item-%03d", templateUID, index+1)
	}
	layoutArea := strings.TrimSpace(item.LayoutArea)
	if layoutArea == "" {
		layoutArea = StationViewLayoutAreaCardPool
	}
	if layoutArea != StationViewLayoutAreaCardPool && layoutArea != StationViewLayoutAreaListLayout {
		return StationViewItem{}, fmt.Errorf("invalid layout_area")
	}
	itemType := strings.TrimSpace(item.ItemType)
	if itemType == "" {
		itemType = "metric_card"
	}
	bindingType := strings.TrimSpace(item.BindingType)
	if bindingType == "" {
		bindingType = StationViewBindingVarName
	}
	visible := item.Visible
	return StationViewItem{
		TemplateUID:    templateUID,
		RegionKey:      layoutArea,
		LayoutArea:     layoutArea,
		ItemUID:        itemUID,
		ItemType:       itemType,
		BindingType:    bindingType,
		BindingKey:     strings.TrimSpace(item.BindingKey),
		BindingJSON:    strings.TrimSpace(item.BindingJSON),
		DisplayJSON:    strings.TrimSpace(item.DisplayJSON),
		SortOrder:      item.SortOrder,
		Pinned:         item.Pinned,
		Visible:        visible,
		SyncScope:      normalizedSyncScope(meta),
		EdgeInstanceID: strings.TrimSpace(meta.EdgeInstanceID),
		UpdatedByNode:  normalizedUpdatedByNode(meta),
		UpdatedByUser:  strings.TrimSpace(meta.UpdatedByUser),
	}, nil
}

func normalizeDetectionStandard(standard *DetectionStandard, meta SyncWriteMeta, now time.Time) {
	standard.StandardCode = strings.TrimSpace(standard.StandardCode)
	standard.Name = firstNonEmptyString(strings.TrimSpace(standard.Name), strings.TrimSpace(standard.DisplayName), standard.StandardCode)
	standard.DisplayName = firstNonEmptyString(strings.TrimSpace(standard.DisplayName), standard.Name)
	standard.ProjectCode = strings.TrimSpace(standard.ProjectCode)
	standard.ProjectGroup = strings.TrimSpace(standard.ProjectGroup)
	standard.Mode = firstNonEmptyString(strings.TrimSpace(standard.Mode), "standard")
	if standard.Version <= 0 {
		standard.Version = 1
	}
	if strings.TrimSpace(meta.SyncScope) != "" {
		standard.SyncScope = strings.TrimSpace(meta.SyncScope)
	}
	if strings.TrimSpace(meta.EdgeInstanceID) != "" {
		standard.EdgeInstanceID = strings.TrimSpace(meta.EdgeInstanceID)
	}
	if strings.TrimSpace(standard.SyncScope) == "" {
		standard.SyncScope = normalizedSyncScope(SyncWriteMeta{SyncScope: standard.SyncScope, EdgeInstanceID: standard.EdgeInstanceID})
	}
	standard.UpdatedByNode = normalizedUpdatedByNode(meta)
	standard.UpdatedByUser = strings.TrimSpace(meta.UpdatedByUser)
	standard.CreatedAt = now
	standard.UpdatedAt = now
}

func normalizeDetectionStandardItem(item *DetectionStandardItem, standard *DetectionStandard, meta SyncWriteMeta, now time.Time) {
	item.StandardID = standard.ID
	item.VarName = strings.TrimSpace(item.VarName)
	item.CheckMethod = firstNonEmptyString(strings.TrimSpace(item.CheckMethod), "numeric_range")
	item.QualityPolicy = firstNonEmptyString(strings.TrimSpace(item.QualityPolicy), "ignore_bad")
	item.SyncScope = normalizedSyncScope(meta)
	item.EdgeInstanceID = strings.TrimSpace(meta.EdgeInstanceID)
	item.UpdatedByNode = normalizedUpdatedByNode(meta)
	item.UpdatedByUser = strings.TrimSpace(meta.UpdatedByUser)
	item.CreatedAt = now
	item.UpdatedAt = now
}

func normalizeTaskFlow(flow *TaskFlow, meta SyncWriteMeta, now time.Time) {
	flow.FlowCode = strings.TrimSpace(flow.FlowCode)
	flow.Name = firstNonEmptyString(strings.TrimSpace(flow.Name), flow.FlowCode)
	flow.TriggerType = firstNonEmptyString(strings.TrimSpace(flow.TriggerType), "data_change")
	flow.ActionType = firstNonEmptyString(strings.TrimSpace(flow.ActionType), "javascript")
	if flow.TimeoutMS <= 0 {
		flow.TimeoutMS = 3000
	}
	if flow.Version <= 0 {
		flow.Version = 1
	}
	flow.SyncScope = normalizedSyncScope(meta)
	flow.EdgeInstanceID = strings.TrimSpace(meta.EdgeInstanceID)
	flow.UpdatedByNode = normalizedUpdatedByNode(meta)
	flow.UpdatedByUser = strings.TrimSpace(meta.UpdatedByUser)
	flow.CreatedAt = now
	flow.UpdatedAt = now
}

func normalizeTaskFlowVar(item *TaskFlowVar, flow *TaskFlow, meta SyncWriteMeta, now time.Time) {
	item.FlowID = flow.ID
	item.ProjectID = flow.ProjectID
	item.VarName = strings.TrimSpace(item.VarName)
	item.Role = firstNonEmptyString(strings.TrimSpace(item.Role), "watch")
	item.SyncScope = normalizedSyncScope(meta)
	item.EdgeInstanceID = strings.TrimSpace(meta.EdgeInstanceID)
	item.UpdatedByNode = normalizedUpdatedByNode(meta)
	item.UpdatedByUser = strings.TrimSpace(meta.UpdatedByUser)
	item.CreatedAt = now
	item.UpdatedAt = now
}

func (q *StationViewQuery) getDetectionStandardTx(tx *gorm.DB, id uint) (DetectionStandard, error) {
	var standard DetectionStandard
	if err := tx.First(&standard, "id = ?", id).Error; err != nil {
		return standard, err
	}
	if err := tx.Where("standard_id = ?", id).Order("sort_order asc, id asc").Find(&standard.Items).Error; err != nil {
		return standard, err
	}
	return standard, nil
}

func (q *StationViewQuery) getTaskFlowTx(tx *gorm.DB, id uint64) (TaskFlow, error) {
	var flow TaskFlow
	err := tx.Preload("Vars").First(&flow, "id = ?", id).Error
	return flow, err
}

func validateDetectionStandardDefinitionTx(tx *gorm.DB, standard *DetectionStandard, items []DetectionStandardItem) error {
	if standard == nil {
		return errors.New("standard is required")
	}
	if strings.TrimSpace(standard.StandardCode) == "" {
		return errors.New("standard_code is required")
	}
	if strings.TrimSpace(standard.Name) == "" {
		return errors.New("name is required")
	}
	if standard.Version <= 0 {
		return errors.New("version must be positive")
	}
	if !standard.Enabled {
		return errors.New("enabled detection standard is required")
	}
	if len(items) == 0 {
		return errors.New("detection standard items are required")
	}
	if standard.ProjectID != nil {
		var project Project
		if err := tx.First(&project, "id = ?", *standard.ProjectID).Error; err != nil {
			return err
		}
		if !project.Enabled {
			return errors.New("project is disabled")
		}
		if strings.TrimSpace(standard.ProjectCode) == "" {
			standard.ProjectCode = project.ProjectCode
		}
		if strings.TrimSpace(standard.ProjectCode) != strings.TrimSpace(project.ProjectCode) {
			return errors.New("project_code does not match project_id")
		}
		if strings.TrimSpace(standard.ProjectGroup) == "" {
			standard.ProjectGroup = strings.TrimSpace(project.ProjectGroup)
		}
		projectEdge := strings.TrimSpace(project.EdgeInstanceID)
		standardEdge := strings.TrimSpace(standard.EdgeInstanceID)
		if projectEdge != "" && standardEdge != "" && projectEdge != standardEdge {
			return ErrEdgeInstanceMismatch
		}
		if standardEdge == "" && projectEdge != "" {
			standard.EdgeInstanceID = projectEdge
		}
		if strings.TrimSpace(standard.SyncScope) == "" && strings.TrimSpace(standard.EdgeInstanceID) != "" {
			standard.SyncScope = "edge"
		}
	}
	seen := make(map[int64]struct{}, len(items))
	for i := range items {
		item := &items[i]
		normalizeDetectionStandardItemDefaults(item)
		if item.VarID == 0 {
			return errors.New("var_id is required")
		}
		if _, ok := seen[item.VarID]; ok {
			return fmt.Errorf("duplicate var_id %d", item.VarID)
		}
		seen[item.VarID] = struct{}{}
		if err := validateDetectionStandardItemValues(*item); err != nil {
			return err
		}
		var tag TagConfig
		if err := tx.First(&tag, "var_id = ?", item.VarID).Error; err != nil {
			return fmt.Errorf("var_id %d does not exist: %w", item.VarID, err)
		}
		if !tag.Enabled {
			return fmt.Errorf("var_id %d is disabled", item.VarID)
		}
		if standard.ProjectID != nil && (tag.ProjectID == nil || *tag.ProjectID != *standard.ProjectID) {
			return fmt.Errorf("var_id %d does not belong to project_id %d", item.VarID, *standard.ProjectID)
		}
		item.VarName = strings.TrimSpace(tag.VarName)
		if strings.TrimSpace(item.DisplayName) == "" {
			item.DisplayName = tag.DisplayName
		}
		if strings.TrimSpace(item.DisplayNameEN) == "" {
			item.DisplayNameEN = tag.DisplayNameEN
		}
		if strings.TrimSpace(item.DisplayNameJA) == "" {
			item.DisplayNameJA = tag.DisplayNameJA
		}
		if strings.TrimSpace(item.Unit) == "" {
			item.Unit = tag.Unit
		}
		if item.DecimalPlaces == 0 && tag.DecimalPlaces > 0 {
			item.DecimalPlaces = tag.DecimalPlaces
		}
	}
	return nil
}

func validateDetectionStandardItemValues(item DetectionStandardItem) error {
	if item.CheckCycleMS < 0 {
		return errors.New("check_cycle_ms must be non-negative")
	}
	if item.LimitDeadband < 0 {
		return errors.New("limit_deadband must be non-negative")
	}
	if item.ViolationHoldMS < 0 || item.RecoverHoldMS < 0 {
		return errors.New("hold times must be non-negative")
	}
	if err := validateDetectionLimitOrder(item.LimitLL, item.LimitL, item.LimitH, item.LimitHH); err != nil {
		return err
	}
	if !validDetectionCheckMethod(item.CheckMethod) {
		return errors.New("invalid check_method")
	}
	if !validDetectionQualityPolicy(item.QualityPolicy) {
		return errors.New("invalid quality_policy")
	}
	return nil
}

func validateDetectionLimitOrder(ll *float64, l *float64, h *float64, hh *float64) error {
	if ll != nil && l != nil && *ll > *l {
		return errors.New("limit_ll must be less than or equal to limit_l")
	}
	if l != nil && h != nil && *l > *h {
		return errors.New("limit_l must be less than or equal to limit_h")
	}
	if h != nil && hh != nil && *h > *hh {
		return errors.New("limit_h must be less than or equal to limit_hh")
	}
	return nil
}

func validDetectionCheckMethod(value string) bool {
	switch strings.TrimSpace(value) {
	case "numeric_range", "bool_equals", "string_equals", "regex":
		return true
	default:
		return false
	}
}

func validDetectionQualityPolicy(value string) bool {
	switch strings.TrimSpace(value) {
	case "ignore_bad", "record_invalid", "fail_on_bad":
		return true
	default:
		return false
	}
}

func normalizeDetectionStandardItemDefaults(item *DetectionStandardItem) {
	item.VarName = strings.TrimSpace(item.VarName)
	item.CheckMethod = firstNonEmptyString(strings.TrimSpace(item.CheckMethod), "numeric_range")
	item.QualityPolicy = firstNonEmptyString(strings.TrimSpace(item.QualityPolicy), "ignore_bad")
	if item.DecimalPlaces == 0 {
		item.DecimalPlaces = 2
	}
}

func detectionStandardVersionValue(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		if typed <= 0 {
			return 0, errors.New("version must be positive")
		}
		return typed, nil
	case int64:
		if typed <= 0 {
			return 0, errors.New("version must be positive")
		}
		return int(typed), nil
	case float64:
		if typed <= 0 || typed != float64(int(typed)) {
			return 0, errors.New("version must be a positive integer")
		}
		return int(typed), nil
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || parsed <= 0 {
			return 0, errors.New("version must be a positive integer")
		}
		return int(parsed), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil || parsed <= 0 {
			return 0, errors.New("version must be a positive integer")
		}
		return parsed, nil
	default:
		return 0, errors.New("version must be a positive integer")
	}
}

func detectionStandardProjectIDValue(value any) (*uint, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case uint:
		return &typed, nil
	case *uint:
		return typed, nil
	case uint64:
		parsed := uint(typed)
		return &parsed, nil
	case int:
		if typed <= 0 {
			return nil, errors.New("project_id must be positive")
		}
		parsed := uint(typed)
		return &parsed, nil
	case int64:
		if typed <= 0 {
			return nil, errors.New("project_id must be positive")
		}
		parsed := uint(typed)
		return &parsed, nil
	case float64:
		if typed <= 0 || typed != float64(uint(typed)) {
			return nil, errors.New("project_id must be a positive integer")
		}
		parsed := uint(typed)
		return &parsed, nil
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || parsed <= 0 {
			return nil, errors.New("project_id must be a positive integer")
		}
		value := uint(parsed)
		return &value, nil
	default:
		return nil, errors.New("project_id must be a positive integer")
	}
}

func detectionStandardHash(standard DetectionStandard, items []DetectionStandardItem) (string, error) {
	type hashItem struct {
		VarID           int64    `json:"var_id"`
		VarName         string   `json:"var_name"`
		DisplayName     string   `json:"display_name"`
		DisplayNameEN   string   `json:"display_name_en"`
		DisplayNameJA   string   `json:"display_name_ja"`
		CheckEnabled    bool     `json:"check_enabled"`
		AlarmEnabled    bool     `json:"alarm_enabled"`
		StoreEnabled    bool     `json:"store_enabled"`
		CheckCycleMS    int      `json:"check_cycle_ms"`
		CheckOnStart    bool     `json:"check_on_start"`
		Required        bool     `json:"required"`
		CheckMethod     string   `json:"check_method"`
		TargetValue     string   `json:"target_value"`
		LimitLL         *float64 `json:"limit_ll"`
		LimitL          *float64 `json:"limit_l"`
		LimitH          *float64 `json:"limit_h"`
		LimitHH         *float64 `json:"limit_hh"`
		LimitDeadband   float64  `json:"limit_deadband"`
		ViolationHoldMS int      `json:"violation_hold_ms"`
		RecoverHoldMS   int      `json:"recover_hold_ms"`
		QualityPolicy   string   `json:"quality_policy"`
		Unit            string   `json:"unit"`
		DecimalPlaces   int      `json:"decimal_places"`
		SortOrder       int      `json:"sort_order"`
	}
	payload := struct {
		StandardCode     string     `json:"standard_code"`
		Name             string     `json:"name"`
		DisplayName      string     `json:"display_name"`
		DisplayNameEN    string     `json:"display_name_en"`
		DisplayNameJA    string     `json:"display_name_ja"`
		ProjectCode      string     `json:"project_code"`
		ProjectGroup     string     `json:"project_group"`
		Mode             string     `json:"mode"`
		ReportTemplateID *uint      `json:"report_template_id"`
		Version          int        `json:"version"`
		Enabled          bool       `json:"enabled"`
		Items            []hashItem `json:"items"`
	}{
		StandardCode:     strings.TrimSpace(standard.StandardCode),
		Name:             strings.TrimSpace(standard.Name),
		DisplayName:      strings.TrimSpace(standard.DisplayName),
		DisplayNameEN:    strings.TrimSpace(standard.DisplayNameEN),
		DisplayNameJA:    strings.TrimSpace(standard.DisplayNameJA),
		ProjectCode:      strings.TrimSpace(standard.ProjectCode),
		ProjectGroup:     strings.TrimSpace(standard.ProjectGroup),
		Mode:             strings.TrimSpace(standard.Mode),
		ReportTemplateID: standard.ReportTemplateID,
		Version:          standard.Version,
		Enabled:          standard.Enabled,
		Items:            make([]hashItem, 0, len(items)),
	}
	sorted := append([]DetectionStandardItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].SortOrder != sorted[j].SortOrder {
			return sorted[i].SortOrder < sorted[j].SortOrder
		}
		if sorted[i].VarID != sorted[j].VarID {
			return sorted[i].VarID < sorted[j].VarID
		}
		return sorted[i].VarName < sorted[j].VarName
	})
	for _, item := range sorted {
		normalizeDetectionStandardItemDefaults(&item)
		payload.Items = append(payload.Items, hashItem{
			VarID:           item.VarID,
			VarName:         strings.TrimSpace(item.VarName),
			DisplayName:     strings.TrimSpace(item.DisplayName),
			DisplayNameEN:   strings.TrimSpace(item.DisplayNameEN),
			DisplayNameJA:   strings.TrimSpace(item.DisplayNameJA),
			CheckEnabled:    item.CheckEnabled,
			AlarmEnabled:    item.AlarmEnabled,
			StoreEnabled:    item.StoreEnabled,
			CheckCycleMS:    item.CheckCycleMS,
			CheckOnStart:    item.CheckOnStart,
			Required:        item.Required,
			CheckMethod:     strings.TrimSpace(item.CheckMethod),
			TargetValue:     strings.TrimSpace(item.TargetValue),
			LimitLL:         item.LimitLL,
			LimitL:          item.LimitL,
			LimitH:          item.LimitH,
			LimitHH:         item.LimitHH,
			LimitDeadband:   item.LimitDeadband,
			ViolationHoldMS: item.ViolationHoldMS,
			RecoverHoldMS:   item.RecoverHoldMS,
			QualityPolicy:   strings.TrimSpace(item.QualityPolicy),
			Unit:            strings.TrimSpace(item.Unit),
			DecimalPlaces:   item.DecimalPlaces,
			SortOrder:       item.SortOrder,
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func normalizedSyncScope(meta SyncWriteMeta) string {
	if scope := strings.TrimSpace(meta.SyncScope); scope != "" {
		return scope
	}
	if strings.TrimSpace(meta.EdgeInstanceID) != "" {
		return "edge"
	}
	return "global"
}

func normalizedUpdatedByNode(meta SyncWriteMeta) string {
	if node := strings.TrimSpace(meta.UpdatedByNode); node != "" {
		return node
	}
	return "main-server"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
