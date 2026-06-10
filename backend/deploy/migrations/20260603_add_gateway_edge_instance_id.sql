ALTER TABLE sys_gateways
  ADD COLUMN edge_instance_id VARCHAR(128) DEFAULT '' AFTER id,
  ADD INDEX idx_gateways_edge_instance_id (edge_instance_id);

UPDATE sys_gateways
SET edge_instance_id = 'edge-local'
WHERE edge_instance_id IS NULL OR edge_instance_id = '';
