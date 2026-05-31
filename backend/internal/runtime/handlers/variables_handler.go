package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type VariablesHandler struct {
	service *services.VariablesService
}

type createVariableRequest struct {
	VarID                  flexibleInt64 `json:"var_id"`
	SourceType             string        `json:"source_type"`
	GatewayID              int           `json:"gateway_id"`
	SourceTopic            string        `json:"source_topic"`
	SourcePath             string        `json:"source_path"`
	RawName                string        `json:"raw_name"`
	ProjectID              *uint         `json:"project_id"`
	ProjectCode            string        `json:"project_code"`
	VarGroup               string        `json:"var_group"`
	VarName                string        `json:"var_name" binding:"required"`
	DisplayName            string        `json:"display_name"`
	DisplayNameEN          string        `json:"display_name_en"`
	DisplayNameJA          string        `json:"display_name_ja"`
	JSONPath               string        `json:"json_path"`
	DataType               string        `json:"data_type" binding:"required"`
	Unit                   string        `json:"unit"`
	DecimalPlaces          int           `json:"decimal_places"`
	ScaleFactor            float64       `json:"scale_factor"`
	OffsetVal              float64       `json:"offset_val"`
	RWMode                 string        `json:"rw_mode"`
	Writable               bool          `json:"writable"`
	WriteSourceID          int           `json:"write_source_id"`
	WritePath              string        `json:"write_path"`
	WriteDataType          string        `json:"write_data_type"`
	WriteMin               *float64      `json:"write_min"`
	WriteMax               *float64      `json:"write_max"`
	WriteEnum              string        `json:"write_enum"`
	WriteRequiresAudit     *bool         `json:"write_requires_audit"`
	SuspiciousValue        *float64      `json:"suspicious_value"`
	DebounceThreshold      *float64      `json:"debounce_threshold"`
	DebounceMS             int           `json:"debounce_ms"`
	Deadband               float64       `json:"deadband"`
	DefaultAlarmEnabled    *bool         `json:"default_alarm_enabled"`
	DefaultLimitLL         *float64      `json:"default_limit_ll"`
	DefaultLimitL          *float64      `json:"default_limit_l"`
	DefaultLimitH          *float64      `json:"default_limit_h"`
	DefaultLimitHH         *float64      `json:"default_limit_hh"`
	DefaultLimitDeadband   float64       `json:"default_limit_deadband"`
	DefaultViolationHoldMS int           `json:"default_violation_hold_ms"`
	DefaultRecoverHoldMS   int           `json:"default_recover_hold_ms"`
	Enabled                *bool         `json:"enabled"`
}

type assignVariableRequest struct {
	ProjectID   *uint  `json:"project_id"`
	ProjectCode string `json:"project_code"`
	VarGroup    string `json:"var_group"`
	Enabled     bool   `json:"enabled"`
}

type bulkRemapKIOProjectsRequest struct {
	ProjectCount         int    `json:"project_count"`
	ProjectCodePrefix    string `json:"project_code_prefix"`
	ProjectDisplayPrefix string `json:"project_display_prefix"`
	ProjectENPrefix      string `json:"project_en_prefix"`
	ProjectJAPrefix      string `json:"project_ja_prefix"`
	RawProjectPrefix     string `json:"raw_project_prefix"`
	VarGroup             string `json:"var_group"`
	VarNamePrefix        string `json:"var_name_prefix"`
	RemapVarName         *bool  `json:"remap_var_name"`
	Enable               *bool  `json:"enable"`
	DryRun               bool   `json:"dry_run"`
}

