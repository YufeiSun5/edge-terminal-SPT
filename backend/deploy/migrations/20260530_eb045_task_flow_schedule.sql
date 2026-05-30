ALTER TABLE sys_task_flows
  ADD COLUMN schedule_interval_ms INT NOT NULL DEFAULT 0 AFTER hold_ms;
