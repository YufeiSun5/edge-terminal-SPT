package handlers

import (
	"net/http"
	"strconv"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type StorageRoutesHandler struct {
	repo *database.Repository
}

type storageRouteRequest struct {
	ProjectID     uint    `json:"project_id"`
	VarID         int64   `json:"var_id"`
	RouteCode     string  `json:"route_code"`
	StorageTarget string  `json:"storage_target"`
	StorageTable  string  `json:"table_name"`
	ColumnName    string  `json:"column_name"`
	ColumnType    string  `json:"column_type"`
	FormFieldKey  string  `json:"form_field_key"`
	QueryAlias    string  `json:"query_alias"`
	TriggerMode   string  `json:"trigger_mode"`
	CycleMS       int     `json:"cycle_ms"`
	Deadband      float64 `json:"deadband"`
	StoreOnStart  *bool   `json:"store_on_start"`
	Enabled       *bool   `json:"enabled"`
}

type storageRoutePatchRequest struct {
	RouteCode     *string  `json:"route_code"`
	StorageTarget *string  `json:"storage_target"`
	StorageTable  *string  `json:"table_name"`
	ColumnName    *string  `json:"column_name"`
	ColumnType    *string  `json:"column_type"`
	FormFieldKey  *string  `json:"form_field_key"`
	QueryAlias    *string  `json:"query_alias"`
	TriggerMode   *string  `json:"trigger_mode"`
	CycleMS       *int     `json:"cycle_ms"`
	Deadband      *float64 `json:"deadband"`
	StoreOnStart  *bool    `json:"store_on_start"`
	Enabled       *bool    `json:"enabled"`
}

func NewStorageRoutesHandler(repo *database.Repository) *StorageRoutesHandler {
	return &StorageRoutesHandler{repo: repo}
}

func (h *StorageRoutesHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/storage-routes", authService.RequirePermission(auth.PermViewRealtime), h.list)
	group.POST("/storage-routes", authService.RequirePermission(auth.PermManageVariables), h.create)
	group.PATCH("/storage-routes/:id", authService.RequirePermission(auth.PermManageVariables), h.patch)
	group.DELETE("/storage-routes/:id", authService.RequirePermission(auth.PermManageVariables), h.delete)
}

func (h *StorageRoutesHandler) list(c *gin.Context) {
	filter := database.StorageRouteFilter{}
	if raw := c.Query("project_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
			return
		}
		ProjectID := uint(value)
		filter.ProjectID = &ProjectID
	}
	if raw := c.Query("var_id"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid var_id"})
			return
		}
		filter.VarID = &value
	}
	if raw := c.Query("enabled"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid enabled"})
			return
		}
		filter.Enabled = &value
	}
	routes, err := h.repo.ListStorageRoutes(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, routes)
}

func (h *StorageRoutesHandler) create(c *gin.Context) {
	var req storageRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	route := models.StorageRoute{
		ProjectID:     req.ProjectID,
		VarID:         req.VarID,
		RouteCode:     req.RouteCode,
		StorageTarget: req.StorageTarget,
		StorageTable:  req.StorageTable,
		ColumnName:    req.ColumnName,
		ColumnType:    req.ColumnType,
		FormFieldKey:  req.FormFieldKey,
		QueryAlias:    req.QueryAlias,
		TriggerMode:   req.TriggerMode,
		CycleMS:       req.CycleMS,
		Deadband:      req.Deadband,
	}
	if req.StoreOnStart != nil {
		route.StoreOnStart = *req.StoreOnStart
	}
	if req.Enabled != nil {
		route.Enabled = *req.Enabled
	}
	if err := h.repo.CreateStorageRoute(&route); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, route)
}

func (h *StorageRoutesHandler) patch(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req storageRoutePatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	route, err := h.repo.UpdateStorageRoute(id, storageRouteUpdates(req))
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, route)
}

func (h *StorageRoutesHandler) delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteStorageRoute(id); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func storageRouteUpdates(req storageRoutePatchRequest) map[string]interface{} {
	updates := make(map[string]interface{})
	setStringUpdate(updates, "route_code", req.RouteCode)
	setStringUpdate(updates, "storage_target", req.StorageTarget)
	setStringUpdate(updates, "table_name", req.StorageTable)
	setStringUpdate(updates, "column_name", req.ColumnName)
	setStringUpdate(updates, "column_type", req.ColumnType)
	setStringUpdate(updates, "form_field_key", req.FormFieldKey)
	setStringUpdate(updates, "query_alias", req.QueryAlias)
	setStringUpdate(updates, "trigger_mode", req.TriggerMode)
	if req.CycleMS != nil {
		updates["cycle_ms"] = *req.CycleMS
	}
	if req.Deadband != nil {
		updates["deadband"] = *req.Deadband
	}
	if req.StoreOnStart != nil {
		updates["store_on_start"] = *req.StoreOnStart
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	return updates
}
