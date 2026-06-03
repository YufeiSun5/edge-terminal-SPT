package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type fakeStore struct {
	usersByID   map[uint]SysUser
	usersByName map[string]SysUser
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		usersByID:   map[uint]SysUser{},
		usersByName: map[string]SysUser{},
	}
}

func (s *fakeStore) putUser(user SysUser) {
	s.usersByID[user.ID] = user
	s.usersByName[user.Username] = user
}

func (s *fakeStore) FindUserByUsername(username string) (SysUser, error) {
	user, ok := s.usersByName[username]
	if !ok {
		return SysUser{}, gorm.ErrRecordNotFound
	}
	return user, nil
}

func (s *fakeStore) FindUserByID(id uint) (SysUser, error) {
	user, ok := s.usersByID[id]
	if !ok {
		return SysUser{}, gorm.ErrRecordNotFound
	}
	return user, nil
}

func (s *fakeStore) ListUsers() ([]SysUser, error) {
	users := make([]SysUser, 0, len(s.usersByID))
	for _, user := range s.usersByID {
		users = append(users, user)
	}
	return users, nil
}

func TestLoginMeUsersAndPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	passwordHash, err := HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeStore()
	store.putUser(SysUser{ID: 1, Username: "admin", PasswordHash: passwordHash, Role: RoleAdmin, Enabled: true, PermissionsVersion: 2})
	svc := NewService(store, NewJWTManager("test-secret", time.Hour), Options{})
	router := gin.New()
	router.POST("/login", svc.Login)
	protected := router.Group("/")
	protected.Use(svc.RequireUser())
	protected.GET("/me", svc.Me)
	protected.GET("/users", svc.RequirePermission(PermManageUsers), svc.ListUsers)

	login := performJSON(router, http.MethodPost, "/login", "", `{"username":"admin","password":"Admin@12345"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	token := extractAccessToken(t, login.Body.String())
	if token == "" || !strings.Contains(login.Body.String(), PermManageUsers) {
		t.Fatalf("unexpected login payload=%s", login.Body.String())
	}
	if me := performJSON(router, http.MethodGet, "/me", token, ""); me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"username":"admin"`) {
		t.Fatalf("me status=%d body=%s", me.Code, me.Body.String())
	}
	if users := performJSON(router, http.MethodGet, "/users", token, ""); users.Code != http.StatusOK || !strings.Contains(users.Body.String(), `"permissions"`) {
		t.Fatalf("users status=%d body=%s", users.Code, users.Body.String())
	}
	if bad := performJSON(router, http.MethodPost, "/login", "", `{"username":"admin","password":"bad"}`); bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status=%d body=%s", bad.Code, bad.Body.String())
	}
}

func TestRequirePermissionRejectsGuest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newFakeStore()
	store.putUser(SysUser{ID: 2, Username: "guest", Role: RoleGuest, Enabled: true, PermissionsVersion: 1})
	jwt := NewJWTManager("test-secret", time.Hour)
	token, err := jwt.Sign(UserTokenSubject{ID: 2, Username: "guest", Role: RoleGuest, PermissionsVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, jwt, Options{})
	router := gin.New()
	router.GET("/admin", svc.RequireUser(), svc.RequirePermission(PermManageUsers), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	resp := performJSON(router, http.MethodGet, "/admin", token, "")
	if resp.Code != http.StatusForbidden {
		t.Fatalf("guest admin status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestVerifySSOTicketUsesEdgeAndSyncedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newFakeStore()
	store.putUser(SysUser{ID: 3, Username: "admin", Role: RoleAdmin, Enabled: true, PermissionsVersion: 4})
	var seenAuth string
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":3,"username":"admin","role":"admin","permissions_version":4}}`))
	}))
	t.Cleanup(edge.Close)
	svc := NewService(store, NewJWTManager("test-secret", time.Hour), Options{
		EdgeBaseURL:         edge.URL,
		EdgeServiceTokenRef: "EDGE_TOKEN",
		Getenv: func(key string) string {
			if key == "EDGE_TOKEN" {
				return "service-secret"
			}
			return ""
		},
	})
	router := gin.New()
	router.POST("/sso-ticket/verify", svc.VerifySSOTicket)
	resp := performJSON(router, http.MethodPost, "/sso-ticket/verify", "", `{"ticket":"ticket-1","edge_id":"edge-a"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("sso status=%d body=%s", resp.Code, resp.Body.String())
	}
	if seenAuth != "Bearer service-secret" || !strings.Contains(resp.Body.String(), `"access_token"`) {
		t.Fatalf("unexpected sso auth=%q body=%s", seenAuth, resp.Body.String())
	}
}

func TestVerifySSOTicketDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(newFakeStore(), NewJWTManager("test-secret", time.Hour), Options{
		EdgeBaseURL:         "http://127.0.0.1:1",
		EdgeServiceTokenRef: "MISSING",
	})
	router := gin.New()
	router.POST("/sso-ticket/verify", svc.VerifySSOTicket)
	missing := performJSON(router, http.MethodPost, "/sso-ticket/verify", "", `{"ticket":"ticket-1","edge_id":"edge-a"}`)
	if missing.Code != http.StatusServiceUnavailable || !strings.Contains(missing.Body.String(), "edge_control_token_missing") {
		t.Fatalf("missing token status=%d body=%s", missing.Code, missing.Body.String())
	}

	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthorized"}`))
	}))
	t.Cleanup(edge.Close)
	svc = NewService(newFakeStore(), NewJWTManager("test-secret", time.Hour), Options{
		EdgeBaseURL:         edge.URL,
		EdgeServiceTokenRef: "EDGE_TOKEN",
		Getenv:              func(string) string { return "service-secret" },
	})
	router = gin.New()
	router.POST("/sso-ticket/verify", svc.VerifySSOTicket)
	unauthorized := performJSON(router, http.MethodPost, "/sso-ticket/verify", "", `{"ticket":"ticket-1","edge_id":"edge-a"}`)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
}

func TestTokenValidationRejectsChangedPermissions(t *testing.T) {
	store := newFakeStore()
	store.putUser(SysUser{ID: 1, Username: "admin", Role: RoleAdmin, Enabled: true, PermissionsVersion: 2})
	jwt := NewJWTManager("test-secret", time.Hour)
	token, err := jwt.Sign(UserTokenSubject{ID: 1, Username: "admin", Role: RoleAdmin, PermissionsVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, jwt, Options{})
	if _, message, ok := svc.authenticateUserToken(token); ok || !strings.Contains(message, "permissions version") {
		t.Fatalf("expected permission version rejection, ok=%v message=%q", ok, message)
	}
}

func TestResolveSyncedUserFallsBackToUsername(t *testing.T) {
	store := newFakeStore()
	store.putUser(SysUser{ID: 9, Username: "admin", Role: RoleAdmin, Enabled: true, PermissionsVersion: 1})
	svc := NewService(store, NewJWTManager("test-secret", time.Hour), Options{})
	user, err := svc.resolveSyncedUser(SysUser{ID: 100, Username: "admin"})
	if err != nil || user.ID != 9 {
		t.Fatalf("fallback user=%+v err=%v", user, err)
	}
}

func performJSON(router http.Handler, method string, path string, token string, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	router.ServeHTTP(rec, req)
	return rec
}

func extractAccessToken(t *testing.T, body string) string {
	t.Helper()
	const marker = `"access_token":"`
	start := strings.Index(body, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := strings.Index(body[start:], `"`)
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}