type variablePatchRequest struct {
	VarName                *string  `json:"var_name"`
	DisplayName            *string  `json:"display_name"`
	DisplayNameEN          *string  `json:"display_name_en"`
	DisplayNameJA          *string  `json:"display_name_ja"`
	DataType               *string  `json:"data_type"`
	Unit                   *string  `json:"unit"`
	DecimalPlaces          *int     `json:"decimal_places"`
	ScaleFactor            *float64 `json:"scale_factor"`
	OffsetVal              *float64 `json:"offset_val"`
	RWMode                 *string  `json:"rw_mode"`
	Writable               *bool    `json:"writable"`
	WriteSourceID          *int     `json:"write_source_id"`
	WritePath              *string  `json:"write_path"`
	WriteDataType          *string  `json:"write_data_type"`
	WriteMin               *float64 `json:"write_min"`
	WriteMax               *float64 `json:"write_max"`
	WriteEnum              *string  `json:"write_enum"`
	WriteRequiresAudit     *bool    `json:"write_requires_audit"`
	SuspiciousValue        *float64 `json:"suspicious_value"`
	DebounceThreshold      *float64 `json:"debounce_threshold"`
	DebounceMS             *int     `json:"debounce_ms"`
	Deadband               *float64 `json:"deadband"`
	DefaultAlarmEnabled    *bool    `json:"default_alarm_enabled"`
	DefaultLimitLL         *float64 `json:"default_limit_ll"`
	DefaultLimitL          *float64 `json:"default_limit_l"`
	DefaultLimitH          *float64 `json:"default_limit_h"`
	DefaultLimitHH         *float64 `json:"default_limit_hh"`
	DefaultLimitDeadband   *float64 `json:"default_limit_deadband"`
	DefaultViolationHoldMS *int     `json:"default_violation_hold_ms"`
	DefaultRecoverHoldMS   *int     `json:"default_recover_hold_ms"`
	ApplyToRunning         bool     `json:"apply_to_running"`
	VarGroup               *string  `json:"var_group"`
	Enabled                *bool    `json:"enabled"`
}

func NewVariablesHandler(service *services.VariablesService) *VariablesHandler {
	return &VariablesHandler{service: service}
}

func (h *VariablesHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/realtime/variables", authService.RequirePermission(auth.PermViewRealtime), h.realtime)
	group.GET("/variables", authService.RequirePermission(auth.PermViewRealtime), h.list)
	group.POST("/variables", authService.RequirePermission(auth.PermManageVariables), h.create)
	group.POST("/variables/bulk-remap/kio-projects", authService.RequirePermission(auth.PermManageVariables), h.bulkRemapKIOProjects)
	group.PATCH("/variables/:variable_id/assignment", authService.RequirePermission(auth.PermManageVariables), h.assign)
	group.PATCH("/variables/:variable_id", authService.RequirePermission(auth.PermManageVariables), h.patch)
	group.DELETE("/variables/:variable_id", authService.RequirePermission(auth.PermManageVariables), h.delete)
}

func (h *VariablesHandler) realtime(c *gin.Context) {
	filter, err := parseRealtimeVariableFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.service.Snapshots(filter))
}

func (h *VariablesHandler) list(c *gin.Context) {
	filter, err := parseTagFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tags, err := h.service.List(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tags)
}

