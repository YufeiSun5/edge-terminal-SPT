package database

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationListFilter struct {
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

func (r *Repository) CreateRuntimeNotification(notification *models.RuntimeNotification) (models.SysNotification, error) {
	if notification == nil {
		return models.SysNotification{}, fmt.Errorf("notification is required")
	}
	payload := "{}"
	if notification.Payload != nil {
		raw, err := json.Marshal(notification.Payload)
		if err != nil {
			return models.SysNotification{}, err
		}
		payload = string(raw)
	}
	targetType := strings.ToLower(strings.TrimSpace(notification.TargetType))
	targetID := strings.TrimSpace(notification.TargetID)
	if targetType == "" {
		if notification.ProjectID > 0 {
			targetType = models.NotificationTargetProject
			targetID = strconv.FormatUint(uint64(notification.ProjectID), 10)
		} else {
			targetType = models.NotificationTargetAll
			targetID = "*"
		}
	}
	now := time.Now()
	item := models.SysNotification{
		EventUID:    notification.ID,
		Type:        notification.Type,
		Level:       notification.Level,
		TargetType:  targetType,
		TargetID:    targetID,
		ProjectID:   notification.ProjectID,
		ProjectCode: notification.ProjectCode,
		TaskID:      notification.TaskID,
		TestNo:      notification.TestNo,
		VarID:       notification.VarID,
		VarName:     notification.VarName,
		DisplayName: notification.DisplayName,
		Message:     notification.Message,
		Payload:     payload,
		OccurredAt:  notification.OccurredAt,
		ExpiresAt:   models.NotificationExpiresAt(notification.Type, notification.OccurredAt),
		CreatedAt:   now,
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error; err != nil {
			return err
		}
		if item.ID == 0 {
			if err := tx.First(&item, "event_uid = ?", notification.ID).Error; err != nil {
				return err
			}
			return nil
		}
		users, err := r.notificationRecipientUsers(tx, targetType, targetID)
		if err != nil {
			return err
		}
		if len(users) == 0 {
			return nil
		}
		recipients := make([]models.SysNotificationRecipient, 0, len(users))
		for _, user := range users {
			recipients = append(recipients, models.SysNotificationRecipient{
				NotificationID: item.ID,
				UserID:         user.ID,
				CreatedAt:      now,
			})
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&recipients).Error
	})
	return item, err
}

