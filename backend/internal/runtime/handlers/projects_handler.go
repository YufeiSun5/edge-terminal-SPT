package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"

	"github.com/gin-gonic/gin"
)

type ProjectsHandler struct {
	repo *database.Repository
}

type createProjectRequest struct {
	ProjectCode   string `json:"project_code" binding:"required"`
	SiteNo        string `json:"site_no"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	DisplayNameEN string `json:"display_name_en"`
	DisplayNameJA string `json:"display_name_ja"`
	ModelName     string `json:"model_name"`
	ImageRef      string `json:"image_ref"`
	Placeholder   bool   `json:"placeholder"`
}

type ProjectPatchRequest struct {
	SiteNo        *string `json:"site_no"`
	Name          *string `json:"name"`
	DisplayName   *string `json:"display_name"`
	DisplayNameEN *string `json:"display_name_en"`
	DisplayNameJA *string `json:"display_name_ja"`
	ModelName     *string `json:"model_name"`
	ImageRef      *string `json:"image_ref"`
	Enabled       *bool   `json:"enabled"`
	Blocked       *bool   `json:"blocked"`
	Placeholder   *bool   `json:"placeholder"`
}

func NewProjectsHandler(repo *database.Repository) *ProjectsHandler {
	return &ProjectsHandler{repo: repo}
}

func (h *ProjectsHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/projects", authService.RequirePermission(auth.PermViewRealtime), h.list)
	group.POST("/projects", authService.RequirePermission(auth.PermManageVariables), h.create)
	group.PATCH("/projects/:id", authService.RequirePermission(auth.PermManageVariables), h.patch)
}

func (h *ProjectsHandler) list(c *gin.Context) {
	Projects, err := h.repo.ListProjects()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, Projects)
}

func (h *ProjectsHandler) create(c *gin.Context) {
	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	Project := &models.Project{
		ProjectCode:   strings.TrimSpace(req.ProjectCode),
		SiteNo:        req.SiteNo,
		Name:          firstNonEmpty(strings.TrimSpace(req.Name), strings.TrimSpace(req.DisplayName)),
		DisplayName:   firstNonEmpty(strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Name)),
		DisplayNameEN: req.DisplayNameEN,
		DisplayNameJA: req.DisplayNameJA,
		ModelName:     req.ModelName,
		ImageRef:      req.ImageRef,
		Enabled:       true,
		Blocked:       false,
		Placeholder:   req.Placeholder,
	}
	if Project.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name or display_name is required"})
		return
	}
	if err := h.repo.CreateProject(Project); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, Project)
}

func (h *ProjectsHandler) patch(c *gin.Context) {
	ProjectID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || ProjectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	var req ProjectPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates, err := ProjectUpdates(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	Project, err := h.repo.UpdateProject(uint(ProjectID64), updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, Project)
}

func ProjectUpdates(req ProjectPatchRequest) (map[string]interface{}, error) {
	updates := make(map[string]interface{})
	setStringUpdate(updates, "site_no", req.SiteNo)
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
	setStringUpdate(updates, "model_name", req.ModelName)
	setStringUpdate(updates, "image_ref", req.ImageRef)
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Blocked != nil {
		updates["blocked"] = *req.Blocked
	}
	if req.Placeholder != nil {
		updates["placeholder"] = *req.Placeholder
	}
	return updates, nil
}
