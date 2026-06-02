package handlers

import (
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

type ProjectsHandler struct {
	repo *database.Repository
}

type createProjectRequest struct {
	ProjectCode    string `json:"project_code" binding:"required"`
	SiteNo         string `json:"site_no"`
	EdgeInstanceID string `json:"edge_instance_id"`
	Name           string `json:"name"`
	DisplayName    string `json:"display_name"`
	DisplayNameEN  string `json:"display_name_en"`
	DisplayNameJA  string `json:"display_name_ja"`
	ModelName      string `json:"model_name"`
	ImageRef       string `json:"image_ref"`
	Placeholder    bool   `json:"placeholder"`
}

type ProjectPatchRequest struct {
	SiteNo         *string `json:"site_no"`
	EdgeInstanceID *string `json:"edge_instance_id"`
	Name           *string `json:"name"`
	DisplayName    *string `json:"display_name"`
	DisplayNameEN  *string `json:"display_name_en"`
	DisplayNameJA  *string `json:"display_name_ja"`
	ModelName      *string `json:"model_name"`
	ImageRef       *string `json:"image_ref"`
	Enabled        *bool   `json:"enabled"`
	Blocked        *bool   `json:"blocked"`
	Placeholder    *bool   `json:"placeholder"`
}

type projectMembersReplaceRequest struct {
	Members []projectMemberRequest `json:"members"`
}

type projectMemberRequest struct {
	UserID        uint   `json:"user_id" binding:"required"`
	MemberRole    string `json:"member_role"`
	NotifyEnabled *bool  `json:"notify_enabled"`
}

func NewProjectsHandler(repo *database.Repository) *ProjectsHandler {
	return &ProjectsHandler{repo: repo}
}

func (h *ProjectsHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/projects", authService.RequirePermission(auth.PermViewRealtime), h.list)
	group.POST("/projects", authService.RequirePermission(auth.PermManageVariables), h.create)
	group.PATCH("/projects/:id", authService.RequirePermission(auth.PermManageVariables), h.patch)
	group.GET("/projects/:id/members", authService.RequirePermission(auth.PermManageUsers), h.listMembers)
	group.PUT("/projects/:id/members", authService.RequirePermission(auth.PermManageUsers), h.replaceMembers)
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
		ProjectCode:    strings.TrimSpace(req.ProjectCode),
		SiteNo:         req.SiteNo,
		EdgeInstanceID: strings.TrimSpace(req.EdgeInstanceID),
		Name:           firstNonEmpty(strings.TrimSpace(req.Name), strings.TrimSpace(req.DisplayName)),
		DisplayName:    firstNonEmpty(strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Name)),
		DisplayNameEN:  req.DisplayNameEN,
		DisplayNameJA:  req.DisplayNameJA,
		ModelName:      req.ModelName,
		ImageRef:       req.ImageRef,
		Enabled:        true,
		Blocked:        false,
		Placeholder:    req.Placeholder,
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
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, Project)
}

func (h *ProjectsHandler) listMembers(c *gin.Context) {
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if _, err := h.repo.GetProject(projectID); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	members, err := h.repo.ListProjectMembers(projectID)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": members, "count": len(members)})
}

func (h *ProjectsHandler) replaceMembers(c *gin.Context) {
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	var req projectMembersReplaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	members := make([]models.SysProjectMember, 0, len(req.Members))
	for _, item := range req.Members {
		if item.UserID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
			return
		}
		notifyEnabled := true
		if item.NotifyEnabled != nil {
			notifyEnabled = *item.NotifyEnabled
		}
		members = append(members, models.SysProjectMember{
			ProjectID:     projectID,
			UserID:        item.UserID,
			MemberRole:    strings.TrimSpace(item.MemberRole),
			NotifyEnabled: notifyEnabled,
		})
	}
	result, err := h.repo.ReplaceProjectMembers(projectID, members)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": result, "count": len(result)})
}

func ProjectUpdates(req ProjectPatchRequest) (map[string]interface{}, error) {
	updates := make(map[string]interface{})
	setStringUpdate(updates, "site_no", req.SiteNo)
	setStringUpdate(updates, "edge_instance_id", req.EdgeInstanceID)
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

func parseProjectIDParam(c *gin.Context) (uint, bool) {
	projectID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || projectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return 0, false
	}
	return uint(projectID64), true
}
