package query

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	AlarmScopeDefault         = "default"
	AlarmScopeDetection       = "detection"
	DetectionAlarmStatusOpen  = "active"
	DetectionAlarmStatusClose = "recovered"

	TaskFlowStatusPending = "pending"
	TaskFlowStatusRunning = "running"
	TaskFlowStatusSuccess = "success"
	TaskFlowStatusFailed  = "failed"
	TaskFlowStatusTimeout = "timeout"
	TaskFlowStatusSkipped = "skipped"

	TaskFlowTriggerDataChange   = "data_change"
	TaskFlowTriggerSchedule     = "schedule"
	TaskFlowTriggerProjectStart = "project_start"
	TaskFlowTriggerProjectEnd   = "project_end"
	TaskFlowTriggerManual       = "manual"
)

type LimitAlarmFilter struct {
	Scope      string
	ProjectID  *uint
	TaskID     *uint
	TestNo     string
	VarID      *int64
	Status     string
	AlarmType  string
	AlarmLevel string
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

type TaskFlowRunFilter struct {
	ProjectID    *uint
	FlowID       *uint64
	FlowCode     string
	Status       string
	TriggerType  string
	TriggerVarID *int64
	OriginFlowID *uint64
	From         *time.Time
	To           *time.Time
	Limit        int
	Offset       int
}

type DetectionLimitAlarm struct {
	ID             uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Scope          string     `gorm:"column:scope" json:"scope"`
	TaskID         uint       `gorm:"column:task_id" json:"task_id"`
	TestNo         string     `gorm:"column:test_no" json:"test_no"`
	ProjectID      uint       `gorm:"column:project_id" json:"project_id"`
	ProjectCode    string     `gorm:"column:project_code" json:"project_code"`
	StandardID     *uint      `gorm:"column:standard_id" json:"standard_id,omitempty"`
	StandardItemID uint       `gorm:"column:standard_item_id" json:"standard_item_id"`
	RunStandardID  uint       `gorm:"column:run_standard_item_id" json:"run_standard_item_id"`
	VarID          int64      `gorm:"column:var_id" json:"var_id"`
	VarName        string     `gorm:"column:var_name" json:"var_name"`
	DisplayName    string     `gorm:"column:display_name" json:"display_name"`
	DisplayNameEN  string     `gorm:"column:display_name_en" json:"display_name_en"`
	DisplayNameJA  string     `gorm:"column:display_name_ja" json:"display_name_ja"`
	CheckMethod    string     `gorm:"column:check_method" json:"check_method"`
	AlarmType      string     `gorm:"column:alarm_type" json:"alarm_type"`
	AlarmLevel     string     `gorm:"column:alarm_level" json:"alarm_level"`
	Status         string     `gorm:"column:status" json:"status"`
	StartValue     *float64   `gorm:"column:start_value" json:"start_value,omitempty"`
	PeakValue      *float64   `gorm:"column:peak_value" json:"peak_value,omitempty"`
	RecoverValue   *float64   `gorm:"column:recover_value" json:"recover_value,omitempty"`
	LimitValue     *float64   `gorm:"column:limit_value" json:"limit_value,omitempty"`
	LimitDeadband  float64    `gorm:"column:limit_deadband" json:"limit_deadband"`
	Quality        int        `gorm:"column:quality" json:"quality"`
	FirstSeenAt    time.Time  `gorm:"column:first_seen_at" json:"first_seen_at"`
	LastSeenAt     time.Time  `gorm:"column:last_seen_at" json:"last_seen_at"`
	RecoveredAt    *time.Time `gorm:"column:recovered_at" json:"recovered_at,omitempty"`
	DurationMS     int64      `gorm:"column:duration_ms" json:"duration_ms"`
	Message        string     `gorm:"column:message" json:"message"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionLimitAlarm) TableName() string { return "detection_limit_alarms" }

func (a DetectionLimitAlarm) MarshalJSON() ([]byte, error) {
	type alias DetectionLimitAlarm
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(a),
		VarIDText: strconv.FormatInt(a.VarID, 10),
	})
}

type TaskFlowRun struct {
	ID            uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	FlowID        uint64     `gorm:"column:flow_id" json:"flow_id"`
	FlowCode      string     `gorm:"column:flow_code" json:"flow_code"`
	ProjectID     uint       `gorm:"column:project_id" json:"project_id"`
	TriggerType   string     `gorm:"column:trigger_type" json:"trigger_type"`
	TriggerVarID  int64      `gorm:"column:trigger_var_id" json:"trigger_var_id"`
	OriginFlowID  uint64     `gorm:"column:origin_flow_id" json:"origin_flow_id"`
	OriginRunID   uint64     `gorm:"column:origin_run_id" json:"origin_run_id"`
	Depth         int        `gorm:"column:depth" json:"depth"`
	Status        string     `gorm:"column:status" json:"status"`
	StartedAt     time.Time  `gorm:"column:started_at" json:"started_at"`
	FinishedAt    *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	DurationMS    int64      `gorm:"column:duration_ms" json:"duration_ms"`
	InputSnapshot string     `gorm:"column:input_snapshot" json:"input_snapshot"`
	ResultJSON    string     `gorm:"column:result_json" json:"result_json"`
	ErrorMessage  string     `gorm:"column:error_message" json:"error_message"`
	ScriptLogs    string     `gorm:"column:script_logs" json:"script_logs"`
	CreatedAt     time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (TaskFlowRun) TableName() string { return "task_flow_runs" }

func (r TaskFlowRun) MarshalJSON() ([]byte, error) {
	type alias TaskFlowRun
	return json.Marshal(struct {
		alias
		TriggerVarIDText string `json:"trigger_var_id_text"`
	}{
		alias:            alias(r),
		TriggerVarIDText: strconv.FormatInt(r.TriggerVarID, 10),
	})
}

type TaskFlowSQLLog struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	RunID        uint64    `gorm:"column:run_id" json:"run_id"`
	FlowID       uint64    `gorm:"column:flow_id" json:"flow_id"`
	SQLText      string    `gorm:"column:sql_text" json:"sql_text"`
	SQLArgs      string    `gorm:"column:sql_args" json:"sql_args"`
	AffectedRows int64     `gorm:"column:affected_rows" json:"affected_rows"`
	DurationMS   int64     `gorm:"column:duration_ms" json:"duration_ms"`
	ErrorMessage string    `gorm:"column:error_message" json:"error_message"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
}

func (TaskFlowSQLLog) TableName() string { return "task_flow_sql_logs" }

func (q *StationViewQuery) ListLimitAlarms(filter LimitAlarmFilter, edgeInstanceID string) ([]DetectionLimitAlarm, int64, int, int, error) {
	if filter.ProjectID != nil {
		if _, err := q.projectForEdge(*filter.ProjectID, edgeInstanceID); err != nil {
			return nil, 0, 0, 0, err
		}
	}
	limit := normalizedLimit(filter.Limit, 100, 500)
	offset := normalizedOffset(filter.Offset)
	stmt := q.db.Model(&DetectionLimitAlarm{})
	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Joins("LEFT JOIN sys_projects p ON p.id = detection_limit_alarms.project_id").
			Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if strings.TrimSpace(filter.Scope) != "" {
		stmt = stmt.Where("detection_limit_alarms.scope = ?", strings.TrimSpace(filter.Scope))
	}
	if filter.ProjectID != nil {
		stmt = stmt.Where("detection_limit_alarms.project_id = ?", *filter.ProjectID)
	}
	if filter.TaskID != nil {
		stmt = stmt.Where("detection_limit_alarms.task_id = ?", *filter.TaskID)
	}
	if strings.TrimSpace(filter.TestNo) != "" {
		stmt = stmt.Where("detection_limit_alarms.test_no = ?", strings.TrimSpace(filter.TestNo))
	}
	if filter.VarID != nil {
		stmt = stmt.Where("detection_limit_alarms.var_id = ?", *filter.VarID)
	}
	if strings.TrimSpace(filter.Status) != "" {
		stmt = stmt.Where("detection_limit_alarms.status = ?", strings.TrimSpace(filter.Status))
	}
	if strings.TrimSpace(filter.AlarmType) != "" {
		stmt = stmt.Where("detection_limit_alarms.alarm_type = ?", strings.TrimSpace(filter.AlarmType))
	}
	if strings.TrimSpace(filter.AlarmLevel) != "" {
		stmt = stmt.Where("detection_limit_alarms.alarm_level = ?", strings.TrimSpace(filter.AlarmLevel))
	}
	if filter.From != nil {
		stmt = stmt.Where("detection_limit_alarms.first_seen_at >= ?", *filter.From)
	}
	if filter.To != nil {
		stmt = stmt.Where("detection_limit_alarms.first_seen_at <= ?", *filter.To)
	}
	var total int64
	if err := stmt.Count(&total).Error; err != nil {
		return nil, 0, 0, 0, err
	}
	var alarms []DetectionLimitAlarm
	err := stmt.Order("detection_limit_alarms.last_seen_at desc, detection_limit_alarms.id desc").Limit(limit).Offset(offset).Find(&alarms).Error
	return alarms, total, limit, offset, err
}

func (q *StationViewQuery) ListTaskFlowRuns(filter TaskFlowRunFilter, edgeInstanceID string) ([]TaskFlowRun, int64, int, int, error) {
	if filter.ProjectID != nil {
		if _, err := q.projectForEdge(*filter.ProjectID, edgeInstanceID); err != nil {
			return nil, 0, 0, 0, err
		}
	}
	limit := normalizedLimit(filter.Limit, 50, 500)
	offset := normalizedOffset(filter.Offset)
	stmt := q.db.Model(&TaskFlowRun{})
	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Joins("LEFT JOIN sys_projects p ON p.id = task_flow_runs.project_id").
			Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if filter.ProjectID != nil {
		stmt = stmt.Where("task_flow_runs.project_id = ?", *filter.ProjectID)
	}
	if filter.FlowID != nil {
		stmt = stmt.Where("task_flow_runs.flow_id = ?", *filter.FlowID)
	}
	if strings.TrimSpace(filter.FlowCode) != "" {
		stmt = stmt.Where("task_flow_runs.flow_code = ?", strings.TrimSpace(filter.FlowCode))
	}
	if strings.TrimSpace(filter.Status) != "" {
		stmt = stmt.Where("task_flow_runs.status = ?", strings.TrimSpace(filter.Status))
	}
	if strings.TrimSpace(filter.TriggerType) != "" {
		stmt = stmt.Where("task_flow_runs.trigger_type = ?", strings.TrimSpace(filter.TriggerType))
	}
	if filter.TriggerVarID != nil {
		stmt = stmt.Where("task_flow_runs.trigger_var_id = ?", *filter.TriggerVarID)
	}
	if filter.OriginFlowID != nil {
		stmt = stmt.Where("task_flow_runs.origin_flow_id = ?", *filter.OriginFlowID)
	}
	if filter.From != nil {
		stmt = stmt.Where("task_flow_runs.started_at >= ?", *filter.From)
	}
	if filter.To != nil {
		stmt = stmt.Where("task_flow_runs.started_at <= ?", *filter.To)
	}
	var total int64
	if err := stmt.Count(&total).Error; err != nil {
		return nil, 0, 0, 0, err
	}
	var runs []TaskFlowRun
	err := stmt.Order("task_flow_runs.started_at desc, task_flow_runs.id desc").Limit(limit).Offset(offset).Find(&runs).Error
	return runs, total, limit, offset, err
}

func (q *StationViewQuery) GetTaskFlowRun(id uint64, edgeInstanceID string) (TaskFlowRun, error) {
	var run TaskFlowRun
	if id == 0 {
		return run, gorm.ErrRecordNotFound
	}
	if err := q.db.First(&run, "id = ?", id).Error; err != nil {
		return run, err
	}
	if run.ProjectID != 0 {
		if _, err := q.projectForEdge(run.ProjectID, edgeInstanceID); err != nil {
			return TaskFlowRun{}, err
		}
	}
	return run, nil
}

func (q *StationViewQuery) ListTaskFlowSQLLogs(runID uint64, limit int, edgeInstanceID string) ([]TaskFlowSQLLog, int, error) {
	if _, err := q.GetTaskFlowRun(runID, edgeInstanceID); err != nil {
		return nil, 0, err
	}
	limit = normalizedLimit(limit, 100, 1000)
	var logs []TaskFlowSQLLog
	err := q.db.Where("run_id = ?", runID).Order("created_at asc, id asc").Limit(limit).Find(&logs).Error
	return logs, limit, err
}

func normalizedLimit(value int, fallback int, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func normalizedOffset(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
