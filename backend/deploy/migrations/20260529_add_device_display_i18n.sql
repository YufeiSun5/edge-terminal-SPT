ALTER TABLE sys_projects
  ADD COLUMN display_name VARCHAR(128) DEFAULT '' AFTER name,
  ADD COLUMN display_name_en VARCHAR(128) DEFAULT '' AFTER display_name,
  ADD COLUMN display_name_ja VARCHAR(128) DEFAULT '' AFTER display_name_en;

UPDATE sys_projects
SET display_name = name
WHERE display_name = '' OR display_name IS NULL;
