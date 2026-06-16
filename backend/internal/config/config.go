package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	ConfigPath string         `json:"-"`
	App        AppConfig      `json:"app"`
	Database   DatabaseConfig `json:"database"`
	Auth       AuthConfig     `json:"auth"`
	Sync       SyncConfig     `json:"sync"`
	Gateways   []GatewaySeed  `json:"gateways"`
}

type AppConfig struct {
	HTTPAddr     string `json:"http_addr"`
	LogicWorkers int    `json:"logic_workers"`
	StoreWorkers int    `json:"store_workers"`
	HistoryBatch int    `json:"history_batch"`
}

type DatabaseConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Password    string `json:"password"`
	Name        string `json:"name"`
	AutoMigrate bool   `json:"auto_migrate"`
}

type AuthConfig struct {
	JWTSecret              string              `json:"jwt_secret"`
	AccessTokenTTLSeconds  int                 `json:"access_token_ttl_seconds"`
	SSOTicketTTLSeconds    int                 `json:"sso_ticket_ttl_seconds"`
	EdgeInstanceID         string              `json:"edge_instance_id"`
	MainSiteURL            string              `json:"main_site_url"`
	BootstrapAdminUsername string              `json:"bootstrap_admin_username"`
	BootstrapAdminPassword string              `json:"bootstrap_admin_password"`
	ServiceClients         []ServiceClientSeed `json:"service_clients"`
}

type SyncConfig struct {
	NodeID                     uint64 `json:"node_id"`
	IDBlockSize                uint64 `json:"id_block_size"`
	ConfigWatchIntervalSeconds int    `json:"config_watch_interval_seconds"`
}

type ServiceClientSeed struct {
	ClientID     string   `json:"client_id"`
	Token        string   `json:"token"`
	Scopes       []string `json:"scopes"`
	AllowedCIDRs []string `json:"allowed_cidrs"`
	Enabled      bool     `json:"enabled"`
}

