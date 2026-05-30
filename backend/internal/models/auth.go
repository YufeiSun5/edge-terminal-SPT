package models

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

type SysServiceClient struct {
	ID         uint       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ClientID   string     `gorm:"column:client_id;size:128;uniqueIndex;not null" json:"client_id"`
	SecretHash string     `gorm:"column:secret_hash;size:255;uniqueIndex;not null" json:"-"`
	Scopes     string     `gorm:"column:scopes;type:text;not null" json:"scopes"`
	Enabled    bool       `gorm:"column:enabled;default:true;index" json:"enabled"`
	ExpiresAt  *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	CreatedAt  time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (SysServiceClient) TableName() string {
	return "sys_service_clients"
}

type SysSSOTicket struct {
	ID                 uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TicketHash         string     `gorm:"column:ticket_hash;size:255;uniqueIndex;not null" json:"-"`
	UserID             uint       `gorm:"column:user_id;not null;index" json:"user_id"`
	Role               string     `gorm:"column:role;size:32;not null" json:"role"`
	PermissionsVersion int64      `gorm:"column:permissions_version;default:1;not null" json:"permissions_version"`
	EdgeInstanceID     string     `gorm:"column:edge_instance_id;size:128;not null;index" json:"edge_instance_id"`
	ExpiresAt          time.Time  `gorm:"column:expires_at;not null;index" json:"expires_at"`
	UsedAt             *time.Time `gorm:"column:used_at" json:"used_at,omitempty"`
	CreatedIP          string     `gorm:"column:created_ip;size:64" json:"created_ip"`
	CreatedAt          time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (SysSSOTicket) TableName() string {
	return "sys_sso_tickets"
}

type SysAuditLog struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ActorType  string    `gorm:"column:actor_type;size:32;not null;index" json:"actor_type"`
	ActorID    string    `gorm:"column:actor_id;size:128;not null;index" json:"actor_id"`
	Action     string    `gorm:"column:action;size:128;not null;index" json:"action"`
	TargetType string    `gorm:"column:target_type;size:64" json:"target_type"`
	TargetID   string    `gorm:"column:target_id;size:128" json:"target_id"`
	Result     string    `gorm:"column:result;size:32;not null" json:"result"`
	Detail     string    `gorm:"column:detail;type:json" json:"detail"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}

func (SysAuditLog) TableName() string {
	return "sys_audit_logs"
}
