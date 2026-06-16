-- Multi-way configuration sync metadata and runtime edge ownership.

ALTER TABLE sys_station_view_templates
  ADD COLUMN sync_scope VARCHAR(32) NOT NULL DEFAULT 'global' AFTER owner_scope,
  ADD COLUMN edge_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER sync_scope,
  ADD COLUMN updated_by_node VARCHAR(64) NOT NULL DEFAULT '' AFTER edge_instance_id,
  ADD COLUMN updated_by_user VARCHAR(128) NOT NULL DEFAULT '' AFTER updated_by_node;

ALTER TABLE sys_station_view_regions
  ADD COLUMN sync_scope VARCHAR(32) NOT NULL DEFAULT 'global' AFTER enabled,
  ADD COLUMN edge_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER sync_scope,
  ADD COLUMN updated_by_node VARCHAR(64) NOT NULL DEFAULT '' AFTER edge_instance_id,
  ADD COLUMN updated_by_user VARCHAR(128) NOT NULL DEFAULT '' AFTER updated_by_node;

ALTER TABLE sys_station_view_items
  ADD COLUMN sync_scope VARCHAR(32) NOT NULL DEFAULT 'global' AFTER visible,
  ADD COLUMN edge_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER sync_scope,
  ADD COLUMN updated_by_node VARCHAR(64) NOT NULL DEFAULT '' AFTER edge_instance_id,
  ADD COLUMN updated_by_user VARCHAR(128) NOT NULL DEFAULT '' AFTER updated_by_node;

ALTER TABLE sys_station_view_assignments
  ADD COLUMN sync_scope VARCHAR(32) NOT NULL DEFAULT 'global' AFTER enabled,
  ADD COLUMN edge_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER sync_scope,
  ADD COLUMN updated_by_node VARCHAR(64) NOT NULL DEFAULT '' AFTER edge_instance_id,
  ADD COLUMN updated_by_user VARCHAR(128) NOT NULL DEFAULT '' AFTER updated_by_node;

ALTER TABLE sys_detection_standards
  ADD COLUMN sync_scope VARCHAR(32) NOT NULL DEFAULT 'global' AFTER enabled,
  ADD COLUMN edge_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER sync_scope,
  ADD COLUMN updated_by_node VARCHAR(64) NOT NULL DEFAULT '' AFTER edge_instance_id,
  ADD COLUMN updated_by_user VARCHAR(128) NOT NULL DEFAULT '' AFTER updated_by_node;

ALTER TABLE sys_detection_standard_items
  ADD COLUMN sync_scope VARCHAR(32) NOT NULL DEFAULT 'global' AFTER sort_order,
  ADD COLUMN edge_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER sync_scope,
  ADD COLUMN updated_by_node VARCHAR(64) NOT NULL DEFAULT '' AFTER edge_instance_id,
  ADD COLUMN updated_by_user VARCHAR(128) NOT NULL DEFAULT '' AFTER updated_by_node;

ALTER TABLE sys_task_flows
  ADD COLUMN version INT NOT NULL DEFAULT 1 AFTER priority,
  ADD COLUMN sync_scope VARCHAR(32) NOT NULL DEFAULT 'global' AFTER version,
  ADD COLUMN edge_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER sync_scope,
  ADD COLUMN updated_by_node VARCHAR(64) NOT NULL DEFAULT '' AFTER edge_instance_id,
  ADD COLUMN updated_by_user VARCHAR(128) NOT NULL DEFAULT '' AFTER updated_by_node;

ALTER TABLE sys_task_flow_vars
  ADD COLUMN sync_scope VARCHAR(32) NOT NULL DEFAULT 'global' AFTER role,
  ADD COLUMN edge_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER sync_scope,
  ADD COLUMN updated_by_node VARCHAR(64) NOT NULL DEFAULT '' AFTER edge_instance_id,
  ADD COLUMN updated_by_user VARCHAR(128) NOT NULL DEFAULT '' AFTER updated_by_node,
  ADD COLUMN updated_at DATETIME(3) NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) AFTER created_at;

ALTER TABLE sys_detection_tasks
  ADD COLUMN edge_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER project_code;

ALTER TABLE task_flow_runs
  ADD COLUMN edge_instance_id VARCHAR(64) NOT NULL DEFAULT '' AFTER project_id;

CREATE INDEX idx_station_view_templates_sync ON sys_station_view_templates (sync_scope, edge_instance_id, updated_at);
CREATE INDEX idx_detection_standards_sync ON sys_detection_standards (sync_scope, edge_instance_id, updated_at);
CREATE INDEX idx_task_flows_sync ON sys_task_flows (sync_scope, edge_instance_id, updated_at);
CREATE INDEX idx_detection_tasks_edge_status ON sys_detection_tasks (edge_instance_id, status);
CREATE INDEX idx_task_flow_runs_edge_started ON task_flow_runs (edge_instance_id, started_at);
