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