type GatewaySeed struct {
	ID               int    `json:"id"`
	EdgeInstanceID   string `json:"edge_instance_id"`
	Name             string `json:"name"`
	Broker           string `json:"broker"`
	ClientID         string `json:"client_id"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	Topic            string `json:"topic"`
	QOS              byte   `json:"qos"`
	ParserType       string `json:"parser_type"`
	KIOClientID      string `json:"kio_client_id"`
	KIOWriter        string `json:"kio_writer"`
	KIOWriteUsername string `json:"kio_write_username"`
	KIOWritePassword string `json:"kio_write_password"`
	SetDataTopic     string `json:"setdata_topic"`
	WriteResultTopic string `json:"write_result_topic"`
	QueryAllTopic    string `json:"query_all_topic"`
	Enabled          bool   `json:"enabled"`
}

func Load(path string) (*Config, error) {
	cfg := Default()
	cfg.ConfigPath = path
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnv(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	normalize(cfg)
	cfg.ConfigPath = path
	applyEnv(cfg)
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	if path == "" {
		return fmt.Errorf("config path is required")
	}
	normalize(cfg)
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	cfg.ConfigPath = path
	return nil
}

func Default() *Config {
	return &Config{
		App: AppConfig{
			HTTPAddr:     "127.0.0.1:18080",
			LogicWorkers: 8,
			StoreWorkers: 4,
			HistoryBatch: 100,
		},
		Database: DatabaseConfig{
			Host:        "127.0.0.1",
			Port:        3306,
			User:        "root",
			Password:    "",
			Name:        "spindle_edge",
			AutoMigrate: true,
		},
		Auth: AuthConfig{
			JWTSecret:              "edge-terminal-dev-secret-change-before-release",
			AccessTokenTTLSeconds:  1800,
			SSOTicketTTLSeconds:    60,
			EdgeInstanceID:         "edge-local",
			MainSiteURL:            "",
			BootstrapAdminUsername: "admin",
			BootstrapAdminPassword: "Admin@12345",
		},
		Sync: SyncConfig{
			NodeID:                     1,
			IDBlockSize:                1000000000000,
			ConfigWatchIntervalSeconds: 5,
		},
	}
}

func normalize(cfg *Config) {
	if cfg.App.HTTPAddr == "" {
		cfg.App.HTTPAddr = "127.0.0.1:18080"
	}
	if cfg.App.LogicWorkers <= 0 {
		cfg.App.LogicWorkers = 8
	}
	if cfg.App.StoreWorkers <= 0 {
		cfg.App.StoreWorkers = 4
	}
	if cfg.App.HistoryBatch <= 0 {
		cfg.App.HistoryBatch = 100
	}
	if cfg.Database.Host == "" {
		cfg.Database.Host = "127.0.0.1"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 3306
	}
	if cfg.Database.User == "" {
		cfg.Database.User = "root"
	}
	if cfg.Database.Name == "" {
		cfg.Database.Name = "spindle_edge"
	}
	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = "edge-terminal-dev-secret-change-before-release"
	}
	if cfg.Auth.AccessTokenTTLSeconds <= 0 {
		cfg.Auth.AccessTokenTTLSeconds = 1800
	}
	if cfg.Auth.SSOTicketTTLSeconds <= 0 {
		cfg.Auth.SSOTicketTTLSeconds = 60
	}
	if cfg.Auth.EdgeInstanceID == "" {
		cfg.Auth.EdgeInstanceID = "edge-local"
	}
	if cfg.Auth.BootstrapAdminUsername == "" {
		cfg.Auth.BootstrapAdminUsername = "admin"
	}
	if cfg.Auth.BootstrapAdminPassword == "" {
		cfg.Auth.BootstrapAdminPassword = "Admin@12345"
	}
	if cfg.Sync.NodeID == 0 {
		cfg.Sync.NodeID = 1
	}
	if cfg.Sync.IDBlockSize == 0 {
		cfg.Sync.IDBlockSize = 1000000000000
	}
	if cfg.Sync.ConfigWatchIntervalSeconds <= 0 {
		cfg.Sync.ConfigWatchIntervalSeconds = 5
	}
}

func applyEnv(cfg *Config) {
	if value := os.Getenv("EDGE_HTTP_ADDR"); value != "" {
		cfg.App.HTTPAddr = value
	}
	if value := os.Getenv("EDGE_DB_HOST"); value != "" {
		cfg.Database.Host = value
	}
	if value := os.Getenv("EDGE_DB_PORT"); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			cfg.Database.Port = port
		}
	}
	if value := os.Getenv("EDGE_DB_USER"); value != "" {
		cfg.Database.User = value
	}
	if value := os.Getenv("EDGE_DB_PASSWORD"); value != "" {
		cfg.Database.Password = value
	}
	if value := os.Getenv("EDGE_DB_NAME"); value != "" {
		cfg.Database.Name = value
	}
	if value := os.Getenv("EDGE_JWT_SECRET"); value != "" {
		cfg.Auth.JWTSecret = value
	}
	if value := os.Getenv("EDGE_ACCESS_TOKEN_TTL_SECONDS"); value != "" {
		if ttl, err := strconv.Atoi(value); err == nil {
			cfg.Auth.AccessTokenTTLSeconds = ttl
		}
	}
	if value := os.Getenv("EDGE_SSO_TICKET_TTL_SECONDS"); value != "" {
		if ttl, err := strconv.Atoi(value); err == nil {
			cfg.Auth.SSOTicketTTLSeconds = ttl
		}
	}
	if value := os.Getenv("EDGE_INSTANCE_ID"); value != "" {
		cfg.Auth.EdgeInstanceID = value
	}
	if value := os.Getenv("EDGE_MAIN_SITE_URL"); value != "" {
		cfg.Auth.MainSiteURL = value
	}
	if value := os.Getenv("EDGE_BOOTSTRAP_ADMIN_USERNAME"); value != "" {
		cfg.Auth.BootstrapAdminUsername = value
	}
	if value := os.Getenv("EDGE_BOOTSTRAP_ADMIN_PASSWORD"); value != "" {
		cfg.Auth.BootstrapAdminPassword = value
	}
	if value := os.Getenv("EDGE_NODE_ID"); value != "" {
		if nodeID, err := strconv.ParseUint(value, 10, 64); err == nil {
			cfg.Sync.NodeID = nodeID
		}
	}
	if value := os.Getenv("EDGE_ID_BLOCK_SIZE"); value != "" {
		if blockSize, err := strconv.ParseUint(value, 10, 64); err == nil {
			cfg.Sync.IDBlockSize = blockSize
		}
	}
	if value := os.Getenv("EDGE_CONFIG_WATCH_INTERVAL_SECONDS"); value != "" {
		if interval, err := strconv.Atoi(value); err == nil {
			cfg.Sync.ConfigWatchIntervalSeconds = interval
		}
	}
	if value := os.Getenv("EDGE_MAIN_SERVICE_TOKEN"); value != "" {
		clientID := os.Getenv("EDGE_MAIN_SERVICE_CLIENT_ID")
		if clientID == "" {
			clientID = "main-server"
		}
		cfg.Auth.ServiceClients = append(cfg.Auth.ServiceClients, ServiceClientSeed{
			ClientID: clientID,
			Token:    value,
			Scopes: []string{
				"service_realtime_read",
				"service_metadata_read",
				"service_runtime_read",
				"service_control_call",
				"service_sso_verify",
				"edge.detection.start",
				"edge.detection.stop",
				"edge.variable.write",
				"edge.alarm.mute",
				"edge.detection.limit_update",
				"edge.feature.refresh",
				"edge.report.request",
			},
			Enabled: true,
		})
	}
}
