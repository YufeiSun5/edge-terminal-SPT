SET @station_view_items_has_pinned := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_station_view_items'
    AND COLUMN_NAME = 'pinned'
);
SET @station_view_items_add_pinned := IF(
  @station_view_items_has_pinned = 0,
  'ALTER TABLE sys_station_view_items ADD COLUMN pinned BOOLEAN NOT NULL DEFAULT FALSE AFTER sort_order',
  'SELECT 1'
);
PREPARE station_view_items_add_pinned_stmt FROM @station_view_items_add_pinned;
EXECUTE station_view_items_add_pinned_stmt;
DEALLOCATE PREPARE station_view_items_add_pinned_stmt;

SET @station_view_items_has_pinned_idx := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'sys_station_view_items'
    AND INDEX_NAME = 'idx_station_view_items_pinned'
);
SET @station_view_items_add_pinned_idx := IF(
  @station_view_items_has_pinned_idx = 0,
  'ALTER TABLE sys_station_view_items ADD INDEX idx_station_view_items_pinned (pinned)',
  'SELECT 1'
);
PREPARE station_view_items_add_pinned_idx_stmt FROM @station_view_items_add_pinned_idx;
EXECUTE station_view_items_add_pinned_idx_stmt;
DEALLOCATE PREPARE station_view_items_add_pinned_idx_stmt;

UPDATE sys_station_view_regions
SET layout_area = CASE
  WHEN region_key = 'left' THEN 'card_pool'
  WHEN region_key = 'right' THEN 'list_layout'
  ELSE layout_area
END
WHERE layout_area = '' OR layout_area IS NULL;

DELETE legacy_regions
FROM sys_station_view_regions legacy_regions
JOIN sys_station_view_regions canonical_regions
  ON canonical_regions.template_uid = legacy_regions.template_uid
 AND canonical_regions.layout_area = legacy_regions.layout_area
 AND canonical_regions.id <> legacy_regions.id
 AND canonical_regions.region_key = canonical_regions.layout_area
WHERE legacy_regions.region_key IN ('left', 'right');

DELETE FROM sys_station_view_items
WHERE template_uid = 'station-default'
  AND binding_key LIKE 'stcfg\_%';
