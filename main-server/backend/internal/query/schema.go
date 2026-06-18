package query

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func EnsureMainServerSchema(db *gorm.DB) error {
	if err := db.AutoMigrate(&MainNotificationRead{}); err != nil {
		return err
	}
	for _, column := range mainServerSharedSchemaColumns() {
		if err := ensureColumn(db, column.table, column.name, column.ddl); err != nil {
			return err
		}
	}
	for _, index := range mainServerSharedSchemaIndexes() {
		if err := ensureIndex(db, index.table, index.name, index.ddl); err != nil {
			return err
		}
	}
	return nil
}

type sharedSchemaColumn struct {
	table string
	name  string
	ddl   string
}

type sharedSchemaIndex struct {
	table string
	name  string
	ddl   string
}

func mainServerSharedSchemaColumns() []sharedSchemaColumn {
	return []sharedSchemaColumn{
		{table: "sys_station_view_templates", name: "sync_scope", ddl: "`sync_scope` varchar(32) NOT NULL DEFAULT 'global'"},
		{table: "sys_station_view_templates", name: "edge_instance_id", ddl: "`edge_instance_id` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_station_view_templates", name: "updated_by_node", ddl: "`updated_by_node` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_station_view_templates", name: "updated_by_user", ddl: "`updated_by_user` varchar(128) NOT NULL DEFAULT ''"},
		{table: "sys_station_view_items", name: "sync_scope", ddl: "`sync_scope` varchar(32) NOT NULL DEFAULT 'global'"},
		{table: "sys_station_view_items", name: "edge_instance_id", ddl: "`edge_instance_id` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_station_view_items", name: "updated_by_node", ddl: "`updated_by_node` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_station_view_items", name: "updated_by_user", ddl: "`updated_by_user` varchar(128) NOT NULL DEFAULT ''"},
		{table: "sys_projects", name: "project_group", ddl: "`project_group` varchar(64) DEFAULT ''"},
		{table: "sys_detection_standards", name: "project_group", ddl: "`project_group` varchar(64) DEFAULT ''"},
		{table: "sys_detection_standards", name: "version", ddl: "`version` int NOT NULL DEFAULT 1"},
		{table: "sys_detection_standards", name: "config_hash", ddl: "`config_hash` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_detection_standards", name: "sync_scope", ddl: "`sync_scope` varchar(32) NOT NULL DEFAULT 'global'"},
		{table: "sys_detection_standards", name: "edge_instance_id", ddl: "`edge_instance_id` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_detection_standards", name: "updated_by_node", ddl: "`updated_by_node` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_detection_standards", name: "updated_by_user", ddl: "`updated_by_user` varchar(128) NOT NULL DEFAULT ''"},
		{table: "sys_detection_standard_items", name: "sync_scope", ddl: "`sync_scope` varchar(32) NOT NULL DEFAULT 'global'"},
		{table: "sys_detection_standard_items", name: "edge_instance_id", ddl: "`edge_instance_id` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_detection_standard_items", name: "updated_by_node", ddl: "`updated_by_node` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_detection_standard_items", name: "updated_by_user", ddl: "`updated_by_user` varchar(128) NOT NULL DEFAULT ''"},
		{table: "sys_task_flows", name: "version", ddl: "`version` int NOT NULL DEFAULT 1"},
		{table: "sys_task_flows", name: "sync_scope", ddl: "`sync_scope` varchar(32) NOT NULL DEFAULT 'global'"},
		{table: "sys_task_flows", name: "edge_instance_id", ddl: "`edge_instance_id` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_task_flows", name: "updated_by_node", ddl: "`updated_by_node` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_task_flows", name: "updated_by_user", ddl: "`updated_by_user` varchar(128) NOT NULL DEFAULT ''"},
		{table: "sys_task_flow_vars", name: "sync_scope", ddl: "`sync_scope` varchar(32) NOT NULL DEFAULT 'global'"},
		{table: "sys_task_flow_vars", name: "edge_instance_id", ddl: "`edge_instance_id` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_task_flow_vars", name: "updated_by_node", ddl: "`updated_by_node` varchar(64) NOT NULL DEFAULT ''"},
		{table: "sys_task_flow_vars", name: "updated_by_user", ddl: "`updated_by_user` varchar(128) NOT NULL DEFAULT ''"},
		{table: "task_flow_runs", name: "edge_instance_id", ddl: "`edge_instance_id` varchar(64) NOT NULL DEFAULT ''"},
	}
}

func mainServerSharedSchemaIndexes() []sharedSchemaIndex {
	return []sharedSchemaIndex{
		{table: "sys_projects", name: "idx_projects_group", ddl: "`idx_projects_group` (`project_group`)"},
		{table: "sys_detection_standards", name: "idx_detection_standards_project_group", ddl: "`idx_detection_standards_project_group` (`project_group`)"},
	}
}

func ensureColumn(db *gorm.DB, table string, column string, ddl string) error {
	table = strings.TrimSpace(table)
	column = strings.TrimSpace(column)
	if table == "" || column == "" || ddl == "" {
		return fmt.Errorf("invalid schema column definition")
	}
	var count int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?",
		table,
		column,
	).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN %s", table, ddl)).Error
}

func ensureIndex(db *gorm.DB, table string, indexName string, ddl string) error {
	table = strings.TrimSpace(table)
	indexName = strings.TrimSpace(indexName)
	if table == "" || indexName == "" || ddl == "" {
		return fmt.Errorf("invalid schema index definition")
	}
	var count int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?",
		table,
		indexName,
	).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD INDEX %s", table, ddl)).Error
}
