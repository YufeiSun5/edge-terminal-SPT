package auth

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"

	"github.com/gin-gonic/gin"
)

const principalContextKey = "auth_principal"

type Store interface {
	FindUserByUsername(username string) (models.SysUser, error)
	FindUserByID(id uint) (models.SysUser, error)
	UpdateUserLastLogin(id uint, at time.Time) error
	FindServiceClientBySecretHash(secretHash string) (models.SysServiceClient, error)
	UpdateServiceClientLastUsed(id uint, at time.Time) error
	CreateSSOTicket(ticket *models.SysSSOTicket) error
	ConsumeSSOTicket(ticketHash string, edgeInstanceID string, now time.Time) (models.SysUser, error)
	CreateAuditLog(entry *models.SysAuditLog) error
}

type Service struct {
	store          Store
	jwt            *JWTManager
	edgeInstanceID string
	mainSiteURL    string
	ssoTicketTTL   time.Duration
	now            func() time.Time
}

type Principal struct {
	AuthType           string   `json:"auth_type"`
	UserID             uint     `json:"user_id,omitempty"`
	Username           string   `json:"username,omitempty"`
	Role               string   `json:"role,omitempty"`
	PermissionsVersion int64    `json:"permissions_version,omitempty"`
	Permissions        []string `json:"permissions,omitempty"`
	ClientID           string   `json:"client_id,omitempty"`
	Scopes             []string `json:"scopes,omitempty"`
}

type Options struct {
	EdgeInstanceID string
	MainSiteURL    string
	SSOTicketTTL   time.Duration
}

func NewService(store Store, jwt *JWTManager, options Options) *Service {
	ttl := options.SSOTicketTTL
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &Service{
		store:          store,
		jwt:            jwt,
		edgeInstanceID: options.EdgeInstanceID,
		mainSiteURL:    options.MainSiteURL,
		ssoTicketTTL:   ttl,
		now:            time.Now,
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
	username := strings.TrimSpace(req.Username)
	user, err := s.store.FindUserByUsername(username)
	if err != nil || !user.Enabled || !CheckPassword(req.Password, user.PasswordHash) {
		s.audit("user", username, "auth.login", "user", username, "failed", "{}")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password", "code": "unauthorized"})
		return
	}
	if !ValidRole(user.Role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "user role is not allowed", "code": "forbidden"})
		return
	}
	now := s.now()
	_ = s.store.UpdateUserLastLogin(user.ID, now)
	token, _, err := s.jwt.Sign(UserTokenSubject{
		ID:                 user.ID,
		Username:           user.Username,
		Role:               user.Role,
		PermissionsVersion: user.PermissionsVersion,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sign token", "code": "internal_error"})
		return
	}
	s.audit("user", strconv.FormatUint(uint64(user.ID), 10), "auth.login", "user", user.Username, "success", "{}")
	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(s.jwt.TTL().Seconds()),
		"user":         userResponse(user),
		"permissions":  PermissionsForRole(user.Role),
	})
}

