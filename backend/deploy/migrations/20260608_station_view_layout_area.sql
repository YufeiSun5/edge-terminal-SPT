ALTER TABLE sys_station_view_regions
  ADD COLUMN layout_area VARCHAR(64) NOT NULL DEFAULT 'card_pool' AFTER region_key,
  ADD INDEX idx_station_view_regions_layout_area (layout_area);

UPDATE sys_station_view_regions
SET layout_area = CASE
  WHEN region_key = 'right' THEN 'list_layout'
  ELSE 'card_pool'
END
WHERE layout_area = '' OR layout_area IS NULL OR layout_area IN ('left', 'right');

UPDATE sys_station_view_regions
SET region_key = layout_area
WHERE region_key IN ('left', 'right');

ALTER TABLE sys_station_view_items
  ADD COLUMN layout_area VARCHAR(64) NOT NULL DEFAULT 'card_pool' AFTER region_key,
  ADD INDEX idx_station_view_items_layout_area (layout_area);

UPDATE sys_station_view_items
SET layout_area = CASE
  WHEN region_key = 'right' THEN 'list_layout'
  ELSE 'card_pool'
END
WHERE layout_area = '' OR layout_area IS NULL OR layout_area IN ('left', 'right');

UPDATE sys_station_view_items
SET region_key = layout_area
WHERE region_key IN ('left', 'right');
