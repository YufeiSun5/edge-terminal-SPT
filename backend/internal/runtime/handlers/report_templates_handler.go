package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type ReportTemplatesHandler struct {
	service *services.ReportTemplatesService
}

type reportTemplateCreateRequest struct {
	TemplateCode string `json:"template_code" binding:"required"`
	Name         string `json:"name" binding:"required"`
	DisplayName  string `json:"display_name"`
	FileRef      string `json:"file_ref" binding:"required"`
	FileKind     string `json:"file_kind"`
	Version      int    `json:"version"`
	Enabled      *bool  `json:"enabled"`
	Remark       string `json:"remark"`
}

type reportTemplatePatchRequest struct {
	TemplateCode *string `json:"template_code"`
	Name         *string `json:"name"`
	DisplayName  *string `json:"display_name"`
	FileRef      *string `json:"file_ref"`
	FileKind     *string `json:"file_kind"`
	Version      *int    `json:"version"`
	Enabled      *bool   `json:"enabled"`
	Remark       *string `json:"remark"`
}

func NewReportTemplatesHandler(service *services.ReportTemplatesService) *ReportTemplatesHandler {
	return &ReportTemplatesHandler{service: service}
}

func (h *ReportTemplatesHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/report-templates", authService.RequirePermission(auth.PermViewRealtime), h.list)
	group.POST("/report-templates", authService.RequirePermission(auth.PermManageVariables), h.create)
	group.PATCH("/report-templates/:id", authService.RequirePermission(auth.PermManageVariables), h.patch)
	group.DELETE("/report-templates/:id", authService.RequirePermission(auth.PermManageVariables), h.delete)
}

func (h *ReportTemplatesHandler) list(c *gin.Context) {
	filter, err := parseReportTemplateFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	templates, err := h.service.List(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, templates)
}

func (h *ReportTemplatesHandler) create(c *gin.Context) {
	var req reportTemplateCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	template, err := h.service.Create(services.CreateReportTemplateInput{
		TemplateCode: req.TemplateCode,
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		FileRef:      req.FileRef,
		FileKind:     req.FileKind,
		Version:      req.Version,
		Enabled:      req.Enabled,
		Remark:       req.Remark,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, template)
}

func (h *ReportTemplatesHandler) patch(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid template id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req reportTemplatePatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates, err := reportTemplateUpdates(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	template, err := h.service.Update(id, updates)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, template)
}

func (h *ReportTemplatesHandler) delete(c *gin.Context) {
	id, err := parseUintParam(c, "id", "invalid template id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.Delete(id); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func parseReportTemplateFilter(c *gin.Context) (database.ReportTemplateFilter, error) {
	var filter database.ReportTemplateFilter
	if raw := c.Query("enabled"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, fmt.Errorf("invalid enabled")
		}
		filter.Enabled = &value
	}
	filter.Keyword = c.Query("keyword")
	return filter, nil
}

func reportTemplateUpdates(req reportTemplatePatchRequest) (map[string]interface{}, error) {
	updates := make(map[string]interface{})
	setString(updates, "template_code", req.TemplateCode)
	setString(updates, "name", req.Name)
	setString(updates, "display_name", req.DisplayName)
	setString(updates, "file_ref", req.FileRef)
	setString(updates, "file_kind", req.FileKind)
	if req.Version != nil {
		if *req.Version <= 0 {
			return nil, fmt.Errorf("version must be positive")
		}
		updates["version"] = *req.Version
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	setString(updates, "remark", req.Remark)
	return updates, nil
}

func setString(updates map[string]interface{}, column string, value *string) {
	if value != nil {
		updates[column] = *value
	}
}
