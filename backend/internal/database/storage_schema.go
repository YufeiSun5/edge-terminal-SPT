package database

import (
	"fmt"
	"strings"

	"spindle-edge/backend/internal/models"
)

func (r *Repository) EnsureProjectWideTable(ProjectID uint, routes []models.DetectionRunStorageRoute) error {
	routesByTable := make(map[string][]models.DetectionRunStorageRoute)
	for _, route := range routes {
		if route.StorageTarget != models.StorageTargetWideTable {
			continue
		}
		if route.ProjectID != ProjectID {
			return fmt.Errorf("run storage route Project mismatch: got %d want %d", route.ProjectID, ProjectID)
		}
		if err := ValidateStorageIdentifier(route.StorageTable); err != nil {
			return err
		}
		routesByTable[route.StorageTable] = append(routesByTable[route.StorageTable], route)
	}
	dialect := r.db.Name()
	for tableName, tableRoutes := range routesByTable {
		if !r.db.Migrator().HasTable(tableName) {
			if err := r.db.Exec(createProjectWideTableSQL(dialect, tableName)).Error; err != nil {
				return err
			}
		} else if err := r.ensureProjectWideIdentityColumns(ProjectID, tableName); err != nil {
			return err
		}
		for _, route := range tableRoutes {
			if err := ValidateStorageIdentifier(route.ColumnName); err != nil {
				return err
			}
			if !isAllowedStorageColumnType(route.ColumnType) {
				return fmt.Errorf("unsupported storage column type %q", route.ColumnType)
			}
			if !r.db.Migrator().HasColumn(tableName, route.ColumnName) {
				if err := r.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s NULL", quoteIdentifier(dialect, tableName), quoteIdentifier(dialect, route.ColumnName), route.ColumnType)).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *Repository) ensureProjectWideIdentityColumns(projectID uint, tableName string) error {
	dialect := r.db.Name()
	quotedTable := quoteIdentifier(dialect, tableName)
	if !r.db.Migrator().HasColumn(tableName, "project_id") {
		columnType := "BIGINT UNSIGNED NOT NULL DEFAULT 0"
		if dialect == "sqlite" {
			columnType = "INTEGER NOT NULL DEFAULT 0"
		}
		if err := r.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", quotedTable, quoteIdentifier(dialect, "project_id"), columnType)).Error; err != nil {
			return err
		}
	}
	if !r.db.Migrator().HasColumn(tableName, "project_code") {
		columnType := "VARCHAR(64) DEFAULT ''"
		if dialect == "sqlite" {
			columnType = "TEXT DEFAULT ''"
		}
		if err := r.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", quotedTable, quoteIdentifier(dialect, "project_code"), columnType)).Error; err != nil {
			return err
		}
	}
	if tableName == ProjectWideTableName(projectID) {
		if err := r.db.Table(tableName).Where("project_id = ?", 0).Update("project_id", projectID).Error; err != nil {
			return err
		}
	}
	return nil
}

func createProjectWideTableSQL(dialect string, tableName string) string {
	quotedTable := quoteIdentifier(dialect, tableName)
	if dialect == "sqlite" {
		return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
id INTEGER PRIMARY KEY AUTOINCREMENT,
task_id INTEGER NOT NULL,
test_no TEXT DEFAULT '',
project_id INTEGER NOT NULL,
project_code TEXT DEFAULT '',
sample_time DATETIME NOT NULL,
sample_bucket_ms INTEGER NOT NULL,
created_at DATETIME NULL,
updated_at DATETIME NULL,
UNIQUE(task_id, sample_bucket_ms)
)`, quotedTable)
	}
	indexPrefix := tableName
	if len(indexPrefix) > 36 {
		indexPrefix = indexPrefix[:36]
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
task_id BIGINT UNSIGNED NOT NULL,
test_no VARCHAR(128) DEFAULT '',
project_id BIGINT UNSIGNED NOT NULL,
project_code VARCHAR(64) DEFAULT '',
sample_time DATETIME(3) NOT NULL,
sample_bucket_ms BIGINT NOT NULL,
created_at DATETIME(3) NULL,
updated_at DATETIME(3) NULL,
UNIQUE KEY uk_%s_task_bucket (task_id, sample_bucket_ms),
INDEX idx_%s_task_id (task_id),
INDEX idx_%s_sample_time (sample_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`, quotedTable, indexPrefix, indexPrefix, indexPrefix)
}

func quoteIdentifier(dialect string, value string) string {
	if dialect == "sqlite" {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func isAllowedStorageColumnType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "DOUBLE", "BIGINT", "TEXT", "TINYINT(1)":
		return true
	default:
		return false
	}
}
