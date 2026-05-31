package models

import (
	"encoding/json"
	"strconv"
	"time"
)

type TagConfig struct {
	VarID                  int64     `gorm:"column:var_id;primaryKey" json:"var_id"`
	GatewayID              int       `gorm:"column:gateway_id;not null;index;uniqueIndex:uk_gateway_source" json:"gateway_id"`
	SourceTopic            string    `gorm:"column:source_topic;size:255" json:"source_topic"`
	SourcePath             string    `gorm:"column:source_path;size:512;not null;uniqueIndex:uk_gateway_source" json:"source_path"`
	SourceType             string    `gorm:"column:source_type;size:32;default:mqtt;not null;index" json:"source_type"`
	RawName                string    `gorm:"column:raw_name;size:255;not null" json:"raw_name"`
	ProjectID              *uint     `gorm:"column:project_id;index" json:"project_id,omitempty"`
	ProjectCode            string    `gorm:"column:project_code;size:64;index" json:"project_code"`
	VarGroup               string    `gorm:"column:var_group;size:128;index" json:"var_group"`
	VarName                string    `gorm:"column:var_name;size:128;not null;index" json:"var_name"`
	DisplayName            string    `gorm:"column:display_name;size:128" json:"display_name"`
	DisplayNameEN          string    `gorm:"column:display_name_en;size:128" json:"display_name_en"`
	DisplayNameJA          string    `gorm:"column:display_name_ja;size:128" json:"display_name_ja"`
	JSONPath               string    `gorm:"column:json_path;size:512;not null" json:"json_path"`
	DataType               string    `gorm:"column:data_type;size:32;not null" json:"data_type"`
	Unit                   string    `gorm:"column:unit;size:32" json:"unit"`
	DecimalPlaces          int       `gorm:"column:decimal_places;default:2" json:"decimal_places"`
	ScaleFactor            float64   `gorm:"column:scale_factor;default:1" json:"scale_factor"`
	OffsetVal              float64   `gorm:"column:offset_val;default:0" json:"offset_val"`
	RWMode                 string    `gorm:"column:rw_mode;size:8;default:R;not null" json:"rw_mode"`
	Writable               bool      `gorm:"column:writable;default:false;index" json:"writable"`
	WriteSourceID          int       `gorm:"column:write_source_id;default:0" json:"write_source_id"`
	WritePath              string    `gorm:"column:write_path;size:512" json:"write_path"`
	WriteDataType          string    `gorm:"column:write_data_type;size:32" json:"write_data_type"`
	WriteMin               *float64  `gorm:"column:write_min" json:"write_min,omitempty"`
	WriteMax               *float64  `gorm:"column:write_max" json:"write_max,omitempty"`
	WriteEnum              string    `gorm:"column:write_enum;type:text" json:"write_enum"`
	WriteRequiresAudit     bool      `gorm:"column:write_requires_audit;default:true" json:"write_requires_audit"`
	SuspiciousValue        *float64  `gorm:"column:suspicious_value" json:"suspicious_value,omitempty"`
	DebounceThreshold      *float64  `gorm:"column:debounce_threshold" json:"debounce_threshold,omitempty"`
	DebounceMS             int       `gorm:"column:debounce_ms;default:0" json:"debounce_ms"`
	Deadband               float64   `gorm:"column:deadband;default:0" json:"deadband"`
	DefaultAlarmEnabled    bool      `gorm:"column:default_alarm_enabled;default:false;index" json:"default_alarm_enabled"`
	DefaultLimitLL         *float64  `gorm:"column:default_limit_ll" json:"default_limit_ll,omitempty"`
	DefaultLimitL          *float64  `gorm:"column:default_limit_l" json:"default_limit_l,omitempty"`
	DefaultLimitH          *float64  `gorm:"column:default_limit_h" json:"default_limit_h,omitempty"`
	DefaultLimitHH         *float64  `gorm:"column:default_limit_hh" json:"default_limit_hh,omitempty"`
	DefaultLimitDeadband   float64   `gorm:"column:default_limit_deadband;default:0" json:"default_limit_deadband"`
	DefaultViolationHoldMS int       `gorm:"column:default_violation_hold_ms;default:0" json:"default_violation_hold_ms"`
	DefaultRecoverHoldMS   int       `gorm:"column:default_recover_hold_ms;default:0" json:"default_recover_hold_ms"`
	Discovered             bool      `gorm:"column:discovered;default:true;index" json:"discovered"`
	Placeholder            bool      `gorm:"column:placeholder;default:false" json:"placeholder"`
	Enabled                bool      `gorm:"column:enabled;default:true;index" json:"enabled"`
	CreatedAt              time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt              time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (TagConfig) TableName() string {
	return "sys_tags"
}

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
