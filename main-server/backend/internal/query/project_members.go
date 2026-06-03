package query

import "time"

type SysProjectMember struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID     uint      `gorm:"column:project_id" json:"project_id"`
	UserID        uint      `gorm:"column:user_id" json:"user_id"`
	MemberRole    string    `gorm:"column:member_role" json:"member_role"`
	NotifyEnabled bool      `gorm:"column:notify_enabled" json:"notify_enabled"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (SysProjectMember) TableName() string { return "sys_project_members" }

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

func (q *StationViewQuery) ListProjectMembers(projectID uint, edgeInstanceID string) ([]ProjectMemberView, error) {
	if _, err := q.projectForEdge(projectID, edgeInstanceID); err != nil {
		return nil, err
	}
	var members []ProjectMemberView
	err := q.db.Table("sys_project_members AS pm").
		Select("pm.id, pm.project_id, pm.user_id, u.username, u.role AS user_role, u.enabled AS user_enabled, pm.member_role, pm.notify_enabled, pm.created_at, pm.updated_at").
		Joins("JOIN sys_users AS u ON u.id = pm.user_id").
		Where("pm.project_id = ?", projectID).
		Order("pm.id ASC").
		Scan(&members).Error
	if members == nil {
		members = []ProjectMemberView{}
	}
	return members, err
}
