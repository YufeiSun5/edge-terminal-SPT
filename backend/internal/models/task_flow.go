package models

import (
	"encoding/json"
	"strconv"
	"time"
)

const (
	TaskFlowTriggerDataChange   = "data_change"
	TaskFlowTriggerSchedule     = "schedule"
	TaskFlowTriggerProjectStart = "project_start"
	TaskFlowTriggerProjectEnd   = "project_end"
	TaskFlowTriggerManual       = "manual"

	TaskFlowActionBuiltinStorageSnapshot       = "builtin.storage_snapshot"
	TaskFlowActionBuiltinStoragePrepare        = "builtin.storage_prepare"
	TaskFlowActionBuiltinStartDetectionRun     = "builtin.start_detection_run"
	TaskFlowActionBuiltinStopDetectionRun      = "builtin.stop_detection_run"
	TaskFlowActionBuiltinPauseDetectionRun     = "builtin.pause_detection_run"
	TaskFlowActionBuiltinResumeDetectionRun    = "builtin.resume_detection_run"
	TaskFlowActionBuiltinFixedDurationGuard    = "builtin.fixed_duration_guard"
	TaskFlowActionBuiltinQualifiedHoldGuard    = "builtin.qualified_hold_guard"
	TaskFlowActionBuiltinRefreshFeatures       = "builtin.refresh_features"
	TaskFlowActionBuiltinMuteDetectionAlarms   = "builtin.mute_detection_alarms"
	TaskFlowActionBuiltinUpdateDetectionLimits = "builtin.update_detection_limits"
	TaskFlowActionBuiltinRegisterReport        = "builtin.register_report"
	TaskFlowActionBuiltinHTTPRequest           = "builtin.http_request"
	TaskFlowActionBuiltinContextSet            = "builtin.context_set"
	TaskFlowActionBuiltinWriteVariable         = "builtin.write_variable"
	TaskFlowActionBuiltinWriteControlVariables = "builtin.write_control_variables"
	TaskFlowActionJavaScript                   = "javascript"

	TaskFlowStatusPending = "pending"
	TaskFlowStatusRunning = "running"
	TaskFlowStatusSuccess = "success"
	TaskFlowStatusFailed  = "failed"
	TaskFlowStatusTimeout = "timeout"
	TaskFlowStatusSkipped = "skipped"

	TaskFlowVarRoleWatch = "watch"
	TaskFlowVarRoleRead  = "read"
	TaskFlowVarRoleWrite = "write"
)

type TaskFlow struct {
	ID                 uint64        `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID          uint          `gorm:"column:project_id;index;not null" json:"project_id"`
	FlowCode           string        `gorm:"column:flow_code;size:128;uniqueIndex;not null" json:"flow_code"`
	Name               string        `gorm:"column:name;size:128;not null" json:"name"`
	Enabled            bool          `gorm:"column:enabled;default:true;index" json:"enabled"`
	TriggerType        string        `gorm:"column:trigger_type;size:32;index;not null" json:"trigger_type"`
	ConditionScript    string        `gorm:"column:condition_script;type:text" json:"condition_script"`
	ActionType         string        `gorm:"column:action_type;size:64;not null" json:"action_type"`
	ActionScript       string        `gorm:"column:action_script;type:text" json:"action_script"`
	ActionPayload      string        `gorm:"column:action_payload;type:text" json:"action_payload"`
	StepsJSON          string        `gorm:"column:steps_json;type:text" json:"steps_json"`
	TimeoutMS          int           `gorm:"column:timeout_ms;default:3000" json:"timeout_ms"`
	CooldownMS         int           `gorm:"column:cooldown_ms;default:0" json:"cooldown_ms"`
	HoldMS             int           `gorm:"column:hold_ms;default:0" json:"hold_ms"`
	ScheduleIntervalMS int           `gorm:"column:schedule_interval_ms;default:0" json:"schedule_interval_ms"`
	Priority           int           `gorm:"column:priority;default:0;index" json:"priority"`
	Remark             string        `gorm:"column:remark;size:255" json:"remark"`
	Vars               []TaskFlowVar `gorm:"foreignKey:FlowID;references:ID" json:"vars,omitempty"`
	CreatedAt          time.Time     `gorm:"column:created_at" json:"created_at"`
	UpdatedAt          time.Time     `gorm:"column:updated_at" json:"updated_at"`
}

func (TaskFlow) TableName() string {
	return "sys_task_flows"
}

type TaskFlowVar struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	FlowID    uint64    `gorm:"column:flow_id;index;not null" json:"flow_id"`
	ProjectID uint      `gorm:"column:project_id;index;not null" json:"project_id"`
	VarID     int64     `gorm:"column:var_id;index;not null" json:"var_id"`
	VarName   string    `gorm:"column:var_name;size:128" json:"var_name"`
	Role      string    `gorm:"column:role;size:32;default:watch;index" json:"role"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (TaskFlowVar) TableName() string {
	return "sys_task_flow_vars"
}

func (v TaskFlowVar) MarshalJSON() ([]byte, error) {
	type alias TaskFlowVar
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(v),
		VarIDText: strconv.FormatInt(v.VarID, 10),
	})
}

type TaskFlowRun struct {
	ID            uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	FlowID        uint64     `gorm:"column:flow_id;index;not null" json:"flow_id"`
	FlowCode      string     `gorm:"column:flow_code;size:128;index" json:"flow_code"`
	ProjectID     uint       `gorm:"column:project_id;index;not null" json:"project_id"`
	TriggerType   string     `gorm:"column:trigger_type;size:32;index" json:"trigger_type"`
	TriggerVarID  int64      `gorm:"column:trigger_var_id;index" json:"trigger_var_id"`
	OriginFlowID  uint64     `gorm:"column:origin_flow_id;index;default:0" json:"origin_flow_id"`
	OriginRunID   uint64     `gorm:"column:origin_run_id;index;default:0" json:"origin_run_id"`
	Depth         int        `gorm:"column:depth;default:0" json:"depth"`
	Status        string     `gorm:"column:status;size:32;index;not null" json:"status"`
	StartedAt     time.Time  `gorm:"column:started_at;index" json:"started_at"`
	FinishedAt    *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	DurationMS    int64      `gorm:"column:duration_ms" json:"duration_ms"`
	InputSnapshot string     `gorm:"column:input_snapshot;type:text" json:"input_snapshot"`
	ResultJSON    string     `gorm:"column:result_json;type:text" json:"result_json"`
	ErrorMessage  string     `gorm:"column:error_message;size:1024" json:"error_message"`
	ScriptLogs    string     `gorm:"column:script_logs;type:text" json:"script_logs"`
	CreatedAt     time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (TaskFlowRun) TableName() string {
	return "task_flow_runs"
}

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
	RunID        uint64    `gorm:"column:run_id;index;not null" json:"run_id"`
	FlowID       uint64    `gorm:"column:flow_id;index;not null" json:"flow_id"`
	SQLText      string    `gorm:"column:sql_text;type:text" json:"sql_text"`
	SQLArgs      string    `gorm:"column:sql_args;type:text" json:"sql_args"`
	AffectedRows int64     `gorm:"column:affected_rows" json:"affected_rows"`
	DurationMS   int64     `gorm:"column:duration_ms" json:"duration_ms"`
	ErrorMessage string    `gorm:"column:error_message;size:1024" json:"error_message"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
}

func (TaskFlowSQLLog) TableName() string {
	return "task_flow_sql_logs"
}
