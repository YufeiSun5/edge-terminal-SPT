package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type StationViewHandler struct {
	repo           *database.Repository
	edgeInstanceID string
}

func NewStationViewHandler(repo *database.Repository, edgeInstanceID string) *StationViewHandler {
	return &StationViewHandler{repo: repo, edgeInstanceID: strings.TrimSpace(edgeInstanceID)}
}

func (h *StationViewHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/station-view/effective", authService.RequirePermission(auth.PermViewRealtime), h.effective)
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
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}
