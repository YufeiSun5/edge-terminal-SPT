CREATE TABLE IF NOT EXISTS sys_detection_plans (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  plan_no VARCHAR(128) NOT NULL,
  source_system VARCHAR(64) NOT NULL,
  external_plan_id VARCHAR(128) NOT NULL,
  external_order_no VARCHAR(128) DEFAULT '',
  factory_no VARCHAR(128) NOT NULL,
  customer_name VARCHAR(128) DEFAULT '',
  device_model VARCHAR(128) DEFAULT '',
  test_item_code VARCHAR(64) DEFAULT '',
  test_item_name VARCHAR(128) DEFAULT '',
  test_sequence INT NOT NULL DEFAULT 0,
  mode VARCHAR(64) DEFAULT 'standard',
  standard_code VARCHAR(64) NOT NULL,
  report_request_json TEXT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  owner_edge_instance_id VARCHAR(64) DEFAULT '',
  owner_project_id BIGINT UNSIGNED NULL,
  owner_project_code VARCHAR(64) DEFAULT '',
  started_task_id BIGINT UNSIGNED NULL,
  started_at DATETIME(3) NULL,
  cancelled_at DATETIME(3) NULL,
  error_message VARCHAR(512) DEFAULT '',
  sync_scope VARCHAR(32) NOT NULL DEFAULT 'global',
  edge_instance_id VARCHAR(64) NOT NULL DEFAULT '',
  updated_by_node VARCHAR(64) NOT NULL DEFAULT '',
  updated_by_user VARCHAR(128) NOT NULL DEFAULT '',
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY uk_detection_plans_plan_no (plan_no),
  UNIQUE KEY uk_detection_plans_source_external (source_system, external_plan_id),
  INDEX idx_detection_plans_status (status),
  INDEX idx_detection_plans_factory_no (factory_no),
  INDEX idx_detection_plans_standard_code (standard_code),
  INDEX idx_detection_plans_owner_task (owner_edge_instance_id, owner_project_id, started_task_id),
  INDEX idx_detection_plans_external_order (external_order_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

SET @detection_plans_has_report_request_json := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_detection_plans'
    AND COLUMN_NAME = 'report_request_json'
);
SET @detection_plans_add_report_request_json := IF(
  @detection_plans_has_report_request_json = 0,
  'ALTER TABLE sys_detection_plans ADD COLUMN report_request_json TEXT NULL AFTER standard_code',
  'SELECT 1'
);
PREPARE detection_plans_add_report_request_json_stmt FROM @detection_plans_add_report_request_json;
EXECUTE detection_plans_add_report_request_json_stmt;
DEALLOCATE PREPARE detection_plans_add_report_request_json_stmt;
