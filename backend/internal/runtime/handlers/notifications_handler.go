package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NotificationsHandler struct {
	repo *database.Repository
}

type userNotificationResponse struct {
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
	VarName     string          `json:"var_name,omitempty"`
	DisplayName string          `json:"display_name,omitempty"`
	Message     string          `json:"message"`
	Payload     json.RawMessage `json:"payload"`
	OccurredAt  string          `json:"occurred_at"`
	CreatedAt   string          `json:"created_at"`
	ReadAt      *string         `json:"read_at,omitempty"`
}

func NewNotificationsHandler(repo *database.Repository) *NotificationsHandler {
	return &NotificationsHandler{repo: repo}
}

func (h *NotificationsHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/notifications", authService.RequirePermission(auth.PermViewRealtime), h.list)
	group.GET("/notifications/unread-count", authService.RequirePermission(auth.PermViewRealtime), h.unreadCount)
	group.POST("/notifications/:id/read", authService.RequirePermission(auth.PermViewRealtime), h.markRead)
	group.POST("/notifications/read-all", authService.RequirePermission(auth.PermViewRealtime), h.markAllRead)
}

func (h *NotificationsHandler) list(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok || principal.AuthType != "user" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user principal is required"})
		return
	}
	filter, err := notificationFilterFromQuery(c, principal.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, total, err := h.repo.ListUserNotifications(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  notificationResponses(items),
		"total":  total,
		"limit":  normalizedNotificationLimitForHandler(filter.Limit),
		"offset": normalizedNotificationOffsetForHandler(filter.Offset),
	})
}

func (h *NotificationsHandler) unreadCount(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok || principal.AuthType != "user" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user principal is required"})
		return
	}
	count, err := h.repo.CountUnreadNotifications(principal.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"unread": count})
}

func (h *NotificationsHandler) markRead(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok || principal.AuthType != "user" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user principal is required"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be positive"})
		return
	}
	if err := h.repo.MarkNotificationRead(principal.UserID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NotificationsHandler) markAllRead(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok || principal.AuthType != "user" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user principal is required"})
		return
	}
	updated, err := h.repo.MarkAllNotificationsRead(principal.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": updated})
}

func notificationFilterFromQuery(c *gin.Context, userID uint) (database.NotificationListFilter, error) {
	limit, err := notificationQueryInt(c, "limit", 50)
	if err != nil {
		return database.NotificationListFilter{}, err
	}
	offset, err := notificationQueryInt(c, "offset", 0)
	if err != nil {
		return database.NotificationListFilter{}, err
	}
	if limit <= 0 {
		return database.NotificationListFilter{}, fmt.Errorf("limit must be positive")
	}
	if offset < 0 {
		return database.NotificationListFilter{}, fmt.Errorf("offset must be non-negative")
	}
	var unread *bool
	if raw := strings.TrimSpace(c.Query("unread")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return database.NotificationListFilter{}, fmt.Errorf("unread must be a boolean")
		}
		unread = &parsed
	}
	var projectID *uint
	if raw := strings.TrimSpace(c.Query("project_id")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || parsed == 0 {
			return database.NotificationListFilter{}, fmt.Errorf("project_id must be positive")
		}
		value := uint(parsed)
		projectID = &value
	}
	return database.NotificationListFilter{
		UserID:    userID,
		Unread:    unread,
		Type:      strings.TrimSpace(c.Query("type")),
		Level:     strings.TrimSpace(c.Query("level")),
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	}, nil
}

func notificationQueryInt(c *gin.Context, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return value, nil
}

func normalizedNotificationLimitForHandler(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func normalizedNotificationOffsetForHandler(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func notificationResponses(items []models.UserNotification) []userNotificationResponse {
	responses := make([]userNotificationResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, notificationResponse(item))
	}
	return responses
}

func notificationResponse(item models.UserNotification) userNotificationResponse {
	payload := json.RawMessage("{}")
	if strings.TrimSpace(item.Payload) != "" && json.Valid([]byte(item.Payload)) {
		payload = json.RawMessage(item.Payload)
	}
	var readAt *string
	if item.ReadAt != nil {
		value := item.ReadAt.Format(time.RFC3339Nano)
		readAt = &value
	}
	return userNotificationResponse{
		ID:          item.ID,
		EventUID:    item.EventUID,
		Type:        item.Type,
		Level:       item.Level,
		TargetType:  item.TargetType,
		TargetID:    item.TargetID,
		ProjectID:   item.ProjectID,
		ProjectCode: item.ProjectCode,
		TaskID:      item.TaskID,
		TestNo:      item.TestNo,
		VarID:       item.VarID,
		VarName:     item.VarName,
		DisplayName: item.DisplayName,
		Message:     item.Message,
		Payload:     payload,
		OccurredAt:  item.OccurredAt.Format(time.RFC3339Nano),
		CreatedAt:   item.CreatedAt.Format(time.RFC3339Nano),
		ReadAt:      readAt,
	}
}
