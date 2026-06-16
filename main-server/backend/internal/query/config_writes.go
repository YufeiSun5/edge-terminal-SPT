package query

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
		hash := detectionStandardHash(*standard, items)
		if err := tx.Model(&DetectionStandard{}).Where("id = ?", standard.ID).Update("config_hash", hash).Error; err != nil {
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
		delete(updates, "id")
		delete(updates, "version")
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
			if len(*items) > 0 {
				if err := tx.Create(items).Error; err != nil {
					return err
				}
			}
		}
		loaded, err := q.getDetectionStandardTx(tx, id)
		if err != nil {
			return err
		}
		hash := detectionStandardHash(loaded, loaded.Items)
		if err := tx.Model(&DetectionStandard{}).Where("id = ?", id).Updates(map[string]any{
			"config_hash": hash,
			"version":     gorm.Expr("version + ?", 1),
			"updated_at":  now,
		}).Error; err != nil {
			return err
		}
		returned, err = q.getDetectionStandardTx(tx, id)
		return err
	})
	return returned, err
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
	standard.Mode = firstNonEmptyString(strings.TrimSpace(standard.Mode), "standard")
	if standard.Version <= 0 {
		standard.Version = 1
	}
	standard.SyncScope = normalizedSyncScope(meta)
	standard.EdgeInstanceID = strings.TrimSpace(meta.EdgeInstanceID)
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

func detectionStandardHash(standard DetectionStandard, items []DetectionStandardItem) string {
	payload, _ := json.Marshal(struct {
		Standard DetectionStandard       `json:"standard"`
		Items    []DetectionStandardItem `json:"items"`
	}{Standard: standard, Items: items})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
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
