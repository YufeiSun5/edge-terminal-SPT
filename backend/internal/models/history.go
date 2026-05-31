package models

import (
	"encoding/json"
	"strconv"
	"time"
)

type HistoryData struct {
	ID          uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	GatewayID   int       `gorm:"column:gateway_id;index" json:"gateway_id"`
	Topic       string    `gorm:"column:topic;size:255" json:"topic"`
	ProjectID   uint      `gorm:"column:project_id;index" json:"project_id"`
	TaskID      uint      `gorm:"column:task_id;index" json:"task_id"`
	TestNo      string    `gorm:"column:test_no;size:128;index" json:"test_no"`
	VarID       int64     `gorm:"column:var_id;index" json:"var_id"`
	VarName     string    `gorm:"column:var_name;size:128" json:"var_name"`
	ProjectCode string    `gorm:"column:project_code;size:64;index" json:"project_code"`
	Value       *float64  `gorm:"column:value" json:"value,omitempty"`
	StrValue    *string   `gorm:"column:str_value;type:text" json:"str_value,omitempty"`
	Quality     int       `gorm:"column:quality;default:1" json:"quality"`
	SourceTime  time.Time `gorm:"column:source_time;index" json:"source_time"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
}

func (HistoryData) TableName() string {
	return "rt_history_data"
}

func (h HistoryData) MarshalJSON() ([]byte, error) {
	type alias HistoryData
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(h),
		VarIDText: strconv.FormatInt(h.VarID, 10),
	})
}
