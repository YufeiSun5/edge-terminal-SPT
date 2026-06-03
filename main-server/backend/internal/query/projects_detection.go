package query

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type DetectionRunStorageRoute struct {
	ID            uint64    `gorm:"column:id;primaryKey" json:"id"`
	TaskID        uint      `gorm:"column:task_id" json:"task_id"`
	TestNo        string    `gorm:"column:test_no" json:"test_no"`
	ProjectID     uint      `gorm:"column:project_id" json:"project_id"`
	VarID         int64     `gorm:"column:var_id" json:"var_id"`
	RouteID       uint64    `gorm:"column:route_id" json:"route_id"`
	RouteCode     string    `gorm:"column:route_code" json:"route_code"`
	StorageTarget string    `gorm:"column:storage_target" json:"storage_target"`
	StorageTable  string    `gorm:"column:table_name" json:"table_name"`
	ColumnName    string    `gorm:"column:column_name" json:"column_name"`
	ColumnType    string    `gorm:"column:column_type" json:"column_type"`
	FormFieldKey  string    `gorm:"column:form_field_key" json:"form_field_key"`
	QueryAlias    string    `gorm:"column:query_alias" json:"query_alias"`
	TriggerMode   string    `gorm:"column:trigger_mode" json:"trigger_mode"`
	CycleMS       int       `gorm:"column:cycle_ms" json:"cycle_ms"`
	Deadband      float64   `gorm:"column:deadband" json:"deadband"`
	StoreOnStart  bool      `gorm:"column:store_on_start" json:"store_on_start"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
}

func (DetectionRunStorageRoute) TableName() string { return "detection_run_storage_routes" }

func (r DetectionRunStorageRoute) MarshalJSON() ([]byte, error) {
	type alias DetectionRunStorageRoute
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(r),
		VarIDText: strconv.FormatInt(r.VarID, 10),
	})
}

type DetectionRunNote struct {
	ID        uint64    `gorm:"column:id;primaryKey" json:"id"`
	TaskID    uint      `gorm:"column:task_id" json:"task_id"`
	NoteType  string    `gorm:"column:note_type" json:"note_type"`
	Content   string    `gorm:"column:content" json:"content"`
	ActorType string    `gorm:"column:actor_type" json:"actor_type"`
	ActorID   string    `gorm:"column:actor_id" json:"actor_id"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (DetectionRunNote) TableName() string { return "detection_run_notes" }

type DetectionRunReport struct {
	ID              uint64     `gorm:"column:id;primaryKey" json:"id"`
	TaskID          uint       `gorm:"column:task_id" json:"task_id"`
	TemplateID      *uint      `gorm:"column:template_id" json:"template_id,omitempty"`
	TemplateCode    string     `gorm:"column:template_code" json:"template_code"`
	TemplateVersion int        `gorm:"column:template_version" json:"template_version"`
	FileRef         string     `gorm:"column:file_ref" json:"file_ref"`
	FileName        string     `gorm:"column:file_name" json:"file_name"`
	Status          string     `gorm:"column:status" json:"status"`
	GeneratedAt     *time.Time `gorm:"column:generated_at" json:"generated_at,omitempty"`
	ErrorMessage    string     `gorm:"column:error_message" json:"error_message"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionRunReport) TableName() string { return "detection_run_reports" }

type DetectionRunReportRequest struct {
	ID              uint64    `gorm:"column:id;primaryKey" json:"id"`
	TaskID          uint      `gorm:"column:task_id" json:"task_id"`
	TestNo          string    `gorm:"column:test_no" json:"test_no"`
	ProjectID       uint      `gorm:"column:project_id" json:"project_id"`
	ProjectCode     string    `gorm:"column:project_code" json:"project_code"`
	TemplateID      *uint     `gorm:"column:template_id" json:"template_id,omitempty"`
	TemplateCode    string    `gorm:"column:template_code" json:"template_code"`
	TemplateVersion int       `gorm:"column:template_version" json:"template_version"`
	VarID           int64     `gorm:"column:var_id" json:"var_id"`
	VarName         string    `gorm:"column:var_name" json:"var_name"`
	DisplayName     string    `gorm:"column:display_name" json:"display_name"`
	DisplayNameEN   string    `gorm:"column:display_name_en" json:"display_name_en"`
	DisplayNameJA   string    `gorm:"column:display_name_ja" json:"display_name_ja"`
	ReportName      string    `gorm:"column:report_name" json:"report_name"`
	VariablesJSON   string    `gorm:"column:variables_json" json:"variables_json,omitempty"`
	ParamsJSON      string    `gorm:"column:params_json" json:"params_json,omitempty"`
	Status          string    `gorm:"column:status" json:"status"`
	Ext1            string    `gorm:"column:ext_1" json:"ext_1"`
	Ext2            string    `gorm:"column:ext_2" json:"ext_2"`
	Ext3            string    `gorm:"column:ext_3" json:"ext_3"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionRunReportRequest) TableName() string { return "detection_run_report_requests" }

type DetectionRunEvent struct {
	ID          uint64    `gorm:"column:id;primaryKey" json:"id"`
	TaskID      uint      `gorm:"column:task_id" json:"task_id"`
	TestNo      string    `gorm:"column:test_no" json:"test_no"`
	ProjectID   uint      `gorm:"column:project_id" json:"project_id"`
	ProjectCode string    `gorm:"column:project_code" json:"project_code"`
	EventType   string    `gorm:"column:event_type" json:"event_type"`
	EventLevel  string    `gorm:"column:event_level" json:"event_level"`
	Message     string    `gorm:"column:message" json:"message"`
	Detail      string    `gorm:"column:detail" json:"detail"`
	OccurredAt  time.Time `gorm:"column:occurred_at" json:"occurred_at"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
}

func (DetectionRunEvent) TableName() string { return "detection_run_events" }

type DetectionRunSummary struct {
	ID              uint64     `gorm:"column:id;primaryKey" json:"id"`
	TaskID          uint       `gorm:"column:task_id" json:"task_id"`
	TestNo          string     `gorm:"column:test_no" json:"test_no"`
	ProjectID       uint       `gorm:"column:project_id" json:"project_id"`
	ProjectCode     string     `gorm:"column:project_code" json:"project_code"`
	ResultStatus    string     `gorm:"column:result_status" json:"result_status"`
	StartedAt       *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	EndedAt         *time.Time `gorm:"column:ended_at" json:"ended_at,omitempty"`
	DurationMS      int64      `gorm:"column:duration_ms" json:"duration_ms"`
	HistoryRows     int64      `gorm:"column:history_rows" json:"history_rows"`
	AlarmTotal      int64      `gorm:"column:alarm_total" json:"alarm_total"`
	AlarmActive     int64      `gorm:"column:alarm_active" json:"alarm_active"`
	AlarmRecovered  int64      `gorm:"column:alarm_recovered" json:"alarm_recovered"`
	AlarmAboveH     int64      `gorm:"column:alarm_above_h" json:"alarm_above_h"`
	AlarmAboveHH    int64      `gorm:"column:alarm_above_hh" json:"alarm_above_hh"`
	AlarmBelowL     int64      `gorm:"column:alarm_below_l" json:"alarm_below_l"`
	AlarmBelowLL    int64      `gorm:"column:alarm_below_ll" json:"alarm_below_ll"`
	FirstAlarmAt    *time.Time `gorm:"column:first_alarm_at" json:"first_alarm_at,omitempty"`
	LastAlarmAt     *time.Time `gorm:"column:last_alarm_at" json:"last_alarm_at,omitempty"`
	LastRefreshedAt time.Time  `gorm:"column:last_refreshed_at" json:"last_refreshed_at"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionRunSummary) TableName() string { return "detection_run_summaries" }

type DetectionRunFeature struct {
	ID              uint64    `gorm:"column:id;primaryKey" json:"id"`
	TaskID          uint      `gorm:"column:task_id" json:"task_id"`
	TestNo          string    `gorm:"column:test_no" json:"test_no"`
	ProjectID       uint      `gorm:"column:project_id" json:"project_id"`
	ProjectCode     string    `gorm:"column:project_code" json:"project_code"`
	VarID           int64     `gorm:"column:var_id" json:"var_id"`
	VarName         string    `gorm:"column:var_name" json:"var_name"`
	SampleCount     int64     `gorm:"column:sample_count" json:"sample_count"`
	AvgValue        *float64  `gorm:"column:avg_value" json:"avg_value,omitempty"`
	MinValue        *float64  `gorm:"column:min_value" json:"min_value,omitempty"`
	MaxValue        *float64  `gorm:"column:max_value" json:"max_value,omitempty"`
	FirstSampleTime time.Time `gorm:"column:first_sample_time" json:"first_sample_time"`
	LastSampleTime  time.Time `gorm:"column:last_sample_time" json:"last_sample_time"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionRunFeature) TableName() string { return "detection_run_features" }

func (f DetectionRunFeature) MarshalJSON() ([]byte, error) {
	type alias DetectionRunFeature
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(f),
		VarIDText: strconv.FormatInt(f.VarID, 10),
	})
}

type DetectionRunFilter struct {
	ProjectID *uint
	Status    string
	TestNo    string
	Start     *time.Time
	End       *time.Time
	Limit     int
}

func (r DetectionRunReportRequest) MarshalJSON() ([]byte, error) {
	type alias DetectionRunReportRequest
	return json.Marshal(struct {
		alias
		VarIDText string          `json:"var_id_text"`
		Variables json.RawMessage `json:"variables"`
		Params    json.RawMessage `json:"params"`
	}{
		alias:     alias(r),
		VarIDText: strconv.FormatInt(r.VarID, 10),
		Variables: rawJSONOrDefault(r.VariablesJSON, "[]"),
		Params:    rawJSONOrDefault(r.ParamsJSON, "{}"),
	})
}

func (i DetectionRunStandardItem) MarshalJSON() ([]byte, error) {
	type alias DetectionRunStandardItem
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(i),
		VarIDText: strconv.FormatInt(i.VarID, 10),
	})
}

func (q *StationViewQuery) ListProjects(edgeInstanceID string) ([]Project, error) {
	var projects []Project
	stmt := q.db.Model(&Project{})
	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Where("(edge_instance_id = ? OR edge_instance_id = '' OR edge_instance_id IS NULL)", edgeInstanceID)
	}
	err := stmt.Order("id asc").Find(&projects).Error
	return projects, err
}

func (q *StationViewQuery) CurrentDetectionRun(projectID uint, edgeInstanceID string) (DetectionTask, error) {
	if _, err := q.projectForEdge(projectID, edgeInstanceID); err != nil {
		return DetectionTask{}, err
	}
	var task DetectionTask
	if err := q.db.
		Where("project_id = ? AND status IN ?", projectID, []string{DetectionStatusRunning, DetectionStatusPaused}).
		Order("started_at desc, id desc").
		First(&task).Error; err != nil {
		return task, err
	}
	return q.GetDetectionRun(task.ID)
}

func (q *StationViewQuery) ActiveDetectionRuns(edgeInstanceID string) ([]DetectionTask, error) {
	stmt := q.db.Model(&DetectionTask{}).
		Where("sys_detection_tasks.status IN ?", []string{DetectionStatusRunning, DetectionStatusPaused})
	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Joins("JOIN sys_projects p ON p.id = sys_detection_tasks.project_id").
			Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	var tasks []DetectionTask
	if err := stmt.Order("sys_detection_tasks.started_at desc, sys_detection_tasks.id desc").Find(&tasks).Error; err != nil {
		return nil, err
	}
	for i := range tasks {
		if err := q.attachRunSnapshots(&tasks[i], false); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func (q *StationViewQuery) ListDetectionRuns(filter DetectionRunFilter, edgeInstanceID string) ([]DetectionTask, int, error) {
	if filter.ProjectID != nil {
		if _, err := q.projectForEdge(*filter.ProjectID, edgeInstanceID); err != nil {
			return nil, 0, err
		}
	}
	limit := normalizedDetectionRunLimit(filter.Limit)
	stmt := q.db.Model(&DetectionTask{})
	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Joins("JOIN sys_projects p ON p.id = sys_detection_tasks.project_id").
			Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if filter.ProjectID != nil {
		stmt = stmt.Where("sys_detection_tasks.project_id = ?", *filter.ProjectID)
	}
	if strings.TrimSpace(filter.Status) != "" {
		stmt = stmt.Where("sys_detection_tasks.status = ?", strings.TrimSpace(filter.Status))
	}
	if strings.TrimSpace(filter.TestNo) != "" {
		stmt = stmt.Where("sys_detection_tasks.test_no = ?", strings.TrimSpace(filter.TestNo))
	}
	if filter.Start != nil {
		stmt = stmt.Where("sys_detection_tasks.started_at >= ?", *filter.Start)
	}
	if filter.End != nil {
		stmt = stmt.Where("sys_detection_tasks.started_at <= ?", *filter.End)
	}
	var tasks []DetectionTask
	if err := stmt.Order("sys_detection_tasks.started_at desc, sys_detection_tasks.id desc").Limit(limit).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	for i := range tasks {
		if err := q.attachRunSnapshots(&tasks[i], false); err != nil {
			return nil, 0, err
		}
	}
	return tasks, limit, nil
}

func (q *StationViewQuery) GetDetectionRun(taskID uint) (DetectionTask, error) {
	var task DetectionTask
	if err := q.db.First(&task, "id = ?", taskID).Error; err != nil {
		return task, err
	}
	if err := q.attachRunSnapshots(&task, true); err != nil {
		return task, err
	}
	return task, nil
}

func (q *StationViewQuery) DetectionRunSummary(taskID uint) (DetectionRunSummary, error) {
	if err := q.ensureDetectionRunExists(taskID); err != nil {
		return DetectionRunSummary{}, err
	}
	var summary DetectionRunSummary
	err := q.db.First(&summary, "task_id = ?", taskID).Error
	return summary, err
}

func (q *StationViewQuery) DetectionRunFeatures(taskID uint) ([]DetectionRunFeature, error) {
	if err := q.ensureDetectionRunExists(taskID); err != nil {
		return nil, err
	}
	var features []DetectionRunFeature
	err := q.db.Where("task_id = ?", taskID).Order("var_id asc, id asc").Find(&features).Error
	return features, err
}

func (q *StationViewQuery) DetectionRunEvents(taskID uint, limit int) ([]DetectionRunEvent, int, error) {
	if err := q.ensureDetectionRunExists(taskID); err != nil {
		return nil, 0, err
	}
	limit = normalizedDetectionRunEventLimit(limit)
	var events []DetectionRunEvent
	err := q.db.Where("task_id = ?", taskID).Order("occurred_at asc, id asc").Limit(limit).Find(&events).Error
	return events, limit, err
}

func (q *StationViewQuery) DetectionRunStorageRoutes(taskID uint) ([]DetectionRunStorageRoute, error) {
	if err := q.ensureDetectionRunExists(taskID); err != nil {
		return nil, err
	}
	var routes []DetectionRunStorageRoute
	err := q.db.Where("task_id = ?", taskID).Order("var_id asc, id asc").Find(&routes).Error
	return routes, err
}

func (q *StationViewQuery) DetectionRunReportRequests(taskID uint) ([]DetectionRunReportRequest, error) {
	if err := q.ensureDetectionRunExists(taskID); err != nil {
		return nil, err
	}
	var requests []DetectionRunReportRequest
	err := q.db.Where("task_id = ?", taskID).Order("id asc").Find(&requests).Error
	return requests, err
}

func (q *StationViewQuery) DetectionRunNotes(taskID uint, limit int, edgeInstanceID string) ([]DetectionRunNote, int, error) {
	if err := q.ensureDetectionRunForEdge(taskID, edgeInstanceID); err != nil {
		return nil, 0, err
	}
	limit = normalizedDetectionRunEventLimit(limit)
	var notes []DetectionRunNote
	err := q.db.Where("task_id = ?", taskID).Order("created_at desc, id desc").Limit(limit).Find(&notes).Error
	return notes, limit, err
}

func (q *StationViewQuery) attachRunSnapshots(task *DetectionTask, includeDetail bool) error {
	var items []DetectionRunStandardItem
	if err := q.db.Where("task_id = ?", task.ID).Order("sort_order asc, id asc").Find(&items).Error; err != nil {
		return err
	}
	task.StandardItems = items
	var routes []DetectionRunStorageRoute
	if err := q.db.Where("task_id = ?", task.ID).Order("var_id asc, id asc").Find(&routes).Error; err != nil {
		return err
	}
	task.StorageRoutes = routes
	if !includeDetail {
		return nil
	}

	var notes []DetectionRunNote
	if err := q.db.Where("task_id = ?", task.ID).Order("created_at desc, id desc").Limit(5).Find(&notes).Error; err == nil {
		task.RecentNotes = notes
	}
	var reports []DetectionRunReport
	if err := q.db.Where("task_id = ?", task.ID).Order("created_at desc, id desc").Find(&reports).Error; err == nil {
		task.Reports = reports
	}
	var reportRequests []DetectionRunReportRequest
	if err := q.db.Where("task_id = ?", task.ID).Order("id asc").Find(&reportRequests).Error; err == nil {
		task.ReportRequests = reportRequests
	}
	return nil
}

func (q *StationViewQuery) projectForEdge(projectID uint, requestedEdgeInstanceID string) (Project, error) {
	var project Project
	if projectID == 0 {
		return project, gorm.ErrRecordNotFound
	}
	if err := q.db.First(&project, "id = ?", projectID).Error; err != nil {
		return project, err
	}
	projectEdge := strings.TrimSpace(project.EdgeInstanceID)
	requestedEdgeInstanceID = strings.TrimSpace(requestedEdgeInstanceID)
	if projectEdge != "" && requestedEdgeInstanceID != "" && projectEdge != requestedEdgeInstanceID {
		return Project{}, gorm.ErrRecordNotFound
	}
	return project, nil
}

func (q *StationViewQuery) ensureDetectionRunExists(taskID uint) error {
	if taskID == 0 {
		return gorm.ErrRecordNotFound
	}
	var count int64
	if err := q.db.Model(&DetectionTask{}).Where("id = ?", taskID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (q *StationViewQuery) ensureDetectionRunForEdge(taskID uint, edgeInstanceID string) error {
	if taskID == 0 {
		return gorm.ErrRecordNotFound
	}
	stmt := q.db.Model(&DetectionTask{}).Where("sys_detection_tasks.id = ?", taskID)
	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Joins("JOIN sys_projects p ON p.id = sys_detection_tasks.project_id").
			Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	var count int64
	if err := stmt.Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func normalizedDetectionRunLimit(limit int) int {
	if limit <= 0 {
		return 200
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func normalizedDetectionRunEventLimit(limit int) int {
	if limit <= 0 {
		return 200
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func rawJSONOrDefault(value string, fallback string) json.RawMessage {
	if json.Valid([]byte(value)) {
		return json.RawMessage(value)
	}
	return json.RawMessage(fallback)
}

func ParseDetectionRunTime(raw string) (time.Time, error) {
	if value, err := time.Parse(time.RFC3339, raw); err == nil {
		return value, nil
	}
	if value, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local); err == nil {
		return value, nil
	}
	return time.Time{}, fmt.Errorf("invalid time")
}
