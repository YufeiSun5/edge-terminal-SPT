package models

import "time"

type RuntimeSetting struct {
	Key       string    `gorm:"column:setting_key;size:128;primaryKey" json:"key"`
	Value     string    `gorm:"column:setting_value;size:512;not null" json:"value"`
	Remark    string    `gorm:"column:remark;size:255" json:"remark"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (RuntimeSetting) TableName() string {
	return "runtime_settings"
}
