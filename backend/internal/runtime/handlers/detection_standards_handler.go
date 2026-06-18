package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type DetectionStandardsHandler struct {
	repo *database.Repository
}

type detectionStandardCreateRequest struct {
	StandardCode     string                         `json:"standard_code" binding:"required"`
	Name             string                         `json:"name"`
	DisplayName      string                         `json:"display_name"`
	DisplayNameEN    string                         `json:"display_name_en"`
	DisplayNameJA    string                         `json:"display_name_ja"`
	ProjectID        *uint                          `json:"project_id"`
	ProjectCode      string                         `json:"project_code"`
	ProjectGroup     string                         `json:"project_group"`
	Mode             string                         `json:"mode"`
	Version          int                            `json:"version"`
	Enabled          *bool                          `json:"enabled"`
	Remark           string                         `json:"remark"`
	ReportTemplateID *uint                          `json:"report_template_id"`
	Items            []detectionStandardItemRequest `json:"items"`
}

type detectionStandardPatchRequest struct {
	StandardCode     *string `json:"standard_code"`
	Name             *string `json:"name"`
	DisplayName      *string `json:"display_name"`
	DisplayNameEN    *string `json:"display_name_en"`
	DisplayNameJA    *string `json:"display_name_ja"`
	ProjectID        *uint   `json:"project_id"`
	ProjectCode      *string `json:"project_code"`
	ProjectGroup     *string `json:"project_group"`
	Mode             *string `json:"mode"`
	Version          *int    `json:"version"`
	Enabled          *bool   `json:"enabled"`
	Remark           *string `json:"remark"`
	ReportTemplateID *uint   `json:"report_template_id"`
}

type detectionStandardItemsReplaceRequest struct {
	Items []detectionStandardItemRequest `json:"items"`
}

type detectionStandardItemRequest struct {
	VarID           flexibleInt64 `json:"var_id" binding:"required"`
	VarName         string        `json:"var_name" binding:"required"`
	DisplayName     string        `json:"display_name"`
	DisplayNameEN   string        `json:"display_name_en"`
	DisplayNameJA   string        `json:"display_name_ja"`
	CheckEnabled    *bool         `json:"check_enabled"`
	AlarmEnabled    *bool         `json:"alarm_enabled"`
	StoreEnabled    *bool         `json:"store_enabled"`
	CheckCycleMS    int           `json:"check_cycle_ms"`
	CheckOnStart    *bool         `json:"check_on_start"`
	Required        bool          `json:"required"`
	CheckMethod     string        `json:"check_method"`
	TargetValue     string        `json:"target_value"`
	LimitLL         *float64      `json:"limit_ll"`
	LimitL          *float64      `json:"limit_l"`
	LimitH          *float64      `json:"limit_h"`
	LimitHH         *float64      `json:"limit_hh"`
	LimitDeadband   float64       `json:"limit_deadband"`
	ViolationHoldMS int           `json:"violation_hold_ms"`
	RecoverHoldMS   int           `json:"recover_hold_ms"`
	QualityPolicy   string        `json:"quality_policy"`
	Unit            string        `json:"unit"`
	DecimalPlaces   int           `json:"decimal_places"`
	SortOrder       int           `json:"sort_order"`
}

func NewDetectionStandardsHandler(repo *database.Repository) *DetectionStandardsHandler {
	return &DetectionStandardsHandler{repo: repo}
}

func (h *DetectionStandardsHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/detection-standards", authService.RequirePermission(auth.PermViewRealtime), h.list)
	group.GET("/detection-standards/favorites", authService.RequirePermission(auth.PermViewRealtime), h.listFavorites)
	group.GET("/detection-standards/recent", authService.RequirePermission(auth.PermViewRealtime), h.listRecent)
	group.POST("/detection-standards/:id/favorite", authService.RequirePermission(auth.PermViewRealtime), h.favorite)
	group.DELETE("/detection-standards/:id/favorite", authService.RequirePermission(auth.PermViewRealtime), h.unfavorite)
	group.GET("/detection-standards/:id", authService.RequirePermission(auth.PermViewRealtime), h.get)
	group.POST("/detection-standards", authService.RequirePermission(auth.PermManageVariables), h.create)
	group.PATCH("/detection-standards/:id", authService.RequirePermission(auth.PermManageVariables), h.patch)
	group.PUT("/detection-standards/:id/items", authService.RequirePermission(auth.PermManageVariables), h.replaceItems)
	group.DELETE("/detection-standards/:id", authService.RequirePermission(auth.PermManageVariables), h.delete)
}

func (h *DetectionStandardsHandler) list(c *gin.Context) {
	filter, err := parseDetectionStandardFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	standards, err := h.repo.ListDetectionStandards(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, standards)
}

func (h *DetectionStandardsHandler) listFavorites(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok || principal.AuthType != "user" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user principal is required"})
		return
	}
	standards, err := h.repo.ListFavoriteDetectionStandards(principal.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": standards, "count": len(standards)})
}

