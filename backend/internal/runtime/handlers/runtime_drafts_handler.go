package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type RuntimeDraftsHandler struct {
	service *services.RuntimeDraftService
}

type runtimeDraftPutRequest struct {
	ScopeType        string `json:"scope_type"`
	ScopeID          string `json:"scope_id"`
	ExpectedRevision *int64 `json:"expected_revision"`
	TTLSec           int    `json:"ttl_sec"`
	Data             any    `json:"data"`
}

func NewRuntimeDraftsHandler(service *services.RuntimeDraftService) *RuntimeDraftsHandler {
	return &RuntimeDraftsHandler{service: service}
}

func (h *RuntimeDraftsHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/runtime-drafts/:namespace", authService.RequirePermission(auth.PermViewRealtime), h.get)
	group.PUT("/runtime-drafts/:namespace", authService.RequirePermission(auth.PermStartDetection), h.put)
	group.DELETE("/runtime-drafts/:namespace", authService.RequirePermission(auth.PermStartDetection), h.clear)
}

func (h *RuntimeDraftsHandler) RegisterServiceRoutes(group *gin.RouterGroup, authService *auth.Service) {
	control := group.Group("/edge-control")
	control.GET("/runtime-drafts/:namespace", authService.RequireServiceScope(auth.ScopeServiceRuntimeRead), h.serviceGet)
}

func (h *RuntimeDraftsHandler) get(c *gin.Context) {
	draft, err := h.service.Get(c.Param("namespace"), c.Query("scope_type"), c.Query("scope_id"))
	if err != nil {
		writeRuntimeDraftError(c, err)
		return
	}
	c.JSON(http.StatusOK, draft)
}

func (h *RuntimeDraftsHandler) put(c *gin.Context) {
	var req runtimeDraftPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_payload"})
		return
	}
	draft, err := h.service.Put(services.RuntimeDraftPutInput{
		Namespace:        c.Param("namespace"),
		ScopeType:        req.ScopeType,
		ScopeID:          req.ScopeID,
		ExpectedRevision: req.ExpectedRevision,
		TTLSec:           req.TTLSec,
		Data:             req.Data,
	})
	if err != nil {
		writeRuntimeDraftError(c, err)
		return
	}
	c.JSON(http.StatusOK, draft)
}

func (h *RuntimeDraftsHandler) clear(c *gin.Context) {
	expectedRevision, ok := optionalInt64Query(c, "expected_revision")
	if !ok {
		return
	}
	err := h.service.Clear(services.RuntimeDraftClearInput{
		Namespace:        c.Param("namespace"),
		ScopeType:        c.Query("scope_type"),
		ScopeID:          c.Query("scope_id"),
		ExpectedRevision: expectedRevision,
	})
	if err != nil {
		writeRuntimeDraftError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *RuntimeDraftsHandler) serviceGet(c *gin.Context) {
	draft, err := h.service.Get(c.Param("namespace"), c.Query("scope_type"), c.Query("scope_id"))
	if err != nil {
		writeRuntimeDraftError(c, err)
		return
	}
	c.JSON(http.StatusOK, draft)
}

func optionalInt64Query(c *gin.Context, key string) (*int64, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + key, "code": "invalid_query"})
		return nil, false
	}
	return &value, true
}

func writeRuntimeDraftError(c *gin.Context, err error) {
	code := "invalid_runtime_draft"
	switch {
	case errors.Is(err, services.ErrRuntimeDraftNotFound):
		code = "not_found"
	case errors.Is(err, services.ErrRuntimeDraftRevisionConflict):
		code = "revision_conflict"
	case errors.Is(err, services.ErrRuntimeDraftTooLarge):
		code = "payload_too_large"
	case errors.Is(err, services.ErrRuntimeDraftProjectRunning):
		code = "project_running"
	}
	c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error(), "code": code})
}
