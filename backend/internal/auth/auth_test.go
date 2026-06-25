package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"spindle-edge/backend/internal/models"

	"github.com/gin-gonic/gin"
)

type fakeStore struct {
	usersByID   map[uint]models.SysUser
	usersByName map[string]models.SysUser
	clients     map[string]models.SysServiceClient
	tickets     map[string]models.SysSSOTicket
	audits      int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		usersByID:   map[uint]models.SysUser{},
		usersByName: map[string]models.SysUser{},
		clients:     map[string]models.SysServiceClient{},
		tickets:     map[string]models.SysSSOTicket{},
	}
}

func (s *fakeStore) putUser(user models.SysUser) {
	s.usersByID[user.ID] = user
	s.usersByName[user.Username] = user
}

func (s *fakeStore) FindUserByUsername(username string) (models.SysUser, error) {
	user, ok := s.usersByName[username]
	if !ok {
		return models.SysUser{}, errors.New("not found")
	}
	return user, nil
}

func (s *fakeStore) FindUserByID(id uint) (models.SysUser, error) {
	user, ok := s.usersByID[id]
	if !ok {
		return models.SysUser{}, errors.New("not found")
	}
	return user, nil
}

func (s *fakeStore) UpdateUserLastLogin(id uint, at time.Time) error {
	user := s.usersByID[id]
	user.LastLoginAt = &at
	s.putUser(user)
	return nil
}

func (s *fakeStore) FindServiceClientBySecretHash(secretHash string) (models.SysServiceClient, error) {
	client, ok := s.clients[secretHash]
	if !ok {
		return models.SysServiceClient{}, errors.New("not found")
	}
	return client, nil
}

func (s *fakeStore) UpdateServiceClientLastUsed(id uint, at time.Time) error {
	for key, client := range s.clients {
		if client.ID == id {
			client.LastUsedAt = &at
			s.clients[key] = client
			return nil
		}
	}
	return nil
}

func (s *fakeStore) CreateSSOTicket(ticket *models.SysSSOTicket) error {
	ticket.ID = uint64(len(s.tickets) + 1)
	s.tickets[ticket.TicketHash] = *ticket
	return nil
}

func (s *fakeStore) ConsumeSSOTicket(ticketHash string, edgeInstanceID string, now time.Time) (models.SysUser, error) {
	ticket, ok := s.tickets[ticketHash]
	if !ok || ticket.EdgeInstanceID != edgeInstanceID || ticket.UsedAt != nil || !ticket.ExpiresAt.After(now) {
		return models.SysUser{}, errors.New("invalid ticket")
	}
	user, ok := s.usersByID[ticket.UserID]
	if !ok || !user.Enabled || user.Role != ticket.Role || user.PermissionsVersion != ticket.PermissionsVersion {
		return models.SysUser{}, errors.New("invalid user")
	}
	ticket.UsedAt = &now
	s.tickets[ticketHash] = ticket
	return user, nil
}

func (s *fakeStore) CreateAuditLog(entry *models.SysAuditLog) error {
	s.audits++
	return nil
}

func TestPermissionsForRoles(t *testing.T) {
	if !ValidRole(RoleAdmin) || ValidRole("owner") {
		t.Fatal("role validation failed")
	}
	if !RoleHasPermission(RoleAdmin, PermKIOWrite) {
		t.Fatal("admin should have kio_write")
	}
	if RoleHasPermission(RoleDeveloper, PermKIOWrite) {
		t.Fatal("developer should not have kio_write by default")
	}
	permissions := PermissionsForRole(RoleGuest)
	permissions[0] = "mutated"
	if PermissionsForRole(RoleGuest)[0] == "mutated" {
		t.Fatal("permissions slice should be copied")
	}
	scopes := NormalizeScopes([]string{"service_sso_verify", "", "service_sso_verify", " service_control_call "})
	if scopes != "service_sso_verify,service_control_call" || !HasScope(scopes, "service_control_call") {
		t.Fatalf("unexpected scopes: %q", scopes)
	}
}

func TestPasswordAndOpaqueTokenHashing(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword("secret", hash) || CheckPassword("wrong", hash) {
		t.Fatal("password check failed")
	}
	token, err := GenerateOpaqueToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || strings.Contains(token, "=") {
		t.Fatalf("unexpected token: %q", token)
	}
	tokenHash := HashOpaqueToken(token)
	if tokenHash == "" || tokenHash == token {
		t.Fatal("opaque token hash failed")
	}
}

