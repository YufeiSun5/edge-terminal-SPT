package models

import (
	"encoding/json"
	"strconv"
	"time"
)

type DetectionTask struct {
	ID                    uint                        `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TestNo                string                      `gorm:"column:test_no;size:128;uniqueIndex;not null" json:"test_no"`
	ProjectID             uint                        `gorm:"column:project_id;index;not null" json:"project_id"`
	ProjectCode           string                      `gorm:"column:project_code;size:64;index" json:"project_code"`
	Mode                  string                      `gorm:"column:mode;size:64;not null" json:"mode"`
	Status                string                      `gorm:"column:status;size:32;index;not null" json:"status"`
	StandardID            *uint                       `gorm:"column:standard_id;index" json:"standard_id,omitempty"`
	StandardCode          string                      `gorm:"column:standard_code;size:64" json:"standard_code"`
	StandardVer           int                         `gorm:"column:standard_version;default:0" json:"standard_version"`
	StartedAt             *time.Time                  `gorm:"column:started_at" json:"started_at,omitempty"`
	EndedAt               *time.Time                  `gorm:"column:ended_at" json:"ended_at,omitempty"`
	LimitCheckEnabled     bool                        `gorm:"column:limit_check_enabled;default:true" json:"limit_check_enabled"`
	EndPolicy             string                      `gorm:"column:end_policy;size:32;default:manual;index" json:"end_policy"`
	DurationSec           int                         `gorm:"column:duration_sec;default:0" json:"duration_sec"`
	QualifiedHoldMS       int                         `gorm:"column:qualified_hold_ms;default:0" json:"qualified_hold_ms"`
	ExpectedEndAt         *time.Time                  `gorm:"column:expected_end_at" json:"expected_end_at,omitempty"`
	PauseStartedAt        *time.Time                  `gorm:"column:pause_started_at" json:"pause_started_at,omitempty"`
	PausedDurationMS      int64                       `gorm:"column:paused_duration_ms;default:0" json:"paused_duration_ms"`
	EndType               string                      `gorm:"column:end_type;size:32" json:"end_type"`
	StopReason            string                      `gorm:"column:stop_reason;size:255" json:"stop_reason"`
	OperatorNote          string                      `gorm:"column:operator_note;size:512" json:"operator_note"`
	CustomConfigJSON      string                      `gorm:"column:custom_config_json;type:text" json:"custom_config_json,omitempty"`
	TemplateRef           string                      `gorm:"column:template_ref;size:128" json:"template_ref"`
	ReportTemplateID      *uint                       `gorm:"column:report_template_id;index" json:"report_template_id,omitempty"`
	ReportTemplateCode    string                      `gorm:"column:report_template_code;size:64" json:"report_template_code"`
	ReportTemplateVersion int                         `gorm:"column:report_template_version;default:0" json:"report_template_version"`
	CreatedAt             time.Time                   `gorm:"column:created_at" json:"created_at"`
	UpdatedAt             time.Time                   `gorm:"column:updated_at" json:"updated_at"`
	StandardItems         []DetectionRunStandardItem  `gorm:"-" json:"standard_items,omitempty"`
	StorageRoutes         []DetectionRunStorageRoute  `gorm:"-" json:"storage_routes,omitempty"`
	Reports               []DetectionRunReport        `gorm:"-" json:"reports,omitempty"`
	ReportRequests        []DetectionRunReportRequest `gorm:"-" json:"report_requests,omitempty"`
	RecentNotes           []DetectionRunNote          `gorm:"-" json:"recent_notes,omitempty"`
}

func (DetectionTask) TableName() string {
	return "sys_detection_tasks"
}

type ActiveTask struct {
	ID              uint                                 `json:"id"`
	TestNo          string                               `json:"test_no"`
	ProjectID       uint                                 `json:"project_id"`
	ProjectCode     string                               `json:"project_code"`
	Mode            string                               `json:"mode"`
	StandardID      *uint                                `json:"standard_id,omitempty"`
	StandardCode    string                               `json:"standard_code"`
	StandardVersion int                                  `json:"standard_version"`
	StandardItems   map[int64]DetectionRunStandardItem   `json:"-"`
	StorageRoutes   map[int64][]DetectionRunStorageRoute `json:"-"`
}

func (task ActiveTask) AllowsStore(varID int64) bool {
	if len(task.StandardItems) == 0 {
		return true
	}
	item, ok := task.StandardItems[varID]
	return ok && item.StoreEnabled
}

func (task ActiveTask) RoutesForStore(varID int64) []DetectionRunStorageRoute {
	if len(task.StorageRoutes) == 0 {
		return nil
	}
	return task.StorageRoutes[varID]
}

type DetectionStandard struct {
	ID               uint                    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	StandardCode     string                  `gorm:"column:standard_code;size:64;uniqueIndex;not null" json:"standard_code"`
	Name             string                  `gorm:"column:name;size:128;not null" json:"name"`
	DisplayName      string                  `gorm:"column:display_name;size:128" json:"display_name"`
	DisplayNameEN    string                  `gorm:"column:display_name_en;size:128" json:"display_name_en"`
	DisplayNameJA    string                  `gorm:"column:display_name_ja;size:128" json:"display_name_ja"`
	ProjectID        *uint                   `gorm:"column:project_id;index" json:"project_id,omitempty"`
	ProjectCode      string                  `gorm:"column:project_code;size:64;index" json:"project_code"`
	Mode             string                  `gorm:"column:mode;size:64;index" json:"mode"`
	ReportTemplateID *uint                   `gorm:"column:report_template_id;index" json:"report_template_id,omitempty"`
	Version          int                     `gorm:"column:version;default:1;not null" json:"version"`
	Enabled          bool                    `gorm:"column:enabled;default:true;index" json:"enabled"`
	Remark           string                  `gorm:"column:remark;size:255" json:"remark"`
	CreatedAt        time.Time               `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time               `gorm:"column:updated_at" json:"updated_at"`
	Items            []DetectionStandardItem `gorm:"-" json:"items,omitempty"`
}

