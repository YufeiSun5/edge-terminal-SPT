package handlers

import (
	"net/http"
	"strconv"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type DetectionPlansHandler struct {
	service *services.DetectionPlansService
}

type startDetectionPlanRequest struct {
	ProjectID      uint   `json:"project_id" binding:"required"`
	OperatorNote   string `json:"operator_note"`
	RequestVarID   int64  `json:"request_var_id"`
	RequestVarName string `json:"request_var_name"`
}

type cancelDetectionPlanRequest struct {
	Reason string `json:"reason"`
}

func NewDetectionPlansHandler(service *services.DetectionPlansService) *DetectionPlansHandler {
	return &DetectionPlansHandler{service: service}
}

func (h *DetectionPlansHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/detection-plans", authService.RequirePermission(auth.PermViewRealtime), h.list)
	group.GET("/detection-plans/:id", authService.RequirePermission(auth.PermViewRealtime), h.get)
	group.POST("/detection-plans/:id/start", authService.RequirePermission(auth.PermStartDetection), h.start)
	group.POST("/detection-plans/:id/cancel", authService.RequirePermission(auth.PermStopDetection), h.cancel)
}

func (h *DetectionPlansHandler) list(c *gin.Context) {
	filter, err := parseDetectionPlanFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, total, err := h.service.List(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "count": len(items), "total": total, "limit": filter.Limit, "offset": filter.Offset})
}

func (h *DetectionPlansHandler) get(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid plan id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	plan, err := h.service.Get(id)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func (h *DetectionPlansHandler) start(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid plan id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req startDetectionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.Start(services.StartDetectionPlanInput{
		PlanID:         id,
		ProjectID:      req.ProjectID,
		OperatorNote:   req.OperatorNote,
		RequestVarID:   req.RequestVarID,
		RequestVarName: req.RequestVarName,
	})
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), errorBody(err))
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *DetectionPlansHandler) cancel(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid plan id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req cancelDetectionPlanRequest
	_ = c.ShouldBindJSON(&req)
	plan, err := h.service.Cancel(id, req.Reason)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func parseDetectionPlanFilter(c *gin.Context) (database.DetectionPlanFilter, error) {
	filter := database.DetectionPlanFilter{
		Status:    c.Query("status"),
		FactoryNo: c.Query("factory_no"),
		Keyword:   c.Query("keyword"),
		Limit:     100,
	}
	if raw := c.Query("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return filter, strconv.ErrSyntax
		}
		filter.Limit = limit
	}
	if raw := c.Query("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return filter, strconv.ErrSyntax
		}
		filter.Offset = offset
	}
	return filter, nil
}
