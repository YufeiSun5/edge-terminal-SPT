package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type StationViewHandler struct {
	repo           *database.Repository
	edgeInstanceID string
}

type stationViewReloadRequest struct {
	ProjectID uint `json:"project_id"`
}

type stationViewTemplatePatchRequest struct {
	Status     *string `json:"status"`
	Version    *int    `json:"version"`
	OwnerScope *string `json:"owner_scope"`
}

type stationViewAssignmentPatchRequest struct {
	TemplateUID *string `json:"template_uid"`
	TargetType  *string `json:"target_type"`
	TargetKey   *string `json:"target_key"`
	Priority    *int    `json:"priority"`
	Enabled     *bool   `json:"enabled"`
}

type stationViewItemsReplaceRequest struct {
	TemplateUID string                        `json:"template_uid"`
	Items       []stationViewItemWriteRequest `json:"items"`
}

type stationViewItemWriteRequest struct {
	ItemUID     string `json:"item_uid"`
	LayoutArea  string `json:"layout_area"`
	ItemType    string `json:"item_type"`
	BindingType string `json:"binding_type"`
	BindingKey  string `json:"binding_key"`
	BindingJSON string `json:"binding_json"`
	DisplayJSON string `json:"display_json"`
	SortOrder   int    `json:"sort_order"`
	Pinned      bool   `json:"pinned"`
	Visible     bool   `json:"visible"`
}

func NewStationViewHandler(repo *database.Repository, edgeInstanceID string) *StationViewHandler {
	return &StationViewHandler{repo: repo, edgeInstanceID: strings.TrimSpace(edgeInstanceID)}
}

func (h *StationViewHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/station-view/effective", authService.RequirePermission(auth.PermViewRealtime), h.effective)
	group.GET("/station-view/templates", authService.RequirePermission(auth.PermViewRealtime), h.templates)
	group.GET("/station-view/items", authService.RequirePermission(auth.PermViewRealtime), h.items)
	group.POST("/station-view/reload", authService.RequirePermission(auth.PermSystemSettings), h.reload)
	group.PUT("/station-view/items", authService.RequirePermission(auth.PermSystemSettings), h.replaceItems)
	group.PATCH("/station-view/templates/:id", authService.RequirePermission(auth.PermSystemSettings), h.patchTemplate)
	group.PATCH("/station-view/assignments/:id", authService.RequirePermission(auth.PermSystemSettings), h.patchAssignment)
}

func (h *StationViewHandler) effective(c *gin.Context) {
	projectID64, err := strconv.ParseUint(c.Query("project_id"), 10, 64)
	if err != nil || projectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required"})
		return
	}
	response, err := h.repo.GetEffectiveStationView(uint(projectID64), h.edgeInstanceID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "station view or project not found"})
		return
	}
	if errors.Is(err, database.ErrStationViewTemplateConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": "station view template assignment conflict", "code": "station_view_template_conflict"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *StationViewHandler) templates(c *gin.Context) {
	items, err := h.repo.ListStationViewTemplates(database.StationViewTemplateFilter{
		Status:     c.Query("status"),
		OwnerScope: c.Query("owner_scope"),
		Keyword:    c.Query("keyword"),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "code": "internal_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "count": len(items), "edge_instance_id": h.edgeInstanceID})
}

func (h *StationViewHandler) items(c *gin.Context) {
	templateUID := strings.TrimSpace(c.Query("template_uid"))
	if templateUID == "" {
		projectID, err := strconv.ParseUint(c.Query("project_id"), 10, 64)
		if err != nil || projectID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "template_uid or project_id is required", "code": "invalid_query"})
			return
		}
		effective, err := h.repo.GetEffectiveStationView(uint(projectID), h.edgeInstanceID)
		if err != nil {
			h.writeStationViewError(c, err)
			return
		}
		templateUID = effective.Template.TemplateUID
	}
	items, err := h.repo.ListStationViewItems(templateUID)
	if err != nil {
		h.writeStationViewError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"template_uid": templateUID, "items": items, "count": len(items), "edge_instance_id": h.edgeInstanceID})
}

