package database

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"spindle-edge/backend/internal/models"
)

func (r *Repository) InsertHistoryBatch(tasks []*models.StoreTask) error {
	if len(tasks) == 0 {
		return nil
	}

	rows := make([]models.HistoryData, 0, len(tasks))
	for _, task := range tasks {
		if task.SkipHistoryRow {
			continue
		}
		row := models.HistoryData{
			GatewayID:   task.GatewayID,
			Topic:       task.Topic,
			ProjectID:   task.ProjectID,
			TaskID:      task.TaskID,
			TestNo:      task.TestNo,
			VarID:       task.VarID,
			VarName:     task.VarName,
			ProjectCode: task.ProjectCode,
			Quality:     task.Quality,
			SourceTime:  task.Timestamp,
		}
		if task.IsString {
			value := task.StrValue
			row.StrValue = &value
		} else {
			value := task.Value
			row.Value = &value
		}
		rows = append(rows, row)
	}

	if len(rows) > 0 {
		if err := r.db.CreateInBatches(rows, len(rows)).Error; err != nil {
			return err
		}
	}
	return r.InsertWideHistoryBatch(tasks)
}

func (r *Repository) InsertWideHistoryBatch(tasks []*models.StoreTask) error {
	rows := make(map[string]*wideHistoryRow)
	for _, task := range tasks {
		if len(task.StorageRoutes) == 0 {
			continue
		}
		for _, route := range task.StorageRoutes {
			if route.StorageTarget != models.StorageTargetWideTable {
				continue
			}
			row, err := makeWideHistoryRow(task, route)
			if err != nil {
				return err
			}
			key := rowKey(row)
			existing := rows[key]
			if existing == nil {
				existing = row
				rows[key] = existing
			}
			existing.Values[route.ColumnName] = storeTaskValue(task)
		}
	}
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := r.upsertWideHistoryRow(rows[key]); err != nil {
			return err
		}
	}
	return nil
}

type wideHistoryRow struct {
	TableName      string
	TaskID         uint
	TestNo         string
	ProjectID      uint
	ProjectCode    string
	SampleTime     time.Time
	SampleBucketMS int64
	Values         map[string]any
}

func makeWideHistoryRow(task *models.StoreTask, route models.DetectionRunStorageRoute) (*wideHistoryRow, error) {
	if err := ValidateStorageIdentifier(route.StorageTable); err != nil {
		return nil, err
	}
	if err := ValidateStorageIdentifier(route.ColumnName); err != nil {
		return nil, err
	}
	now := time.Now()
	sampleTime := task.Timestamp
	if sampleTime.IsZero() {
		sampleTime = now
	}
	return &wideHistoryRow{
		TableName:      route.StorageTable,
		TaskID:         task.TaskID,
		TestNo:         task.TestNo,
		ProjectID:      task.ProjectID,
		ProjectCode:    task.ProjectCode,
		SampleTime:     sampleTime,
		SampleBucketMS: sampleTime.UnixMilli(),
		Values:         make(map[string]any),
	}, nil
}

func storeTaskValue(task *models.StoreTask) any {
	if task.IsString {
		return task.StrValue
	}
	return task.Value
}

func rowKey(row *wideHistoryRow) string {
	return fmt.Sprintf("%s:%d:%d", row.TableName, row.TaskID, row.SampleBucketMS)
}

