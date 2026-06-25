package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const principalContextKey = "auth_principal"

type Store interface {
	FindUserByUsername(username string) (SysUser, error)
	FindUserByID(id uint) (SysUser, error)
	ListUsers() ([]SysUser, error)
}

type Service struct {
	store           Store
	jwt             *JWTManager
	edgeBaseURL     string
	serviceTokenRef string
	getenv          func(string) string
	httpClient      *http.Client
}

type Principal struct {
	AuthType           string   `json:"auth_type"`
	UserID             uint     `json:"user_id,omitempty"`
	Username           string   `json:"username,omitempty"`
	Role               string   `json:"role,omitempty"`
	PermissionsVersion int64    `json:"permissions_version,omitempty"`
	Permissions        []string `json:"permissions,omitempty"`
}

type Options struct {
	EdgeBaseURL         string
	EdgeServiceTokenRef string
	HTTPClient          *http.Client
	Getenv              func(string) string
}

func NewService(store Store, jwt *JWTManager, options Options) *Service {
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	getenv := options.Getenv
	if getenv == nil {
		getenv = func(key string) string { return "" }
	}
	return &Service{
		store:           store,
		jwt:             jwt,
		edgeBaseURL:     strings.TrimRight(options.EdgeBaseURL, "/"),
		serviceTokenRef: strings.TrimSpace(options.EdgeServiceTokenRef),
		getenv:          getenv,
		httpClient:      client,
	}
}

func (s *Service) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid login request", "code": "invalid_request"})
		return
	}
	user, err := s.store.FindUserByUsername(strings.TrimSpace(req.Username))
	if err != nil || !user.Enabled || !CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password", "code": "unauthorized"})
		return
	}
	if !ValidRole(user.Role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "user role is not allowed", "code": "forbidden"})
		return
	}
	s.writeLoginResponse(c, user)
}

func (s *Service) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Service) Refresh(c *gin.Context) {
	principal, ok := PrincipalFromContext(c)
	if !ok || principal.AuthType != "user" {
		writeUnauthorized(c, "login required")
		return
	}
	user, err := s.store.FindUserByID(principal.UserID)
	if err != nil || !user.Enabled {
		writeUnauthorized(c, "user disabled or missing")
		return
	}
	s.writeLoginResponse(c, user)
}

func (s *Service) Me(c *gin.Context) {
	principal, ok := PrincipalFromContext(c)
	if !ok || principal.AuthType != "user" {
		writeUnauthorized(c, "login required")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":                  principal.UserID,
			"username":            principal.Username,
			"role":                principal.Role,
			"permissions_version": principal.PermissionsVersion,
		},
		"permissions": principal.Permissions,
	})
}

func (s *Service) CreateSSOTicket(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":       "main server does not create edge SSO tickets",
		"code":        "main_server_sso_ticket_unsupported",
		"next_action": "create the one-time SSO ticket from the edge backend and verify it on the main server",
	})
}

