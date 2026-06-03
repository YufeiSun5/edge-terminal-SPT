package query

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type SyncDiagnostics struct {
	OverallStatus string                `json:"overall_status"`
	CheckedAt     time.Time             `json:"checked_at"`
	Tables        []SyncDiagnosticTable `json:"tables"`
	MissingTables []string              `json:"missing_tables,omitempty"`
	Warnings      []string              `json:"warnings,omitempty"`
}

type SyncDiagnosticTable struct {
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	RowCount     int64      `json:"row_count"`
	LatestColumn string     `json:"latest_column,omitempty"`
	LatestAt     *time.Time `json:"latest_at,omitempty"`
	Error        string     `json:"error,omitempty"`
}

type syncDiagnosticTableSpec struct {
	name         string
	latestColumn string
}

func (q *StationViewQuery) SyncDiagnostics() SyncDiagnostics {
	specs := []syncDiagnosticTableSpec{
		{name: "sys_gateways", latestColumn: "updated_at"},
		{name: "sys_projects", latestColumn: "updated_at"},
		{name: "sys_project_members", latestColumn: "updated_at"},
		{name: "sys_users", latestColumn: "updated_at"},
		{name: "sys_tags", latestColumn: "updated_at"},
		{name: "sys_detection_tasks", latestColumn: "updated_at"},
		{name: "sys_detection_standards", latestColumn: "updated_at"},
		{name: "sys_detection_standard_items", latestColumn: "updated_at"},
		{name: "sys_detection_standard_favorites", latestColumn: "updated_at"},
		{name: "sys_detection_standard_recents", latestColumn: "updated_at"},
		{name: "sys_storage_routes", latestColumn: "updated_at"},
		{name: "sys_task_flows", latestColumn: "updated_at"},
		{name: "sys_task_flow_vars", latestColumn: "created_at"},
		{name: "detection_run_standard_items", latestColumn: "created_at"},
		{name: "detection_run_storage_routes", latestColumn: "created_at"},
		{name: "detection_run_report_requests", latestColumn: "updated_at"},
		{name: "detection_run_summaries", latestColumn: "updated_at"},
		{name: "detection_run_features", latestColumn: "updated_at"},
		{name: "detection_limit_alarms", latestColumn: "updated_at"},
		{name: "sys_report_templates", latestColumn: "updated_at"},
		{name: "rt_history_data", latestColumn: "created_at"},
		{name: "task_flow_runs", latestColumn: "created_at"},
		{name: "task_flow_sql_logs", latestColumn: "created_at"},
		{name: "sys_audit_logs", latestColumn: "created_at"},
		{name: "sys_notifications", latestColumn: "created_at"},
		{name: "sys_notification_recipients", latestColumn: "created_at"},
		{name: "sys_station_view_templates", latestColumn: "updated_at"},
		{name: "sys_station_view_regions"},
		{name: "sys_station_view_items"},
		{name: "sys_station_view_assignments"},
	}
	result := SyncDiagnostics{
		OverallStatus: "ok",
		CheckedAt:     time.Now(),
		Tables:        make([]SyncDiagnosticTable, 0, len(specs)),
	}
	for _, spec := range specs {
		table := q.inspectSyncTable(spec)
		if table.Status != "ok" {
			result.OverallStatus = "degraded"
			if table.Status == "missing" {
				result.MissingTables = append(result.MissingTables, table.Name)
			}
		}
		result.Tables = append(result.Tables, table)
	}
	if len(result.MissingTables) > 0 {
		result.Warnings = append(result.Warnings, "one or more synchronized edge tables are not available in the main-server mirror database")
	}
	return result
}

func (q *StationViewQuery) inspectSyncTable(spec syncDiagnosticTableSpec) SyncDiagnosticTable {
	table := SyncDiagnosticTable{Name: spec.name, Status: "ok", LatestColumn: spec.latestColumn}
	if err := q.db.Table(spec.name).Count(&table.RowCount).Error; err != nil {
		table.Status = classifySyncTableError(err)
		table.Error = err.Error()
		return table
	}
	if spec.latestColumn == "" {
		return table
	}
	var latest sql.NullString
	if err := q.db.Table(spec.name).Select(fmt.Sprintf("MAX(%s)", spec.latestColumn)).Scan(&latest).Error; err != nil {
		table.Status = "error"
		table.Error = err.Error()
		return table
	}
	if latest.Valid {
		if parsed, ok := parseSyncDiagnosticTime(latest.String); ok {
			table.LatestAt = &parsed
		}
	}
	return table
}

func classifySyncTableError(err error) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "no such table") || strings.Contains(message, "doesn't exist") || strings.Contains(message, "does not exist") {
		return "missing"
	}
	return "error"
}

func parseSyncDiagnosticTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if value, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return value, true
		}
	}
	return time.Time{}, false
}
