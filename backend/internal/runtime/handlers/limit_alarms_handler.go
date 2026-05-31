package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"

	"github.com/gin-gonic/gin"
)

type LimitAlarmsHandler struct {
	repo *database.Repository
}

func NewLimitAlarmsHandler(repo *database.Repository) *LimitAlarmsHandler {
	return &LimitAlarmsHandler{repo: repo}
}

func (h *LimitAlarmsHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/limit-alarms", authService.RequirePermission(auth.PermViewRealtime), h.list)
}

func (h *LimitAlarmsHandler) list(c *gin.Context) {
	filter, err := limitAlarmFilterFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, total, err := h.repo.ListLimitAlarms(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  limitAlarmLimitForResponse(filter.Limit),
		"offset": limitAlarmOffsetForResponse(filter.Offset),
	})
}

func limitAlarmFilterFromQuery(c *gin.Context) (database.LimitAlarmFilter, error) {
	var filter database.LimitAlarmFilter
	scope := strings.TrimSpace(c.Query("scope"))
	if scope != "" && scope != models.AlarmScopeDefault && scope != models.AlarmScopeDetection {
		return filter, fmt.Errorf("scope must be default or detection")
	}
	filter.Scope = scope
	if projectID, err := parseOptionalUintQuery(c, "project_id"); err != nil {
		return filter, err
	} else {
		filter.ProjectID = projectID
	}
	if taskID, err := parseOptionalUintQuery(c, "task_id"); err != nil {
		return filter, err
	} else {
		filter.TaskID = taskID
	}
	if varID, err := parseOptionalInt64Query(c, "var_id"); err != nil {
		return filter, err
	} else {
		filter.VarID = varID
	}
	filter.TestNo = strings.TrimSpace(c.Query("test_no"))
	status, err := normalizeLimitAlarmStatus(c.Query("status"))
	if err != nil {
		return filter, err
	}
	filter.Status = status
	filter.AlarmType = strings.TrimSpace(c.Query("alarm_type"))
	filter.AlarmLevel = strings.TrimSpace(c.Query("alarm_level"))
	if filter.AlarmLevel == "" {
		filter.AlarmLevel = strings.TrimSpace(c.Query("level"))
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		value, err := parseQueryTime(raw)
		if err != nil {
			return filter, fmt.Errorf("invalid from")
		}
		filter.From = &value
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		value, err := parseQueryTime(raw)
		if err != nil {
			return filter, fmt.Errorf("invalid to")
		}
		filter.To = &value
	}
	limit, err := parseOptionalIntQuery(c, "limit", 100)
	if err != nil {
		return filter, err
	}
	if limit <= 0 {
		return filter, fmt.Errorf("limit must be positive")
	}
	offset, err := parseOptionalIntQuery(c, "offset", 0)
	if err != nil {
		return filter, err
	}
	if offset < 0 {
		return filter, fmt.Errorf("offset must be non-negative")
	}
	filter.Limit = limit
	filter.Offset = offset
	return filter, nil
}

func normalizeLimitAlarmStatus(raw string) (string, error) {
	status := strings.ToLower(strings.TrimSpace(raw))
	switch status {
	case "", models.DetectionAlarmStatusActive, models.DetectionAlarmStatusClosed:
		return status, nil
	case "closed":
		return models.DetectionAlarmStatusClosed, nil
	default:
		return "", fmt.Errorf("status must be active, recovered, or closed")
	}
}

func parseOptionalUintQuery(c *gin.Context, key string) (*uint, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return nil, fmt.Errorf("%s must be positive", key)
	}
	parsed := uint(value)
	return &parsed, nil
}

func parseOptionalInt64Query(c *gin.Context, key string) (*int64, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, fmt.Errorf("%s must be positive", key)
	}
	return &value, nil
}

func parseOptionalIntQuery(c *gin.Context, key string, fallback int) (int, error) {
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

func limitAlarmLimitForResponse(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func limitAlarmOffsetForResponse(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
