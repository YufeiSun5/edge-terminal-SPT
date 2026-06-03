package query

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type AuditLogFilter struct {
	ActorType  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Result     string
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

type NotificationFilter struct {
	UserID    uint
	Unread    *bool
	Type      string
	Level     string
	ProjectID *uint
	From      *time.Time
	To        *time.Time
	Keyword   string
	Limit     int
	Offset    int
}

type AuditLog struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ActorType  string    `gorm:"column:actor_type" json:"actor_type"`
	ActorID    string    `gorm:"column:actor_id" json:"actor_id"`
	Action     string    `gorm:"column:action" json:"action"`
	TargetType string    `gorm:"column:target_type" json:"target_type"`
	TargetID   string    `gorm:"column:target_id" json:"target_id"`
	Result     string    `gorm:"column:result" json:"result"`
	Detail     string    `gorm:"column:detail" json:"detail"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AuditLog) TableName() string { return "sys_audit_logs" }

type SysNotification struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	EventUID    string     `gorm:"column:event_uid" json:"event_uid"`
	Type        string     `gorm:"column:type" json:"type"`
	Level       string     `gorm:"column:level" json:"level"`
	TargetType  string     `gorm:"column:target_type" json:"target_type"`
	TargetID    string     `gorm:"column:target_id" json:"target_id"`
	ProjectID   uint       `gorm:"column:project_id" json:"project_id"`
	ProjectCode string     `gorm:"column:project_code" json:"project_code"`
	TaskID      uint       `gorm:"column:task_id" json:"task_id"`
	TestNo      string     `gorm:"column:test_no" json:"test_no"`
	VarID       int64      `gorm:"column:var_id" json:"var_id"`
	VarName     string     `gorm:"column:var_name" json:"var_name"`
	DisplayName string     `gorm:"column:display_name" json:"display_name"`
	Message     string     `gorm:"column:message" json:"message"`
	Payload     string     `gorm:"column:payload" json:"payload"`
	OccurredAt  time.Time  `gorm:"column:occurred_at" json:"occurred_at"`
	ExpiresAt   *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (SysNotification) TableName() string { return "sys_notifications" }

type SysNotificationRecipient struct {
	ID             uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NotificationID uint64     `gorm:"column:notification_id;uniqueIndex:uk_notification_recipient" json:"notification_id"`
	UserID         uint       `gorm:"column:user_id;uniqueIndex:uk_notification_recipient" json:"user_id"`
	ReadAt         *time.Time `gorm:"column:read_at" json:"read_at,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (SysNotificationRecipient) TableName() string { return "sys_notification_recipients" }

type UserNotification struct {
	ID          uint64     `gorm:"column:id" json:"id"`
	EventUID    string     `gorm:"column:event_uid" json:"event_uid"`
	Type        string     `gorm:"column:type" json:"type"`
	Level       string     `gorm:"column:level" json:"level"`
	TargetType  string     `gorm:"column:target_type" json:"target_type"`
	TargetID    string     `gorm:"column:target_id" json:"target_id"`
	ProjectID   uint       `gorm:"column:project_id" json:"project_id"`
	ProjectCode string     `gorm:"column:project_code" json:"project_code"`
	TaskID      uint       `gorm:"column:task_id" json:"task_id,omitempty"`
	TestNo      string     `gorm:"column:test_no" json:"test_no,omitempty"`
	VarID       int64      `gorm:"column:var_id" json:"var_id,omitempty"`
	VarName     string     `gorm:"column:var_name" json:"var_name,omitempty"`
	DisplayName string     `gorm:"column:display_name" json:"display_name,omitempty"`
	Message     string     `gorm:"column:message" json:"message"`
	Payload     string     `gorm:"column:payload" json:"payload"`
	OccurredAt  time.Time  `gorm:"column:occurred_at" json:"occurred_at"`
	ExpiresAt   *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at"`
	ReadAt      *time.Time `gorm:"column:read_at" json:"read_at,omitempty"`
}

func (n UserNotification) MarshalJSON() ([]byte, error) {
	payload := json.RawMessage("{}")
	if strings.TrimSpace(n.Payload) != "" && json.Valid([]byte(n.Payload)) {
		payload = json.RawMessage(n.Payload)
	}
	return json.Marshal(struct {
		ID          uint64          `json:"id"`
		EventUID    string          `json:"event_uid"`
		Type        string          `json:"type"`
		Level       string          `json:"level"`
		TargetType  string          `json:"target_type"`
		TargetID    string          `json:"target_id"`
		ProjectID   uint            `json:"project_id"`
		ProjectCode string          `json:"project_code"`
		TaskID      uint            `json:"task_id,omitempty"`
		TestNo      string          `json:"test_no,omitempty"`
		VarID       int64           `json:"var_id,omitempty"`
		VarIDText   string          `json:"var_id_text,omitempty"`
		VarName     string          `json:"var_name,omitempty"`
		DisplayName string          `json:"display_name,omitempty"`
		Message     string          `json:"message"`
		Payload     json.RawMessage `json:"payload"`
		OccurredAt  time.Time       `json:"occurred_at"`
		ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
		CreatedAt   time.Time       `json:"created_at"`
		ReadAt      *time.Time      `json:"read_at,omitempty"`
	}{
		ID:          n.ID,
		EventUID:    n.EventUID,
		Type:        n.Type,
		Level:       n.Level,
		TargetType:  n.TargetType,
		TargetID:    n.TargetID,
		ProjectID:   n.ProjectID,
		ProjectCode: n.ProjectCode,
		TaskID:      n.TaskID,
		TestNo:      n.TestNo,
		VarID:       n.VarID,
		VarIDText:   optionalInt64Text(n.VarID),
		VarName:     n.VarName,
		DisplayName: n.DisplayName,
		Message:     n.Message,
		Payload:     payload,
		OccurredAt:  n.OccurredAt,
		ExpiresAt:   n.ExpiresAt,
		CreatedAt:   n.CreatedAt,
		ReadAt:      n.ReadAt,
	})
}

func (q *StationViewQuery) ListAuditLogs(filter AuditLogFilter) ([]AuditLog, int64, int, int, error) {
	limit := normalizedLimit(filter.Limit, 50, 200)
	offset := normalizedOffset(filter.Offset)
	stmt := q.db.Model(&AuditLog{})
	if strings.TrimSpace(filter.ActorType) != "" {
		stmt = stmt.Where("actor_type = ?", strings.TrimSpace(filter.ActorType))
	}
	if strings.TrimSpace(filter.ActorID) != "" {
		stmt = stmt.Where("actor_id = ?", strings.TrimSpace(filter.ActorID))
	}
	if strings.TrimSpace(filter.Action) != "" {
		stmt = stmt.Where("action = ?", strings.TrimSpace(filter.Action))
	}
	if strings.TrimSpace(filter.TargetType) != "" {
		stmt = stmt.Where("target_type = ?", strings.TrimSpace(filter.TargetType))
	}
	if strings.TrimSpace(filter.TargetID) != "" {
		stmt = stmt.Where("target_id = ?", strings.TrimSpace(filter.TargetID))
	}
	if strings.TrimSpace(filter.Result) != "" {
		stmt = stmt.Where("result = ?", strings.TrimSpace(filter.Result))
	}
	if filter.From != nil {
		stmt = stmt.Where("created_at >= ?", *filter.From)
	}
	if filter.To != nil {
		stmt = stmt.Where("created_at <= ?", *filter.To)
	}
	var total int64
	if err := stmt.Count(&total).Error; err != nil {
		return nil, 0, 0, 0, err
	}
	var items []AuditLog
	err := stmt.Order("created_at desc, id desc").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, limit, offset, err
}

func (q *StationViewQuery) ListUserNotifications(filter NotificationFilter, edgeInstanceID string) ([]UserNotification, int64, int, int, error) {
	if filter.UserID == 0 {
		return nil, 0, 0, 0, gorm.ErrRecordNotFound
	}
	if filter.ProjectID != nil {
		if _, err := q.projectForEdge(*filter.ProjectID, edgeInstanceID); err != nil {
			return nil, 0, 0, 0, err
		}
	}
	limit := normalizedLimit(filter.Limit, 50, 200)
	offset := normalizedOffset(filter.Offset)
	stmt := notificationBaseQuery(q.db, filter, edgeInstanceID)
	var total int64
	if err := stmt.Count(&total).Error; err != nil {
		return nil, 0, 0, 0, err
	}
	var items []UserNotification
	err := stmt.Select("n.id, n.event_uid, n.type, n.level, n.target_type, n.target_id, n.project_id, n.project_code, n.task_id, n.test_no, n.var_id, n.var_name, n.display_name, n.message, n.payload, n.occurred_at, n.expires_at, n.created_at, r.read_at").
		Order("n.occurred_at desc, n.id desc").
		Limit(limit).
		Offset(offset).
		Scan(&items).Error
	return items, total, limit, offset, err
}

func (q *StationViewQuery) CountUnreadNotifications(filter NotificationFilter, edgeInstanceID string) (int64, error) {
	if filter.UserID == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	if filter.ProjectID != nil {
		if _, err := q.projectForEdge(*filter.ProjectID, edgeInstanceID); err != nil {
			return 0, err
		}
	}
	unread := true
	filter.Unread = &unread
	var count int64
	err := notificationBaseQuery(q.db, filter, edgeInstanceID).Count(&count).Error
	return count, err
}

func notificationBaseQuery(db *gorm.DB, filter NotificationFilter, edgeInstanceID string) *gorm.DB {
	stmt := db.Table("sys_notifications AS n").
		Joins("JOIN sys_notification_recipients AS r ON r.notification_id = n.id").
		Joins("LEFT JOIN sys_projects p ON p.id = n.project_id").
		Where("r.user_id = ?", filter.UserID).
		Where("(n.expires_at IS NULL OR n.expires_at > ?)", time.Now())

	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Where("(n.project_id = 0 OR p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if filter.Unread != nil {
		if *filter.Unread {
			stmt = stmt.Where("r.read_at IS NULL")
		} else {
			stmt = stmt.Where("r.read_at IS NOT NULL")
		}
	}
	if strings.TrimSpace(filter.Type) != "" {
		stmt = stmt.Where("n.type = ?", strings.TrimSpace(filter.Type))
	}
	if strings.TrimSpace(filter.Level) != "" {
		stmt = stmt.Where("n.level = ?", strings.TrimSpace(filter.Level))
	}
	if filter.ProjectID != nil {
		stmt = stmt.Where("n.project_id = ?", *filter.ProjectID)
	}
	if filter.From != nil {
		stmt = stmt.Where("n.occurred_at >= ?", *filter.From)
	}
	if filter.To != nil {
		stmt = stmt.Where("n.occurred_at <= ?", *filter.To)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		stmt = stmt.Where(
			"n.message LIKE ? OR n.var_name LIKE ? OR n.display_name LIKE ? OR n.project_code LIKE ? OR n.test_no LIKE ? OR n.event_uid LIKE ?",
			like, like, like, like, like, like,
		)
	}
	return stmt
}

func optionalInt64Text(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}