func (h *StationViewHandler) reload(c *gin.Context) {
	var req stationViewReloadRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid reload payload", "code": "invalid_payload"})
			return
		}
	}
	response, err := h.repo.ReloadStationView(req.ProjectID, h.edgeInstanceID)
	if err != nil {
		h.writeStationViewError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *StationViewHandler) replaceItems(c *gin.Context) {
	var req stationViewItemsReplaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid station view items payload", "code": "invalid_payload"})
		return
	}
	templateUID := strings.TrimSpace(req.TemplateUID)
	if templateUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template_uid is required", "code": "invalid_template_uid"})
		return
	}
	items := make([]models.StationViewItemDTO, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, models.StationViewItemDTO{
			ItemUID:     item.ItemUID,
			LayoutArea:  item.LayoutArea,
			ItemType:    item.ItemType,
			BindingType: item.BindingType,
			BindingKey:  item.BindingKey,
			BindingJSON: item.BindingJSON,
			DisplayJSON: item.DisplayJSON,
			SortOrder:   item.SortOrder,
			Pinned:      item.Pinned,
			Visible:     item.Visible,
		})
	}
	saved, err := h.repo.ReplaceStationViewItems(templateUID, items)
	if err != nil {
		h.writeStationViewError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"template_uid": templateUID, "items": saved, "count": len(saved), "edge_instance_id": h.edgeInstanceID})
}

func (h *StationViewHandler) patchTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id", "code": "invalid_template_id"})
		return
	}
	var req stationViewTemplatePatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template payload", "code": "invalid_payload"})
		return
	}
	updates, err := stationViewTemplateUpdates(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_payload"})
		return
	}
	template, err := h.repo.UpdateStationViewTemplate(uint(id), updates)
	if err != nil {
		h.writeStationViewError(c, err)
		return
	}
	c.JSON(http.StatusOK, template)
}

func (h *StationViewHandler) patchAssignment(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignment id", "code": "invalid_assignment_id"})
		return
	}
	var req stationViewAssignmentPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignment payload", "code": "invalid_payload"})
		return
	}
	updates, err := stationViewAssignmentUpdates(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_payload"})
		return
	}
	assignment, err := h.repo.UpdateStationViewAssignment(uint(id), updates)
	if err != nil {
		h.writeStationViewError(c, err)
		return
	}
	c.JSON(http.StatusOK, assignment)
}

func (h *StationViewHandler) writeStationViewError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "station view or project not found", "code": "station_view_not_found"})
		return
	}
	if errors.Is(err, database.ErrStationViewTemplateConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": "station view template assignment conflict", "code": "station_view_template_conflict"})
		return
	}
	if strings.Contains(err.Error(), "station view") || strings.Contains(err.Error(), "duplicate station view") || strings.Contains(err.Error(), "invalid station view") {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_payload"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "code": "internal_error"})
}

func stationViewTemplateUpdates(req stationViewTemplatePatchRequest) (map[string]interface{}, error) {
	updates := map[string]interface{}{}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		switch status {
		case "draft", "published", "disabled":
			updates["status"] = status
		default:
			return nil, errors.New("invalid template status")
		}
	}
	if req.Version != nil {
		if *req.Version <= 0 {
			return nil, errors.New("invalid template version")
		}
		updates["version"] = *req.Version
	}
	if req.OwnerScope != nil {
		ownerScope := strings.TrimSpace(*req.OwnerScope)
		if ownerScope == "" {
			return nil, errors.New("invalid owner_scope")
		}
		updates["owner_scope"] = ownerScope
	}
	return updates, nil
}

func stationViewAssignmentUpdates(req stationViewAssignmentPatchRequest) (map[string]interface{}, error) {
	updates := map[string]interface{}{}
	if req.TemplateUID != nil {
		templateUID := strings.TrimSpace(*req.TemplateUID)
		if templateUID == "" {
			return nil, errors.New("invalid template_uid")
		}
		updates["template_uid"] = templateUID
	}
	if req.TargetType != nil {
		targetType := strings.TrimSpace(*req.TargetType)
		switch targetType {
		case "global", "edge", "model", "project":
			updates["target_type"] = targetType
		default:
			return nil, errors.New("invalid target_type")
		}
	}
	if req.TargetKey != nil {
		targetKey := strings.TrimSpace(*req.TargetKey)
		if targetKey == "" {
			return nil, errors.New("invalid target_key")
		}
		updates["target_key"] = targetKey
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	return updates, nil
}
