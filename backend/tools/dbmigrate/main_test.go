package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"spindle-edge/backend/internal/config"

	"gorm.io/gorm"
)

func TestRunSuccessAndErrors(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"database":{"name":"edge"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run(
		configPath,
		func(cfg config.DatabaseConfig) (*gorm.DB, error) {
			if cfg.Name != "edge" {
				t.Fatalf("unexpected database cfg: %+v", cfg)
			}
			return &gorm.DB{}, nil
		},
		func(db *gorm.DB) error { return nil },
		func(db *gorm.DB) ([]columnInfo, error) {
			return []columnInfo{{ColumnName: "store_trigger", ColumnType: "varchar(32)", ColumnDefault: "on_detection", IsNullable: "NO"}}, nil
		},
		&out,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "database=edge") || !strings.Contains(out.String(), "store_trigger") {
		t.Fatalf("unexpected output: %s", out.String())
	}

	badConfigPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badConfigPath, []byte(`{bad`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(badConfigPath, nil, nil, nil, &out); err == nil {
		t.Fatal("expected config error")
	}
	if err := run(configPath, func(config.DatabaseConfig) (*gorm.DB, error) {
		return nil, errors.New("connect failed")
	}, nil, nil, &out); err == nil {
		t.Fatal("expected connect error")
	}
	if err := run(configPath, func(config.DatabaseConfig) (*gorm.DB, error) {
		return &gorm.DB{}, nil
	}, func(*gorm.DB) error {
		return errors.New("migrate failed")
	}, nil, &out); err == nil {
		t.Fatal("expected migrate error")
	}
	if err := run(configPath, func(config.DatabaseConfig) (*gorm.DB, error) {
		return &gorm.DB{}, nil
	}, func(*gorm.DB) error {
		return nil
	}, func(*gorm.DB) ([]columnInfo, error) {
		return nil, errors.New("read failed")
	}, &out); err == nil {
		t.Fatal("expected read columns error")
	}
}