func (r *Repository) upsertWideHistoryRow(row *wideHistoryRow) error {
	if len(row.Values) == 0 {
		return nil
	}
	dialect := r.db.Name()
	table := quoteIdentifier(dialect, row.TableName)
	columns := make([]string, 0, len(row.Values))
	for column := range row.Values {
		if err := ValidateStorageIdentifier(column); err != nil {
			return err
		}
		columns = append(columns, column)
	}
	sort.Strings(columns)
	now := time.Now()
	insertColumns := []string{"task_id", "test_no", "project_id", "project_code", "sample_time", "sample_bucket_ms", "created_at", "updated_at"}
	values := []any{row.TaskID, row.TestNo, row.ProjectID, row.ProjectCode, row.SampleTime, row.SampleBucketMS, now, now}
	for _, column := range columns {
		insertColumns = append(insertColumns, column)
		values = append(values, row.Values[column])
	}
	quotedColumns := make([]string, 0, len(insertColumns))
	placeholders := make([]string, 0, len(insertColumns))
	for _, column := range insertColumns {
		quotedColumns = append(quotedColumns, quoteIdentifier(dialect, column))
		placeholders = append(placeholders, "?")
	}
	updateParts := make([]string, 0, len(columns)+2)
	if dialect == "sqlite" {
		for _, column := range columns {
			quoted := quoteIdentifier(dialect, column)
			updateParts = append(updateParts, fmt.Sprintf("%s = excluded.%s", quoted, quoted))
		}
		updateParts = append(updateParts, "updated_at = excluded.updated_at", "sample_time = excluded.sample_time")
		sqlText := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(task_id, sample_bucket_ms) DO UPDATE SET %s`,
			table, joinSQL(quotedColumns), joinSQL(placeholders), joinSQL(updateParts))
		return r.db.Exec(sqlText, values...).Error
	}
	for _, column := range columns {
		quoted := quoteIdentifier(dialect, column)
		updateParts = append(updateParts, fmt.Sprintf("%s = VALUES(%s)", quoted, quoted))
	}
	updateParts = append(updateParts, "updated_at = VALUES(updated_at)", "sample_time = VALUES(sample_time)")
	sqlText := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s) ON DUPLICATE KEY UPDATE %s`,
		table, joinSQL(quotedColumns), joinSQL(placeholders), joinSQL(updateParts))
	return r.db.Exec(sqlText, values...).Error
}

func joinSQL(parts []string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += ", "
		}
		result += part
	}
	return result
}

