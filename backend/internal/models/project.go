package models

import "time"

type Project struct {
	ID            uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectCode   string    `gorm:"column:project_code;size:64;uniqueIndex;not null" json:"project_code"`
	SiteNo        string    `gorm:"column:site_no;size:64;index" json:"site_no"`
	Name          string    `gorm:"column:name;size:128;not null" json:"name"`
	DisplayName   string    `gorm:"column:display_name;size:128" json:"display_name"`
	DisplayNameEN string    `gorm:"column:display_name_en;size:128" json:"display_name_en"`
	DisplayNameJA string    `gorm:"column:display_name_ja;size:128" json:"display_name_ja"`
	ModelName     string    `gorm:"column:model_name;size:128" json:"model_name"`
	ImageRef      string    `gorm:"column:image_ref;size:255" json:"image_ref"`
	Enabled       bool      `gorm:"column:enabled;default:true;index" json:"enabled"`
	Blocked       bool      `gorm:"column:blocked;default:false;index" json:"blocked"`
	Placeholder   bool      `gorm:"column:placeholder;default:false" json:"placeholder"`
	CurrentTaskID *uint     `gorm:"column:current_task_id" json:"current_task_id,omitempty"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Project) TableName() string {
	return "sys_projects"
}
