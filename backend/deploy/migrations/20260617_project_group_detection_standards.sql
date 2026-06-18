SET @has_project_group_column := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_projects'
    AND COLUMN_NAME = 'project_group'
);
SET @sql := IF(
  @has_project_group_column = 0,
  'ALTER TABLE sys_projects ADD COLUMN project_group VARCHAR(64) DEFAULT '''' AFTER model_name',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_projects_group_index := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_projects'
    AND INDEX_NAME = 'idx_projects_group'
);
SET @sql := IF(
  @has_projects_group_index = 0,
  'ALTER TABLE sys_projects ADD INDEX idx_projects_group (project_group)',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_standard_group_column := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_detection_standards'
    AND COLUMN_NAME = 'project_group'
);
SET @sql := IF(
  @has_standard_group_column = 0,
  'ALTER TABLE sys_detection_standards ADD COLUMN project_group VARCHAR(64) DEFAULT '''' AFTER project_code',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_standard_group_index := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_detection_standards'
    AND INDEX_NAME = 'idx_detection_standards_project_group'
);
SET @sql := IF(
  @has_standard_group_index = 0,
  'ALTER TABLE sys_detection_standards ADD INDEX idx_detection_standards_project_group (project_group)',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE sys_projects
SET project_group = 'AC'
WHERE project_group = ''
  AND project_code REGEXP '^AC-[0-9][0-9]$';

UPDATE sys_detection_standards s
JOIN sys_projects p ON p.id = s.project_id
SET s.project_group = p.project_group
WHERE s.project_group = ''
  AND p.project_group <> '';