func (h *VariablesHandler) create(c *gin.Context) {
	var req createVariableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tag, err := h.service.Create(services.CreateVariableInput{
		VarID:                  req.VarID.Int64(),
		SourceType:             req.SourceType,
		GatewayID:              req.GatewayID,
		SourceTopic:            req.SourceTopic,
		SourcePath:             req.SourcePath,
		RawName:                req.RawName,
		ProjectID:              req.ProjectID,
		ProjectCode:            req.ProjectCode,
		VarGroup:               req.VarGroup,
		VarName:                req.VarName,
		DisplayName:            req.DisplayName,
		DisplayNameEN:          req.DisplayNameEN,
		DisplayNameJA:          req.DisplayNameJA,
		JSONPath:               req.JSONPath,
		DataType:               req.DataType,
		Unit:                   req.Unit,
		DecimalPlaces:          req.DecimalPlaces,
		ScaleFactor:            req.ScaleFactor,
		OffsetVal:              req.OffsetVal,
		RWMode:                 req.RWMode,
		Writable:               req.Writable,
		WriteSourceID:          req.WriteSourceID,
		WritePath:              req.WritePath,
		WriteDataType:          req.WriteDataType,
		WriteMin:               req.WriteMin,
		WriteMax:               req.WriteMax,
		WriteEnum:              req.WriteEnum,
		WriteRequiresAudit:     req.WriteRequiresAudit,
		SuspiciousValue:        req.SuspiciousValue,
		DebounceThreshold:      req.DebounceThreshold,
		DebounceMS:             req.DebounceMS,
		Deadband:               req.Deadband,
		DefaultAlarmEnabled:    req.DefaultAlarmEnabled,
		DefaultLimitLL:         req.DefaultLimitLL,
		DefaultLimitL:          req.DefaultLimitL,
		DefaultLimitH:          req.DefaultLimitH,
		DefaultLimitHH:         req.DefaultLimitHH,
		DefaultLimitDeadband:   req.DefaultLimitDeadband,
		DefaultViolationHoldMS: req.DefaultViolationHoldMS,
		DefaultRecoverHoldMS:   req.DefaultRecoverHoldMS,
		Enabled:                req.Enabled,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tag)
}

func (h *VariablesHandler) assign(c *gin.Context) {
	variableID, err := strconv.ParseInt(c.Param("variable_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid variable_id"})
		return
	}
	var req assignVariableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.Assign(variableID, req.ProjectID, req.ProjectCode, req.VarGroup, req.Enabled); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *VariablesHandler) bulkRemapKIOProjects(c *gin.Context) {
	var req bulkRemapKIOProjectsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.BulkRemapKIOProjects(services.BulkRemapKIOProjectsInput{
		ProjectCount:         req.ProjectCount,
		ProjectCodePrefix:    req.ProjectCodePrefix,
		ProjectDisplayPrefix: req.ProjectDisplayPrefix,
		ProjectENPrefix:      req.ProjectENPrefix,
		ProjectJAPrefix:      req.ProjectJAPrefix,
		RawProjectPrefix:     req.RawProjectPrefix,
		VarGroup:             req.VarGroup,
		VarNamePrefix:        req.VarNamePrefix,
		RemapVarName:         req.RemapVarName,
		Enable:               req.Enable,
		DryRun:               req.DryRun,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *VariablesHandler) patch(c *gin.Context) {
	variableID, err := strconv.ParseInt(c.Param("variable_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid variable_id"})
		return
	}
	var req variablePatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tag, err := h.service.Update(variableID, variableUpdates(req), services.UpdateVariableOptions{ApplyToRunning: req.ApplyToRunning})
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tag)
}

