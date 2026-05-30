ALTER TABLE sys_tags
  ADD COLUMN source_type VARCHAR(32) NOT NULL DEFAULT 'mqtt' AFTER source_path,
  ADD INDEX idx_sys_tags_source_type (source_type);

ALTER TABLE sys_detection_standards
  ADD COLUMN report_template_id BIGINT UNSIGNED NULL AFTER mode,
  ADD INDEX idx_detection_standards_report_template_id (report_template_id);

CREATE TABLE IF NOT EXISTS sys_report_templates (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  template_code VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  display_name VARCHAR(128) DEFAULT '',
  file_ref VARCHAR(512) NOT NULL,
  file_kind VARCHAR(32) NOT NULL DEFAULT 'xlsx',
  version INT NOT NULL DEFAULT 1,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  remark VARCHAR(255) DEFAULT '',
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY uk_report_templates_code (template_code),
  INDEX idx_report_templates_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE sys_detection_tasks
  ADD COLUMN duration_sec INT NOT NULL DEFAULT 0 AFTER ended_at,
  ADD COLUMN expected_end_at DATETIME(3) NULL AFTER duration_sec,
  ADD COLUMN end_type VARCHAR(32) DEFAULT '' AFTER expected_end_at,
  ADD COLUMN operator_note VARCHAR(512) DEFAULT '' AFTER stop_reason,
  ADD COLUMN report_template_id BIGINT UNSIGNED NULL AFTER template_ref,
  ADD COLUMN report_template_code VARCHAR(64) DEFAULT '' AFTER report_template_id,
  ADD COLUMN report_template_version INT NOT NULL DEFAULT 0 AFTER report_template_code,
  ADD INDEX idx_detection_report_template_id (report_template_id);

CREATE TABLE IF NOT EXISTS detection_run_notes (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  task_id BIGINT UNSIGNED NOT NULL,
  note_type VARCHAR(32) NOT NULL DEFAULT 'memo',
  content TEXT NOT NULL,
  actor_type VARCHAR(32) DEFAULT '',
  actor_id VARCHAR(128) DEFAULT '',
  created_at DATETIME(3) NULL,
  INDEX idx_detection_run_notes_task_id (task_id),
  INDEX idx_detection_run_notes_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS detection_run_reports (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  task_id BIGINT UNSIGNED NOT NULL,
  template_id BIGINT UNSIGNED NULL,
  template_code VARCHAR(64) DEFAULT '',
  template_version INT NOT NULL DEFAULT 0,
  file_ref VARCHAR(512) NOT NULL,
  file_name VARCHAR(255) DEFAULT '',
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  generated_at DATETIME(3) NULL,
  error_message VARCHAR(512) DEFAULT '',
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  INDEX idx_detection_run_reports_task_id (task_id),
  INDEX idx_detection_run_reports_template_id (template_id),
  INDEX idx_detection_run_reports_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
