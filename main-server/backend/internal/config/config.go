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
	Auth     AuthConfig     `json:"auth"`
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
	if cfg.Database.Host == "" || cfg.Database.User == "" || cfg.Database.Name == "" {
		return nil, fmt.Errorf("database host, user, and name are required")
	}
	return &cfg, nil
}

func (e EdgeConfig) IsEnabled() bool {
	return e.Enabled == nil || *e.Enabled
}
