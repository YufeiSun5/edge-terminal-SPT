package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"

	"github.com/gin-gonic/gin"
)

type AuditLogsHandler struct {
	repo *database.Repository
}

func NewAuditLogsHandler(repo *database.Repository) *AuditLogsHandler {
	return &AuditLogsHandler{repo: repo}
}

func (h *AuditLogsHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/audit-logs", authService.RequirePermission(auth.PermSystemSettings), h.list)
}

func (h *AuditLogsHandler) list(c *gin.Context) {
	filter, err := auditLogFilterFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, total, err := h.repo.ListAuditLogs(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  normalizedAuditLimit(filter.Limit),
		"offset": normalizedAuditOffset(filter.Offset),
	})
}

func auditLogFilterFromQuery(c *gin.Context) (database.AuditLogListFilter, error) {
	limit, err := auditQueryInt(c, "limit", 50)
	if err != nil {
		return database.AuditLogListFilter{}, err
	}
	offset, err := auditQueryInt(c, "offset", 0)
	if err != nil {
		return database.AuditLogListFilter{}, err
	}
	if limit <= 0 {
		return database.AuditLogListFilter{}, fmt.Errorf("limit must be positive")
	}
	if offset < 0 {
		return database.AuditLogListFilter{}, fmt.Errorf("offset must be non-negative")
	}
	from, err := auditQueryTime(firstNonEmpty(c.Query("created_from"), c.Query("from")))
	if err != nil {
		return database.AuditLogListFilter{}, fmt.Errorf("invalid from time")
	}
	to, err := auditQueryTime(firstNonEmpty(c.Query("created_to"), c.Query("to")))
	if err != nil {
		return database.AuditLogListFilter{}, fmt.Errorf("invalid to time")
	}
	return database.AuditLogListFilter{
		ActorType:  strings.TrimSpace(c.Query("actor_type")),
		ActorID:    strings.TrimSpace(c.Query("actor_id")),
		Action:     strings.TrimSpace(c.Query("action")),
		TargetType: strings.TrimSpace(c.Query("target_type")),
		TargetID:   strings.TrimSpace(c.Query("target_id")),
		Result:     strings.TrimSpace(c.Query("result")),
		From:       from,
		To:         to,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func auditQueryInt(c *gin.Context, key string, fallback int) (int, error) {
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

func auditQueryTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("invalid time")
}

func normalizedAuditLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func normalizedAuditOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