func (h *VariablesHandler) delete(c *gin.Context) {
	variableID, err := strconv.ParseInt(c.Param("variable_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid variable_id"})
		return
	}
	if err := h.service.Delete(variableID); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func variableUpdates(req variablePatchRequest) map[string]interface{} {
	updates := make(map[string]interface{})
	setStringUpdate(updates, "var_name", req.VarName)
	setStringUpdate(updates, "display_name", req.DisplayName)
	setStringUpdate(updates, "display_name_en", req.DisplayNameEN)
	setStringUpdate(updates, "display_name_ja", req.DisplayNameJA)
	setStringUpdate(updates, "data_type", req.DataType)
	setStringUpdate(updates, "unit", req.Unit)
	setStringUpdate(updates, "var_group", req.VarGroup)
	if req.DecimalPlaces != nil {
		updates["decimal_places"] = *req.DecimalPlaces
	}
	if req.ScaleFactor != nil {
		updates["scale_factor"] = *req.ScaleFactor
	}
	if req.OffsetVal != nil {
		updates["offset_val"] = *req.OffsetVal
	}
	setStringUpdate(updates, "rw_mode", req.RWMode)
	if req.Writable != nil {
		updates["writable"] = *req.Writable
	}
	if req.WriteSourceID != nil {
		updates["write_source_id"] = *req.WriteSourceID
	}
	setStringUpdate(updates, "write_path", req.WritePath)
	setStringUpdate(updates, "write_data_type", req.WriteDataType)
	if req.WriteMin != nil {
		updates["write_min"] = req.WriteMin
	}
	if req.WriteMax != nil {
		updates["write_max"] = req.WriteMax
	}
	setStringUpdate(updates, "write_enum", req.WriteEnum)
	if req.WriteRequiresAudit != nil {
		updates["write_requires_audit"] = *req.WriteRequiresAudit
	}
	if req.SuspiciousValue != nil {
		updates["suspicious_value"] = req.SuspiciousValue
	}
	if req.DebounceThreshold != nil {
		updates["debounce_threshold"] = req.DebounceThreshold
	}
	if req.DebounceMS != nil {
		updates["debounce_ms"] = *req.DebounceMS
	}
	if req.Deadband != nil {
		updates["deadband"] = *req.Deadband
	}
	if req.DefaultAlarmEnabled != nil {
		updates["default_alarm_enabled"] = *req.DefaultAlarmEnabled
	}
	if req.DefaultLimitLL != nil {
		updates["default_limit_ll"] = req.DefaultLimitLL
	}
	if req.DefaultLimitL != nil {
		updates["default_limit_l"] = req.DefaultLimitL
	}
	if req.DefaultLimitH != nil {
		updates["default_limit_h"] = req.DefaultLimitH
	}
	if req.DefaultLimitHH != nil {
		updates["default_limit_hh"] = req.DefaultLimitHH
	}
	if req.DefaultLimitDeadband != nil {
		updates["default_limit_deadband"] = *req.DefaultLimitDeadband
	}
	if req.DefaultViolationHoldMS != nil {
		updates["default_violation_hold_ms"] = *req.DefaultViolationHoldMS
	}
	if req.DefaultRecoverHoldMS != nil {
		updates["default_recover_hold_ms"] = *req.DefaultRecoverHoldMS
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	return updates
}

func parseTagFilter(c *gin.Context) (database.TagFilter, error) {
	var filter database.TagFilter
	if raw := c.Query("gateway_id"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filter, fmt.Errorf("invalid gateway_id")
		}
		filter.GatewayID = &value
	}
	if raw := c.Query("enabled"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, fmt.Errorf("invalid enabled")
		}
		filter.Enabled = &value
	}
	if raw := c.Query("discovered"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, fmt.Errorf("invalid discovered")
		}
		filter.Discovered = &value
	}
	filter.SourceType = c.Query("source_type")
	filter.Keyword = c.Query("keyword")
	return filter, nil
}

func parseRealtimeVariableFilter(c *gin.Context) (services.RealtimeVariableFilter, error) {
	var filter services.RealtimeVariableFilter
	if raw := c.Query("gateway_id"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filter, fmt.Errorf("invalid gateway_id")
		}
		filter.GatewayID = &value
	}
	if raw := c.Query("project_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return filter, fmt.Errorf("invalid project_id")
		}
		projectID := uint(value)
		filter.ProjectID = &projectID
	}
	if raw := c.Query("device_id"); raw != "" && filter.ProjectID == nil {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return filter, fmt.Errorf("invalid device_id")
		}
		projectID := uint(value)
		filter.ProjectID = &projectID
	}
	filter.SourceType = strings.TrimSpace(c.Query("source_type"))
	varIDs, err := parseVarIDQueryValues(c.QueryArray("var_id"))
	if err != nil {
		return filter, err
	}
	filter.VarIDs = varIDs
	return filter, nil
}

func parseVarIDQueryValues(rawValues []string) (map[int64]bool, error) {
	if len(rawValues) == 0 {
		return nil, nil
	}
	values := make(map[int64]bool, len(rawValues))
	for _, rawValue := range rawValues {
		for _, raw := range strings.Split(rawValue, ",") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid var_id")
			}
			values[parsed] = true
		}
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values, nil
}