func TestJWTValidateRejectsTamperedAndExpiredTokens(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	manager := NewJWTManager("test-secret", time.Minute)
	manager.now = func() time.Time { return now }
	token, claims, err := manager.Sign(UserTokenSubject{ID: 7, Username: "admin", Role: RoleAdmin, PermissionsVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "7" || claims.Role != RoleAdmin {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if _, err := manager.Validate(token); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Validate(token + "x"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected invalid token, got %v", err)
	}
	manager.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := manager.Validate(token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected expired token, got %v", err)
	}
}

func TestLoginMeSSOTicketVerifyFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newFakeStore()
	passwordHash, err := HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	store.putUser(models.SysUser{
		ID:                 1,
		Username:           "admin",
		PasswordHash:       passwordHash,
		Role:               RoleAdmin,
		Enabled:            true,
		PermissionsVersion: 1,
	})
	serviceToken := "main-service-token"
	store.clients[HashOpaqueToken(serviceToken)] = models.SysServiceClient{
		ClientID: "main-server",
		Scopes:   ScopeServiceSSOVerify,
		Enabled:  true,
	}

	jwt := NewJWTManager("test-secret", 30*time.Minute)
	svc := NewService(store, jwt, Options{
		EdgeInstanceID: "edge-001",
		MainSiteURL:    "https://main.example.com/sso/edge",
		SSOTicketTTL:   time.Minute,
	})
	router := gin.New()
	router.POST("/login", svc.Login)
	protected := router.Group("")
	protected.Use(svc.RequireUser())
	protected.GET("/me", svc.Me)
	protected.POST("/refresh", svc.Refresh)
	protected.POST("/sso-ticket", svc.RequirePermission(PermSSOHandoff), svc.CreateSSOTicket)
	router.POST("/sso-ticket/verify", svc.RequireServiceScope(ScopeServiceSSOVerify), svc.VerifySSOTicket)

	loginBody := `{"username":"admin","password":"Admin@12345"}`
	loginResp := performJSON(router, http.MethodPost, "/login", "", loginBody)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginResp.Code, loginResp.Body.String())
	}
	var loginPayload map[string]any
	mustDecode(t, loginResp, &loginPayload)
	accessToken, _ := loginPayload["access_token"].(string)
	if accessToken == "" || store.audits == 0 {
		t.Fatal("expected access token and audit")
	}

	meResp := performJSON(router, http.MethodGet, "/me", accessToken, "")
	if meResp.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", meResp.Code, meResp.Body.String())
	}

	refreshResp := performJSON(router, http.MethodPost, "/refresh", accessToken, "")
	if refreshResp.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", refreshResp.Code, refreshResp.Body.String())
	}
	var refreshPayload map[string]any
	mustDecode(t, refreshResp, &refreshPayload)
	refreshedToken, _ := refreshPayload["access_token"].(string)
	if refreshedToken == "" || refreshedToken == accessToken || refreshPayload["expires_in"].(float64) != float64(int(jwt.TTL().Seconds())) {
		t.Fatalf("unexpected refresh payload: %+v", refreshPayload)
	}

	ticketResp := performJSON(router, http.MethodPost, "/sso-ticket", accessToken, "")
	if ticketResp.Code != http.StatusOK {
		t.Fatalf("ticket status=%d body=%s", ticketResp.Code, ticketResp.Body.String())
	}
	var ticketPayload map[string]any
	mustDecode(t, ticketResp, &ticketPayload)
	ticket, _ := ticketPayload["ticket"].(string)
	mainURL, _ := ticketPayload["main_site_url"].(string)
	if ticket == "" || !strings.Contains(mainURL, "ticket=") || !strings.Contains(mainURL, "edge_id=edge-001") {
		t.Fatalf("unexpected ticket payload: %+v", ticketPayload)
	}

	verifyBody := `{"ticket":"` + ticket + `","edge_id":"edge-001"}`
	verifyResp := performJSON(router, http.MethodPost, "/sso-ticket/verify", serviceToken, verifyBody)
	if verifyResp.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", verifyResp.Code, verifyResp.Body.String())
	}
	reuseResp := performJSON(router, http.MethodPost, "/sso-ticket/verify", serviceToken, verifyBody)
	if reuseResp.Code != http.StatusUnauthorized {
		t.Fatalf("reuse status=%d body=%s", reuseResp.Code, reuseResp.Body.String())
	}
}

func TestRefreshRejectsExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newFakeStore()
	store.putUser(models.SysUser{
		ID:                 1,
		Username:           "admin",
		Role:               RoleAdmin,
		Enabled:            true,
		PermissionsVersion: 1,
	})
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	jwt := NewJWTManager("test-secret", time.Minute)
	jwt.now = func() time.Time { return now }
	token, _, err := jwt.Sign(UserTokenSubject{ID: 1, Username: "admin", Role: RoleAdmin, PermissionsVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	jwt.now = func() time.Time { return now.Add(2 * time.Minute) }
	svc := NewService(store, jwt, Options{})
	router := gin.New()
	router.POST("/refresh", svc.RequireUser(), svc.Refresh)

	resp := performJSON(router, http.MethodPost, "/refresh", token, "")
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected expired refresh to be unauthorized, got status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestUserAndServiceAuthorizationFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newFakeStore()
	store.putUser(models.SysUser{
		ID:                 2,
		Username:           "guest",
		PasswordHash:       "unused",
		Role:               RoleGuest,
		Enabled:            true,
		PermissionsVersion: 1,
	})
	serviceToken := "service-token"
	store.clients[HashOpaqueToken(serviceToken)] = models.SysServiceClient{
		ClientID: "main-server",
		Scopes:   ScopeServiceRealtimeRead,
		Enabled:  true,
	}
	jwt := NewJWTManager("test-secret", time.Minute)
	token, _, err := jwt.Sign(UserTokenSubject{ID: 2, Username: "guest", Role: RoleGuest, PermissionsVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, jwt, Options{})
	router := gin.New()
	router.GET("/write", svc.RequireUser(), svc.RequirePermission(PermKIOWrite), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.POST("/verify", svc.RequireServiceScope(ScopeServiceSSOVerify), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	if resp := performJSON(router, http.MethodGet, "/write", "", ""); resp.Code != http.StatusUnauthorized {
		t.Fatalf("missing user token status=%d", resp.Code)
	}
	if resp := performJSON(router, http.MethodGet, "/write", token, ""); resp.Code != http.StatusForbidden {
		t.Fatalf("missing permission status=%d body=%s", resp.Code, resp.Body.String())
	}
	if resp := performJSON(router, http.MethodPost, "/verify", serviceToken, "{}"); resp.Code != http.StatusForbidden {
		t.Fatalf("missing service scope status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func performJSON(router http.Handler, method string, path string, bearer string, body string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func mustDecode(t *testing.T, resp *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(resp.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response: %v body=%s", err, resp.Body.String())
	}
}
