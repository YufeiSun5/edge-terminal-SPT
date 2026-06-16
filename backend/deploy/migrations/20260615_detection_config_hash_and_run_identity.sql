SET @has_standard_config_hash := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_standards' AND COLUMN_NAME = 'config_hash'
);
SET @sql := IF(@has_standard_config_hash = 0, 'ALTER TABLE sys_detection_standards ADD COLUMN config_hash VARCHAR(64) DEFAULT '''' AFTER version', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_factory_no := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND COLUMN_NAME = 'factory_no');
SET @sql := IF(@has_factory_no = 0, 'ALTER TABLE sys_detection_tasks ADD COLUMN factory_no VARCHAR(128) DEFAULT '''' AFTER test_no', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_customer_name := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND COLUMN_NAME = 'customer_name');
SET @sql := IF(@has_customer_name = 0, 'ALTER TABLE sys_detection_tasks ADD COLUMN customer_name VARCHAR(128) DEFAULT '''' AFTER factory_no', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_device_model := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND COLUMN_NAME = 'device_model');
SET @sql := IF(@has_device_model = 0, 'ALTER TABLE sys_detection_tasks ADD COLUMN device_model VARCHAR(128) DEFAULT '''' AFTER customer_name', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_config_enabled := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND COLUMN_NAME = 'config_enabled');
SET @sql := IF(@has_config_enabled = 0, 'ALTER TABLE sys_detection_tasks ADD COLUMN config_enabled BOOLEAN NOT NULL DEFAULT FALSE AFTER standard_version', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_config_status := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND COLUMN_NAME = 'config_status');
SET @sql := IF(@has_config_status = 0, 'ALTER TABLE sys_detection_tasks ADD COLUMN config_status VARCHAR(32) NOT NULL DEFAULT ''disabled'' AFTER config_enabled', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_config_code := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND COLUMN_NAME = 'config_code');
SET @sql := IF(@has_config_code = 0, 'ALTER TABLE sys_detection_tasks ADD COLUMN config_code VARCHAR(64) DEFAULT '''' AFTER config_status', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_config_name := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND COLUMN_NAME = 'config_name');
SET @sql := IF(@has_config_name = 0, 'ALTER TABLE sys_detection_tasks ADD COLUMN config_name VARCHAR(128) DEFAULT '''' AFTER config_code', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_config_version := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND COLUMN_NAME = 'config_version');
SET @sql := IF(@has_config_version = 0, 'ALTER TABLE sys_detection_tasks ADD COLUMN config_version INT NOT NULL DEFAULT 0 AFTER config_name', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_task_config_hash := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND COLUMN_NAME = 'config_hash');
SET @sql := IF(@has_task_config_hash = 0, 'ALTER TABLE sys_detection_tasks ADD COLUMN config_hash VARCHAR(64) DEFAULT '''' AFTER config_version', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_current_revision := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND COLUMN_NAME = 'current_config_revision');
SET @sql := IF(@has_current_revision = 0, 'ALTER TABLE sys_detection_tasks ADD COLUMN current_config_revision INT NOT NULL DEFAULT 1 AFTER config_hash', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_factory_idx := (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND INDEX_NAME = 'idx_detection_factory_no');
SET @sql := IF(@has_factory_idx = 0, 'ALTER TABLE sys_detection_tasks ADD INDEX idx_detection_factory_no (factory_no)', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_config_code_idx := (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND INDEX_NAME = 'idx_detection_config_code');
SET @sql := IF(@has_config_code_idx = 0, 'ALTER TABLE sys_detection_tasks ADD INDEX idx_detection_config_code (config_code)', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_config_status_idx := (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sys_detection_tasks' AND INDEX_NAME = 'idx_detection_config_status');
SET @sql := IF(@has_config_status_idx = 0, 'ALTER TABLE sys_detection_tasks ADD INDEX idx_detection_config_status (config_status)', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

UPDATE sys_detection_tasks
SET
  factory_no = COALESCE(factory_no, ''),
  customer_name = COALESCE(customer_name, ''),
  device_model = COALESCE(device_model, ''),
  config_enabled = CASE WHEN COALESCE(standard_id, 0) > 0 THEN TRUE ELSE COALESCE(config_enabled, FALSE) END,
  config_status = CASE WHEN COALESCE(standard_id, 0) > 0 OR COALESCE(standard_code, '') <> '' THEN 'applied' ELSE 'disabled' END,
  config_code = CASE WHEN COALESCE(config_code, '') = '' THEN COALESCE(standard_code, '') ELSE config_code END,
  config_version = CASE WHEN COALESCE(config_version, 0) = 0 THEN COALESCE(standard_version, 0) ELSE config_version END,
  current_config_revision = CASE WHEN COALESCE(current_config_revision, 0) = 0 THEN 1 ELSE current_config_revision END;

SET @has_item_revision := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'detection_run_standard_items' AND COLUMN_NAME = 'config_revision');
SET @sql := IF(@has_item_revision = 0, 'ALTER TABLE detection_run_standard_items ADD COLUMN config_revision INT NOT NULL DEFAULT 1 AFTER standard_item_id', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_effective_from := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'detection_run_standard_items' AND COLUMN_NAME = 'effective_from');
SET @sql := IF(@has_effective_from = 0, 'ALTER TABLE detection_run_standard_items ADD COLUMN effective_from DATETIME(3) NULL AFTER sort_order', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_effective_to := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'detection_run_standard_items' AND COLUMN_NAME = 'effective_to');
SET @sql := IF(@has_effective_to = 0, 'ALTER TABLE detection_run_standard_items ADD COLUMN effective_to DATETIME(3) NULL AFTER effective_from', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_item_revision_idx := (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'detection_run_standard_items' AND INDEX_NAME = 'idx_run_standard_items_config_revision');
SET @sql := IF(@has_item_revision_idx = 0, 'ALTER TABLE detection_run_standard_items ADD INDEX idx_run_standard_items_config_revision (config_revision)', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_effective_from_idx := (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'detection_run_standard_items' AND INDEX_NAME = 'idx_run_standard_items_effective_from');
SET @sql := IF(@has_effective_from_idx = 0, 'ALTER TABLE detection_run_standard_items ADD INDEX idx_run_standard_items_effective_from (effective_from)', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_effective_to_idx := (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'detection_run_standard_items' AND INDEX_NAME = 'idx_run_standard_items_effective_to');
SET @sql := IF(@has_effective_to_idx = 0, 'ALTER TABLE detection_run_standard_items ADD INDEX idx_run_standard_items_effective_to (effective_to)', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

UPDATE detection_run_standard_items i
JOIN sys_detection_tasks t ON t.id = i.task_id
SET
  i.config_revision = CASE WHEN COALESCE(i.config_revision, 0) = 0 THEN 1 ELSE i.config_revision END,
  i.effective_from = COALESCE(i.effective_from, t.started_at);

CREATE TABLE IF NOT EXISTS runtime_settings (
  setting_key VARCHAR(128) PRIMARY KEY,
  setting_value VARCHAR(512) NOT NULL,
  remark VARCHAR(255) DEFAULT '',
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO runtime_settings (setting_key, setting_value, remark, created_at, updated_at)
VALUES
  ('detection_config_ready_timeout_ms', '60000', 'Detection config readiness wait timeout in milliseconds.', NOW(3), NOW(3)),
  ('detection_config_ready_interval_ms', '5000', 'Detection config readiness retry interval in milliseconds.', NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE setting_key = VALUES(setting_key);