func (s *Service) VerifySSOTicket(c *gin.Context) {
	var req struct {
		Ticket string `json:"ticket" binding:"required"`
		EdgeID string `json:"edge_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sso verify request", "code": "invalid_request"})
		return
	}
	edgeUser, err := s.verifyEdgeSSOTicket(c.Request.Context(), req.Ticket, req.EdgeID)
	if err != nil {
		s.writeSSOError(c, err)
		return
	}
	user, err := s.resolveSyncedUser(edgeUser)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":       "sso user is not available in synchronized sys_users",
			"code":        "user_sync_not_ready",
			"next_action": "wait for database synchronization or verify sys_users is synced from the edge side",
		})
		return
	}
	s.writeLoginResponse(c, user)
}

func (s *Service) ListUsers(c *gin.Context) {
	users, err := s.store.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "users query failed", "code": "internal_error"})
		return
	}
	c.JSON(http.StatusOK, usersResponse(users))
}

func (s *Service) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			writeUnauthorized(c, "bearer token required")
			return
		}
		principal, message, ok := s.authenticateUserToken(token)
		if !ok {
			writeUnauthorized(c, message)
			return
		}
		c.Set(principalContextKey, principal)
		c.Next()
	}
}

func (s *Service) RequireUserFromBearerOrQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			token = strings.TrimSpace(c.Query("access_token"))
			ok = token != ""
		}
		if !ok {
			writeUnauthorized(c, "bearer token required")
			return
		}
		principal, message, ok := s.authenticateUserToken(token)
		if !ok {
			writeUnauthorized(c, message)
			return
		}
		c.Set(principalContextKey, principal)
		c.Next()
	}
}

func (s *Service) RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := PrincipalFromContext(c)
		if !ok || principal.AuthType != "user" {
			writeUnauthorized(c, "login required")
			return
		}
		if !RoleHasPermission(principal.Role, permission) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "permission " + permission + " required", "code": "forbidden"})
			return
		}
		c.Next()
	}
}

func (s *Service) authenticateUserToken(token string) (Principal, string, bool) {
	claims, err := s.jwt.Validate(token)
	if errors.Is(err, ErrTokenExpired) {
		return Principal{}, "token expired", false
	}
	if err != nil {
		return Principal{}, "token invalid", false
	}
	userID64, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil {
		return Principal{}, "token subject invalid", false
	}
	user, err := s.store.FindUserByID(uint(userID64))
	if err != nil || !user.Enabled {
		return Principal{}, "user disabled or missing", false
	}
	if user.PermissionsVersion != claims.PermissionsVersion || user.Role != claims.Role {
		return Principal{}, "token permissions version invalid", false
	}
	return Principal{
		AuthType:           "user",
		UserID:             user.ID,
		Username:           user.Username,
		Role:               user.Role,
		PermissionsVersion: user.PermissionsVersion,
		Permissions:        PermissionsForRole(user.Role),
	}, "", true
}

func (s *Service) writeLoginResponse(c *gin.Context, user SysUser) {
	token, err := s.jwt.Sign(UserTokenSubject{
		ID:                 user.ID,
		Username:           user.Username,
		Role:               user.Role,
		PermissionsVersion: user.PermissionsVersion,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sign token", "code": "internal_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(s.jwt.TTL().Seconds()),
		"user":         userResponse(user),
		"permissions":  PermissionsForRole(user.Role),
	})
}

type edgeSSOError string

const (
	edgeSSOMissingToken edgeSSOError = "missing_token"
	edgeSSODisabled     edgeSSOError = "disabled"
	edgeSSOUnavailable  edgeSSOError = "unavailable"
	edgeSSOUnauthorized edgeSSOError = "unauthorized"
	edgeSSOInvalid      edgeSSOError = "invalid"
)

func (e edgeSSOError) Error() string {
	return string(e)
}

func (s *Service) verifyEdgeSSOTicket(ctx context.Context, ticket string, edgeID string) (SysUser, error) {
	if s.edgeBaseURL == "" {
		return SysUser{}, edgeSSODisabled
	}
	token := strings.TrimSpace(s.getenv(s.serviceTokenRef))
	if token == "" {
		return SysUser{}, edgeSSOMissingToken
	}
	endpoint, err := url.JoinPath(s.edgeBaseURL, "/api/v1/auth/sso-ticket/verify")
	if err != nil {
		return SysUser{}, edgeSSOInvalid
	}
	body, err := json.Marshal(gin.H{"ticket": ticket, "edge_id": edgeID})
	if err != nil {
		return SysUser{}, edgeSSOInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return SysUser{}, edgeSSOInvalid
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return SysUser{}, edgeSSOUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return SysUser{}, edgeSSOUnavailable
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return SysUser{}, edgeSSOUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SysUser{}, edgeSSOUnavailable
	}
	var payload struct {
		User SysUser `json:"user"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.User.ID == 0 {
		return SysUser{}, edgeSSOInvalid
	}
	return payload.User, nil
}

func (s *Service) resolveSyncedUser(edgeUser SysUser) (SysUser, error) {
	if edgeUser.ID != 0 {
		user, err := s.store.FindUserByID(edgeUser.ID)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return SysUser{}, err
		}
	}
	if strings.TrimSpace(edgeUser.Username) == "" {
		return SysUser{}, gorm.ErrRecordNotFound
	}
	return s.store.FindUserByUsername(edgeUser.Username)
}

func (s *Service) writeSSOError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, edgeSSOMissingToken):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":             "edge sso service token is missing",
			"code":              "edge_control_token_missing",
			"service_token_ref": s.serviceTokenRef,
		})
	case errors.Is(err, edgeSSODisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":       "edge sso verification is disabled",
			"code":        "edge_control_disabled",
			"next_action": "configure edge base_url and service token before verifying edge SSO tickets",
		})
	case errors.Is(err, edgeSSOUnauthorized):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "sso ticket is invalid", "code": "unauthorized"})
	default:
		c.JSON(http.StatusBadGateway, gin.H{
			"error":       "edge backend unavailable",
			"code":        "edge_backend_unavailable",
			"next_action": "check edge base_url, service token, and edge backend health",
		})
	}
}

func PrincipalFromContext(c *gin.Context) (Principal, bool) {
	raw, ok := c.Get(principalContextKey)
	if !ok {
		return Principal{}, false
	}
	principal, ok := raw.(Principal)
	return principal, ok
}

func bearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != ""
}

func writeUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message, "code": "unauthorized"})
}

func userResponse(user SysUser) gin.H {
	return gin.H{
		"id":                  user.ID,
		"username":            user.Username,
		"role":                user.Role,
		"permissions_version": user.PermissionsVersion,
	}
}

func usersResponse(users []SysUser) []gin.H {
	response := make([]gin.H, 0, len(users))
	for _, user := range users {
		response = append(response, gin.H{
			"id":                  user.ID,
			"username":            user.Username,
			"role":                user.Role,
			"enabled":             user.Enabled,
			"permissions_version": user.PermissionsVersion,
			"permissions":         PermissionsForRole(user.Role),
			"last_login_at":       user.LastLoginAt,
			"created_at":          user.CreatedAt,
			"updated_at":          user.UpdatedAt,
		})
	}
	return response
}
