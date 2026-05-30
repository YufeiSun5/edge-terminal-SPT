package models

import "time"

type ReportTemplate struct {
	ID           uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TemplateCode string    `gorm:"column:template_code;size:64;uniqueIndex;not null" json:"template_code"`
	Name         string    `gorm:"column:name;size:128;not null" json:"name"`
	DisplayName  string    `gorm:"column:display_name;size:128" json:"display_name"`
	FileRef      string    `gorm:"column:file_ref;size:512;not null" json:"file_ref"`
	FileKind     string    `gorm:"column:file_kind;size:32;not null;default:xlsx" json:"file_kind"`
	Version      int       `gorm:"column:version;default:1;not null" json:"version"`
	Enabled      bool      `gorm:"column:enabled;default:true;index" json:"enabled"`
	Remark       string    `gorm:"column:remark;size:255" json:"remark"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (ReportTemplate) TableName() string {
	return "sys_report_templates"
}

type DetectionRunReport struct {
	ID              uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID          uint       `gorm:"column:task_id;index;not null" json:"task_id"`
	TemplateID      *uint      `gorm:"column:template_id;index" json:"template_id,omitempty"`
	TemplateCode    string     `gorm:"column:template_code;size:64" json:"template_code"`
	TemplateVersion int        `gorm:"column:template_version;default:0" json:"template_version"`
	FileRef         string     `gorm:"column:file_ref;size:512;not null" json:"file_ref"`
	FileName        string     `gorm:"column:file_name;size:255" json:"file_name"`
	Status          string     `gorm:"column:status;size:32;not null;default:pending" json:"status"`
	GeneratedAt     *time.Time `gorm:"column:generated_at" json:"generated_at,omitempty"`
	ErrorMessage    string     `gorm:"column:error_message;size:512" json:"error_message"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionRunReport) TableName() string {
	return "detection_run_reports"
}
