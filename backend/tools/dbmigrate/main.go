package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/database"

	"gorm.io/gorm"
)

type columnInfo struct {
	ColumnName    string `gorm:"column:COLUMN_NAME"`
	ColumnType    string `gorm:"column:COLUMN_TYPE"`
	ColumnDefault string `gorm:"column:COLUMN_DEFAULT"`
	IsNullable    string `gorm:"column:IS_NULLABLE"`
}

func main() {
	configPath := flag.String("config", "configs/config.json", "backend config file")
	flag.Parse()

	if err := run(*configPath, database.Connect, database.AutoMigrate, readI18NColumns, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(
	configPath string,
	connect func(config.DatabaseConfig) (*gorm.DB, error),
	migrate func(*gorm.DB) error,
	readColumns func(*gorm.DB) ([]columnInfo, error),
	out io.Writer,
) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := connect(cfg.Database)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	if err := migrate(db); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}

	columns, err := readColumns(db)
	if err != nil {
		return fmt.Errorf("read columns: %w", err)
	}

	if _, err := fmt.Fprintf(out, "database=%s\n", cfg.Database.Name); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if _, err := fmt.Fprintln(out, "sys_tags columns:"); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	for _, column := range columns {
		if _, err := fmt.Fprintf(out, "- %s %s default=%q nullable=%s\n", column.ColumnName, column.ColumnType, column.ColumnDefault, column.IsNullable); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
	}
	return nil
}

func readI18NColumns(db *gorm.DB) ([]columnInfo, error) {
	var columns []columnInfo
	err := db.Raw(`
SELECT CONCAT(TABLE_NAME, '.', COLUMN_NAME) AS COLUMN_NAME, COLUMN_TYPE, COALESCE(COLUMN_DEFAULT, '') AS COLUMN_DEFAULT, IS_NULLABLE
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND (
    (TABLE_NAME = 'sys_tags' AND COLUMN_NAME IN ('display_name', 'display_name_en', 'display_name_ja', 'project_id', 'project_code'))
    OR
    (TABLE_NAME = 'sys_projects' AND COLUMN_NAME IN ('project_code', 'display_name', 'display_name_en', 'display_name_ja'))
  )
ORDER BY TABLE_NAME, ORDINAL_POSITION
`).Scan(&columns).Error
	return columns, err
}
