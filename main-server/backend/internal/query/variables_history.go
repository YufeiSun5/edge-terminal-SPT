package query

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const StorageTargetWideTable = "wide_table"

type VariableFilter struct {
	GatewayID   *int
	ProjectID   *uint
	Assigned    *bool
	Enabled     *bool
	Discovered  *bool
	Writable    *bool
	SourceType  string
	ProjectCode string
	VarGroup    string
	Keyword     string
	Limit       int
	Offset      int
}

type HistoryFilter struct {
	ProjectID   *uint
	TaskID      *uint
	VarID       *int64
	ProjectCode string
	TestNo      string
	FactoryNo   string
	Start       *time.Time
	End         *time.Time
	Limit       int
}

type HistoryData struct {
	ID          uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	GatewayID   int       `gorm:"column:gateway_id" json:"gateway_id"`
	Topic       string    `gorm:"column:topic" json:"topic"`
	ProjectID   uint      `gorm:"column:project_id" json:"project_id"`
	TaskID      uint      `gorm:"column:task_id" json:"task_id"`
	TestNo      string    `gorm:"column:test_no" json:"test_no"`
	VarID       int64     `gorm:"column:var_id" json:"var_id"`
	VarName     string    `gorm:"column:var_name" json:"var_name"`
	ProjectCode string    `gorm:"column:project_code" json:"project_code"`
	Value       *float64  `gorm:"column:value" json:"value,omitempty"`
	StrValue    *string   `gorm:"column:str_value" json:"str_value,omitempty"`
	Quality     int       `gorm:"column:quality" json:"quality"`
	SourceTime  time.Time `gorm:"column:source_time" json:"source_time"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
}

func (HistoryData) TableName() string { return "rt_history_data" }

func (h HistoryData) MarshalJSON() ([]byte, error) {
	type alias HistoryData
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(h),
		VarIDText: strconv.FormatInt(h.VarID, 10),
	})
}

func (q *StationViewQuery) ListVariables(filter VariableFilter, edgeInstanceID string) ([]TagConfig, error) {
	stmt, err := q.variablesQuery(filter, edgeInstanceID)
	if err != nil {
		return nil, err
	}
	var tags []TagConfig
	err = stmt.Order("sys_tags.project_id asc, sys_tags.var_group asc, sys_tags.var_name asc, sys_tags.var_id asc").Find(&tags).Error
	return tags, err
}

func (q *StationViewQuery) ListVariablesPage(filter VariableFilter, edgeInstanceID string) ([]TagConfig, int64, int, int, error) {
	stmt, err := q.variablesQuery(filter, edgeInstanceID)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	limit := normalizedLimit(filter.Limit, 100, 500)
	offset := normalizedOffset(filter.Offset)
	var total int64
	if err := stmt.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, limit, offset, err
	}
	var tags []TagConfig
	err = stmt.Order("sys_tags.project_id asc, sys_tags.var_group asc, sys_tags.var_name asc, sys_tags.var_id asc").Limit(limit).Offset(offset).Find(&tags).Error
	return tags, total, limit, offset, err
}

func (q *StationViewQuery) variablesQuery(filter VariableFilter, edgeInstanceID string) (*gorm.DB, error) {
	if filter.ProjectID != nil {
		if _, err := q.projectForEdge(*filter.ProjectID, edgeInstanceID); err != nil {
			return nil, err
		}
	}
	stmt := q.db.Model(&TagConfig{})
	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Joins("LEFT JOIN sys_projects p ON p.id = sys_tags.project_id").
			Where("(sys_tags.project_id IS NULL OR p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if filter.GatewayID != nil {
		stmt = stmt.Where("sys_tags.gateway_id = ?", *filter.GatewayID)
	}
	if filter.ProjectID != nil {
		stmt = stmt.Where("sys_tags.project_id = ?", *filter.ProjectID)
	}
	if filter.Assigned != nil {
		if *filter.Assigned {
			stmt = stmt.Where("sys_tags.project_id IS NOT NULL")
		} else {
			stmt = stmt.Where("sys_tags.project_id IS NULL")
		}
	}
	if filter.Enabled != nil {
		stmt = stmt.Where("sys_tags.enabled = ?", *filter.Enabled)
	}
	if filter.Discovered != nil {
		stmt = stmt.Where("sys_tags.discovered = ?", *filter.Discovered)
	}
	if filter.Writable != nil {
		stmt = stmt.Where("sys_tags.writable = ?", *filter.Writable)
	}
	if strings.TrimSpace(filter.SourceType) != "" {
		stmt = stmt.Where("sys_tags.source_type = ?", strings.TrimSpace(filter.SourceType))
	}
	if strings.TrimSpace(filter.ProjectCode) != "" {
		stmt = stmt.Where("sys_tags.project_code = ?", strings.TrimSpace(filter.ProjectCode))
	}
	if strings.TrimSpace(filter.VarGroup) != "" {
		stmt = stmt.Where("sys_tags.var_group = ?", strings.TrimSpace(filter.VarGroup))
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		stmt = stmt.Where(
			"sys_tags.var_name LIKE ? OR sys_tags.raw_name LIKE ? OR sys_tags.display_name LIKE ? OR sys_tags.display_name_en LIKE ? OR sys_tags.display_name_ja LIKE ?",
			like, like, like, like, like,
		)
	}
	return stmt, nil
}

func (q *StationViewQuery) QueryHistoryData(filter HistoryFilter, edgeInstanceID string) ([]HistoryData, int, error) {
	if filter.ProjectID != nil {
		if _, err := q.projectForEdge(*filter.ProjectID, edgeInstanceID); err != nil {
			return nil, 0, err
		}
	}
	limit := normalizedHistoryLimit(filter.Limit)
	if rows, err := q.QueryWideHistoryData(filter, edgeInstanceID); err != nil {
		return nil, 0, err
	} else if len(rows) > 0 {
		return rows, limit, nil
	}

	stmt := q.db.Model(&HistoryData{})
	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Joins("LEFT JOIN sys_projects p ON p.id = rt_history_data.project_id").
			Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if filter.ProjectID != nil {
		stmt = stmt.Where("rt_history_data.project_id = ?", *filter.ProjectID)
	}
	if filter.TaskID != nil {
		stmt = stmt.Where("rt_history_data.task_id = ?", *filter.TaskID)
	}
	if filter.VarID != nil {
		stmt = stmt.Where("rt_history_data.var_id = ?", *filter.VarID)
	}
	if strings.TrimSpace(filter.ProjectCode) != "" {
		stmt = stmt.Where("rt_history_data.project_code = ?", strings.TrimSpace(filter.ProjectCode))
	}
	if strings.TrimSpace(filter.TestNo) != "" {
		stmt = stmt.Where("rt_history_data.test_no = ?", strings.TrimSpace(filter.TestNo))
	}
	if strings.TrimSpace(filter.FactoryNo) != "" {
		stmt = stmt.Where("rt_history_data.task_id IN (?)", q.db.Model(&DetectionTask{}).Select("id").Where("factory_no = ?", strings.TrimSpace(filter.FactoryNo)))
	}
	if filter.Start != nil {
		stmt = stmt.Where("rt_history_data.source_time >= ?", *filter.Start)
	}
	if filter.End != nil {
		stmt = stmt.Where("rt_history_data.source_time <= ?", *filter.End)
	}
	var rows []HistoryData
	err := stmt.Order("rt_history_data.source_time asc, rt_history_data.id asc").Limit(limit).Find(&rows).Error
	return rows, limit, err
}

func (q *StationViewQuery) QueryWideHistoryData(filter HistoryFilter, edgeInstanceID string) ([]HistoryData, error) {
	limit := normalizedHistoryLimit(filter.Limit)
	routes, err := q.historyStorageRoutes(filter, edgeInstanceID)
	if err != nil {
		return nil, err
	}
	if len(routes) == 0 {
		return nil, nil
	}
	routesByTable := make(map[string][]DetectionRunStorageRoute)
	for _, route := range routes {
		if route.StorageTarget != StorageTargetWideTable {
			continue
		}
		if err := validateStorageIdentifier(route.StorageTable); err != nil {
			return nil, err
		}
		routesByTable[route.StorageTable] = append(routesByTable[route.StorageTable], route)
	}
	if len(routesByTable) == 0 {
		return nil, nil
	}
	tableNames := make([]string, 0, len(routesByTable))
	for tableName := range routesByTable {
		tableNames = append(tableNames, tableName)
	}
	sort.Strings(tableNames)
	allRows := make([]HistoryData, 0)
	for _, tableName := range tableNames {
		if !q.db.Migrator().HasTable(tableName) {
			continue
		}
		rows, err := q.queryWideHistoryTable(filter, tableName, routesByTable[tableName], limit-len(allRows))
		if err != nil {
			return nil, err
		}
		allRows = append(allRows, rows...)
		if len(allRows) >= limit {
			break
		}
	}
	sort.SliceStable(allRows, func(i, j int) bool {
		if allRows[i].SourceTime.Equal(allRows[j].SourceTime) {
			return allRows[i].ID < allRows[j].ID
		}
		return allRows[i].SourceTime.Before(allRows[j].SourceTime)
	})
	if len(allRows) > limit {
		allRows = allRows[:limit]
	}
	return allRows, nil
}

func (q *StationViewQuery) historyStorageRoutes(filter HistoryFilter, edgeInstanceID string) ([]DetectionRunStorageRoute, error) {
	stmt := q.db.Model(&DetectionRunStorageRoute{}).
		Where("detection_run_storage_routes.storage_target = ?", StorageTargetWideTable)
	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Joins("LEFT JOIN sys_projects p ON p.id = detection_run_storage_routes.project_id").
			Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if filter.TaskID != nil {
		stmt = stmt.Where("detection_run_storage_routes.task_id = ?", *filter.TaskID)
	}
	if filter.VarID != nil {
		stmt = stmt.Where("detection_run_storage_routes.var_id = ?", *filter.VarID)
	}
	if filter.ProjectID != nil {
		stmt = stmt.Where("detection_run_storage_routes.project_id = ?", *filter.ProjectID)
	}
	if strings.TrimSpace(filter.TestNo) != "" {
		stmt = stmt.Where("detection_run_storage_routes.test_no = ?", strings.TrimSpace(filter.TestNo))
	}
	if strings.TrimSpace(filter.FactoryNo) != "" {
		stmt = stmt.Where("detection_run_storage_routes.task_id IN (?)", q.db.Model(&DetectionTask{}).Select("id").Where("factory_no = ?", strings.TrimSpace(filter.FactoryNo)))
	}
	if filter.TaskID == nil && filter.ProjectID == nil && strings.TrimSpace(filter.TestNo) == "" && strings.TrimSpace(filter.FactoryNo) == "" {
		return nil, nil
	}
	var routes []DetectionRunStorageRoute
	err := stmt.Order("detection_run_storage_routes.var_id asc, detection_run_storage_routes.id asc").Find(&routes).Error
	return routes, err
}

func (q *StationViewQuery) queryWideHistoryTable(filter HistoryFilter, tableName string, routes []DetectionRunStorageRoute, limit int) ([]HistoryData, error) {
	if limit <= 0 {
		return nil, nil
	}
	columns := make([]string, 0, len(routes))
	routeByColumn := make(map[string]DetectionRunStorageRoute, len(routes))
	for _, route := range routes {
		if _, ok := routeByColumn[route.ColumnName]; ok {
			continue
		}
		if err := validateStorageIdentifier(route.ColumnName); err != nil {
			return nil, err
		}
		routeByColumn[route.ColumnName] = route
		columns = append(columns, route.ColumnName)
	}
	if len(columns) == 0 {
		return nil, nil
	}
	sort.Strings(columns)
	hasProjectID := q.db.Migrator().HasColumn(tableName, "project_id")
	hasProjectCode := q.db.Migrator().HasColumn(tableName, "project_code")
	selectColumns := []string{"id", "task_id", "test_no", "sample_time", "sample_bucket_ms"}
	if hasProjectID {
		selectColumns = append(selectColumns, "project_id")
	}
	if hasProjectCode {
		selectColumns = append(selectColumns, "project_code")
	}
	selectColumns = append(selectColumns, columns...)
	quoted := make([]string, 0, len(selectColumns))
	dialect := q.db.Name()
	for _, column := range selectColumns {
		quoted = append(quoted, quoteIdentifier(dialect, column))
	}
	stmt := q.db.Table(tableName).Select(joinSQL(quoted))
	if filter.TaskID != nil {
		stmt = stmt.Where("task_id = ?", *filter.TaskID)
	}
	if strings.TrimSpace(filter.TestNo) != "" {
		stmt = stmt.Where("test_no = ?", strings.TrimSpace(filter.TestNo))
	}
	if strings.TrimSpace(filter.FactoryNo) != "" {
		stmt = stmt.Where("task_id IN (?)", q.db.Model(&DetectionTask{}).Select("id").Where("factory_no = ?", strings.TrimSpace(filter.FactoryNo)))
	}
	if strings.TrimSpace(filter.ProjectCode) != "" && hasProjectCode {
		stmt = stmt.Where("project_code = ?", strings.TrimSpace(filter.ProjectCode))
	}
	if filter.Start != nil {
		stmt = stmt.Where("sample_time >= ?", *filter.Start)
	}
	if filter.End != nil {
		stmt = stmt.Where("sample_time <= ?", *filter.End)
	}
	sqlRows, err := stmt.Order("sample_time asc, id asc").Limit(limit).Rows()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = sqlRows.Close()
	}()
	return scanWideHistoryRows(sqlRows, columns, routeByColumn, limit)
}

func normalizedHistoryLimit(limit int) int {
	if limit <= 0 {
		return 2000
	}
	if limit > 10000 {
		return 10000
	}
	return limit
}

func scanWideHistoryRows(sqlRows *sql.Rows, dynamicColumns []string, routeByColumn map[string]DetectionRunStorageRoute, limit int) ([]HistoryData, error) {
	columnNames, err := sqlRows.Columns()
	if err != nil {
		return nil, err
	}
	rows := make([]HistoryData, 0)
	for sqlRows.Next() {
		values := make([]any, len(columnNames))
		pointers := make([]any, len(columnNames))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := sqlRows.Scan(pointers...); err != nil {
			return nil, err
		}
		fixed := map[string]any{}
		dynamic := map[string]any{}
		for i, name := range columnNames {
			if _, ok := routeByColumn[name]; ok {
				dynamic[name] = values[i]
			} else {
				fixed[name] = values[i]
			}
		}
		for _, column := range dynamicColumns {
			value, ok := dynamic[column]
			if !ok || value == nil {
				continue
			}
			route := routeByColumn[column]
			projectID := uint(int64FromSQL(fixed["project_id"]))
			if projectID == 0 {
				projectID = route.ProjectID
			}
			row := HistoryData{
				ID:          uint64(int64FromSQL(fixed["id"])),
				ProjectID:   projectID,
				TaskID:      uint(int64FromSQL(fixed["task_id"])),
				TestNo:      stringFromSQL(fixed["test_no"]),
				VarID:       route.VarID,
				VarName:     firstStorageName(route.QueryAlias, route.FormFieldKey, route.ColumnName),
				ProjectCode: stringFromSQL(fixed["project_code"]),
				Quality:     1,
				SourceTime:  timeFromSQL(fixed["sample_time"]),
			}
			if isTextColumn(route.ColumnType) {
				str := stringFromSQL(value)
				row.StrValue = &str
			} else {
				number := floatFromSQL(value)
				row.Value = &number
			}
			rows = append(rows, row)
			if len(rows) >= limit {
				return rows, sqlRows.Err()
			}
		}
	}
	return rows, sqlRows.Err()
}

func firstStorageName(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func quoteIdentifier(dialect string, identifier string) string {
	if dialect == "mysql" {
		return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func joinSQL(parts []string) string {
	return strings.Join(parts, ", ")
}

func validateStorageIdentifier(value string) error {
	if value == "" {
		return fmt.Errorf("empty storage identifier")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("invalid storage identifier")
	}
	return nil
}

func int64FromSQL(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case uint64:
		return int64(v)
	case []byte:
		parsed, _ := strconv.ParseInt(string(v), 10, 64)
		return parsed
	default:
		return 0
	}
}

func floatFromSQL(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case []byte:
		parsed, _ := strconv.ParseFloat(string(v), 64)
		return parsed
	default:
		return 0
	}
}

func stringFromSQL(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func timeFromSQL(value any) time.Time {
	switch v := value.(type) {
	case time.Time:
		return v
	case string:
		parsed, _ := time.Parse(time.RFC3339Nano, v)
		return parsed
	case []byte:
		parsed, _ := time.Parse(time.RFC3339Nano, string(v))
		return parsed
	default:
		return time.Time{}
	}
}

func isTextColumn(columnType string) bool {
	return strings.EqualFold(columnType, "TEXT") || strings.EqualFold(columnType, "VARCHAR") || strings.EqualFold(columnType, "STRING")
}

func ParseHistoryTime(raw string) (time.Time, error) {
	if value, err := time.Parse(time.RFC3339, raw); err == nil {
		return value, nil
	}
	if value, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local); err == nil {
		return value, nil
	}
	return time.Time{}, fmt.Errorf("invalid time")
}
