package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"
)

var runtimeNotificationSeq uint64

type RuntimeNotification struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Level       string         `json:"level"`
	TargetType  string         `json:"target_type,omitempty"`
	TargetID    string         `json:"target_id,omitempty"`
	ProjectID   uint           `json:"project_id"`
	ProjectCode string         `json:"project_code"`
	TaskID      uint           `json:"task_id,omitempty"`
	TestNo      string         `json:"test_no,omitempty"`
	VarID       int64          `json:"var_id,omitempty"`
	VarName     string         `json:"var_name,omitempty"`
	DisplayName string         `json:"display_name,omitempty"`
	Message     string         `json:"message"`
	Payload     map[string]any `json:"payload,omitempty"`
	OccurredAt  time.Time      `json:"occurred_at"`
}

func (n RuntimeNotification) MarshalJSON() ([]byte, error) {
	type alias RuntimeNotification
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text,omitempty"`
	}{
		alias:     alias(n),
		VarIDText: optionalInt64Text(n.VarID),
	})
}

type SysNotification struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	EventUID    string     `gorm:"column:event_uid;size:128;uniqueIndex;not null" json:"event_uid"`
	Type        string     `gorm:"column:type;size:64;index;not null" json:"type"`
	Level       string     `gorm:"column:level;size:32;index;not null" json:"level"`
	TargetType  string     `gorm:"column:target_type;size:32;index;not null" json:"target_type"`
	TargetID    string     `gorm:"column:target_id;size:128;index" json:"target_id"`
	ProjectID   uint       `gorm:"column:project_id;index;default:0" json:"project_id"`
	ProjectCode string     `gorm:"column:project_code;size:64;index" json:"project_code"`
	TaskID      uint       `gorm:"column:task_id;index;default:0" json:"task_id,omitempty"`
	TestNo      string     `gorm:"column:test_no;size:128;index" json:"test_no,omitempty"`
	VarID       int64      `gorm:"column:var_id;index;default:0" json:"var_id,omitempty"`
	VarName     string     `gorm:"column:var_name;size:128;index" json:"var_name,omitempty"`
	DisplayName string     `gorm:"column:display_name;size:128" json:"display_name,omitempty"`
	Message     string     `gorm:"column:message;size:512" json:"message"`
	Payload     string     `gorm:"column:payload;type:json" json:"payload"`
	OccurredAt  time.Time  `gorm:"column:occurred_at;index;not null" json:"occurred_at"`
	ExpiresAt   *time.Time `gorm:"column:expires_at;index" json:"expires_at,omitempty"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (SysNotification) TableName() string {
	return "sys_notifications"
}

type SysNotificationRecipient struct {
	ID             uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NotificationID uint64     `gorm:"column:notification_id;uniqueIndex:uk_notification_recipient;not null" json:"notification_id"`
	UserID         uint       `gorm:"column:user_id;uniqueIndex:uk_notification_recipient;index;not null" json:"user_id"`
	ReadAt         *time.Time `gorm:"column:read_at;index" json:"read_at,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (SysNotificationRecipient) TableName() string {
	return "sys_notification_recipients"
}

type UserNotification struct {
	ID          uint64     `json:"id" gorm:"column:id"`
	EventUID    string     `json:"event_uid" gorm:"column:event_uid"`
	Type        string     `json:"type" gorm:"column:type"`
	Level       string     `json:"level" gorm:"column:level"`
	TargetType  string     `json:"target_type" gorm:"column:target_type"`
	TargetID    string     `json:"target_id" gorm:"column:target_id"`
	ProjectID   uint       `json:"project_id" gorm:"column:project_id"`
	ProjectCode string     `json:"project_code" gorm:"column:project_code"`
	TaskID      uint       `json:"task_id,omitempty" gorm:"column:task_id"`
	TestNo      string     `json:"test_no,omitempty" gorm:"column:test_no"`
	VarID       int64      `json:"var_id,omitempty" gorm:"column:var_id"`
	VarName     string     `json:"var_name,omitempty" gorm:"column:var_name"`
	DisplayName string     `json:"display_name,omitempty" gorm:"column:display_name"`
	Message     string     `json:"message" gorm:"column:message"`
	Payload     string     `json:"payload" gorm:"column:payload"`
	OccurredAt  time.Time  `json:"occurred_at" gorm:"column:occurred_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" gorm:"column:expires_at"`
	CreatedAt   time.Time  `json:"created_at" gorm:"column:created_at"`
	ReadAt      *time.Time `json:"read_at,omitempty" gorm:"column:read_at"`
}

const AlarmNotificationRetentionDays = 90

func NotificationExpiresAt(notificationType string, occurredAt time.Time) *time.Time {
	if !IsAlarmNotificationType(notificationType) {
		return nil
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	expiresAt := occurredAt.AddDate(0, 0, AlarmNotificationRetentionDays)
	return &expiresAt
}

func IsAlarmNotificationType(notificationType string) bool {
	return notificationType == NotificationAlarmLimitEnter ||
		notificationType == NotificationAlarmLimitRecover ||
		notificationType == NotificationAlarmLimitLevelChange
}

func optionalInt64Text(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func NewRuntimeNotification(notificationType string, level string, message string, occurredAt time.Time) *RuntimeNotification {
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	seq := atomic.AddUint64(&runtimeNotificationSeq, 1)
	return &RuntimeNotification{
		ID:         fmt.Sprintf("%s-%d-%d", notificationType, occurredAt.UnixNano(), seq),
		Type:       notificationType,
		Level:      level,
		Message:    message,
		OccurredAt: occurredAt,
	}
}

func RuntimeNotificationFromAlarm(notificationType string, level string, alarm DetectionLimitAlarm, payload map[string]any) *RuntimeNotification {
	occurredAt := alarm.LastSeenAt
	if notificationType == NotificationAlarmLimitEnter {
		occurredAt = alarm.FirstSeenAt
	}
	notification := NewRuntimeNotification(notificationType, level, alarm.Message, occurredAt)
	notification.TargetType = NotificationTargetProject
	notification.TargetID = fmt.Sprintf("%d", alarm.ProjectID)
	notification.ProjectID = alarm.ProjectID
	notification.ProjectCode = alarm.ProjectCode
	notification.TaskID = alarm.TaskID
	notification.TestNo = alarm.TestNo
	notification.VarID = alarm.VarID
	notification.VarName = alarm.VarName
	notification.DisplayName = alarm.DisplayName
	if notification.Message == "" {
		notification.Message = fmt.Sprintf("%s %s", alarm.VarName, alarm.AlarmType)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["scope"] = alarm.Scope
	notification.Payload = payload
	return notification
}

func RuntimeNotificationFromDetectionTask(notificationType string, level string, task DetectionTask, message string, payload map[string]any) *RuntimeNotification {
	occurredAt := time.Now()
	switch notificationType {
	case NotificationDetectionRunStarted:
		if task.StartedAt != nil {
			occurredAt = *task.StartedAt
		}
	case NotificationDetectionRunStopped, NotificationDetectionAbnormalStop, NotificationDetectionResultOK, NotificationDetectionResultNG:
		if task.EndedAt != nil {
			occurredAt = *task.EndedAt
		}
	}
	notification := NewRuntimeNotification(notificationType, level, message, occurredAt)
	notification.TargetType = NotificationTargetProject
	notification.TargetID = fmt.Sprintf("%d", task.ProjectID)
	notification.ProjectID = task.ProjectID
	notification.ProjectCode = task.ProjectCode
	notification.TaskID = task.ID
	notification.TestNo = task.TestNo
	notification.Payload = payload
	return notification
}
