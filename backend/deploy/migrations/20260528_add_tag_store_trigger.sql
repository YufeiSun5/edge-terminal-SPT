ALTER TABLE sys_tags
  ADD COLUMN store_trigger VARCHAR(32) NOT NULL DEFAULT 'on_detection' AFTER store_mode;

UPDATE sys_tags
SET store_trigger = 'on_detection'
WHERE store_trigger = '';