func (s *Service) Logout(c *gin.Context) {
	if principal, ok := PrincipalFromContext(c); ok {
		s.audit("user", strconv.FormatUint(uint64(principal.UserID), 10), "auth.logout", "user", principal.Username, "success", "{}")
	}
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
	token, _, err := s.jwt.Sign(UserTokenSubject{
		ID:                 user.ID,
		Username:           user.Username,
		Role:               user.Role,
		PermissionsVersion: user.PermissionsVersion,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sign token", "code": "internal_error"})
		return
	}
	s.audit("user", strconv.FormatUint(uint64(user.ID), 10), "auth.refresh", "user", user.Username, "success", "{}")
	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(s.jwt.TTL().Seconds()),
		"user":         userResponse(user),
		"permissions":  PermissionsForRole(user.Role),
	})
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
	principal, ok := PrincipalFromContext(c)
	if !ok || principal.AuthType != "user" {
		writeUnauthorized(c, "login required")
		return
	}
	ticket, err := GenerateOpaqueToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create sso ticket", "code": "internal_error"})
		return
	}
	expiresAt := s.now().Add(s.ssoTicketTTL)
	if err := s.store.CreateSSOTicket(&models.SysSSOTicket{
		TicketHash:         HashOpaqueToken(ticket),
		UserID:             principal.UserID,
		Role:               principal.Role,
		PermissionsVersion: principal.PermissionsVersion,
		EdgeInstanceID:     s.edgeInstanceID,
		ExpiresAt:          expiresAt,
		CreatedIP:          c.ClientIP(),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create sso ticket", "code": "internal_error"})
		return
	}
	s.audit("user", strconv.FormatUint(uint64(principal.UserID), 10), "auth.sso_ticket.create", "sso_ticket", "", "success", "{}")
	c.JSON(http.StatusOK, gin.H{
		"ticket":           ticket,
		"expires_in":       int(s.ssoTicketTTL.Seconds()),
		"edge_instance_id": s.edgeInstanceID,
		"main_site_url":    s.buildMainSiteURL(ticket),
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
	user, err := s.store.ConsumeSSOTicket(HashOpaqueToken(req.Ticket), req.EdgeID, s.now())
	if err != nil {
		s.audit("service", serviceActorID(c), "auth.sso_ticket.verify", "sso_ticket", "", "failed", "{}")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "sso ticket is invalid", "code": "unauthorized"})
		return
	}
	s.audit("service", serviceActorID(c), "auth.sso_ticket.verify", "user", strconv.FormatUint(uint64(user.ID), 10), "success", "{}")
	c.JSON(http.StatusOK, gin.H{
		"edge_instance_id": req.EdgeID,
		"user":             userResponse(user),
	})
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
			writeForbidden(c, fmt.Sprintf("permission %s required", permission))
			return
		}
		c.Next()
	}
}

func (s *Service) RequireServiceScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			writeUnauthorized(c, "service token required")
			return
		}
		client, err := s.store.FindServiceClientBySecretHash(HashOpaqueToken(token))
		if err != nil || !client.Enabled {
			writeUnauthorized(c, "service token invalid")
			return
		}
		if client.ExpiresAt != nil && !client.ExpiresAt.After(s.now()) {
			writeUnauthorized(c, "service token expired")
			return
		}
		if !serviceClientAllowsIP(client.AllowedCIDRs, c.ClientIP()) {
			writeForbidden(c, "service client source ip is not allowed")
			return
		}
		if !HasScope(client.Scopes, scope) {
			writeForbidden(c, fmt.Sprintf("scope %s required", scope))
			return
		}
		_ = s.store.UpdateServiceClientLastUsed(client.ID, s.now())
		c.Set(principalContextKey, Principal{
			AuthType: "service",
			ClientID: client.ClientID,
			Scopes:   ParseScopes(client.Scopes),
		})
		c.Next()
	}
}

func serviceClientAllowsIP(allowedCIDRs string, clientIP string) bool {
	allowedCIDRs = strings.TrimSpace(allowedCIDRs)
	if allowedCIDRs == "" {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil {
		return false
	}
	for _, raw := range strings.Split(allowedCIDRs, ",") {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if !strings.Contains(item, "/") {
			allowed := net.ParseIP(item)
			if allowed != nil && allowed.Equal(ip) {
				return true
			}
			continue
		}
		_, network, err := net.ParseCIDR(item)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func PrincipalFromContext(c *gin.Context) (Principal, bool) {
	raw, ok := c.Get(principalContextKey)
	if !ok {
		return Principal{}, false
	}
	principal, ok := raw.(Principal)
	return principal, ok
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

func writeForbidden(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": message, "code": "forbidden"})
}

func userResponse(user models.SysUser) gin.H {
	return gin.H{
		"id":                  user.ID,
		"username":            user.Username,
		"role":                user.Role,
		"permissions_version": user.PermissionsVersion,
	}
}

func (s *Service) buildMainSiteURL(ticket string) string {
	if s.mainSiteURL == "" {
		return ""
	}
	parsed, err := url.Parse(s.mainSiteURL)
	if err != nil {
		return ""
	}
	query := parsed.Query()
	query.Set("ticket", ticket)
	query.Set("edge_id", s.edgeInstanceID)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (s *Service) audit(actorType string, actorID string, action string, targetType string, targetID string, result string, detail string) {
	if s.store == nil {
		return
	}
	_ = s.store.CreateAuditLog(&models.SysAuditLog{
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Result:     result,
		Detail:     detail,
		CreatedAt:  s.now(),
	})
}

func serviceActorID(c *gin.Context) string {
	principal, ok := PrincipalFromContext(c)
	if !ok {
		return ""
	}
	return principal.ClientID
}
