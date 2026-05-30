package main

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSuccessAndErrors(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	schemaPath := filepath.Join(dir, "schema.sql")
	if err := os.WriteFile(configPath, []byte(`{"database":{"user":"root","password":"pw","host":"127.0.0.1","port":3306,"name":"edge"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(schemaPath, []byte(`CREATE DATABASE edge;`), 0o600); err != nil {
		t.Fatal(err)
	}
	db := &fakeSQLDB{}
	var out bytes.Buffer
	err := run(configPath, schemaPath, func(driver string, dsn string) (sqlExecutor, error) {
		if driver != "mysql" || !strings.Contains(dsn, "multiStatements=true") {
			t.Fatalf("unexpected open args driver=%s dsn=%s", driver, dsn)
		}
		return db, nil
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if db.executed != "CREATE DATABASE edge;" || !db.closed || !strings.Contains(out.String(), "initialized database=edge") {
		t.Fatalf("unexpected run result executed=%q closed=%v out=%q", db.executed, db.closed, out.String())
	}

	badConfigPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badConfigPath, []byte(`{bad`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(badConfigPath, schemaPath, nil, &out); err == nil {
		t.Fatal("expected config error")
	}
	if err := run(configPath, filepath.Join(dir, "missing.sql"), nil, &out); err == nil {
		t.Fatal("expected schema error")
	}
	if err := run(configPath, schemaPath, func(string, string) (sqlExecutor, error) {
		return nil, errors.New("open failed")
	}, &out); err == nil {
		t.Fatal("expected open error")
	}
	if err := run(configPath, schemaPath, func(string, string) (sqlExecutor, error) {
		return &fakeSQLDB{execErr: errors.New("exec failed")}, nil
	}, &out); err == nil {
		t.Fatal("expected exec error")
	}
}

type fakeSQLDB struct {
	executed string
	closed   bool
	execErr  error
}

func (db *fakeSQLDB) Exec(query string, args ...interface{}) (sql.Result, error) {
	db.executed = query
	return fakeResult{}, db.execErr
}

func (db *fakeSQLDB) Close() error {
	db.closed = true
	return nil
}

type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (fakeResult) RowsAffected() (int64, error) {
	return 0, nil
}
