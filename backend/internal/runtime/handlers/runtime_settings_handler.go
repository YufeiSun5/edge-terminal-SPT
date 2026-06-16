package handlers

import (
	"net/http"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type RuntimeSettingsHandler struct {
	service *services.RuntimeSettingsService
}

func NewRuntimeSettingsHandler(service *services.RuntimeSettingsService) *RuntimeSettingsHandler {
	return &RuntimeSettingsHandler{service: service}
}

func (h *RuntimeSettingsHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/system/runtime-settings", authService.RequirePermission(auth.PermSystemSettings), h.get)
	group.PATCH("/system/runtime-settings", authService.RequirePermission(auth.PermSystemSettings), h.patch)
}

func (h *RuntimeSettingsHandler) get(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.View())
}

func (h *RuntimeSettingsHandler) patch(c *gin.Context) {
	var req services.RuntimeSettingsUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	view, err := h.service.Update(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}
