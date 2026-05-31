package models

import (
	"encoding/json"
	"strconv"
	"time"
)

type StorageRoute struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID     uint      `gorm:"column:project_id;index;not null" json:"project_id"`
	VarID         int64     `gorm:"column:var_id;index;not null;uniqueIndex:uk_storage_route_var_code" json:"var_id"`
	RouteCode     string    `gorm:"column:route_code;size:128;not null;uniqueIndex:uk_storage_route_var_code" json:"route_code"`
	StorageTarget string    `gorm:"column:storage_target;size:32;default:wide_table;not null;index" json:"storage_target"`
	StorageTable  string    `gorm:"column:table_name;size:128;not null;index" json:"table_name"`
	ColumnName    string    `gorm:"column:column_name;size:64;not null;index" json:"column_name"`
	ColumnType    string    `gorm:"column:column_type;size:32;not null" json:"column_type"`
	FormFieldKey  string    `gorm:"column:form_field_key;size:128" json:"form_field_key"`
	QueryAlias    string    `gorm:"column:query_alias;size:128" json:"query_alias"`
	TriggerMode   string    `gorm:"column:trigger_mode;size:32;default:on_cycle;not null" json:"trigger_mode"`
	CycleMS       int       `gorm:"column:cycle_ms;default:0" json:"cycle_ms"`
	Deadband      float64   `gorm:"column:deadband;default:0" json:"deadband"`
	StoreOnStart  bool      `gorm:"column:store_on_start;default:false" json:"store_on_start"`
	Enabled       bool      `gorm:"column:enabled;default:false;index" json:"enabled"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (StorageRoute) TableName() string {
	return "sys_storage_routes"
}

func (r StorageRoute) MarshalJSON() ([]byte, error) {
	type alias StorageRoute
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(r),
		VarIDText: strconv.FormatInt(r.VarID, 10),
	})
}

type DetectionRunStorageRoute struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID        uint      `gorm:"column:task_id;index;not null" json:"task_id"`
	TestNo        string    `gorm:"column:test_no;size:128;index" json:"test_no"`
	ProjectID     uint      `gorm:"column:project_id;index;not null" json:"project_id"`
	VarID         int64     `gorm:"column:var_id;index;not null" json:"var_id"`
	RouteID       uint64    `gorm:"column:route_id;index;not null" json:"route_id"`
	RouteCode     string    `gorm:"column:route_code;size:128;not null" json:"route_code"`
	StorageTarget string    `gorm:"column:storage_target;size:32;default:wide_table;not null;index" json:"storage_target"`
	StorageTable  string    `gorm:"column:table_name;size:128;not null;index" json:"table_name"`
	ColumnName    string    `gorm:"column:column_name;size:64;not null" json:"column_name"`
	ColumnType    string    `gorm:"column:column_type;size:32;not null" json:"column_type"`
	FormFieldKey  string    `gorm:"column:form_field_key;size:128" json:"form_field_key"`
	QueryAlias    string    `gorm:"column:query_alias;size:128" json:"query_alias"`
	TriggerMode   string    `gorm:"column:trigger_mode;size:32;default:on_cycle;not null" json:"trigger_mode"`
	CycleMS       int       `gorm:"column:cycle_ms;default:0" json:"cycle_ms"`
	Deadband      float64   `gorm:"column:deadband;default:0" json:"deadband"`
	StoreOnStart  bool      `gorm:"column:store_on_start;default:false" json:"store_on_start"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
}

func (DetectionRunStorageRoute) TableName() string {
	return "detection_run_storage_routes"
}

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
