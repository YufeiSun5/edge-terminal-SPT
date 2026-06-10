package reports

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type MainReportNotification struct {
	ID        uint64    `gorm:"column:id;primaryKey" json:"id"`
	JobID     uint64    `gorm:"column:job_id;index" json:"job_id"`
	EventID   uint64    `gorm:"column:event_id;index" json:"event_id"`
	Type      string    `gorm:"column:type;size:64;index" json:"type"`
	Level     string    `gorm:"column:level;size:32;index" json:"level"`
	Title     string    `gorm:"column:title;size:255" json:"title"`
	Message   string    `gorm:"column:message;size:512" json:"message"`
	Payload   string    `gorm:"column:payload;type:text" json:"-"`
	CreatedAt time.Time `gorm:"column:created_at;index" json:"created_at"`
}

func (MainReportNotification) TableName() string { return "main_report_notifications" }

type MainReportNotificationRecipient struct {
	ID             uint64     `gorm:"column:id;primaryKey" json:"id"`
	NotificationID uint64     `gorm:"column:notification_id;uniqueIndex:idx_report_notification_user" json:"notification_id"`
	UserID         uint       `gorm:"column:user_id;uniqueIndex:idx_report_notification_user;index" json:"user_id"`
	ReadAt         *time.Time `gorm:"column:read_at;index" json:"read_at,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (MainReportNotificationRecipient) TableName() string {
	return "main_report_notification_recipients"
}

type NotificationFilter struct {
	JobID  *uint64
	Level  string
	Unread *bool
	Limit  int
	Offset int
}

type ReportNotificationDTO struct {
	ID        uint64          `json:"id"`
	JobID     uint64          `json:"job_id"`
	EventID   uint64          `json:"event_id"`
	Type      string          `json:"type"`
	Level     string          `json:"level"`
	Title     string          `json:"title"`
	Message   string          `json:"message"`
	Payload   json.RawMessage `json:"payload"`
	ReadAt    *time.Time      `json:"read_at,omitempty"`
	Unread    bool            `json:"unread"`
	CreatedAt time.Time       `json:"created_at"`
}

type reportNotificationRow struct {
	ID        uint64
	JobID     uint64
	EventID   uint64
	Type      string
	Level     string
	Title     string
	Message   string
	Payload   string
	ReadAt    *time.Time
	CreatedAt time.Time
}

type reportNotificationUser struct {
	ID      uint
	Role    string
	Enabled bool
}

func (reportNotificationUser) TableName() string { return "sys_users" }

func (s *Service) ListNotifications(userID uint, filter NotificationFilter) ([]ReportNotificationDTO, int64, int, int, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	stmt := s.notificationQuery(userID, filter)
	var total int64
	if err := stmt.Count(&total).Error; err != nil {
		return nil, 0, 0, 0, err
	}
	var rows []reportNotificationRow
	if err := s.notificationQuery(userID, filter).
		Select("n.id, n.job_id, n.event_id, n.type, n.level, n.title, n.message, n.payload, n.created_at, r.read_at").
		Order("n.created_at desc, n.id desc").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error; err != nil {
		return nil, 0, 0, 0, err
	}
	return notificationDTOs(rows), total, limit, offset, nil
}

func (s *Service) UnreadNotificationCount(userID uint, filter NotificationFilter) (int64, error) {
	unread := true
	filter.Unread = &unread
	var total int64
	if err := s.notificationQuery(userID, filter).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (s *Service) MarkNotificationRead(userID uint, notificationID uint64) error {
	var recipient MainReportNotificationRecipient
	if err := s.db.Where("notification_id = ? AND user_id = ?", notificationID, userID).First(&recipient).Error; err != nil {
		return err
	}
	if recipient.ReadAt != nil {
		return nil
	}
	now := time.Now()
	return s.db.Model(&MainReportNotificationRecipient{}).
		Where("id = ?", recipient.ID).
		Update("read_at", &now).Error
}

func (s *Service) MarkNotificationsRead(userID uint, filter NotificationFilter) (int64, error) {
	now := time.Now()
	query := s.notificationRecipientQuery(userID, filter).Where("r.read_at IS NULL")
	var ids []uint64
	if err := query.Pluck("r.id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := s.db.Model(&MainReportNotificationRecipient{}).Where("id IN ?", ids).Update("read_at", &now)
	return result.RowsAffected, result.Error
}

func (s *Service) notificationQuery(userID uint, filter NotificationFilter) *gorm.DB {
	return s.notificationRecipientQuery(userID, filter)
}

func (s *Service) notificationRecipientQuery(userID uint, filter NotificationFilter) *gorm.DB {
	stmt := s.db.Table("main_report_notification_recipients AS r").
		Joins("JOIN main_report_notifications AS n ON n.id = r.notification_id").
		Where("r.user_id = ?", userID)
	if filter.JobID != nil {
		stmt = stmt.Where("n.job_id = ?", *filter.JobID)
	}
	if strings.TrimSpace(filter.Level) != "" {
		stmt = stmt.Where("n.level = ?", strings.TrimSpace(filter.Level))
	}
	if filter.Unread != nil {
		if *filter.Unread {
			stmt = stmt.Where("r.read_at IS NULL")
		} else {
			stmt = stmt.Where("r.read_at IS NOT NULL")
		}
	}
	return stmt
}

func notificationDTOs(rows []reportNotificationRow) []ReportNotificationDTO {
	items := make([]ReportNotificationDTO, 0, len(rows))
	for _, row := range rows {
		payload := json.RawMessage(`{}`)
		if strings.TrimSpace(row.Payload) != "" && json.Valid([]byte(row.Payload)) {
			payload = json.RawMessage(row.Payload)
		}
		items = append(items, ReportNotificationDTO{
			ID:        row.ID,
			JobID:     row.JobID,
			EventID:   row.EventID,
			Type:      row.Type,
			Level:     row.Level,
			Title:     row.Title,
			Message:   row.Message,
			Payload:   payload,
			ReadAt:    row.ReadAt,
			Unread:    row.ReadAt == nil,
			CreatedAt: row.CreatedAt,
		})
	}
	return items
}

func (s *Service) recordNotificationForEvent(event MainReportJobEvent) error {
	spec, ok := notificationSpecForEvent(event)
	if !ok {
		return nil
	}
	payload, err := notificationPayload(event)
	if err != nil {
		return err
	}
	notification := MainReportNotification{
		JobID:     event.JobID,
		EventID:   event.ID,
		Type:      "report.job",
		Level:     spec.level,
		Title:     spec.title,
		Message:   spec.message,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&notification).Error; err != nil {
			return err
		}
		var users []reportNotificationUser
		if err := tx.Where("enabled = ?", true).Find(&users).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		recipients := make([]MainReportNotificationRecipient, 0, len(users))
		now := time.Now()
		for _, user := range users {
			if user.ID == 0 {
				continue
			}
			recipients = append(recipients, MainReportNotificationRecipient{
				NotificationID: notification.ID,
				UserID:         user.ID,
				CreatedAt:      now,
				UpdatedAt:      now,
			})
		}
		if len(recipients) == 0 {
			return nil
		}
		return tx.Create(&recipients).Error
	})
}

type reportNotificationSpec struct {
	level   string
	title   string
	message string
}

func notificationSpecForEvent(event MainReportJobEvent) (reportNotificationSpec, bool) {
	switch event.EventType {
	case EventStarted:
		return reportNotificationSpec{level: "info", title: "报表开始生成", message: "主服务器已开始生成检测报表"}, true
	case EventSucceeded:
		return reportNotificationSpec{level: "success", title: "报表生成完成", message: "检测报表已生成，可下载查看"}, true
	case EventFailed:
		level := strings.TrimSpace(event.Level)
		if level == "" {
			level = "warning"
		}
		return reportNotificationSpec{level: level, title: "报表生成失败", message: event.Message}, true
	default:
		return reportNotificationSpec{}, false
	}
}

func notificationPayload(event MainReportJobEvent) (string, error) {
	eventPayload := json.RawMessage(`{}`)
	if strings.TrimSpace(event.Payload) != "" && json.Valid([]byte(event.Payload)) {
		eventPayload = json.RawMessage(event.Payload)
	}
	raw, err := json.Marshal(map[string]any{
		"job_id":        event.JobID,
		"event_id":      event.ID,
		"event_type":    event.EventType,
		"event_level":   event.Level,
		"event_message": event.Message,
		"event_payload": eventPayload,
	})
	if err != nil {
		return "", fmt.Errorf("marshal notification payload: %w", err)
	}
	return string(raw), nil
}
