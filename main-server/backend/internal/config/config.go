package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	App      AppConfig      `json:"app"`
	Database DatabaseConfig `json:"database"`
	Edge     EdgeConfig     `json:"edge"`
	Edges    []EdgeConfig   `json:"edges"`
	Auth     AuthConfig     `json:"auth"`
	Reports  ReportConfig   `json:"reports"`
}

type AppConfig struct {
	HTTPAddr string `json:"http_addr"`
}

type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type EdgeConfig struct {
	BaseURL           string `json:"base_url"`
	EdgeInstanceID    string `json:"edge_instance_id"`
	ServiceTokenRef   string `json:"service_token_ref"`
	Enabled           *bool  `json:"enabled"`
	QueryProxyEnabled bool   `json:"query_proxy_enabled"`
}

type AuthConfig struct {
	JWTSecret             string `json:"jwt_secret"`
	JWTSecretRef          string `json:"jwt_secret_ref"`
	AccessTokenTTLSeconds int    `json:"access_token_ttl_seconds"`
}

type ReportConfig struct {
	ArtifactDir            string `json:"artifact_dir"`
	DefaultTemplateCode    string `json:"default_template_code"`
	DefaultTemplateVersion int    `json:"default_template_version"`
	DefaultTemplateFileRef string `json:"default_template_file_ref"`
	WorkerEnabled          *bool  `json:"worker_enabled"`
	WorkerIntervalSeconds  int    `json:"worker_interval_seconds"`
	MaxAttempts            int    `json:"max_attempts"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.App.HTTPAddr == "" {
		cfg.App.HTTPAddr = "0.0.0.0:19080"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 3306
	}
	if cfg.Edge.BaseURL == "" {
		cfg.Edge.BaseURL = "http://127.0.0.1:18080"
	}
	if cfg.Edge.EdgeInstanceID == "" {
		cfg.Edge.EdgeInstanceID = "edge-local"
	}
	if cfg.Edge.ServiceTokenRef == "" {
		cfg.Edge.ServiceTokenRef = "EDGE_MAIN_SERVICE_TOKEN"
	}
	cfg.EnsureEdges()
	if cfg.Auth.JWTSecretRef == "" {
		cfg.Auth.JWTSecretRef = "MAIN_SERVER_JWT_SECRET"
	}
	if value := os.Getenv(cfg.Auth.JWTSecretRef); value != "" {
		cfg.Auth.JWTSecret = value
	}
	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = "main-server-dev-secret-change-before-release"
	}
	if cfg.Auth.AccessTokenTTLSeconds <= 0 {
		cfg.Auth.AccessTokenTTLSeconds = 1800
	}
	if cfg.Reports.ArtifactDir == "" {
		cfg.Reports.ArtifactDir = "data/reports"
	}
	if cfg.Reports.DefaultTemplateCode == "" {
		cfg.Reports.DefaultTemplateCode = "SPINDLE_DEFAULT_REPORT"
	}
	if cfg.Reports.DefaultTemplateVersion <= 0 {
		cfg.Reports.DefaultTemplateVersion = 1
	}
	if cfg.Reports.DefaultTemplateFileRef == "" {
		cfg.Reports.DefaultTemplateFileRef = "templates/default-report-template.xlsx"
	}
	if cfg.Reports.WorkerIntervalSeconds <= 0 {
		cfg.Reports.WorkerIntervalSeconds = 30
	}
	if cfg.Reports.MaxAttempts <= 0 {
		cfg.Reports.MaxAttempts = 3
	}
	if cfg.Database.Host == "" || cfg.Database.User == "" || cfg.Database.Name == "" {
		return nil, fmt.Errorf("database host, user, and name are required")
	}
	return &cfg, nil
}

func (cfg *Config) EnsureEdges() {
	if len(cfg.Edges) == 0 {
		cfg.Edges = []EdgeConfig{cfg.Edge}
	}
	seen := map[string]bool{}
	cleaned := make([]EdgeConfig, 0, len(cfg.Edges))
	for _, edge := range cfg.Edges {
		if edge.BaseURL == "" {
			edge.BaseURL = cfg.Edge.BaseURL
		}
		if edge.EdgeInstanceID == "" {
			edge.EdgeInstanceID = cfg.Edge.EdgeInstanceID
		}
		if edge.ServiceTokenRef == "" {
			edge.ServiceTokenRef = cfg.Edge.ServiceTokenRef
		}
		if edge.EdgeInstanceID == "" || seen[edge.EdgeInstanceID] {
			continue
		}
		seen[edge.EdgeInstanceID] = true
		cleaned = append(cleaned, edge)
	}
	if len(cleaned) == 0 {
		cleaned = []EdgeConfig{cfg.Edge}
	}
	cfg.Edges = cleaned
	cfg.Edge = cleaned[0]
}

func (e EdgeConfig) IsEnabled() bool {
	return e.Enabled == nil || *e.Enabled
}

func (r ReportConfig) IsWorkerEnabled() bool {
	return r.WorkerEnabled == nil || *r.WorkerEnabled
}
