package models

import "time"

type Project struct {
	ID             uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectCode    string    `gorm:"column:project_code;size:64;uniqueIndex;not null" json:"project_code"`
	SiteNo         string    `gorm:"column:site_no;size:64;index" json:"site_no"`
	EdgeInstanceID string    `gorm:"column:edge_instance_id;size:128;index" json:"edge_instance_id"`
	Name           string    `gorm:"column:name;size:128;not null" json:"name"`
	DisplayName    string    `gorm:"column:display_name;size:128" json:"display_name"`
	DisplayNameEN  string    `gorm:"column:display_name_en;size:128" json:"display_name_en"`
	DisplayNameJA  string    `gorm:"column:display_name_ja;size:128" json:"display_name_ja"`
	ModelName      string    `gorm:"column:model_name;size:128" json:"model_name"`
	ImageRef       string    `gorm:"column:image_ref;size:255" json:"image_ref"`
	Enabled        bool      `gorm:"column:enabled;default:true;index" json:"enabled"`
	Blocked        bool      `gorm:"column:blocked;default:false;index" json:"blocked"`
	Placeholder    bool      `gorm:"column:placeholder;default:false" json:"placeholder"`
	CurrentTaskID  *uint     `gorm:"column:current_task_id" json:"current_task_id,omitempty"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Project) TableName() string {
	return "sys_projects"
}

type SysProjectMember struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID     uint      `gorm:"column:project_id;uniqueIndex:uk_project_member;index;not null" json:"project_id"`
	UserID        uint      `gorm:"column:user_id;uniqueIndex:uk_project_member;index;not null" json:"user_id"`
	MemberRole    string    `gorm:"column:member_role;size:32;default:'member'" json:"member_role"`
	NotifyEnabled bool      `gorm:"column:notify_enabled;index;not null" json:"notify_enabled"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (SysProjectMember) TableName() string {
	return "sys_project_members"
}

type ProjectMemberView struct {
	ID            uint64    `json:"id"`
	ProjectID     uint      `json:"project_id"`
	UserID        uint      `json:"user_id"`
	Username      string    `json:"username"`
	UserRole      string    `json:"user_role"`
	UserEnabled   bool      `json:"user_enabled"`
	MemberRole    string    `json:"member_role"`
	NotifyEnabled bool      `json:"notify_enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
