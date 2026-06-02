package models

import (
	"encoding/json"
	"strconv"
	"time"
)

const (
	StationViewStatusDraft     = "draft"
	StationViewStatusPublished = "published"
	StationViewStatusDisabled  = "disabled"
)

const (
	StationViewTargetGlobal  = "global"
	StationViewTargetEdge    = "edge"
	StationViewTargetModel   = "model"
	StationViewTargetProject = "project"
)

const (
	StationViewBindingVarName        = "var_name"
	StationViewBindingVarGroup       = "var_group"
	StationViewBindingDetectionItems = "detection_items"
	StationViewBindingAlarmSummary   = "alarm_summary"
	StationViewBindingRunState       = "run_state"
	StationViewBindingManual         = "manual"
)

type StationViewTemplate struct {
	ID            uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TemplateUID   string    `gorm:"column:template_uid;size:128;uniqueIndex;not null" json:"template_uid"`
	TemplateCode  string    `gorm:"column:template_code;size:64;uniqueIndex;not null" json:"template_code"`
	Name          string    `gorm:"column:name;size:128;not null" json:"name"`
	DisplayName   string    `gorm:"column:display_name;size:128" json:"display_name"`
	DisplayNameEN string    `gorm:"column:display_name_en;size:128" json:"display_name_en"`
	DisplayNameJA string    `gorm:"column:display_name_ja;size:128" json:"display_name_ja"`
	Version       int       `gorm:"column:version;default:1;not null" json:"version"`
	Status        string    `gorm:"column:status;size:32;default:published;index;not null" json:"status"`
	OwnerScope    string    `gorm:"column:owner_scope;size:32;default:edge;index;not null" json:"owner_scope"`
	LayoutJSON    string    `gorm:"column:layout_json;type:text" json:"layout_json"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (StationViewTemplate) TableName() string {
	return "sys_station_view_templates"
}

type StationViewRegion struct {
	ID          uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TemplateUID string    `gorm:"column:template_uid;size:128;uniqueIndex:uk_station_view_region;index;not null" json:"template_uid"`
	RegionKey   string    `gorm:"column:region_key;size:64;uniqueIndex:uk_station_view_region;not null" json:"region_key"`
	RegionType  string    `gorm:"column:region_type;size:64;not null" json:"region_type"`
	LayoutJSON  string    `gorm:"column:layout_json;type:text" json:"layout_json"`
	SortOrder   int       `gorm:"column:sort_order;default:0;index" json:"sort_order"`
	Enabled     bool      `gorm:"column:enabled;default:true;index;not null" json:"enabled"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (StationViewRegion) TableName() string {
	return "sys_station_view_regions"
}

type StationViewItem struct {
	ID          uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TemplateUID string    `gorm:"column:template_uid;size:128;index;not null" json:"template_uid"`
	RegionKey   string    `gorm:"column:region_key;size:64;index;not null" json:"region_key"`
	ItemUID     string    `gorm:"column:item_uid;size:128;uniqueIndex;not null" json:"item_uid"`
	ItemType    string    `gorm:"column:item_type;size:64;not null" json:"item_type"`
	BindingType string    `gorm:"column:binding_type;size:64;index;not null" json:"binding_type"`
	BindingKey  string    `gorm:"column:binding_key;size:128;index" json:"binding_key"`
	BindingJSON string    `gorm:"column:binding_json;type:text" json:"binding_json"`
	DisplayJSON string    `gorm:"column:display_json;type:text" json:"display_json"`
	SortOrder   int       `gorm:"column:sort_order;default:0;index" json:"sort_order"`
	Visible     bool      `gorm:"column:visible;default:true;index;not null" json:"visible"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (StationViewItem) TableName() string {
	return "sys_station_view_items"
}

type StationViewAssignment struct {
	ID          uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TemplateUID string    `gorm:"column:template_uid;size:128;index;not null" json:"template_uid"`
	TargetType  string    `gorm:"column:target_type;size:32;uniqueIndex:uk_station_view_assignment;index;not null" json:"target_type"`
	TargetKey   string    `gorm:"column:target_key;size:128;uniqueIndex:uk_station_view_assignment;index;not null" json:"target_key"`
	Priority    int       `gorm:"column:priority;default:0;index" json:"priority"`
	Enabled     bool      `gorm:"column:enabled;default:true;index;not null" json:"enabled"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (StationViewAssignment) TableName() string {
	return "sys_station_view_assignments"
}

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
	RegionKey  string `json:"region_key"`
	RegionType string `json:"region_type"`
	LayoutJSON string `json:"layout_json,omitempty"`
	SortOrder  int    `json:"sort_order"`
}

type StationViewItemDTO struct {
	ItemUID          string                       `json:"item_uid"`
	RegionKey        string                       `json:"region_key"`
	ItemType         string                       `json:"item_type"`
	BindingType      string                       `json:"binding_type"`
	BindingKey       string                       `json:"binding_key"`
	BindingJSON      string                       `json:"binding_json,omitempty"`
	DisplayJSON      string                       `json:"display_json,omitempty"`
	SortOrder        int                          `json:"sort_order"`
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

func StationViewBindingFromTag(source string, tag TagConfig, sortOrder int) StationViewResolvedBinding {
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

func StationViewBindingFromRunItem(item DetectionRunStandardItem) StationViewResolvedBinding {
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

func (b StationViewResolvedBinding) MarshalJSON() ([]byte, error) {
	type alias StationViewResolvedBinding
	return json.Marshal(alias(b))
}

func firstDisplayName(displayName string, fallback string) string {
	if displayName != "" {
		return displayName
	}
	return fallback
}
