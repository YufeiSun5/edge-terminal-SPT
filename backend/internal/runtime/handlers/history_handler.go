package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"

	"github.com/gin-gonic/gin"
)

type HistoryHandler struct {
	repo *database.Repository
}

func NewHistoryHandler(repo *database.Repository) *HistoryHandler {
	return &HistoryHandler{repo: repo}
}

func (h *HistoryHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/history/data", authService.RequirePermission(auth.PermViewHistory), h.data)
}

func (h *HistoryHandler) data(c *gin.Context) {
	filter, err := parseHistoryFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rows, err := h.repo.QueryHistoryData(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items": rows,
		"limit": filter.Limit,
		"count": len(rows),
	})
}

func parseHistoryFilter(c *gin.Context) (database.HistoryFilter, error) {
	var filter database.HistoryFilter
	if raw := c.Query("project_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return filter, fmt.Errorf("invalid project_id")
		}
		ProjectID := uint(value)
		filter.ProjectID = &ProjectID
	}
	if raw := c.Query("task_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return filter, fmt.Errorf("invalid task_id")
		}
		taskID := uint(value)
		filter.TaskID = &taskID
	}
	filter.ProjectCode = c.Query("project_code")
	filter.TestNo = c.Query("test_no")
	if raw := c.Query("start"); raw != "" {
		value, err := parseQueryTime(raw)
		if err != nil {
			return filter, fmt.Errorf("invalid start")
		}
		filter.Start = &value
	}
	if raw := c.Query("end"); raw != "" {
		value, err := parseQueryTime(raw)
		if err != nil {
			return filter, fmt.Errorf("invalid end")
		}
		filter.End = &value
	}
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return filter, fmt.Errorf("invalid limit")
		}
		filter.Limit = value
	}
	return filter, nil
}
