//go:build smoke_tools

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunHealthSmoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" && r.URL.Path != "/api/v1/auth/login" {
			if got := r.Header.Get("Authorization"); got != "Bearer smoke-token" {
				http.Error(w, "missing token", http.StatusUnauthorized)
				return
			}
		}
		switch r.URL.RequestURI() {
		case "/health":
			writeJSON(w, healthResponse{Status: "ok", Tags: 3, Channels: map[string]int{"logic": 0}, Gateways: map[string]any{"1": map[string]any{"active": true}}})
		case "/api/v1/auth/login":
			writeJSON(w, loginResponse{AccessToken: "smoke-token"})
		case "/api/v1/auth/me":
			writeJSON(w, map[string]any{"username": "admin"})
		case "/api/v1/projects":
			writeJSON(w, []projectSummary{{ID: 7, ProjectCode: "AC-07"}})
		case "/api/v1/projects/7/members":
			writeJSON(w, map[string]any{"items": []any{}, "count": 0})
		case "/api/v1/variables", "/api/v1/gateway-configs", "/api/v1/task-modules", "/api/v1/task-flow-templates", "/api/v1/detection-runs/active":
			writeJSON(w, []any{})
		case "/api/v1/runtime/channels", "/api/v1/notifications/unread-count":
			writeJSON(w, map[string]any{"ok": true})
		case "/api/v1/task-flow-runs?limit=1":
			writeJSON(w, map[string]any{"items": []any{}, "total": 0})
		case "/api/v1/devices":
			http.NotFound(w, r)
		default:
			t.Fatalf("unexpected request %s", r.URL.RequestURI())
		}
	}))
	defer server.Close()

	if err := run(server.URL, "admin", "Admin@12345"); err != nil {
		t.Fatalf("run failed: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		panic(err)
	}
}
