package database

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

var storageIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func ProjectWideTableName(ProjectID uint) string {
	return fmt.Sprintf("rt_project_%d_data", ProjectID)
}

func NormalizeStorageColumnName(value string, fallback string, varID int64) string {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		candidate = strings.TrimSpace(fallback)
	}
	if candidate == "" {
		candidate = fmt.Sprintf("var_%d", varID)
	}
	candidate = strings.ToLower(candidate)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range candidate {
		var out rune
		switch {
		case r >= 'a' && r <= 'z':
			out = r
		case r >= '0' && r <= '9':
			out = r
		case r == '_':
			out = '_'
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			out = '_'
		default:
			out = '_'
		}
		if out == '_' {
			if lastUnderscore {
				continue
			}
			lastUnderscore = true
		} else {
			lastUnderscore = false
		}
		b.WriteRune(out)
	}
	normalized := strings.Trim(b.String(), "_")
	if normalized == "" {
		normalized = fmt.Sprintf("var_%d", varID)
	}
	if normalized[0] >= '0' && normalized[0] <= '9' {
		normalized = "v_" + normalized
	}
	if len(normalized) > 64 {
		normalized = strings.TrimRight(normalized[:64], "_")
	}
	if normalized == "" {
		normalized = fmt.Sprintf("var_%d", varID)
	}
	return normalized
}

func ValidateStorageIdentifier(value string) error {
	if !storageIdentifierPattern.MatchString(value) {
		return fmt.Errorf("invalid storage identifier %q", value)
	}
	return nil
}

func StorageColumnTypeForDataType(dataType string) string {
	switch strings.ToUpper(strings.TrimSpace(dataType)) {
	case "BOOL", "BOOLEAN":
		return "TINYINT(1)"
	case "INT", "INTEGER", "LONG":
		return "BIGINT"
	case "STRING", "TEXT", "VARCHAR", "CHAR":
		return "TEXT"
	default:
		return "DOUBLE"
	}
}

func (r *Repository) ListStorageRoutesByProject(ProjectID uint) ([]models.StorageRoute, error) {
	return r.ListStorageRoutes(StorageRouteFilter{ProjectID: &ProjectID})
}