func (r *Repository) QueryHistoryData(filter HistoryFilter) ([]models.HistoryData, error) {
	if rows, err := r.QueryWideHistoryData(filter); err != nil {
		return nil, err
	} else if len(rows) > 0 {
		return rows, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 2000
	}
	if limit > 10000 {
		limit = 10000
	}

	query := r.db.Model(&models.HistoryData{})
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.TaskID != nil {
		query = query.Where("task_id = ?", *filter.TaskID)
	}
	if filter.ProjectCode != "" {
		query = query.Where("project_code = ?", filter.ProjectCode)
	}
	if filter.TestNo != "" {
		query = query.Where("test_no = ?", filter.TestNo)
	}
	if filter.Start != nil {
		query = query.Where("source_time >= ?", *filter.Start)
	}
	if filter.End != nil {
		query = query.Where("source_time <= ?", *filter.End)
	}

	var rows []models.HistoryData
	err := query.Order("source_time asc, id asc").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *Repository) QueryWideHistoryData(filter HistoryFilter) ([]models.HistoryData, error) {
	limit := normalizedHistoryLimit(filter.Limit)
	routes, err := r.historyStorageRoutes(filter)
	if err != nil {
		return nil, err
	}
	if len(routes) == 0 {
		return nil, nil
	}
	routesByTable := make(map[string][]models.DetectionRunStorageRoute)
	for _, route := range routes {
		if route.StorageTarget != models.StorageTargetWideTable {
			continue
		}
		if err := ValidateStorageIdentifier(route.StorageTable); err != nil {
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
	allRows := make([]models.HistoryData, 0)
	for _, tableName := range tableNames {
		if !r.db.Migrator().HasTable(tableName) {
			continue
		}
		rows, err := r.queryWideHistoryTable(filter, tableName, routesByTable[tableName], limit-len(allRows))
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

func (r *Repository) queryWideHistoryTable(filter HistoryFilter, tableName string, routes []models.DetectionRunStorageRoute, limit int) ([]models.HistoryData, error) {
	if limit <= 0 {
		return nil, nil
	}
	columns := make([]string, 0, len(routes))
	routeByColumn := make(map[string]models.DetectionRunStorageRoute, len(routes))
	for _, route := range routes {
		if _, ok := routeByColumn[route.ColumnName]; ok {
			continue
		}
		if err := ValidateStorageIdentifier(route.ColumnName); err != nil {
			return nil, err
		}
		routeByColumn[route.ColumnName] = route
		columns = append(columns, route.ColumnName)
	}
	if len(columns) == 0 {
		return nil, nil
	}
	sort.Strings(columns)
	hasProjectID := r.db.Migrator().HasColumn(tableName, "project_id")
	hasProjectCode := r.db.Migrator().HasColumn(tableName, "project_code")
	selectColumns := []string{"id", "task_id", "test_no", "sample_time", "sample_bucket_ms"}
	if hasProjectID {
		selectColumns = append(selectColumns, "project_id")
	}
	if hasProjectCode {
		selectColumns = append(selectColumns, "project_code")
	}
	selectColumns = append(selectColumns, columns...)
	quoted := make([]string, 0, len(selectColumns))
	dialect := r.db.Name()
	for _, column := range selectColumns {
		quoted = append(quoted, quoteIdentifier(dialect, column))
	}
	query := r.db.Table(tableName).Select(joinSQL(quoted))
	if filter.TaskID != nil {
		query = query.Where("task_id = ?", *filter.TaskID)
	}
	if filter.TestNo != "" {
		query = query.Where("test_no = ?", filter.TestNo)
	}
	if filter.ProjectCode != "" && hasProjectCode {
		query = query.Where("project_code = ?", filter.ProjectCode)
	}
	if filter.Start != nil {
		query = query.Where("sample_time >= ?", *filter.Start)
	}
	if filter.End != nil {
		query = query.Where("sample_time <= ?", *filter.End)
	}
	sqlRows, err := query.Order("sample_time asc, id asc").Limit(limit).Rows()
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

func (r *Repository) historyStorageRoutes(filter HistoryFilter) ([]models.DetectionRunStorageRoute, error) {
	query := r.db.Model(&models.DetectionRunStorageRoute{}).Where("storage_target = ?", models.StorageTargetWideTable)
	if filter.TaskID != nil {
		query = query.Where("task_id = ?", *filter.TaskID)
	}
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.TestNo != "" {
		query = query.Where("test_no = ?", filter.TestNo)
	}
	if filter.TaskID == nil && filter.ProjectID == nil && filter.TestNo == "" {
		return nil, nil
	}
	var routes []models.DetectionRunStorageRoute
	err := query.Order("var_id asc, id asc").Find(&routes).Error
	return routes, err
}

func scanWideHistoryRows(sqlRows *sql.Rows, dynamicColumns []string, routeByColumn map[string]models.DetectionRunStorageRoute, limit int) ([]models.HistoryData, error) {
	columnNames, err := sqlRows.Columns()
	if err != nil {
		return nil, err
	}
	rows := make([]models.HistoryData, 0)
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
			if !ok || isNilSQLValue(value) {
				continue
			}
			route := routeByColumn[column]
			projectID := uint(int64FromSQL(fixed["project_id"]))
			if projectID == 0 {
				projectID = route.ProjectID
			}
			projectCode := stringFromSQL(fixed["project_code"])
			row := models.HistoryData{
				ID:          uint64(int64FromSQL(fixed["id"])),
				ProjectID:   projectID,
				TaskID:      uint(int64FromSQL(fixed["task_id"])),
				TestNo:      stringFromSQL(fixed["test_no"]),
				VarID:       route.VarID,
				VarName:     firstStorageName(route.QueryAlias, route.FormFieldKey, route.ColumnName),
				ProjectCode: projectCode,
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

func isNilSQLValue(value any) bool {
	return value == nil
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
		var parsed int64
		_, _ = fmt.Sscan(string(v), &parsed)
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
		var parsed float64
		_, _ = fmt.Sscan(string(v), &parsed)
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
	default:
		return time.Time{}
	}
}

func isTextColumn(columnType string) bool {
	return columnType == "TEXT"
}
