ALTER TABLE sys_tags
  ADD COLUMN display_name_en VARCHAR(128) DEFAULT '' AFTER display_name,
  ADD COLUMN display_name_ja VARCHAR(128) DEFAULT '' AFTER display_name_en;