func (r *Repository) notificationRecipientUsers(tx *gorm.DB, targetType string, targetID string) ([]models.SysUser, error) {
	query := tx.Select("id").Where("enabled = ?", true)
	switch strings.ToLower(strings.TrimSpace(targetType)) {
	case "", models.NotificationTargetAll:
	case models.NotificationTargetProject:
		projectID, err := strconv.ParseUint(strings.TrimSpace(targetID), 10, 64)
		if err != nil || projectID == 0 {
			return nil, nil
		}
		var memberCount int64
		if err := tx.Model(&models.SysProjectMember{}).Where("project_id = ?", uint(projectID)).Count(&memberCount).Error; err != nil {
			return nil, err
		}
		if memberCount == 0 {
			break
		}
		query = tx.Table("sys_users").
			Select("sys_users.id").
			Joins("JOIN sys_project_members AS pm ON pm.user_id = sys_users.id").
			Where("sys_users.enabled = ? AND pm.project_id = ? AND pm.notify_enabled = ?", true, uint(projectID), true)
	case models.NotificationTargetUser:
		userID, err := strconv.ParseUint(strings.TrimSpace(targetID), 10, 64)
		if err != nil || userID == 0 {
			return nil, nil
		}
		query = query.Where("id = ?", uint(userID))
	case models.NotificationTargetRole:
		role := strings.ToLower(strings.TrimSpace(targetID))
		if role == "" {
			return nil, nil
		}
		query = query.Where("LOWER(role) = ?", role)
	default:
		return nil, nil
	}
	var users []models.SysUser
	if err := query.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (r *Repository) ListUserNotifications(filter NotificationListFilter) ([]models.UserNotification, int64, error) {
	limit := normalizedNotificationLimit(filter.Limit)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query := r.db.Table("sys_notifications AS n").
		Joins("JOIN sys_notification_recipients AS r ON r.notification_id = n.id").
		Where("r.user_id = ?", filter.UserID).
		Where(unexpiredNotificationCondition("n"), time.Now())
	if filter.Unread != nil {
		if *filter.Unread {
			query = query.Where("r.read_at IS NULL")
		} else {
			query = query.Where("r.read_at IS NOT NULL")
		}
	}
	query = applyNotificationFilters(query, filter, "n")

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.UserNotification
	err := query.Select("n.id, n.event_uid, n.type, n.level, n.target_type, n.target_id, n.project_id, n.project_code, n.task_id, n.test_no, n.var_id, n.var_name, n.display_name, n.message, n.payload, n.occurred_at, n.expires_at, n.created_at, r.read_at").
		Order("n.occurred_at DESC, n.id DESC").
		Limit(limit).
		Offset(offset).
		Scan(&items).Error
	return items, total, err
}

func (r *Repository) CountUnreadNotifications(userID uint) (int64, error) {
	return r.CountUnreadNotificationsWithFilter(NotificationListFilter{UserID: userID})
}

func (r *Repository) CountUnreadNotificationsWithFilter(filter NotificationListFilter) (int64, error) {
	var count int64
	query := r.db.Table("sys_notification_recipients AS r").
		Joins("JOIN sys_notifications AS n ON n.id = r.notification_id").
		Where("r.user_id = ? AND r.read_at IS NULL", filter.UserID).
		Where(unexpiredNotificationCondition("n"), time.Now())
	query = applyNotificationFilters(query, filter, "n")
	err := query.Count(&count).Error
	return count, err
}

func (r *Repository) MarkNotificationRead(userID uint, notificationID uint64) error {
	now := time.Now()
	visibleNotification := r.db.Model(&models.SysNotification{}).
		Select("id").
		Where("id = ?", notificationID).
		Where(unexpiredNotificationCondition("sys_notifications"), now)
	result := r.db.Model(&models.SysNotificationRecipient{}).
		Where("user_id = ? AND notification_id = ?", userID, notificationID).
		Where("notification_id IN (?)", visibleNotification).
		Updates(map[string]any{"read_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) MarkAllNotificationsRead(userID uint) (int64, error) {
	return r.MarkUserNotificationsRead(NotificationListFilter{UserID: userID})
}

func (r *Repository) MarkUserNotificationsRead(filter NotificationListFilter) (int64, error) {
	if filter.Unread != nil && !*filter.Unread {
		return 0, nil
	}
	now := time.Now()
	visibleNotifications := r.db.Model(&models.SysNotification{}).
		Select("id").
		Where(unexpiredNotificationCondition("sys_notifications"), now)
	visibleNotifications = applyNotificationFilters(visibleNotifications, filter, "sys_notifications")
	result := r.db.Model(&models.SysNotificationRecipient{}).
		Where("user_id = ? AND read_at IS NULL", filter.UserID).
		Where("notification_id IN (?)", visibleNotifications).
		Updates(map[string]any{"read_at": now})
	return result.RowsAffected, result.Error
}

func normalizedNotificationLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func unexpiredNotificationCondition(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return "(expires_at IS NULL OR expires_at > ?)"
	}
	return fmt.Sprintf("(%s.expires_at IS NULL OR %s.expires_at > ?)", alias, alias)
}

func applyNotificationFilters(query *gorm.DB, filter NotificationListFilter, alias string) *gorm.DB {
	column := func(name string) string {
		if strings.TrimSpace(alias) == "" {
			return name
		}
		return alias + "." + name
	}
	if filter.Type != "" {
		query = query.Where(column("type")+" = ?", filter.Type)
	}
	if filter.Level != "" {
		query = query.Where(column("level")+" = ?", filter.Level)
	}
	if filter.ProjectID != nil {
		query = query.Where(column("project_id")+" = ?", *filter.ProjectID)
	}
	if filter.From != nil {
		query = query.Where(column("occurred_at")+" >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where(column("occurred_at")+" <= ?", *filter.To)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where(
			column("message")+" LIKE ? OR "+column("var_name")+" LIKE ? OR "+column("display_name")+" LIKE ? OR "+column("project_code")+" LIKE ? OR "+column("test_no")+" LIKE ? OR "+column("event_uid")+" LIKE ?",
			like, like, like, like, like, like,
		)
	}
	return query
}