func (DetectionStandard) TableName() string {
	return "sys_detection_standards"
}

type DetectionStandardFavorite struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID     uint      `gorm:"column:user_id;uniqueIndex:uk_standard_favorite_user_standard;index;not null" json:"user_id"`
	StandardID uint      `gorm:"column:standard_id;uniqueIndex:uk_standard_favorite_user_standard;index;not null" json:"standard_id"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionStandardFavorite) TableName() string {
	return "sys_detection_standard_favorites"
}

type DetectionStandardRecent struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID     uint      `gorm:"column:user_id;uniqueIndex:uk_standard_recent_user_standard_project;index;not null" json:"user_id"`
	StandardID uint      `gorm:"column:standard_id;uniqueIndex:uk_standard_recent_user_standard_project;index;not null" json:"standard_id"`
	ProjectID  uint      `gorm:"column:project_id;uniqueIndex:uk_standard_recent_user_standard_project;index;not null" json:"project_id"`
	LastUsedAt time.Time `gorm:"column:last_used_at;index" json:"last_used_at"`
	UseCount   int       `gorm:"column:use_count;default:0" json:"use_count"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionStandardRecent) TableName() string {
	return "sys_detection_standard_recents"
}

type DetectionStandardItem struct {
	ID              uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	StandardID      uint      `gorm:"column:standard_id;index;not null" json:"standard_id"`
	VarID           int64     `gorm:"column:var_id;index;not null" json:"var_id"`
	VarName         string    `gorm:"column:var_name;size:128;not null" json:"var_name"`
	DisplayName     string    `gorm:"column:display_name;size:128" json:"display_name"`
	DisplayNameEN   string    `gorm:"column:display_name_en;size:128" json:"display_name_en"`
	DisplayNameJA   string    `gorm:"column:display_name_ja;size:128" json:"display_name_ja"`
	CheckEnabled    bool      `gorm:"column:check_enabled;default:true;index" json:"check_enabled"`
	AlarmEnabled    bool      `gorm:"column:alarm_enabled;default:true;index" json:"alarm_enabled"`
	StoreEnabled    bool      `gorm:"column:store_enabled;default:true;index" json:"store_enabled"`
	CheckCycleMS    int       `gorm:"column:check_cycle_ms;default:0" json:"check_cycle_ms"`
	CheckOnStart    bool      `gorm:"column:check_on_start" json:"check_on_start"`
	Required        bool      `gorm:"column:required;default:false" json:"required"`
	CheckMethod     string    `gorm:"column:check_method;size:32;default:numeric_range;not null" json:"check_method"`
	TargetValue     string    `gorm:"column:target_value;size:255" json:"target_value"`
	LimitLL         *float64  `gorm:"column:limit_ll" json:"limit_ll,omitempty"`
	LimitL          *float64  `gorm:"column:limit_l" json:"limit_l,omitempty"`
	LimitH          *float64  `gorm:"column:limit_h" json:"limit_h,omitempty"`
	LimitHH         *float64  `gorm:"column:limit_hh" json:"limit_hh,omitempty"`
	LimitDeadband   float64   `gorm:"column:limit_deadband;default:0" json:"limit_deadband"`
	ViolationHoldMS int       `gorm:"column:violation_hold_ms;default:0" json:"violation_hold_ms"`
	RecoverHoldMS   int       `gorm:"column:recover_hold_ms;default:0" json:"recover_hold_ms"`
	QualityPolicy   string    `gorm:"column:quality_policy;size:32;default:ignore_bad;not null" json:"quality_policy"`
	Unit            string    `gorm:"column:unit;size:32" json:"unit"`
	DecimalPlaces   int       `gorm:"column:decimal_places;default:2" json:"decimal_places"`
	SortOrder       int       `gorm:"column:sort_order;default:0" json:"sort_order"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionStandardItem) TableName() string {
	return "sys_detection_standard_items"
}

func (i DetectionStandardItem) MarshalJSON() ([]byte, error) {
	type alias DetectionStandardItem
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(i),
		VarIDText: strconv.FormatInt(i.VarID, 10),
	})
}

type DetectionRunStandardItem struct {
	ID                             uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID                         uint      `gorm:"column:task_id;index;not null" json:"task_id"`
	TestNo                         string    `gorm:"column:test_no;size:128;index" json:"test_no"`
	StandardID                     uint      `gorm:"column:standard_id;index;not null" json:"standard_id"`
	StandardItemID                 uint      `gorm:"column:standard_item_id;index;not null" json:"standard_item_id"`
	VarID                          int64     `gorm:"column:var_id;index;not null" json:"var_id"`
	VarName                        string    `gorm:"column:var_name;size:128;not null" json:"var_name"`
	DisplayName                    string    `gorm:"column:display_name;size:128" json:"display_name"`
	DisplayNameEN                  string    `gorm:"column:display_name_en;size:128" json:"display_name_en"`
	DisplayNameJA                  string    `gorm:"column:display_name_ja;size:128" json:"display_name_ja"`
	CheckEnabled                   bool      `gorm:"column:check_enabled;default:true;index" json:"check_enabled"`
	AlarmEnabled                   bool      `gorm:"column:alarm_enabled;default:true;index" json:"alarm_enabled"`
	StoreEnabled                   bool      `gorm:"column:store_enabled;default:true;index" json:"store_enabled"`
	CheckCycleMS                   int       `gorm:"column:check_cycle_ms;default:0" json:"check_cycle_ms"`
	CheckOnStart                   bool      `gorm:"column:check_on_start" json:"check_on_start"`
	Required                       bool      `gorm:"column:required;default:false" json:"required"`
	CheckMethod                    string    `gorm:"column:check_method;size:32;default:numeric_range;not null" json:"check_method"`
	TargetValue                    string    `gorm:"column:target_value;size:255" json:"target_value"`
	LimitLL                        *float64  `gorm:"column:limit_ll" json:"limit_ll,omitempty"`
	LimitL                         *float64  `gorm:"column:limit_l" json:"limit_l,omitempty"`
	LimitH                         *float64  `gorm:"column:limit_h" json:"limit_h,omitempty"`
	LimitHH                        *float64  `gorm:"column:limit_hh" json:"limit_hh,omitempty"`
	LimitDeadband                  float64   `gorm:"column:limit_deadband;default:0" json:"limit_deadband"`
	ViolationHoldMS                int       `gorm:"column:violation_hold_ms;default:0" json:"violation_hold_ms"`
	RecoverHoldMS                  int       `gorm:"column:recover_hold_ms;default:0" json:"recover_hold_ms"`
	QualityPolicy                  string    `gorm:"column:quality_policy;size:32;default:ignore_bad;not null" json:"quality_policy"`
	VariableDefaultAlarmEnabled    bool      `gorm:"column:variable_default_alarm_enabled;default:false" json:"variable_default_alarm_enabled"`
	VariableDefaultLimitLL         *float64  `gorm:"column:variable_default_limit_ll" json:"variable_default_limit_ll,omitempty"`
	VariableDefaultLimitL          *float64  `gorm:"column:variable_default_limit_l" json:"variable_default_limit_l,omitempty"`
	VariableDefaultLimitH          *float64  `gorm:"column:variable_default_limit_h" json:"variable_default_limit_h,omitempty"`
	VariableDefaultLimitHH         *float64  `gorm:"column:variable_default_limit_hh" json:"variable_default_limit_hh,omitempty"`
	VariableDefaultLimitDeadband   float64   `gorm:"column:variable_default_limit_deadband;default:0" json:"variable_default_limit_deadband"`
	VariableDefaultViolationHoldMS int       `gorm:"column:variable_default_violation_hold_ms;default:0" json:"variable_default_violation_hold_ms"`
	VariableDefaultRecoverHoldMS   int       `gorm:"column:variable_default_recover_hold_ms;default:0" json:"variable_default_recover_hold_ms"`
	Unit                           string    `gorm:"column:unit;size:32" json:"unit"`
	DecimalPlaces                  int       `gorm:"column:decimal_places;default:2" json:"decimal_places"`
	SortOrder                      int       `gorm:"column:sort_order;default:0" json:"sort_order"`
	CreatedAt                      time.Time `gorm:"column:created_at" json:"created_at"`
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

func (DetectionRunStandardItem) TableName() string {
	return "detection_run_standard_items"
}

type DetectionRunNote struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID    uint      `gorm:"column:task_id;index;not null" json:"task_id"`
	NoteType  string    `gorm:"column:note_type;size:32;not null;default:memo" json:"note_type"`
	Content   string    `gorm:"column:content;type:text;not null" json:"content"`
	ActorType string    `gorm:"column:actor_type;size:32" json:"actor_type"`
	ActorID   string    `gorm:"column:actor_id;size:128" json:"actor_id"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (DetectionRunNote) TableName() string {
	return "detection_run_notes"
}

type DetectionRunEvent struct {
	ID          uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID      uint      `gorm:"column:task_id;index;not null" json:"task_id"`
	TestNo      string    `gorm:"column:test_no;size:128;index" json:"test_no"`
	ProjectID   uint      `gorm:"column:project_id;index;not null" json:"project_id"`
	ProjectCode string    `gorm:"column:project_code;size:64;index" json:"project_code"`
	EventType   string    `gorm:"column:event_type;size:64;index;not null" json:"event_type"`
	EventLevel  string    `gorm:"column:event_level;size:32;index" json:"event_level"`
	Message     string    `gorm:"column:message;size:512" json:"message"`
	Detail      string    `gorm:"column:detail;type:text" json:"detail"`
	OccurredAt  time.Time `gorm:"column:occurred_at;index;not null" json:"occurred_at"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
}

func (DetectionRunEvent) TableName() string {
	return "detection_run_events"
}

type DetectionRunSummary struct {
	ID              uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID          uint       `gorm:"column:task_id;uniqueIndex;not null" json:"task_id"`
	TestNo          string     `gorm:"column:test_no;size:128;index" json:"test_no"`
	ProjectID       uint       `gorm:"column:project_id;index;not null" json:"project_id"`
	ProjectCode     string     `gorm:"column:project_code;size:64;index" json:"project_code"`
	ResultStatus    string     `gorm:"column:result_status;size:32;index;not null" json:"result_status"`
	StartedAt       *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	EndedAt         *time.Time `gorm:"column:ended_at" json:"ended_at,omitempty"`
	DurationMS      int64      `gorm:"column:duration_ms;default:0" json:"duration_ms"`
	HistoryRows     int64      `gorm:"column:history_rows;default:0" json:"history_rows"`
	AlarmTotal      int64      `gorm:"column:alarm_total;default:0" json:"alarm_total"`
	AlarmActive     int64      `gorm:"column:alarm_active;default:0" json:"alarm_active"`
	AlarmRecovered  int64      `gorm:"column:alarm_recovered;default:0" json:"alarm_recovered"`
	AlarmAboveH     int64      `gorm:"column:alarm_above_h;default:0" json:"alarm_above_h"`
	AlarmAboveHH    int64      `gorm:"column:alarm_above_hh;default:0" json:"alarm_above_hh"`
	AlarmBelowL     int64      `gorm:"column:alarm_below_l;default:0" json:"alarm_below_l"`
	AlarmBelowLL    int64      `gorm:"column:alarm_below_ll;default:0" json:"alarm_below_ll"`
	FirstAlarmAt    *time.Time `gorm:"column:first_alarm_at" json:"first_alarm_at,omitempty"`
	LastAlarmAt     *time.Time `gorm:"column:last_alarm_at" json:"last_alarm_at,omitempty"`
	LastRefreshedAt time.Time  `gorm:"column:last_refreshed_at;index" json:"last_refreshed_at"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionRunSummary) TableName() string {
	return "detection_run_summaries"
}

type DetectionRunFeature struct {
	ID              uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID          uint      `gorm:"column:task_id;uniqueIndex:uk_detection_feature_task_var;not null" json:"task_id"`
	TestNo          string    `gorm:"column:test_no;size:128;index" json:"test_no"`
	ProjectID       uint      `gorm:"column:project_id;index;not null" json:"project_id"`
	ProjectCode     string    `gorm:"column:project_code;size:64;index" json:"project_code"`
	VarID           int64     `gorm:"column:var_id;uniqueIndex:uk_detection_feature_task_var;not null" json:"var_id"`
	VarName         string    `gorm:"column:var_name;size:128;index" json:"var_name"`
	SampleCount     int64     `gorm:"column:sample_count;default:0" json:"sample_count"`
	AvgValue        *float64  `gorm:"column:avg_value" json:"avg_value,omitempty"`
	MinValue        *float64  `gorm:"column:min_value" json:"min_value,omitempty"`
	MaxValue        *float64  `gorm:"column:max_value" json:"max_value,omitempty"`
	FirstSampleTime time.Time `gorm:"column:first_sample_time" json:"first_sample_time"`
	LastSampleTime  time.Time `gorm:"column:last_sample_time" json:"last_sample_time"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionRunFeature) TableName() string {
	return "detection_run_features"
}

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

type DetectionLimitAlarm struct {
	ID             uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Scope          string     `gorm:"column:scope;size:32;default:detection;index;not null" json:"scope"`
	TaskID         uint       `gorm:"column:task_id;index;not null" json:"task_id"`
	TestNo         string     `gorm:"column:test_no;size:128;index" json:"test_no"`
	ProjectID      uint       `gorm:"column:project_id;index;not null" json:"project_id"`
	ProjectCode    string     `gorm:"column:project_code;size:64;index" json:"project_code"`
	StandardID     *uint      `gorm:"column:standard_id;index" json:"standard_id,omitempty"`
	StandardItemID uint       `gorm:"column:standard_item_id;index" json:"standard_item_id"`
	RunStandardID  uint       `gorm:"column:run_standard_item_id;index" json:"run_standard_item_id"`
	VarID          int64      `gorm:"column:var_id;index;not null" json:"var_id"`
	VarName        string     `gorm:"column:var_name;size:128;index;not null" json:"var_name"`
	DisplayName    string     `gorm:"column:display_name;size:128" json:"display_name"`
	DisplayNameEN  string     `gorm:"column:display_name_en;size:128" json:"display_name_en"`
	DisplayNameJA  string     `gorm:"column:display_name_ja;size:128" json:"display_name_ja"`
	CheckMethod    string     `gorm:"column:check_method;size:32;not null" json:"check_method"`
	AlarmType      string     `gorm:"column:alarm_type;size:32;index;not null" json:"alarm_type"`
	AlarmLevel     string     `gorm:"column:alarm_level;size:32;index;not null" json:"alarm_level"`
	Status         string     `gorm:"column:status;size:32;index;not null" json:"status"`
	StartValue     *float64   `gorm:"column:start_value" json:"start_value,omitempty"`
	PeakValue      *float64   `gorm:"column:peak_value" json:"peak_value,omitempty"`
	RecoverValue   *float64   `gorm:"column:recover_value" json:"recover_value,omitempty"`
	LimitValue     *float64   `gorm:"column:limit_value" json:"limit_value,omitempty"`
	LimitDeadband  float64    `gorm:"column:limit_deadband;default:0" json:"limit_deadband"`
	Quality        int        `gorm:"column:quality;default:1" json:"quality"`
	FirstSeenAt    time.Time  `gorm:"column:first_seen_at;index;not null" json:"first_seen_at"`
	LastSeenAt     time.Time  `gorm:"column:last_seen_at;index;not null" json:"last_seen_at"`
	RecoveredAt    *time.Time `gorm:"column:recovered_at;index" json:"recovered_at,omitempty"`
	DurationMS     int64      `gorm:"column:duration_ms;default:0" json:"duration_ms"`
	Message        string     `gorm:"column:message;size:512" json:"message"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionLimitAlarm) TableName() string {
	return "detection_limit_alarms"
}

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

type DetectionLimitAlarmEvent struct {
	Action            string
	PreviousAlarmType string
	Alarm             DetectionLimitAlarm
}