func (r *Repository) ListStorageRoutes(filter StorageRouteFilter) ([]models.StorageRoute, error) {
	var routes []models.StorageRoute
	query := r.db.Model(&models.StorageRoute{})
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.VarID != nil {
		query = query.Where("var_id = ?", *filter.VarID)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	err := query.Order("project_id asc, var_id asc, id asc").Find(&routes).Error
	return routes, err
}

func (r *Repository) ListRunStorageRoutes(taskID uint) ([]models.DetectionRunStorageRoute, error) {
	var routes []models.DetectionRunStorageRoute
	err := r.db.Where("task_id = ?", taskID).Order("var_id asc, id asc").Find(&routes).Error
	return routes, err
}

func (r *Repository) EnsureDefaultStorageRouteForTag(tag models.TagConfig) (*models.StorageRoute, error) {
	return ensureDefaultStorageRouteForTag(r.db, tag)
}

func (r *Repository) CreateStorageRoute(route *models.StorageRoute) error {
	if err := r.prepareStorageRoute(route); err != nil {
		return err
	}
	now := time.Now()
	route.CreatedAt = now
	route.UpdatedAt = now
	return r.db.Create(route).Error
}

func (r *Repository) UpdateStorageRoute(id uint64, updates map[string]interface{}) (models.StorageRoute, error) {
	var current models.StorageRoute
	if err := r.db.First(&current, "id = ?", id).Error; err != nil {
		return current, err
	}
	next := mergeStorageRouteUpdates(current, updates)
	if err := r.prepareStorageRoute(&next); err != nil {
		return current, err
	}
	updates["updated_at"] = time.Now()
	if err := r.db.Model(&models.StorageRoute{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return current, err
	}
	err := r.db.First(&current, "id = ?", id).Error
	return current, err
}

func (r *Repository) DeleteStorageRoute(id uint64) error {
	var referenced int64
	if err := r.db.Model(&models.DetectionRunStorageRoute{}).Where("route_id = ?", id).Count(&referenced).Error; err != nil {
		return err
	}
	if referenced > 0 {
		return ErrReferenced
	}
	result := r.db.Delete(&models.StorageRoute{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) prepareStorageRoute(route *models.StorageRoute) error {
	route.StorageTarget = strings.ToLower(strings.TrimSpace(route.StorageTarget))
	if route.StorageTarget == "" {
		route.StorageTarget = models.StorageTargetWideTable
	}
	if route.ProjectID == 0 {
		return fmt.Errorf("project_id is required")
	}
	if route.VarID == 0 {
		return fmt.Errorf("var_id is required")
	}
	var tag models.TagConfig
	if err := r.db.First(&tag, "var_id = ?", route.VarID).Error; err != nil {
		return err
	}
	if tag.ProjectID == nil || *tag.ProjectID != route.ProjectID {
		return fmt.Errorf("storage route variable must belong to project_id %d", route.ProjectID)
	}
	if strings.TrimSpace(route.RouteCode) == "" {
		route.RouteCode = defaultRouteCodeForColumn(route.ProjectID, route.VarID, route.ColumnName)
	}
	if strings.TrimSpace(route.StorageTable) == "" {
		route.StorageTable = ProjectWideTableName(route.ProjectID)
	}
	if strings.TrimSpace(route.ColumnName) == "" {
		route.ColumnName = NormalizeStorageColumnName(firstStorageName(route.FormFieldKey, route.QueryAlias), tag.VarName, tag.VarID)
	}
	if strings.TrimSpace(route.ColumnType) == "" {
		route.ColumnType = StorageColumnTypeForDataType(tag.DataType)
	}
	if strings.TrimSpace(route.TriggerMode) == "" {
		route.TriggerMode = models.StoreTriggerOnCycle
	}
	if err := validateStorageRoute(*route); err != nil {
		return err
	}
	return ensureRouteColumnUnique(r.db, route)
}

func ensureDefaultStorageRouteForTag(db *gorm.DB, tag models.TagConfig) (*models.StorageRoute, error) {
	if tag.ProjectID == nil {
		return nil, nil
	}
	var existing models.StorageRoute
	err := db.Where("project_id = ? AND var_id = ? AND route_code = ?", *tag.ProjectID, tag.VarID, defaultRouteCode(tag)).
		First(&existing).Error
	if err == nil {
		if !existing.Enabled {
			next := makeDefaultStorageRoute(tag)
			next.ID = existing.ID
			if err := ensureRouteColumnUnique(db, &next); err != nil {
				return nil, err
			}
			updates := map[string]interface{}{
				"storage_target": next.StorageTarget,
				"table_name":     next.StorageTable,
				"column_name":    next.ColumnName,
				"column_type":    next.ColumnType,
				"updated_at":     time.Now(),
			}
			if err := db.Model(&models.StorageRoute{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return nil, err
			}
			if err := db.First(&existing, "id = ?", existing.ID).Error; err != nil {
				return nil, err
			}
		}
		return &existing, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	route := makeDefaultStorageRoute(tag)
	if err := ensureRouteColumnUnique(db, &route); err != nil {
		return nil, err
	}
	if err := validateStorageRoute(route); err != nil {
		return nil, err
	}
	if err := db.Create(&route).Error; err != nil {
		return nil, err
	}
	return &route, nil
}

func makeDefaultStorageRoute(tag models.TagConfig) models.StorageRoute {
	now := time.Now()
	ProjectID := uint(0)
	if tag.ProjectID != nil {
		ProjectID = *tag.ProjectID
	}
	tableName := ProjectWideTableName(ProjectID)
	columnName := NormalizeStorageColumnName("", tag.VarName, tag.VarID)
	return models.StorageRoute{
		ProjectID:     ProjectID,
		VarID:         tag.VarID,
		RouteCode:     defaultRouteCode(tag),
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  tableName,
		ColumnName:    columnName,
		ColumnType:    StorageColumnTypeForDataType(tag.DataType),
		TriggerMode:   models.StoreTriggerOnCycle,
		CycleMS:       0,
		Deadband:      0,
		StoreOnStart:  false,
		Enabled:       false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func defaultRouteCode(tag models.TagConfig) string {
	return fmt.Sprintf("project_%d_var_%d_default", valueUint(tag.ProjectID), tag.VarID)
}

func defaultRouteCodeForColumn(ProjectID uint, varID int64, columnName string) string {
	column := strings.TrimSpace(columnName)
	if column == "" {
		column = "default"
	}
	return fmt.Sprintf("project_%d_var_%d_%s", ProjectID, varID, column)
}

func valueUint(value *uint) uint {
	if value == nil {
		return 0
	}
	return *value
}

func firstStorageName(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ensureRouteColumnUnique(db *gorm.DB, route *models.StorageRoute) error {
	base := route.ColumnName
	if base == "" {
		base = fmt.Sprintf("var_%d", route.VarID)
	}
	if err := ValidateStorageIdentifier(base); err != nil {
		return err
	}
	column := base
	var count int64
	if err := db.Model(&models.StorageRoute{}).
		Where("project_id = ? AND table_name = ? AND column_name = ? AND var_id <> ?", route.ProjectID, route.StorageTable, column, route.VarID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		route.ColumnName = column
		return nil
	}
	suffix := fmt.Sprintf("_v%d", route.VarID)
	maxBaseLen := 64 - len(suffix)
	if maxBaseLen < 1 {
		maxBaseLen = 1
	}
	if len(base) > maxBaseLen {
		base = strings.TrimRight(base[:maxBaseLen], "_")
	}
	route.ColumnName = base + suffix
	return ValidateStorageIdentifier(route.ColumnName)
}

func validateStorageRoute(route models.StorageRoute) error {
	if route.ProjectID == 0 {
		return fmt.Errorf("project_id is required")
	}
	if route.VarID == 0 {
		return fmt.Errorf("var_id is required")
	}
	if route.RouteCode == "" {
		return fmt.Errorf("route_code is required")
	}
	if route.StorageTarget == "" {
		return fmt.Errorf("storage_target is required")
	}
	if route.StorageTarget != models.StorageTargetWideTable && route.StorageTarget != models.StorageTargetNone {
		return fmt.Errorf("storage route target must be wide_table or none")
	}
	if err := ValidateStorageIdentifier(route.StorageTable); err != nil {
		return err
	}
	if err := ValidateStorageIdentifier(route.ColumnName); err != nil {
		return err
	}
	if route.CycleMS < 0 {
		return fmt.Errorf("cycle_ms must be greater than or equal to 0")
	}
	if route.Deadband < 0 {
		return fmt.Errorf("deadband must be greater than or equal to 0")
	}
	if !isValidStorageRouteTrigger(route.TriggerMode) {
		return fmt.Errorf("invalid trigger_mode %q", route.TriggerMode)
	}
	return nil
}

func isValidStorageRouteTrigger(value string) bool {
	switch value {
	case models.StoreTriggerOnStart, models.StoreTriggerOnCycle, models.StoreTriggerOnChange, models.StoreTriggerOnDetection, models.StoreTriggerAlways:
		return true
	default:
		return false
	}
}

func mergeStorageRouteUpdates(route models.StorageRoute, updates map[string]interface{}) models.StorageRoute {
	for key, value := range updates {
		switch key {
		case "route_code":
			route.RouteCode = value.(string)
		case "storage_target":
			route.StorageTarget = value.(string)
		case "table_name":
			route.StorageTable = value.(string)
		case "column_name":
			route.ColumnName = value.(string)
		case "column_type":
			route.ColumnType = value.(string)
		case "form_field_key":
			route.FormFieldKey = value.(string)
		case "query_alias":
			route.QueryAlias = value.(string)
		case "trigger_mode":
			route.TriggerMode = value.(string)
		case "cycle_ms":
			route.CycleMS = value.(int)
		case "deadband":
			route.Deadband = value.(float64)
		case "store_on_start":
			route.StoreOnStart = value.(bool)
		case "enabled":
			route.Enabled = value.(bool)
		}
	}
	return route
}

func freezeDetectionRunStorageRoutes(tx *gorm.DB, task *models.DetectionTask, runItems []models.DetectionRunStandardItem, now time.Time) ([]models.DetectionRunStorageRoute, error) {
	tags, err := loadTagsForStorageRoutes(tx, task.ProjectID, runItems)
	if err != nil {
		return nil, err
	}
	itemByVarID := make(map[int64]models.DetectionRunStandardItem, len(runItems))
	for _, item := range runItems {
		if item.StoreEnabled {
			itemByVarID[item.VarID] = item
		}
	}
	runRoutes := make([]models.DetectionRunStorageRoute, 0, len(tags))
	for _, tag := range tags {
		defaultRoute, err := ensureDefaultStorageRouteForTag(tx, tag)
		if err != nil {
			return nil, err
		}
		if item, ok := itemByVarID[tag.VarID]; ok && defaultRoute != nil && !defaultRoute.Enabled {
			cycleMS := item.CheckCycleMS
			if cycleMS <= 0 {
				cycleMS = 10000
			}
			if err := tx.Model(&models.StorageRoute{}).Where("id = ?", defaultRoute.ID).Updates(map[string]interface{}{
				"enabled":        true,
				"trigger_mode":   models.StoreTriggerOnCycle,
				"cycle_ms":       cycleMS,
				"store_on_start": true,
				"updated_at":     now,
			}).Error; err != nil {
				return nil, err
			}
		}
		routes, err := loadEnabledStorageRoutesForTag(tx, task.ProjectID, tag.VarID)
		if err != nil {
			return nil, err
		}
		for _, route := range routes {
			if route.StorageTarget == models.StorageTargetNone {
				continue
			}
			runRoutes = append(runRoutes, models.DetectionRunStorageRoute{
				TaskID:        task.ID,
				TestNo:        task.TestNo,
				ProjectID:     task.ProjectID,
				VarID:         tag.VarID,
				RouteID:       route.ID,
				RouteCode:     route.RouteCode,
				StorageTarget: route.StorageTarget,
				StorageTable:  route.StorageTable,
				ColumnName:    route.ColumnName,
				ColumnType:    route.ColumnType,
				FormFieldKey:  route.FormFieldKey,
				QueryAlias:    route.QueryAlias,
				TriggerMode:   route.TriggerMode,
				CycleMS:       route.CycleMS,
				Deadband:      route.Deadband,
				StoreOnStart:  route.StoreOnStart,
				CreatedAt:     now,
			})
		}
	}
	if len(runRoutes) > 0 {
		if err := tx.Create(&runRoutes).Error; err != nil {
			return nil, err
		}
	}
	return runRoutes, nil
}

func loadEnabledStorageRoutesForTag(db *gorm.DB, projectID uint, varID int64) ([]models.StorageRoute, error) {
	var routes []models.StorageRoute
	err := db.Where("project_id = ? AND var_id = ? AND enabled = ?", projectID, varID, true).
		Order("id asc").
		Find(&routes).Error
	return routes, err
}

func loadTagsForStorageRoutes(db *gorm.DB, ProjectID uint, runItems []models.DetectionRunStandardItem) ([]models.TagConfig, error) {
	query := db.Model(&models.TagConfig{}).Where("project_id = ? AND enabled = ?", ProjectID, true)
	if len(runItems) > 0 {
		varIDs := make([]int64, 0, len(runItems))
		seen := make(map[int64]struct{}, len(runItems))
		for _, item := range runItems {
			if !item.StoreEnabled {
				continue
			}
			if _, ok := seen[item.VarID]; ok {
				continue
			}
			seen[item.VarID] = struct{}{}
			varIDs = append(varIDs, item.VarID)
		}
		if len(varIDs) == 0 {
			return nil, nil
		}
		query = query.Where("var_id IN ?", varIDs)
	}
	var tags []models.TagConfig
	if err := query.Order("var_id asc").Find(&tags).Error; err != nil {
		return nil, err
	}
	for i := range tags {
		applyTagPersistenceDefaults(&tags[i])
	}
	return tags, nil
}
