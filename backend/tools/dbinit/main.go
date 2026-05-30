package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"spindle-edge/backend/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

type sqlExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Close() error
}

func main() {
	configPath := flag.String("config", "configs/config.json", "backend config file")
	schemaPath := flag.String("schema", "deploy/schema.sql", "schema SQL file")
	flag.Parse()

	open := func(driver string, dsn string) (sqlExecutor, error) {
		return sql.Open(driver, dsn)
	}
	if err := run(*configPath, *schemaPath, open, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(configPath string, schemaPath string, open func(string, string) (sqlExecutor, error), out io.Writer) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}

	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
	)
	db, err := open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if _, err := db.Exec(string(schema)); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	if _, err := fmt.Fprintf(out, "initialized database=%s from %s\n", cfg.Database.Name, schemaPath); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
