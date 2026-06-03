package handlers

import (
	"net/http"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type EdgeRealtimeHandler struct {
	variables *services.VariablesService
}

func NewEdgeRealtimeHandler(variables *services.VariablesService) *EdgeRealtimeHandler {
	return &EdgeRealtimeHandler{variables: variables}
}

func (h *EdgeRealtimeHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	control := group.Group("/edge-control")
	control.GET("/realtime/variables", authService.RequireServiceScope(auth.ScopeServiceRealtimeRead), h.realtimeVariables)
}

func (h *EdgeRealtimeHandler) realtimeVariables(c *gin.Context) {
	filter, err := parseRealtimeVariableFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.variables.Snapshots(filter))
}
