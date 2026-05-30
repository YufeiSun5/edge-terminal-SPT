package handlers

import (
	"net/http"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type SystemConfigHandler struct {
	service *services.SystemConfigService
}

func NewSystemConfigHandler(service *services.SystemConfigService) *SystemConfigHandler {
	return &SystemConfigHandler{service: service}
}

func (h *SystemConfigHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/system/database-config", authService.RequirePermission(auth.PermSystemSettings), h.getDatabaseConfig)
	group.PATCH("/system/database-config", authService.RequirePermission(auth.PermSystemSettings), h.patchDatabaseConfig)
	group.POST("/system/database-config/test", authService.RequirePermission(auth.PermSystemSettings), h.testDatabaseConfig)
}

func (h *SystemConfigHandler) getDatabaseConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.DatabaseConfig())
}

func (h *SystemConfigHandler) patchDatabaseConfig(c *gin.Context) {
	var req services.DatabaseConfigUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.UpdateDatabaseConfig(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *SystemConfigHandler) testDatabaseConfig(c *gin.Context) {
	var req services.DatabaseConfigUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result := h.service.TestDatabaseConfig(c.Request.Context(), req)
	status := http.StatusOK
	if !result.OK {
		status = http.StatusBadGateway
	}
	c.JSON(status, result)
}
