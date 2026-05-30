package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type DetectionRunsHandler struct {
	service *services.DetectionRunsService
}

type startDetectionRequest struct {
	ProjectID        uint   `json:"project_id" binding:"required"`
	TestNo           string `json:"test_no" binding:"required"`
	Mode             string `json:"mode" binding:"required"`
	StandardID       *uint  `json:"standard_id"`
	DurationSec      int    `json:"duration_sec"`
	OperatorNote     string `json:"operator_note"`
	ReportTemplateID *uint  `json:"report_template_id"`
}

type stopDetectionRequest struct {
	Reason string `json:"reason"`
}

type addNoteRequest struct {
	NoteType string `json:"note_type"`
	Content  string `json:"content" binding:"required"`
}

func NewDetectionRunsHandler(service *services.DetectionRunsService) *DetectionRunsHandler {
	return &DetectionRunsHandler{service: service}
}

func (h *DetectionRunsHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/detection-runs/active", authService.RequirePermission(auth.PermViewRealtime), h.active)
	group.GET("/detection-runs/current", authService.RequirePermission(auth.PermViewRealtime), h.current)
	group.GET("/detection-runs", authService.RequirePermission(auth.PermViewRealtime), h.list)
	group.GET("/detection-runs/:id", authService.RequirePermission(auth.PermViewRealtime), h.get)
	group.GET("/detection-runs/:id/summary", authService.RequirePermission(auth.PermViewRealtime), h.summary)
	group.GET("/detection-runs/:id/features", authService.RequirePermission(auth.PermViewRealtime), h.features)
	group.GET("/detection-runs/:id/events", authService.RequirePermission(auth.PermViewRealtime), h.listEvents)
	group.GET("/detection-runs/:id/storage-routes", authService.RequirePermission(auth.PermViewRealtime), h.storageRoutes)
	group.POST("/detection-runs", authService.RequirePermission(auth.PermStartDetection), h.start)
	group.POST("/detection-runs/:id/stop", authService.RequirePermission(auth.PermStopDetection), h.stop)
	group.POST("/detection-runs/:id/abnormal-stop", authService.RequirePermission(auth.PermStopDetection), h.abnormalStop)
	group.POST("/detection-runs/:id/pause", authService.RequirePermission(auth.PermStopDetection), h.pause)
	group.POST("/detection-runs/:id/resume", authService.RequirePermission(auth.PermStartDetection), h.resume)
	group.GET("/detection-runs/:id/notes", authService.RequirePermission(auth.PermViewRealtime), h.listNotes)
	group.POST("/detection-runs/:id/notes", authService.RequirePermission(auth.PermStopDetection), h.addNote)
}

func (h *DetectionRunsHandler) active(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Active())
}

func (h *DetectionRunsHandler) current(c *gin.Context) {
	raw := c.Query("project_id")
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required"})
		return
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
		return
	}
	task, err := h.service.Current(uint(value))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (h *DetectionRunsHandler) list(c *gin.Context) {
	filter, err := parseDetectionTaskFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tasks, err := h.service.List(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": tasks, "count": len(tasks), "limit": filter.Limit})
}

func (h *DetectionRunsHandler) get(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	task, err := h.service.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (h *DetectionRunsHandler) summary(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	summary, err := h.service.Summary(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (h *DetectionRunsHandler) features(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	features, err := h.service.Features(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": features, "count": len(features)})
}

func (h *DetectionRunsHandler) listEvents(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := parsePositiveLimit(c, 200, 1000)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	events, err := h.service.ListEvents(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": events, "count": len(events), "limit": limit})
}

func (h *DetectionRunsHandler) storageRoutes(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	routes, err := h.service.StorageRoutes(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": routes, "count": len(routes)})
}

func (h *DetectionRunsHandler) start(c *gin.Context) {
	var req startDetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	task, err := h.service.Start(database.StartDetectionOptions{
		ProjectID:        req.ProjectID,
		TestNo:           req.TestNo,
		Mode:             req.Mode,
		StandardID:       req.StandardID,
		DurationSec:      req.DurationSec,
		OperatorNote:     req.OperatorNote,
		ReportTemplateID: req.ReportTemplateID,
	})
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (h *DetectionRunsHandler) stop(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req stopDetectionRequest
	_ = c.ShouldBindJSON(&req)
	task, err := h.service.Stop(id, req.Reason)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (h *DetectionRunsHandler) abnormalStop(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req stopDetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	task, err := h.service.AbnormalStop(id, req.Reason)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (h *DetectionRunsHandler) pause(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req stopDetectionRequest
	_ = c.ShouldBindJSON(&req)
	task, err := h.service.Pause(id, req.Reason)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (h *DetectionRunsHandler) resume(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	task, err := h.service.Resume(id)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (h *DetectionRunsHandler) listNotes(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := parsePositiveLimit(c, 200, 0)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	notes, err := h.service.ListNotes(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": notes, "count": len(notes), "limit": limit})
}

func (h *DetectionRunsHandler) addNote(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid task id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req addNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	actorType := "user"
	actorID := ""
	if principal, ok := auth.PrincipalFromContext(c); ok {
		actorID = strconv.FormatUint(uint64(principal.UserID), 10)
	}
	note, err := h.service.AddNote(services.AddNoteInput{
		TaskID:    id,
		NoteType:  req.NoteType,
		Content:   req.Content,
		ActorType: actorType,
		ActorID:   actorID,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, note)
}

func parseDetectionTaskFilter(c *gin.Context) (database.DetectionTaskFilter, error) {
	var filter database.DetectionTaskFilter
	if raw := c.Query("project_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return filter, fmt.Errorf("invalid project_id")
		}
		ProjectID := uint(value)
		filter.ProjectID = &ProjectID
	}
	filter.Status = c.Query("status")
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

func parseUintParam(c *gin.Context, name string, message string) (uint, error) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("%s", message)
	}
	return uint(value), nil
}

func parsePositiveLimit(c *gin.Context, fallback int, max int) (int, error) {
	limit := fallback
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("invalid limit")
		}
		limit = value
	}
	if max > 0 && limit > max {
		limit = max
	}
	return limit, nil
}

func parseQueryTime(raw string) (time.Time, error) {
	if value, err := time.Parse(time.RFC3339, raw); err == nil {
		return value, nil
	}
	if value, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local); err == nil {
		return value, nil
	}
	return time.Time{}, fmt.Errorf("invalid time")
}
