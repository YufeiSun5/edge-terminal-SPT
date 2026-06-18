package query

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	StationViewStatusPublished      = "published"
	StationViewTargetGlobal         = "global"
	StationViewTargetEdge           = "edge"
	StationViewTargetModel          = "model"
	StationViewTargetProject        = "project"
	StationViewBindingVarName       = "var_name"
	StationViewBindingVarGroup      = "var_group"
	StationViewBindingRunItems      = "detection_items"
	StationViewBindingAlarmSummary  = "alarm_summary"
	StationViewBindingRunState      = "run_state"
	StationViewLayoutAreaCardPool   = "card_pool"
	StationViewLayoutAreaListLayout = "list_layout"
	DetectionStatusRunning          = "running"
	DetectionStatusPaused           = "paused"
	DetectionStatusStopped          = "stopped"
)

var (
	ErrStationViewSyncNotReady     = errors.New("station view synced tables are not ready")
	ErrStationViewTemplateConflict = errors.New("station view template assignment conflict")
)

type StationViewQuery struct {
	db *gorm.DB
}

func NewStationViewQuery(db *gorm.DB) *StationViewQuery {
	return &StationViewQuery{db: db}
}

type Project struct {
	ID             uint      `gorm:"column:id;primaryKey" json:"id"`
	ProjectCode    string    `gorm:"column:project_code" json:"project_code"`
	SiteNo         string    `gorm:"column:site_no" json:"site_no"`
	EdgeInstanceID string    `gorm:"column:edge_instance_id" json:"edge_instance_id"`
	Name           string    `gorm:"column:name" json:"name"`
	DisplayName    string    `gorm:"column:display_name" json:"display_name"`
	DisplayNameEN  string    `gorm:"column:display_name_en" json:"display_name_en"`
	DisplayNameJA  string    `gorm:"column:display_name_ja" json:"display_name_ja"`
	ModelName      string    `gorm:"column:model_name" json:"model_name"`
	ProjectGroup   string    `gorm:"column:project_group" json:"project_group"`
	ImageRef       string    `gorm:"column:image_ref" json:"image_ref"`
	Enabled        bool      `gorm:"column:enabled" json:"enabled"`
	Blocked        bool      `gorm:"column:blocked" json:"blocked"`
	Placeholder    bool      `gorm:"column:placeholder" json:"placeholder"`
	CurrentTaskID  *uint     `gorm:"column:current_task_id" json:"current_task_id,omitempty"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Project) TableName() string { return "sys_projects" }

type TagConfig struct {
	VarID                  int64     `gorm:"column:var_id;primaryKey" json:"var_id"`
	GatewayID              int       `gorm:"column:gateway_id" json:"gateway_id"`
	SourceTopic            string    `gorm:"column:source_topic" json:"source_topic"`
	SourcePath             string    `gorm:"column:source_path" json:"source_path"`
	SourceType             string    `gorm:"column:source_type" json:"source_type"`
	RawName                string    `gorm:"column:raw_name" json:"raw_name"`
	ProjectID              *uint     `gorm:"column:project_id" json:"project_id,omitempty"`
	ProjectCode            string    `gorm:"column:project_code" json:"project_code"`
	VarGroup               string    `gorm:"column:var_group" json:"var_group"`
	VarName                string    `gorm:"column:var_name" json:"var_name"`
	DisplayName            string    `gorm:"column:display_name" json:"display_name"`
	DisplayNameEN          string    `gorm:"column:display_name_en" json:"display_name_en"`
	DisplayNameJA          string    `gorm:"column:display_name_ja" json:"display_name_ja"`
	JSONPath               string    `gorm:"column:json_path" json:"json_path"`
	DataType               string    `gorm:"column:data_type" json:"data_type"`
	Unit                   string    `gorm:"column:unit" json:"unit"`
	DecimalPlaces          int       `gorm:"column:decimal_places" json:"decimal_places"`
	ScaleFactor            float64   `gorm:"column:scale_factor" json:"scale_factor"`
	OffsetVal              float64   `gorm:"column:offset_val" json:"offset_val"`
	RWMode                 string    `gorm:"column:rw_mode" json:"rw_mode"`
	Writable               bool      `gorm:"column:writable" json:"writable"`
	WriteSourceID          int       `gorm:"column:write_source_id" json:"write_source_id"`
	WritePath              string    `gorm:"column:write_path" json:"write_path"`
	WriteDataType          string    `gorm:"column:write_data_type" json:"write_data_type"`
	WriteMin               *float64  `gorm:"column:write_min" json:"write_min,omitempty"`
	WriteMax               *float64  `gorm:"column:write_max" json:"write_max,omitempty"`
	WriteEnum              string    `gorm:"column:write_enum" json:"write_enum"`
	WriteRequiresAudit     bool      `gorm:"column:write_requires_audit" json:"write_requires_audit"`
	SuspiciousValue        *float64  `gorm:"column:suspicious_value" json:"suspicious_value,omitempty"`
	DebounceThreshold      *float64  `gorm:"column:debounce_threshold" json:"debounce_threshold,omitempty"`
	DebounceMS             int       `gorm:"column:debounce_ms" json:"debounce_ms"`
	Deadband               float64   `gorm:"column:deadband" json:"deadband"`
	DefaultAlarmEnabled    bool      `gorm:"column:default_alarm_enabled" json:"default_alarm_enabled"`
	DefaultLimitLL         *float64  `gorm:"column:default_limit_ll" json:"default_limit_ll,omitempty"`
	DefaultLimitL          *float64  `gorm:"column:default_limit_l" json:"default_limit_l,omitempty"`
	DefaultLimitH          *float64  `gorm:"column:default_limit_h" json:"default_limit_h,omitempty"`
	DefaultLimitHH         *float64  `gorm:"column:default_limit_hh" json:"default_limit_hh,omitempty"`
	DefaultLimitDeadband   float64   `gorm:"column:default_limit_deadband" json:"default_limit_deadband"`
	DefaultViolationHoldMS int       `gorm:"column:default_violation_hold_ms" json:"default_violation_hold_ms"`
	DefaultRecoverHoldMS   int       `gorm:"column:default_recover_hold_ms" json:"default_recover_hold_ms"`
	Discovered             bool      `gorm:"column:discovered" json:"discovered"`
	Placeholder            bool      `gorm:"column:placeholder" json:"placeholder"`
	Enabled                bool      `gorm:"column:enabled" json:"enabled"`
	CreatedAt              time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt              time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (TagConfig) TableName() string { return "sys_tags" }

func (t TagConfig) MarshalJSON() ([]byte, error) {
	type alias TagConfig
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(t),
		VarIDText: strconv.FormatInt(t.VarID, 10),
	})
}

type StationViewTemplate struct {
	ID             uint      `gorm:"column:id;primaryKey"`
	TemplateUID    string    `gorm:"column:template_uid"`
	TemplateCode   string    `gorm:"column:template_code"`
	Name           string    `gorm:"column:name"`
	DisplayName    string    `gorm:"column:display_name"`
	DisplayNameEN  string    `gorm:"column:display_name_en"`
	DisplayNameJA  string    `gorm:"column:display_name_ja"`
	Version        int       `gorm:"column:version"`
	Status         string    `gorm:"column:status"`
	OwnerScope     string    `gorm:"column:owner_scope"`
	SyncScope      string    `gorm:"column:sync_scope" json:"sync_scope"`
	EdgeInstanceID string    `gorm:"column:edge_instance_id" json:"edge_instance_id"`
	UpdatedByNode  string    `gorm:"column:updated_by_node" json:"updated_by_node"`
	UpdatedByUser  string    `gorm:"column:updated_by_user" json:"updated_by_user"`
	LayoutJSON     string    `gorm:"column:layout_json"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (StationViewTemplate) TableName() string { return "sys_station_view_templates" }

type StationViewRegion struct {
	ID          uint   `gorm:"column:id;primaryKey"`
	TemplateUID string `gorm:"column:template_uid"`
	RegionKey   string `gorm:"column:region_key"`
	LayoutArea  string `gorm:"column:layout_area"`
	RegionType  string `gorm:"column:region_type"`
	LayoutJSON  string `gorm:"column:layout_json"`
	SortOrder   int    `gorm:"column:sort_order"`
	Enabled     bool   `gorm:"column:enabled"`
}

func (StationViewRegion) TableName() string { return "sys_station_view_regions" }

type StationViewItem struct {
	ID             uint   `gorm:"column:id;primaryKey"`
	TemplateUID    string `gorm:"column:template_uid"`
	RegionKey      string `gorm:"column:region_key"`
	LayoutArea     string `gorm:"column:layout_area"`
	ItemUID        string `gorm:"column:item_uid"`
	ItemType       string `gorm:"column:item_type"`
	BindingType    string `gorm:"column:binding_type"`
	BindingKey     string `gorm:"column:binding_key"`
	BindingJSON    string `gorm:"column:binding_json"`
	DisplayJSON    string `gorm:"column:display_json"`
	SortOrder      int    `gorm:"column:sort_order"`
	Pinned         bool   `gorm:"column:pinned"`
	Visible        bool   `gorm:"column:visible"`
	SyncScope      string `gorm:"column:sync_scope"`
	EdgeInstanceID string `gorm:"column:edge_instance_id"`
	UpdatedByNode  string `gorm:"column:updated_by_node"`
	UpdatedByUser  string `gorm:"column:updated_by_user"`
}

func (StationViewItem) TableName() string { return "sys_station_view_items" }

type StationViewAssignment struct {
	ID          uint      `gorm:"column:id;primaryKey"`
	TemplateUID string    `gorm:"column:template_uid"`
	TargetType  string    `gorm:"column:target_type"`
	TargetKey   string    `gorm:"column:target_key"`
	Priority    int       `gorm:"column:priority"`
	Enabled     bool      `gorm:"column:enabled"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (StationViewAssignment) TableName() string { return "sys_station_view_assignments" }

type StationViewTemplateFilter struct {
	Status     string
	OwnerScope string
	Keyword    string
}

type StationViewTemplateListItem struct {
	ID            uint                       `json:"id"`
	TemplateUID   string                     `json:"template_uid"`
	TemplateCode  string                     `json:"template_code"`
	Name          string                     `json:"name"`
	DisplayName   string                     `json:"display_name"`
	DisplayNameEN string                     `json:"display_name_en"`
	DisplayNameJA string                     `json:"display_name_ja"`
	Version       int                        `json:"version"`
	Status        string                     `json:"status"`
	OwnerScope    string                     `json:"owner_scope"`
	LayoutJSON    string                     `json:"layout_json,omitempty"`
	Assignments   []StationViewAssignmentDTO `json:"assignments,omitempty"`
	CreatedAt     time.Time                  `json:"created_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`
}

type StationViewAssignmentDTO struct {
	ID          uint      `json:"id"`
	TemplateUID string    `json:"template_uid"`
	TargetType  string    `json:"target_type"`
	TargetKey   string    `json:"target_key"`
	Priority    int       `json:"priority"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DetectionTask struct {
	ID                    uint                        `gorm:"column:id;primaryKey" json:"id"`
	TestNo                string                      `gorm:"column:test_no" json:"test_no"`
	FactoryNo             string                      `gorm:"column:factory_no" json:"factory_no"`
	CustomerName          string                      `gorm:"column:customer_name" json:"customer_name"`
	DeviceModel           string                      `gorm:"column:device_model" json:"device_model"`
	ProjectID             uint                        `gorm:"column:project_id" json:"project_id"`
	ProjectCode           string                      `gorm:"column:project_code" json:"project_code"`
	Mode                  string                      `gorm:"column:mode" json:"mode"`
	Status                string                      `gorm:"column:status" json:"status"`
	StandardID            *uint                       `gorm:"column:standard_id" json:"standard_id,omitempty"`
	StandardCode          string                      `gorm:"column:standard_code" json:"standard_code"`
	StandardVer           int                         `gorm:"column:standard_version" json:"standard_version"`
	ConfigEnabled         bool                        `gorm:"column:config_enabled" json:"config_enabled"`
	ConfigStatus          string                      `gorm:"column:config_status" json:"config_status"`
	ConfigCode            string                      `gorm:"column:config_code" json:"config_code"`
	ConfigName            string                      `gorm:"column:config_name" json:"config_name"`
	ConfigVersion         int                         `gorm:"column:config_version" json:"config_version"`
	ConfigHash            string                      `gorm:"column:config_hash" json:"config_hash"`
	CurrentConfigRevision int                         `gorm:"column:current_config_revision" json:"current_config_revision"`
	StartedAt             *time.Time                  `gorm:"column:started_at" json:"started_at,omitempty"`
	EndedAt               *time.Time                  `gorm:"column:ended_at" json:"ended_at,omitempty"`
	LimitCheckEnabled     bool                        `gorm:"column:limit_check_enabled" json:"limit_check_enabled"`
	EndPolicy             string                      `gorm:"column:end_policy" json:"end_policy"`
	DurationSec           int                         `gorm:"column:duration_sec" json:"duration_sec"`
	QualifiedHoldMS       int                         `gorm:"column:qualified_hold_ms" json:"qualified_hold_ms"`
	ExpectedEndAt         *time.Time                  `gorm:"column:expected_end_at" json:"expected_end_at,omitempty"`
	PauseStartedAt        *time.Time                  `gorm:"column:pause_started_at" json:"pause_started_at,omitempty"`
	PausedDurationMS      int64                       `gorm:"column:paused_duration_ms" json:"paused_duration_ms"`
	EndType               string                      `gorm:"column:end_type" json:"end_type"`
	StopReason            string                      `gorm:"column:stop_reason" json:"stop_reason"`
	OperatorNote          string                      `gorm:"column:operator_note" json:"operator_note"`
	CustomConfigJSON      string                      `gorm:"column:custom_config_json" json:"custom_config_json,omitempty"`
	TemplateRef           string                      `gorm:"column:template_ref" json:"template_ref"`
	ReportTemplateID      *uint                       `gorm:"column:report_template_id" json:"report_template_id,omitempty"`
	ReportTemplateCode    string                      `gorm:"column:report_template_code" json:"report_template_code"`
	ReportTemplateVersion int                         `gorm:"column:report_template_version" json:"report_template_version"`
	CreatedAt             time.Time                   `gorm:"column:created_at" json:"created_at"`
	UpdatedAt             time.Time                   `gorm:"column:updated_at" json:"updated_at"`
	StandardItems         []DetectionRunStandardItem  `gorm:"-" json:"standard_items,omitempty"`
	StorageRoutes         []DetectionRunStorageRoute  `gorm:"-" json:"storage_routes,omitempty"`
	Reports               []DetectionRunReport        `gorm:"-" json:"reports,omitempty"`
	ReportRequests        []DetectionRunReportRequest `gorm:"-" json:"report_requests,omitempty"`
	RecentNotes           []DetectionRunNote          `gorm:"-" json:"recent_notes,omitempty"`
}

func (DetectionTask) TableName() string { return "sys_detection_tasks" }

type DetectionRunStandardItem struct {
	ID                             uint       `gorm:"column:id;primaryKey" json:"id"`
	TaskID                         uint       `gorm:"column:task_id" json:"task_id"`
	TestNo                         string     `gorm:"column:test_no" json:"test_no"`
	StandardID                     uint       `gorm:"column:standard_id" json:"standard_id"`
	StandardItemID                 uint       `gorm:"column:standard_item_id" json:"standard_item_id"`
	ConfigRevision                 int        `gorm:"column:config_revision" json:"config_revision"`
	VarID                          int64      `gorm:"column:var_id" json:"var_id"`
	VarName                        string     `gorm:"column:var_name" json:"var_name"`
	DisplayName                    string     `gorm:"column:display_name" json:"display_name"`
	DisplayNameEN                  string     `gorm:"column:display_name_en" json:"display_name_en"`
	DisplayNameJA                  string     `gorm:"column:display_name_ja" json:"display_name_ja"`
	CheckEnabled                   bool       `gorm:"column:check_enabled" json:"check_enabled"`
	AlarmEnabled                   bool       `gorm:"column:alarm_enabled" json:"alarm_enabled"`
	StoreEnabled                   bool       `gorm:"column:store_enabled" json:"store_enabled"`
	CheckCycleMS                   int        `gorm:"column:check_cycle_ms" json:"check_cycle_ms"`
	CheckOnStart                   bool       `gorm:"column:check_on_start" json:"check_on_start"`
	Required                       bool       `gorm:"column:required" json:"required"`
	CheckMethod                    string     `gorm:"column:check_method" json:"check_method"`
	TargetValue                    string     `gorm:"column:target_value" json:"target_value"`
	LimitLL                        *float64   `gorm:"column:limit_ll" json:"limit_ll,omitempty"`
	LimitL                         *float64   `gorm:"column:limit_l" json:"limit_l,omitempty"`
	LimitH                         *float64   `gorm:"column:limit_h" json:"limit_h,omitempty"`
	LimitHH                        *float64   `gorm:"column:limit_hh" json:"limit_hh,omitempty"`
	LimitDeadband                  float64    `gorm:"column:limit_deadband" json:"limit_deadband"`
	ViolationHoldMS                int        `gorm:"column:violation_hold_ms" json:"violation_hold_ms"`
	RecoverHoldMS                  int        `gorm:"column:recover_hold_ms" json:"recover_hold_ms"`
	QualityPolicy                  string     `gorm:"column:quality_policy" json:"quality_policy"`
	VariableDefaultAlarmEnabled    bool       `gorm:"column:variable_default_alarm_enabled" json:"variable_default_alarm_enabled"`
	VariableDefaultLimitLL         *float64   `gorm:"column:variable_default_limit_ll" json:"variable_default_limit_ll,omitempty"`
	VariableDefaultLimitL          *float64   `gorm:"column:variable_default_limit_l" json:"variable_default_limit_l,omitempty"`
	VariableDefaultLimitH          *float64   `gorm:"column:variable_default_limit_h" json:"variable_default_limit_h,omitempty"`
	VariableDefaultLimitHH         *float64   `gorm:"column:variable_default_limit_hh" json:"variable_default_limit_hh,omitempty"`
	VariableDefaultLimitDeadband   float64    `gorm:"column:variable_default_limit_deadband" json:"variable_default_limit_deadband"`
	VariableDefaultViolationHoldMS int        `gorm:"column:variable_default_violation_hold_ms" json:"variable_default_violation_hold_ms"`
	VariableDefaultRecoverHoldMS   int        `gorm:"column:variable_default_recover_hold_ms" json:"variable_default_recover_hold_ms"`
	Unit                           string     `gorm:"column:unit" json:"unit"`
	DecimalPlaces                  int        `gorm:"column:decimal_places" json:"decimal_places"`
	SortOrder                      int        `gorm:"column:sort_order" json:"sort_order"`
	EffectiveFrom                  *time.Time `gorm:"column:effective_from" json:"effective_from,omitempty"`
	EffectiveTo                    *time.Time `gorm:"column:effective_to" json:"effective_to,omitempty"`
	CreatedAt                      time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (DetectionRunStandardItem) TableName() string { return "detection_run_standard_items" }

type StationViewEffectiveResponse struct {
	EdgeInstanceID string                    `json:"edge_instance_id"`
	Project        StationViewProjectRef     `json:"project"`
	Template       StationViewTemplateRef    `json:"template"`
	Regions        []StationViewRegionDTO    `json:"regions"`
	Items          []StationViewItemDTO      `json:"items"`
	WSSubscription StationViewWSSubscription `json:"ws_subscription"`
	HTTPCompanion  StationViewHTTPCompanion  `json:"http_companion"`
	Warnings       []string                  `json:"warnings,omitempty"`
}

type StationViewProjectRef struct {
	ID            uint   `json:"id"`
	ProjectCode   string `json:"project_code"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	DisplayNameEN string `json:"display_name_en"`
	DisplayNameJA string `json:"display_name_ja"`
	ModelName     string `json:"model_name"`
}

type StationViewTemplateRef struct {
	TemplateUID   string `json:"template_uid"`
	TemplateCode  string `json:"template_code"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	DisplayNameEN string `json:"display_name_en"`
	DisplayNameJA string `json:"display_name_ja"`
	Version       int    `json:"version"`
	Status        string `json:"status"`
	OwnerScope    string `json:"owner_scope"`
	LayoutJSON    string `json:"layout_json,omitempty"`
}

type StationViewRegionDTO struct {
	LayoutArea string `json:"layout_area"`
	LayoutType string `json:"layout_type"`
	LayoutJSON string `json:"layout_json,omitempty"`
	SortOrder  int    `json:"sort_order"`
}

type StationViewItemDTO struct {
	ItemUID          string                       `json:"item_uid"`
	LayoutArea       string                       `json:"layout_area"`
	ItemType         string                       `json:"item_type"`
	BindingType      string                       `json:"binding_type"`
	BindingKey       string                       `json:"binding_key"`
	BindingJSON      string                       `json:"binding_json,omitempty"`
	DisplayJSON      string                       `json:"display_json,omitempty"`
	SortOrder        int                          `json:"sort_order"`
	Pinned           bool                         `json:"pinned"`
	Visible          bool                         `json:"visible"`
	ResolvedBindings []StationViewResolvedBinding `json:"resolved_bindings,omitempty"`
}

type StationViewResolvedBinding struct {
	Source        string   `json:"source"`
	VarID         int64    `json:"var_id,omitempty"`
	VarIDText     string   `json:"var_id_text,omitempty"`
	VarName       string   `json:"var_name,omitempty"`
	VarGroup      string   `json:"var_group,omitempty"`
	DisplayName   string   `json:"display_name,omitempty"`
	DisplayNameEN string   `json:"display_name_en,omitempty"`
	DisplayNameJA string   `json:"display_name_ja,omitempty"`
	DataType      string   `json:"data_type,omitempty"`
	Unit          string   `json:"unit,omitempty"`
	DecimalPlaces int      `json:"decimal_places"`
	LimitLL       *float64 `json:"limit_ll,omitempty"`
	LimitL        *float64 `json:"limit_l,omitempty"`
	LimitH        *float64 `json:"limit_h,omitempty"`
	LimitHH       *float64 `json:"limit_hh,omitempty"`
	CheckEnabled  bool     `json:"check_enabled,omitempty"`
	AlarmEnabled  bool     `json:"alarm_enabled,omitempty"`
	SortOrder     int      `json:"sort_order"`
}

type StationViewWSSubscription struct {
	Topics    []string `json:"topics"`
	ProjectID uint     `json:"project_id"`
	VarIDs    []string `json:"var_ids"`
}

type StationViewHTTPCompanion struct {
	CurrentRunRequired bool `json:"current_run_required"`
	AlarmSummary       bool `json:"alarm_summary"`
}

type stationViewAssignmentCandidate struct {
	Assignment StationViewAssignment
	Template   StationViewTemplate
	Score      int
}

func (q *StationViewQuery) Effective(projectID uint, requestedEdgeInstanceID string) (StationViewEffectiveResponse, error) {
	if projectID == 0 {
		return StationViewEffectiveResponse{}, gorm.ErrRecordNotFound
	}
	var project Project
	if err := q.db.First(&project, "id = ?", projectID).Error; err != nil {
		return StationViewEffectiveResponse{}, err
	}
	edgeInstanceID := strings.TrimSpace(requestedEdgeInstanceID)
	if strings.TrimSpace(project.EdgeInstanceID) != "" {
		if edgeInstanceID != "" && edgeInstanceID != strings.TrimSpace(project.EdgeInstanceID) {
			return StationViewEffectiveResponse{}, gorm.ErrRecordNotFound
		}
		edgeInstanceID = strings.TrimSpace(project.EdgeInstanceID)
	}

	template, err := q.resolveTemplate(project, edgeInstanceID)
	if err != nil {
		return StationViewEffectiveResponse{}, err
	}
	regions, err := q.loadRegions(template.TemplateUID)
	if err != nil {
		return StationViewEffectiveResponse{}, err
	}
	items, err := q.loadItems(template.TemplateUID)
	if err != nil {
		return StationViewEffectiveResponse{}, err
	}
	tags, err := q.loadProjectTags(projectID)
	if err != nil {
		return StationViewEffectiveResponse{}, err
	}
	currentRun, hasCurrentRun, err := q.loadCurrentRun(projectID)
	if err != nil {
		return StationViewEffectiveResponse{}, err
	}

	warnings := []string{}
	seenVarIDs := map[int64]bool{}
	responseItems := make([]StationViewItemDTO, 0, len(items))
	httpCompanion := StationViewHTTPCompanion{}
	for _, item := range items {
		dto := StationViewItemDTO{
			ItemUID:     item.ItemUID,
			LayoutArea:  itemLayoutArea(item),
			ItemType:    item.ItemType,
			BindingType: item.BindingType,
			BindingKey:  item.BindingKey,
			BindingJSON: item.BindingJSON,
			DisplayJSON: item.DisplayJSON,
			SortOrder:   item.SortOrder,
			Pinned:      item.Pinned,
			Visible:     item.Visible,
		}
		bindings, itemWarnings := resolveItemBindings(item, tags, currentRun, hasCurrentRun)
		dto.ResolvedBindings = bindings
		warnings = append(warnings, itemWarnings...)
		for _, binding := range bindings {
			if binding.VarID > 0 {
				seenVarIDs[binding.VarID] = true
			}
		}
		if item.BindingType == StationViewBindingRunItems || item.BindingType == StationViewBindingRunState {
			httpCompanion.CurrentRunRequired = true
		}
		if item.BindingType == StationViewBindingAlarmSummary {
			httpCompanion.AlarmSummary = true
		}
		responseItems = append(responseItems, dto)
	}

	varIDs := make([]string, 0, len(seenVarIDs))
	for varID := range seenVarIDs {
		varIDs = append(varIDs, strconv.FormatInt(varID, 10))
	}
	sort.Slice(varIDs, func(i, j int) bool {
		left, _ := strconv.ParseInt(varIDs[i], 10, 64)
		right, _ := strconv.ParseInt(varIDs[j], 10, 64)
		return left < right
	})

	return StationViewEffectiveResponse{
		EdgeInstanceID: edgeInstanceID,
		Project: StationViewProjectRef{
			ID:            project.ID,
			ProjectCode:   project.ProjectCode,
			Name:          project.Name,
			DisplayName:   project.DisplayName,
			DisplayNameEN: project.DisplayNameEN,
			DisplayNameJA: project.DisplayNameJA,
			ModelName:     project.ModelName,
		},
		Template: StationViewTemplateRef{
			TemplateUID:   template.TemplateUID,
			TemplateCode:  template.TemplateCode,
			Name:          template.Name,
			DisplayName:   template.DisplayName,
			DisplayNameEN: template.DisplayNameEN,
			DisplayNameJA: template.DisplayNameJA,
			Version:       template.Version,
			Status:        template.Status,
			OwnerScope:    template.OwnerScope,
			LayoutJSON:    template.LayoutJSON,
		},
		Regions:        regionDTOs(regions),
		Items:          responseItems,
		WSSubscription: StationViewWSSubscription{Topics: []string{"realtime.variables"}, ProjectID: projectID, VarIDs: varIDs},
		HTTPCompanion:  httpCompanion,
		Warnings:       warnings,
	}, nil
}

func (q *StationViewQuery) ListStationViewTemplates(filter StationViewTemplateFilter) ([]StationViewTemplateListItem, error) {
	db := q.db.Model(&StationViewTemplate{})
	if status := strings.TrimSpace(filter.Status); status != "" {
		db = db.Where("status = ?", status)
	}
	if ownerScope := strings.TrimSpace(filter.OwnerScope); ownerScope != "" {
		db = db.Where("owner_scope = ?", ownerScope)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("template_code LIKE ? OR name LIKE ? OR display_name LIKE ?", like, like, like)
	}
	var templates []StationViewTemplate
	if err := db.Order("updated_at DESC, id DESC").Find(&templates).Error; err != nil {
		if isMissingTableError(err) {
			return nil, ErrStationViewSyncNotReady
		}
		return nil, err
	}
	var assignments []StationViewAssignment
	if err := q.db.Order("target_type ASC, target_key ASC, priority DESC, id ASC").Find(&assignments).Error; err != nil {
		if isMissingTableError(err) {
			return nil, ErrStationViewSyncNotReady
		}
		return nil, err
	}
	assignmentsByTemplate := make(map[string][]StationViewAssignmentDTO)
	for _, assignment := range assignments {
		assignmentsByTemplate[assignment.TemplateUID] = append(assignmentsByTemplate[assignment.TemplateUID], stationViewAssignmentDTO(assignment))
	}
	items := make([]StationViewTemplateListItem, 0, len(templates))
	for _, template := range templates {
		items = append(items, stationViewTemplateListItem(template, assignmentsByTemplate[template.TemplateUID]))
	}
	return items, nil
}

func (q *StationViewQuery) resolveTemplate(project Project, edgeInstanceID string) (StationViewTemplate, error) {
	var assignments []StationViewAssignment
	if err := q.db.Where("enabled = ?", true).Find(&assignments).Error; err != nil {
		return StationViewTemplate{}, err
	}
	if len(assignments) == 0 {
		return StationViewTemplate{}, ErrStationViewSyncNotReady
	}
	candidates := make([]stationViewAssignmentCandidate, 0, len(assignments))
	for _, assignment := range assignments {
		score := assignmentScore(assignment, project, edgeInstanceID)
		if score <= 0 {
			continue
		}
		var template StationViewTemplate
		err := q.db.Where("template_uid = ? AND status = ?", assignment.TemplateUID, StationViewStatusPublished).First(&template).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return StationViewTemplate{}, err
		}
		candidates = append(candidates, stationViewAssignmentCandidate{Assignment: assignment, Template: template, Score: score + assignment.Priority})
	}
	if len(candidates) == 0 {
		return StationViewTemplate{}, ErrStationViewSyncNotReady
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Template.Version != candidates[j].Template.Version {
			return candidates[i].Template.Version > candidates[j].Template.Version
		}
		return candidates[i].Assignment.ID < candidates[j].Assignment.ID
	})
	if len(candidates) > 1 && candidates[0].Score == candidates[1].Score && candidates[0].Template.Version == candidates[1].Template.Version {
		return StationViewTemplate{}, ErrStationViewTemplateConflict
	}
	return candidates[0].Template, nil
}

func (q *StationViewQuery) loadRegions(templateUID string) ([]StationViewRegion, error) {
	var regions []StationViewRegion
	err := q.db.Where("template_uid = ? AND enabled = ?", templateUID, true).Order("sort_order ASC, id ASC").Find(&regions).Error
	return regions, err
}

func (q *StationViewQuery) loadItems(templateUID string) ([]StationViewItem, error) {
	var items []StationViewItem
	err := q.db.Where("template_uid = ? AND visible = ?", templateUID, true).Order("layout_area ASC, pinned DESC, sort_order ASC, id ASC").Find(&items).Error
	return items, err
}

func (q *StationViewQuery) loadProjectTags(projectID uint) ([]TagConfig, error) {
	var tags []TagConfig
	err := q.db.Where("project_id = ? AND enabled = ?", projectID, true).Order("var_group ASC, var_name ASC, var_id ASC").Find(&tags).Error
	return tags, err
}

func (q *StationViewQuery) loadCurrentRun(projectID uint) (DetectionTask, bool, error) {
	var task DetectionTask
	err := q.db.Where("project_id = ? AND status IN ?", projectID, []string{DetectionStatusRunning, DetectionStatusPaused}).Order("started_at DESC, id DESC").First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DetectionTask{}, false, nil
	}
	if err != nil {
		return DetectionTask{}, false, err
	}
	var items []DetectionRunStandardItem
	if err := q.db.Where("task_id = ?", task.ID).Order("sort_order ASC, id ASC").Find(&items).Error; err != nil {
		return DetectionTask{}, false, err
	}
	task.StandardItems = items
	return task, true, nil
}

func assignmentScore(assignment StationViewAssignment, project Project, edgeInstanceID string) int {
	targetKey := strings.TrimSpace(assignment.TargetKey)
	switch assignment.TargetType {
	case StationViewTargetProject:
		if targetKey == project.ProjectCode || targetKey == strconv.FormatUint(uint64(project.ID), 10) {
			return 400
		}
	case StationViewTargetEdge:
		if edgeInstanceID != "" && targetKey == edgeInstanceID {
			return 300
		}
	case StationViewTargetModel:
		if project.ModelName != "" && targetKey == project.ModelName {
			return 200
		}
	case StationViewTargetGlobal:
		if targetKey == "" || targetKey == "*" {
			return 100
		}
	}
	return 0
}

func resolveItemBindings(item StationViewItem, tags []TagConfig, currentRun DetectionTask, hasCurrentRun bool) ([]StationViewResolvedBinding, []string) {
	switch item.BindingType {
	case StationViewBindingVarGroup:
		return resolveVarGroup(item, tags), nil
	case StationViewBindingVarName:
		return resolveVarName(item, tags)
	case StationViewBindingRunItems:
		if !hasCurrentRun {
			return nil, []string{item.ItemUID + ": no current detection run"}
		}
		bindings := make([]StationViewResolvedBinding, 0, len(currentRun.StandardItems))
		for _, runItem := range currentRun.StandardItems {
			bindings = append(bindings, bindingFromRunItem(runItem))
		}
		sort.Slice(bindings, func(i, j int) bool {
			if bindings[i].SortOrder != bindings[j].SortOrder {
				return bindings[i].SortOrder < bindings[j].SortOrder
			}
			return bindings[i].VarID < bindings[j].VarID
		})
		return bindings, nil
	default:
		return nil, nil
	}
}

func resolveVarGroup(item StationViewItem, tags []TagConfig) []StationViewResolvedBinding {
	bindings := make([]StationViewResolvedBinding, 0, len(tags))
	for idx, tag := range tags {
		if item.BindingKey != "" && tag.VarGroup != item.BindingKey {
			continue
		}
		bindings = append(bindings, bindingFromTag("project_variable", tag, item.SortOrder+idx))
	}
	return bindings
}

func resolveVarName(item StationViewItem, tags []TagConfig) ([]StationViewResolvedBinding, []string) {
	bindings := []StationViewResolvedBinding{}
	for _, tag := range tags {
		if tag.VarName == item.BindingKey {
			bindings = append(bindings, bindingFromTag("project_variable", tag, item.SortOrder))
		}
	}
	if len(bindings) == 0 {
		return nil, []string{item.ItemUID + ": var_name not found in project"}
	}
	if len(bindings) > 1 {
		return bindings, []string{item.ItemUID + ": var_name matched multiple project variables"}
	}
	return bindings, nil
}

func bindingFromTag(source string, tag TagConfig, sortOrder int) StationViewResolvedBinding {
	return StationViewResolvedBinding{
		Source:        source,
		VarID:         tag.VarID,
		VarIDText:     strconv.FormatInt(tag.VarID, 10),
		VarName:       tag.VarName,
		VarGroup:      tag.VarGroup,
		DisplayName:   firstDisplayName(tag.DisplayName, tag.VarName),
		DisplayNameEN: tag.DisplayNameEN,
		DisplayNameJA: tag.DisplayNameJA,
		DataType:      tag.DataType,
		Unit:          tag.Unit,
		DecimalPlaces: tag.DecimalPlaces,
		LimitLL:       tag.DefaultLimitLL,
		LimitL:        tag.DefaultLimitL,
		LimitH:        tag.DefaultLimitH,
		LimitHH:       tag.DefaultLimitHH,
		CheckEnabled:  tag.DefaultAlarmEnabled,
		AlarmEnabled:  tag.DefaultAlarmEnabled,
		SortOrder:     sortOrder,
	}
}

func bindingFromRunItem(item DetectionRunStandardItem) StationViewResolvedBinding {
	return StationViewResolvedBinding{
		Source:        "detection_item",
		VarID:         item.VarID,
		VarIDText:     strconv.FormatInt(item.VarID, 10),
		VarName:       item.VarName,
		DisplayName:   firstDisplayName(item.DisplayName, item.VarName),
		DisplayNameEN: item.DisplayNameEN,
		DisplayNameJA: item.DisplayNameJA,
		Unit:          item.Unit,
		DecimalPlaces: item.DecimalPlaces,
		LimitLL:       item.LimitLL,
		LimitL:        item.LimitL,
		LimitH:        item.LimitH,
		LimitHH:       item.LimitHH,
		CheckEnabled:  item.CheckEnabled,
		AlarmEnabled:  item.AlarmEnabled,
		SortOrder:     item.SortOrder,
	}
}

func regionDTOs(regions []StationViewRegion) []StationViewRegionDTO {
	result := make([]StationViewRegionDTO, 0, len(regions))
	for _, region := range regions {
		result = append(result, StationViewRegionDTO{
			LayoutArea: regionLayoutArea(region),
			LayoutType: region.RegionType,
			LayoutJSON: region.LayoutJSON,
			SortOrder:  region.SortOrder,
		})
	}
	return result
}

func itemLayoutArea(item StationViewItem) string {
	if layoutArea := strings.TrimSpace(item.LayoutArea); layoutArea != "" {
		return layoutArea
	}
	return strings.TrimSpace(item.RegionKey)
}

func regionLayoutArea(region StationViewRegion) string {
	if layoutArea := strings.TrimSpace(region.LayoutArea); layoutArea != "" {
		return layoutArea
	}
	return strings.TrimSpace(region.RegionKey)
}

func stationViewTemplateListItem(template StationViewTemplate, assignments []StationViewAssignmentDTO) StationViewTemplateListItem {
	return StationViewTemplateListItem{
		ID:            template.ID,
		TemplateUID:   template.TemplateUID,
		TemplateCode:  template.TemplateCode,
		Name:          template.Name,
		DisplayName:   template.DisplayName,
		DisplayNameEN: template.DisplayNameEN,
		DisplayNameJA: template.DisplayNameJA,
		Version:       template.Version,
		Status:        template.Status,
		OwnerScope:    template.OwnerScope,
		LayoutJSON:    template.LayoutJSON,
		Assignments:   assignments,
		CreatedAt:     template.CreatedAt,
		UpdatedAt:     template.UpdatedAt,
	}
}

func stationViewAssignmentDTO(assignment StationViewAssignment) StationViewAssignmentDTO {
	return StationViewAssignmentDTO(assignment)
}

func isMissingTableError(err error) bool {
	return classifySyncTableError(err) == "missing"
}

func firstDisplayName(displayName string, fallback string) string {
	if displayName != "" {
		return displayName
	}
	return fallback
}
