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
	QueryProxyEnabled bool   `json:"query_proxy_enabled"`
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
	if cfg.Database.Host == "" || cfg.Database.User == "" || cfg.Database.Name == "" {
		return nil, fmt.Errorf("database host, user, and name are required")
	}
	return &cfg, nil
}