func (h *DetectionStandardsHandler) listRecent(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok || principal.AuthType != "user" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user principal is required"})
		return
	}
	limit, err := parsePositiveLimit(c, 20, 100)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var projectID *uint
	if raw := c.Query("project_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
			return
		}
		parsed := uint(value)
		projectID = &parsed
	}
	standards, err := h.repo.ListRecentDetectionStandards(principal.UserID, projectID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": standards, "count": len(standards), "limit": limit})
}

func (h *DetectionStandardsHandler) favorite(c *gin.Context) {
	h.setFavorite(c, true)
}

func (h *DetectionStandardsHandler) unfavorite(c *gin.Context) {
	h.setFavorite(c, false)
}

func (h *DetectionStandardsHandler) setFavorite(c *gin.Context, favorite bool) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok || principal.AuthType != "user" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user principal is required"})
		return
	}
	standardID, err := parseUintParam(c, "id", "invalid standard id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.repo.SetDetectionStandardFavorite(principal.UserID, standardID, favorite); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "favorite": favorite})
}

func (h *DetectionStandardsHandler) get(c *gin.Context) {
	standardID, err := parseUintParam(c, "id", "invalid standard id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	standard, err := h.repo.GetDetectionStandard(standardID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, standard)
}

func (h *DetectionStandardsHandler) create(c *gin.Context) {
	var req detectionStandardCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	standard, err := detectionStandardFromCreate(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, err := detectionStandardItemsFromRequests(req.Items)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.repo.CreateDetectionStandard(&standard, items); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	standard, err = h.repo.GetDetectionStandard(standard.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, standard)
}

func (h *DetectionStandardsHandler) patch(c *gin.Context) {
	standardID, err := parseUintParam(c, "id", "invalid standard id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req detectionStandardPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates, err := detectionStandardUpdates(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	standard, err := h.repo.UpdateDetectionStandard(standardID, updates)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, standard)
}

func (h *DetectionStandardsHandler) replaceItems(c *gin.Context) {
	standardID, err := parseUintParam(c, "id", "invalid standard id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req detectionStandardItemsReplaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, err := detectionStandardItemsFromRequests(req.Items)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	standard, err := h.repo.ReplaceDetectionStandardItems(standardID, items)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, standard)
}

func (h *DetectionStandardsHandler) delete(c *gin.Context) {
	standardID, err := parseUintParam(c, "id", "invalid standard id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.repo.DeleteDetectionStandard(standardID); err != nil {
		if errors.Is(err, database.ErrReferenced) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func detectionStandardFromCreate(req detectionStandardCreateRequest) (models.DetectionStandard, error) {
	code := strings.TrimSpace(req.StandardCode)
	name := firstNonEmpty(strings.TrimSpace(req.Name), strings.TrimSpace(req.DisplayName))
	if code == "" {
		return models.DetectionStandard{}, fmt.Errorf("standard_code is required")
	}
	if name == "" {
		return models.DetectionStandard{}, fmt.Errorf("name or display_name is required")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	version := req.Version
	if version <= 0 {
		version = 1
	}
	return models.DetectionStandard{
		StandardCode:     code,
		Name:             name,
		DisplayName:      firstNonEmpty(strings.TrimSpace(req.DisplayName), name),
		DisplayNameEN:    req.DisplayNameEN,
		DisplayNameJA:    req.DisplayNameJA,
		ProjectID:        req.ProjectID,
		ProjectCode:      strings.TrimSpace(req.ProjectCode),
		ProjectGroup:     strings.TrimSpace(req.ProjectGroup),
		Mode:             req.Mode,
		Version:          version,
		Enabled:          enabled,
		Remark:           req.Remark,
		ReportTemplateID: req.ReportTemplateID,
	}, nil
}

func detectionStandardUpdates(req detectionStandardPatchRequest) (map[string]interface{}, error) {
	updates := make(map[string]interface{})
	if req.StandardCode != nil {
		code := strings.TrimSpace(*req.StandardCode)
		if code == "" {
			return nil, fmt.Errorf("standard_code cannot be empty")
		}
		updates["standard_code"] = code
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		updates["name"] = name
	}
	setStringUpdate(updates, "display_name", req.DisplayName)
	setStringUpdate(updates, "display_name_en", req.DisplayNameEN)
	setStringUpdate(updates, "display_name_ja", req.DisplayNameJA)
	if req.ProjectID != nil {
		updates["project_id"] = *req.ProjectID
	}
	setStringUpdate(updates, "project_code", req.ProjectCode)
	setStringUpdate(updates, "project_group", req.ProjectGroup)
	setStringUpdate(updates, "mode", req.Mode)
	if req.Version != nil {
		if *req.Version <= 0 {
			return nil, fmt.Errorf("version must be positive")
		}
		updates["version"] = *req.Version
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	setStringUpdate(updates, "remark", req.Remark)
	if req.ReportTemplateID != nil {
		updates["report_template_id"] = *req.ReportTemplateID
	}
	return updates, nil
}

func detectionStandardItemsFromRequests(requests []detectionStandardItemRequest) ([]models.DetectionStandardItem, error) {
	items := make([]models.DetectionStandardItem, 0, len(requests))
	for _, req := range requests {
		checkMethod := firstNonEmpty(strings.TrimSpace(req.CheckMethod), models.CheckMethodNumericRange)
		if !isValidCheckMethod(checkMethod) {
			return nil, fmt.Errorf("invalid check_method")
		}
		qualityPolicy := firstNonEmpty(strings.TrimSpace(req.QualityPolicy), models.QualityPolicyIgnoreBad)
		if !isValidQualityPolicy(qualityPolicy) {
			return nil, fmt.Errorf("invalid quality_policy")
		}
		if req.LimitDeadband < 0 {
			return nil, fmt.Errorf("limit_deadband must be non-negative")
		}
		if err := validateDetectionLimitOrder(req.LimitLL, req.LimitL, req.LimitH, req.LimitHH); err != nil {
			return nil, err
		}
		if req.ViolationHoldMS < 0 || req.RecoverHoldMS < 0 {
			return nil, fmt.Errorf("hold times must be non-negative")
		}
		if req.CheckCycleMS < 0 {
			return nil, fmt.Errorf("check_cycle_ms must be non-negative")
		}
		checkEnabled := true
		if req.CheckEnabled != nil {
			checkEnabled = *req.CheckEnabled
		}
		alarmEnabled := true
		if req.AlarmEnabled != nil {
			alarmEnabled = *req.AlarmEnabled
		}
		storeEnabled := true
		if req.StoreEnabled != nil {
			storeEnabled = *req.StoreEnabled
		}
		checkOnStart := true
		if req.CheckOnStart != nil {
			checkOnStart = *req.CheckOnStart
		}
		decimalPlaces := req.DecimalPlaces
		if decimalPlaces == 0 {
			decimalPlaces = 2
		}
		items = append(items, models.DetectionStandardItem{
			VarID:           req.VarID.Int64(),
			VarName:         req.VarName,
			DisplayName:     req.DisplayName,
			DisplayNameEN:   req.DisplayNameEN,
			DisplayNameJA:   req.DisplayNameJA,
			CheckEnabled:    checkEnabled,
			AlarmEnabled:    alarmEnabled,
			StoreEnabled:    storeEnabled,
			CheckCycleMS:    req.CheckCycleMS,
			CheckOnStart:    checkOnStart,
			Required:        req.Required,
			CheckMethod:     checkMethod,
			TargetValue:     strings.TrimSpace(req.TargetValue),
			LimitLL:         req.LimitLL,
			LimitL:          req.LimitL,
			LimitH:          req.LimitH,
			LimitHH:         req.LimitHH,
			LimitDeadband:   req.LimitDeadband,
			ViolationHoldMS: req.ViolationHoldMS,
			RecoverHoldMS:   req.RecoverHoldMS,
			QualityPolicy:   qualityPolicy,
			Unit:            req.Unit,
			DecimalPlaces:   decimalPlaces,
			SortOrder:       req.SortOrder,
		})
	}
	return items, nil
}

func validateDetectionLimitOrder(ll *float64, l *float64, h *float64, hh *float64) error {
	if ll != nil && l != nil && *ll > *l {
		return fmt.Errorf("limit_ll must be less than or equal to limit_l")
	}
	if l != nil && h != nil && *l > *h {
		return fmt.Errorf("limit_l must be less than or equal to limit_h")
	}
	if h != nil && hh != nil && *h > *hh {
		return fmt.Errorf("limit_h must be less than or equal to limit_hh")
	}
	return nil
}

func isValidCheckMethod(value string) bool {
	switch value {
	case models.CheckMethodNumericRange, models.CheckMethodBoolEquals, models.CheckMethodStringEquals, models.CheckMethodRegex:
		return true
	default:
		return false
	}
}

func isValidQualityPolicy(value string) bool {
	switch value {
	case models.QualityPolicyIgnoreBad, models.QualityPolicyRecordInvalid, models.QualityPolicyFailOnBad:
		return true
	default:
		return false
	}
}

func parseDetectionStandardFilter(c *gin.Context) (database.DetectionStandardFilter, error) {
	var filter database.DetectionStandardFilter
	if raw := c.Query("project_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return filter, fmt.Errorf("invalid project_id")
		}
		ProjectID := uint(value)
		filter.ProjectID = &ProjectID
	}
	filter.ProjectCode = c.Query("project_code")
	filter.ProjectGroup = c.Query("project_group")
	filter.Mode = c.Query("mode")
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
