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

type UsersHandler struct {
	repo *database.Repository
}

type userCreateRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role"`
	Enabled  *bool  `json:"enabled"`
}

type userPatchRequest struct {
	Username *string `json:"username"`
	Role     *string `json:"role"`
	Enabled  *bool   `json:"enabled"`
}

type userResetPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

func NewUsersHandler(repo *database.Repository) *UsersHandler {
	return &UsersHandler{repo: repo}
}

func (h *UsersHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/users", authService.RequirePermission(auth.PermManageUsers), h.list)
	group.POST("/users", authService.RequirePermission(auth.PermManageUsers), h.create)
	group.PATCH("/users/:id", authService.RequirePermission(auth.PermManageUsers), h.patch)
	group.POST("/users/:id/reset-password", authService.RequirePermission(auth.PermManageUsers), h.resetPassword)
	group.DELETE("/users/:id", authService.RequirePermission(auth.PermManageUsers), h.delete)
}

func (h *UsersHandler) list(c *gin.Context) {
	users, err := h.repo.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userListResponse(users))
}

func (h *UsersHandler) create(c *gin.Context) {
	var req userCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role := firstNonEmpty(req.Role, auth.RoleGuest)
	if !auth.ValidRole(role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}
	user := models.SysUser{
		Username:     strings.TrimSpace(req.Username),
		PasswordHash: passwordHash,
		Role:         role,
		Enabled:      true,
	}
	if req.Enabled != nil {
		user.Enabled = *req.Enabled
	}
	if err := h.repo.CreateUser(&user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userResponse(user))
}

func (h *UsersHandler) patch(c *gin.Context) {
	userID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || userID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	var req userPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates, err := userUpdates(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Enabled != nil && !*req.Enabled {
		if principal, ok := auth.PrincipalFromContext(c); ok && uint(userID64) == principal.UserID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot disable current user"})
			return
		}
	}
	user, err := h.repo.UpdateUser(uint(userID64), updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userResponse(user))
}

func (h *UsersHandler) resetPassword(c *gin.Context) {
	userID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || userID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	var req userResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}
	user, err := h.repo.UpdateUser(uint(userID64), map[string]interface{}{"password_hash": passwordHash})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userResponse(user))
}

func (h *UsersHandler) delete(c *gin.Context) {
	userID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || userID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if principal, ok := auth.PrincipalFromContext(c); ok && uint(userID64) == principal.UserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete current user"})
		return
	}
	if err := h.repo.DeleteUser(uint(userID64)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func userUpdates(req userPatchRequest) (map[string]interface{}, error) {
	updates := make(map[string]interface{})
	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if username == "" {
			return nil, fmt.Errorf("username is required")
		}
		updates["username"] = username
	}
	if req.Role != nil {
		if !auth.ValidRole(*req.Role) {
			return nil, fmt.Errorf("invalid role")
		}
		updates["role"] = *req.Role
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	return updates, nil
}

func userListResponse(users []models.SysUser) []gin.H {
	result := make([]gin.H, 0, len(users))
	for _, user := range users {
		result = append(result, userResponse(user))
	}
	return result
}

func userResponse(user models.SysUser) gin.H {
	return gin.H{
		"id":                  user.ID,
		"username":            user.Username,
		"role":                user.Role,
		"enabled":             user.Enabled,
		"permissions_version": user.PermissionsVersion,
		"permissions":         auth.PermissionsForRole(user.Role),
		"last_login_at":       user.LastLoginAt,
		"created_at":          user.CreatedAt,
		"updated_at":          user.UpdatedAt,
	}
}
