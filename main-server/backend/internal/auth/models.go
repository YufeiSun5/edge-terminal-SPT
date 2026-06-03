package auth

import "time"

type SysUser struct {
	ID                 uint       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Username           string     `gorm:"column:username;size:64;uniqueIndex;not null" json:"username"`
	PasswordHash       string     `gorm:"column:password_hash;size:255;not null" json:"-"`
	Role               string     `gorm:"column:role;size:32;not null" json:"role"`
	Enabled            bool       `gorm:"column:enabled;default:true;index" json:"enabled"`
	PermissionsVersion int64      `gorm:"column:permissions_version;default:1;not null" json:"permissions_version"`
	LastLoginAt        *time.Time `gorm:"column:last_login_at" json:"last_login_at,omitempty"`
	CreatedAt          time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (SysUser) TableName() string {
	return "sys_users"
}
