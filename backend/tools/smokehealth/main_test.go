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
			writeJSON(w, healthResponse{Status: "ok", Tags: 3, Channels: validHealthChannels(), Gateways: map[string]any{"1": map[string]any{"active": true}}})
		case "/api/v1/auth/login":
			writeJSON(w, loginResponse{AccessToken: "smoke-token"})
		case "/api/v1/auth/me":
			writeJSON(w, map[string]any{"username": "admin"})
		case "/api/v1/projects":
			writeJSON(w, []projectSummary{{ID: 7, ProjectCode: "AC-07"}})
		case "/api/v1/projects/7/members":
			writeJSON(w, map[string]any{"items": []any{}, "count": 0})
		case "/api/v1/storage-routes?project_id=7&enabled=true":
			writeJSON(w, map[string]any{"items": []any{}, "total": 0})
		case "/api/v1/detection-standards?project_id=7&enabled=true":
			writeJSON(w, []any{})
		case "/api/v1/realtime/variables?project_id=7":
			writeJSON(w, []any{})
		case "/api/v1/history/data?project_id=7&limit=1":
			writeJSON(w, []any{})
		case "/api/v1/limit-alarms?project_id=7&limit=1":
			writeJSON(w, map[string]any{"items": []any{}, "total": 0, "limit": 1, "offset": 0})
		case "/api/v1/detection-runs/current?project_id=7":
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, map[string]any{"error": "no running detection for project"})
		case "/api/v1/variables", "/api/v1/gateway-configs", "/api/v1/report-templates?enabled=true", "/api/v1/task-modules", "/api/v1/task-flow-templates", "/api/v1/detection-runs/active":
			writeJSON(w, []any{})
		case "/api/v1/task-flows/runtime":
			writeJSON(w, map[string]any{"queue": map[string]any{"name": "task_flow", "len": 0, "cap": 1000, "usage": 0, "pressure": false, "impact": "task impact", "next_action": "no action"}, "guards": 0, "submitted": 0, "enqueued": 0, "dropped": 0, "pressure_threshold": 0.8, "pressure": false})
		case "/api/v1/runtime/channels":
			writeJSON(w, validHealthChannels())
		case "/api/v1/notifications/unread-count":
			writeJSON(w, map[string]any{"ok": true})
		case "/api/v1/runtime/channels/detail":
			items := make([]any, 0, len(expectedCoreChannels))
			for _, name := range expectedCoreChannels {
				items = append(items, map[string]any{"name": name, "len": 0, "cap": 2000, "usage": 0, "dropped": 0, "pressure": false, "impact": name + " impact", "next_action": "no action"})
			}
			writeJSON(w, map[string]any{"items": items, "pressure": []any{}, "pressure_threshold": 0.8})
		case "/api/v1/runtime/notifications":
			writeJSON(w, map[string]any{"subscribers": 0, "buffered": 0, "capacity": 0, "usage": 0, "pressure_threshold": 0.8, "pressure": false, "impact": "notify impact", "next_action": "no action", "published": 0, "delivered": 0, "dropped": 0})
		case "/api/v1/runtime/workers":
			items := make([]any, 0, len(expectedCoreWorkers))
			for _, name := range expectedCoreWorkers {
				items = append(items, map[string]any{"name": name, "active": 1, "starts": 1, "exits": 0, "panics": 0, "health": "ok", "impact": name + " impact", "next_action": "no action"})
			}
			writeJSON(w, map[string]any{"items": items})
		case "/api/v1/audit-logs?action=auth.login&result=success&limit=1":
			writeJSON(w, map[string]any{"items": []any{}, "total": 0, "limit": 1, "offset": 0})
		case "/api/v1/notifications?unread=true&type=detection.run_started&level=info&project_id=1&from=2026-01-01T00%3A00%3A00Z&to=2026-12-31T23%3A59%3A59Z&keyword=smoke&limit=1":
			writeJSON(w, map[string]any{"items": []any{}, "total": 0, "limit": 1, "offset": 0})
		case "/api/v1/notifications/unread-count?type=detection.run_started&level=info&project_id=1&from=2026-01-01T00%3A00%3A00Z&to=2026-12-31T23%3A59%3A59Z&keyword=smoke":
			writeJSON(w, map[string]any{"unread": 0})
		case "/api/v1/notifications/read-all?type=detection.run_started&level=info&project_id=1&from=2026-01-01T00%3A00%3A00Z&to=2026-12-31T23%3A59%3A59Z&keyword=smoke":
			if r.Method != http.MethodPost {
				t.Fatalf("read-all method=%s", r.Method)
			}
			writeJSON(w, map[string]any{"updated": 0})
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

func TestValidateWorkerDiagnosticsRejectsUnhealthyWorker(t *testing.T) {
	panics := uint64(1)
	items := validWorkerDiagnostics(&panics)
	items[0].Health = "degraded"
	items[0].NextAction = "check logs"
	err := validateWorkerDiagnostics(items)
	if err == nil {
		t.Fatal("expected unhealthy worker to fail")
	}
}

