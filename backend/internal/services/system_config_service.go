package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/database"
)

type SystemConfigService struct {
	mu  sync.RWMutex
	cfg *config.Config
}

type DatabaseConfigView struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	User            string `json:"user"`
	Name            string `json:"name"`
	AutoMigrate     bool   `json:"auto_migrate"`
	PasswordSet     bool   `json:"password_set"`
	RestartRequired bool   `json:"restart_required"`
}

type DatabaseConfigUpdate struct {
	Host        *string `json:"host"`
	Port        *int    `json:"port"`
	User        *string `json:"user"`
	Password    *string `json:"password"`
	Name        *string `json:"name"`
	AutoMigrate *bool   `json:"auto_migrate"`
}

type DatabaseConfigTestResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func NewSystemConfigService(cfg *config.Config) *SystemConfigService {
	return &SystemConfigService{cfg: cfg}
}

func (s *SystemConfigService) DatabaseConfig() DatabaseConfigView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return databaseConfigView(s.cfg.Database, false)
}

func (s *SystemConfigService) UpdateDatabaseConfig(input DatabaseConfigUpdate) (DatabaseConfigView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := *s.cfg
	next.Database = mergeDatabaseConfig(next.Database, input)
	if err := validateDatabaseConfig(next.Database); err != nil {
		return DatabaseConfigView{}, err
	}
	if err := config.Save(s.cfg.ConfigPath, &next); err != nil {
		return DatabaseConfigView{}, err
	}
	s.cfg.Database = next.Database
	return databaseConfigView(next.Database, true), nil
}

func (s *SystemConfigService) TestDatabaseConfig(ctx context.Context, input DatabaseConfigUpdate) DatabaseConfigTestResult {
	s.mu.RLock()
	cfg := mergeDatabaseConfig(s.cfg.Database, input)
	s.mu.RUnlock()
	if err := validateDatabaseConfig(cfg); err != nil {
		return DatabaseConfigTestResult{OK: false, Error: err.Error()}
	}
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := database.TestConnection(testCtx, cfg); err != nil {
		return DatabaseConfigTestResult{OK: false, Error: err.Error()}
	}
	return DatabaseConfigTestResult{OK: true}
}

func mergeDatabaseConfig(current config.DatabaseConfig, input DatabaseConfigUpdate) config.DatabaseConfig {
	if input.Host != nil {
		current.Host = strings.TrimSpace(*input.Host)
	}
	if input.Port != nil {
		current.Port = *input.Port
	}
	if input.User != nil {
		current.User = strings.TrimSpace(*input.User)
	}
	if input.Password != nil {
		current.Password = *input.Password
	}
	if input.Name != nil {
		current.Name = strings.TrimSpace(*input.Name)
	}
	if input.AutoMigrate != nil {
		current.AutoMigrate = *input.AutoMigrate
	}
	return current
}

func validateDatabaseConfig(cfg config.DatabaseConfig) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("host is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.User) == "" {
		return fmt.Errorf("user is required")
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

func databaseConfigView(cfg config.DatabaseConfig, restartRequired bool) DatabaseConfigView {
	return DatabaseConfigView{
		Host:            cfg.Host,
		Port:            cfg.Port,
		User:            cfg.User,
		Name:            cfg.Name,
		AutoMigrate:     cfg.AutoMigrate,
		PasswordSet:     cfg.Password != "",
		RestartRequired: restartRequired,
	}
}
