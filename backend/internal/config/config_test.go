package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultAndMissingFileLoad(t *testing.T) {
	t.Setenv("EDGE_HTTP_ADDR", "127.0.0.1:19090")
	t.Setenv("EDGE_JWT_SECRET", "secret-from-env")
	t.Setenv("EDGE_WS_SNAPSHOT_INTERVAL_MS", "200")
	t.Setenv("EDGE_MAIN_SERVICE_TOKEN", "service-token")
	t.Setenv("EDGE_MAIN_SERVICE_CLIENT_ID", "main")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.HTTPAddr != "127.0.0.1:19090" || cfg.Auth.JWTSecret != "secret-from-env" {
		t.Fatalf("env overrides not applied: %+v", cfg)
	}
	if cfg.Realtime.WSSnapshotIntervalMS != 250 {
		t.Fatalf("expected realtime interval clamp to 250ms, got %+v", cfg.Realtime)
	}
	if len(cfg.Auth.ServiceClients) != 1 || cfg.Auth.ServiceClients[0].ClientID != "main" || !cfg.Auth.ServiceClients[0].Enabled {
		t.Fatalf("service client env seed not applied: %+v", cfg.Auth.ServiceClients)
	}
	scopes := strings.Join(cfg.Auth.ServiceClients[0].Scopes, ",")
	for _, required := range []string{"service_realtime_read", "service_metadata_read", "service_runtime_read", "service_control_call"} {
		if !strings.Contains(scopes, required) {
			t.Fatalf("service client env seed missing scope %s: %+v", required, cfg.Auth.ServiceClients[0].Scopes)
		}
	}
}

func TestLoadNormalizesPartialConfigAndEnvTTLs(t *testing.T) {
	t.Setenv("EDGE_ACCESS_TOKEN_TTL_SECONDS", "900")
	t.Setenv("EDGE_SSO_TICKET_TTL_SECONDS", "45")
	t.Setenv("EDGE_DB_PORT", "3310")
	t.Setenv("EDGE_DB_USER", "edge")
	t.Setenv("EDGE_DB_PASSWORD", "pw")
	t.Setenv("EDGE_DB_NAME", "edge_db")
	t.Setenv("EDGE_INSTANCE_ID", "edge-001")
	t.Setenv("EDGE_MAIN_SITE_URL", "https://main.example.com/sso")
	t.Setenv("EDGE_BOOTSTRAP_ADMIN_USERNAME", "root")
	t.Setenv("EDGE_BOOTSTRAP_ADMIN_PASSWORD", "Root@12345")

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"app":{},"database":{"host":"db"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.LogicWorkers != 8 || cfg.Database.Host != "db" || cfg.Database.Port != 3310 || cfg.Database.User != "edge" {
		t.Fatalf("unexpected normalized cfg: %+v", cfg)
	}
	if cfg.Auth.AccessTokenTTLSeconds != 900 || cfg.Auth.SSOTicketTTLSeconds != 45 || cfg.Auth.EdgeInstanceID != "edge-001" {
		t.Fatalf("unexpected auth cfg: %+v", cfg.Auth)
	}
	if cfg.Realtime.WSSnapshotIntervalMS != 500 {
		t.Fatalf("unexpected realtime default: %+v", cfg.Realtime)
	}
}

func TestRealtimeIntervalClamp(t *testing.T) {
	cfg := Default()
	cfg.Realtime.WSSnapshotIntervalMS = 6000
	normalize(cfg)
	if cfg.Realtime.WSSnapshotIntervalMS != 5000 {
		t.Fatalf("expected upper clamp, got %d", cfg.Realtime.WSSnapshotIntervalMS)
	}
	cfg.Realtime.WSSnapshotIntervalMS = 10
	normalize(cfg)
	if cfg.Realtime.WSSnapshotIntervalMS != 250 {
		t.Fatalf("expected lower clamp, got %d", cfg.Realtime.WSSnapshotIntervalMS)
	}
}

func TestLoadRejectsInvalidJSONAndReadErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{bad`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected parse error")
	}
}