func TestValidateHealthRequiresCoreChannels(t *testing.T) {
	channels := validHealthChannels()
	delete(channels, expectedCoreChannels[0])
	err := validateHealth(healthResponse{Status: "ok", Channels: channels})
	if err == nil {
		t.Fatal("expected missing health core channel to fail")
	}
}

func TestValidateHealthRejectsBadStatus(t *testing.T) {
	err := validateHealth(healthResponse{Status: "degraded", Channels: validHealthChannels()})
	if err == nil {
		t.Fatal("expected bad health status to fail")
	}
}

func TestValidateChannelMapRequiresCoreChannels(t *testing.T) {
	channels := validHealthChannels()
	delete(channels, expectedCoreChannels[len(expectedCoreChannels)-1])
	if err := validateChannelMap("runtime/channels", channels); err == nil {
		t.Fatal("expected missing compact runtime channel to fail")
	}
}

func TestValidateChannelDiagnosticsChecksEveryItem(t *testing.T) {
	dropped := uint64(0)
	items := validChannelDiagnostics(&dropped)
	items[1].Dropped = nil
	err := validateChannelDiagnostics(items)
	if err == nil {
		t.Fatal("expected missing dropped counter on second channel to fail")
	}
}

func TestValidateChannelDiagnosticsRequiresCoreChannels(t *testing.T) {
	dropped := uint64(0)
	items := validChannelDiagnostics(&dropped)
	items = items[:len(items)-1]
	if err := validateChannelDiagnostics(items); err == nil {
		t.Fatal("expected missing core channel to fail")
	}
}

func TestValidateNotificationDiagnosticsRequiresCounters(t *testing.T) {
	published := uint64(1)
	delivered := uint64(1)
	err := validateNotificationDiagnostics(notificationDiagnostic{
		PressureThreshold: 0.8,
		Impact:            "notify impact",
		NextAction:        "no action",
		Published:         &published,
		Delivered:         &delivered,
	})
	if err == nil {
		t.Fatal("expected missing dropped counter to fail")
	}
}

func TestValidateWorkerDiagnosticsRejectsMissingPanicsCounter(t *testing.T) {
	panics := uint64(0)
	items := validWorkerDiagnostics(&panics)
	items[0].Panics = nil
	err := validateWorkerDiagnostics(items)
	if err == nil {
		t.Fatal("expected missing panics counter to fail")
	}
}

func TestValidateWorkerDiagnosticsRejectsMissingLifecycleCounters(t *testing.T) {
	panics := uint64(0)
	items := validWorkerDiagnostics(&panics)
	items[0].Starts = nil
	err := validateWorkerDiagnostics(items)
	if err == nil {
		t.Fatal("expected missing starts counter to fail")
	}

	items = validWorkerDiagnostics(&panics)
	items[0].Exits = nil
	err = validateWorkerDiagnostics(items)
	if err == nil {
		t.Fatal("expected missing exits counter to fail")
	}
}

func TestValidateWorkerDiagnosticsRejectsMissingActiveCounter(t *testing.T) {
	panics := uint64(0)
	items := validWorkerDiagnostics(&panics)
	items[0].Active = nil
	err := validateWorkerDiagnostics(items)
	if err == nil {
		t.Fatal("expected missing active counter to fail")
	}
}

func TestValidateWorkerDiagnosticsRejectsInactiveCoreWorker(t *testing.T) {
	panics := uint64(0)
	items := validWorkerDiagnostics(&panics)
	inactive := 0
	items[0].Active = &inactive
	err := validateWorkerDiagnostics(items)
	if err == nil {
		t.Fatal("expected inactive core worker to fail")
	}
}

func TestValidateWorkerDiagnosticsRequiresCoreWorkers(t *testing.T) {
	panics := uint64(0)
	items := validWorkerDiagnostics(&panics)
	items = items[:len(items)-1]
	if err := validateWorkerDiagnostics(items); err == nil {
		t.Fatal("expected missing core worker to fail")
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		panic(err)
	}
}

func validChannelDiagnostics(dropped *uint64) []channelDiagnostic {
	items := make([]channelDiagnostic, 0, len(expectedCoreChannels))
	for _, name := range expectedCoreChannels {
		items = append(items, channelDiagnostic{
			Name:       name,
			Dropped:    dropped,
			Impact:     name + " impact",
			NextAction: "no action",
		})
	}
	return items
}

func validWorkerDiagnostics(panics *uint64) []workerDiagnostic {
	items := make([]workerDiagnostic, 0, len(expectedCoreWorkers))
	active := 1
	starts := uint64(1)
	exits := uint64(0)
	for _, name := range expectedCoreWorkers {
		items = append(items, workerDiagnostic{
			Name:       name,
			Active:     &active,
			Starts:     &starts,
			Exits:      &exits,
			Health:     "ok",
			Panics:     panics,
			Impact:     name + " impact",
			NextAction: "no action",
		})
	}
	return items
}

func validHealthChannels() map[string]int {
	channels := make(map[string]int, len(expectedCoreChannels))
	for _, name := range expectedCoreChannels {
		channels[name] = 0
	}
	return channels
}
