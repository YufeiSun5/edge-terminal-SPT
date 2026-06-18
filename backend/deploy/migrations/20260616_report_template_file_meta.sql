SET @report_templates_has_file_sha256 := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_report_templates'
    AND COLUMN_NAME = 'file_sha256'
);
SET @report_templates_add_file_sha256 := IF(
  @report_templates_has_file_sha256 = 0,
  'ALTER TABLE sys_report_templates ADD COLUMN file_sha256 VARCHAR(64) DEFAULT '''' AFTER file_kind',
  'SELECT 1'
);
PREPARE report_templates_add_file_sha256_stmt FROM @report_templates_add_file_sha256;
EXECUTE report_templates_add_file_sha256_stmt;
DEALLOCATE PREPARE report_templates_add_file_sha256_stmt;

SET @report_templates_has_file_size := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_report_templates'
    AND COLUMN_NAME = 'file_size'
);
SET @report_templates_add_file_size := IF(
  @report_templates_has_file_size = 0,
  'ALTER TABLE sys_report_templates ADD COLUMN file_size BIGINT NOT NULL DEFAULT 0 AFTER file_sha256',
  'SELECT 1'
);
PREPARE report_templates_add_file_size_stmt FROM @report_templates_add_file_size;
EXECUTE report_templates_add_file_size_stmt;
DEALLOCATE PREPARE report_templates_add_file_size_stmt;
