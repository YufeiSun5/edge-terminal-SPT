# Edge Backend Architecture

This backend is initialized from the reusable SPD_JGHJ acquisition pattern:

- MQTT is the primary live data ingress.
- Gateways are data sources, not Project parents. Variables belong to a gateway/source path first, then can be assigned to a virtual Project group.
- Projects are business groupings used by pages and detection tasks. One Project may contain variables discovered from different MQTT gateways.
- The hot path is memory-first: runtime-eligible tags live in a Go `map[int64]*Tag` guarded by `sync.RWMutex`. A tag is runtime-eligible only when `enabled=true` and `project_id` is assigned.
- MQTT callbacks do not perform MySQL writes. They enqueue messages into buffered channels.
- Logic workers parse JSON and update the in-memory tag snapshot.
- Discovery workers only use the first KingIOT/KIO full snapshot after connection to create variables. Later incremental push messages update runtime values only and do not run variable discovery.
- The storage bus batches historical data into MySQL only when the variable has a Project assignment and that Project has a running detection task. During EB-037 migration they keep the compatibility narrow write to `rt_history_data` and also write registered `wide_table` routes to the generated Project table. Wide-table writes are grouped by `table/task_id/sample_bucket_ms`, so several variables sampled at the same instant become one upsert row instead of one SQL statement per variable.
- Redis is intentionally not required in this first backend slice. If needed later, add it behind a cache interface for cross-process sharing or dashboard fan-out.

## Runtime Flow

```text
MQTT Gateway
  -> Discovery Channel -> sys_tags discovered variables
  -> Logic Channel -> Logic Workers -> runtime-eligible topic indexed tags -> cleaning filter -> TagManager Map
  -> if Project has running detection task -> StorageBus -> bucket(project_id + table_name) -> rt_history_data insert + project wide-table upsert
  -> if running standard item enables alarm -> Alarm Channel -> MySQL Batch Insert/Recover Update -> Notify Channel -> sys_notifications + WS notification.event
```

Realtime values are normalized before they are exposed to business code. The runtime cleaning filter performs type conversion, scale/offset, quality normalization, suspicious-value filtering, debounce, runtime deadband, and runtime first-frame handling. It only handles known business variables that have been assigned to a Project and enabled; newly discovered unknown variables remain database candidates until assignment. The filter only updates the in-memory tag runtime state and must not query MySQL, calculate limits, or write history directly.

The current project concept is represented by `sys_projects.project_id`. `site_no` is an auxiliary site/project number, not the realtime map key. Project display labels come from `display_name`, `display_name_en`, and `display_name_ja`, with fallback to `name` and `project_code`.

The EB-033 runtime memory layers are:

- `GlobalTagMap`: `var_id -> *Tag`, the only stable realtime value and cleaning state for a variable.
- `GatewayTopicIndex`: `gateway_id + topic -> []*Tag`, the hot-path lookup that avoids scanning all tags on every MQTT message.
- `ProjectRealtimeView`: `project_id -> []var_id`, used to assemble project pages and WebSocket realtime variables.
- `ProjectRunContext`: `project_id -> ActiveRunContext`, used for the active detection task's store/check rules and per-task limit alarm state.

Limits and detection rules do not belong in `GlobalTagMap`. They are read from detection standards and frozen into run context/snapshots. Running business limit alarms are evaluated from `ProjectRunContext` and `detection_run_standard_items`: data changes and the cycle scanner call the same in-memory state machine, then enqueue `DetectionLimitAlarmEvent` records to the async `Alarm` channel. Future running-config updates must update `ProjectRunContext` with audit/business events, not mutate global tag values.

## Backend Module Boundaries

- `internal/runtime/kernel.go` wires startup, middleware, health, auth entrypoints, services, and route registration only.
- `internal/runtime/handlers` owns HTTP parameter parsing, permission binding, status-code mapping, and response DTO shape for users, Projects, variables, history, detection standards/runs, gateways/KIO, and report templates.
- `internal/services` owns business invariants and runtime coordination, such as TagManager reloads after variable changes and TaskManager updates after detection run state transitions.
- `internal/database` keeps one `Repository` type in one package, split by business file for auth, gateways, variables, Projects, history, detection, and reports.
- `internal/models` keeps one package for GORM models and runtime tag DTOs, split by domain file without changing table names, JSON fields, or GORM tags.

## Core Data Model

- `sys_gateways`: MQTT station/source configuration.
- `sys_tags`: variables discovered from the first full snapshot or maintained manually. Key fields are `gateway_id`, `source_path`, `source_type`, `project_id`, `var_group`, `enabled`. `source_type=mqtt` is auto-discovered ingress, `source_type=manual` is manually mapped upstream data, and `source_type=virtual` is a backend/user-created variable with `gateway_id=0`, `discovered=false`, and `placeholder=true`. Auto-discovered unknown variables are inserted as disabled, unassigned candidates; only tags with `enabled=true` and an assigned `project_id` are loaded into the runtime `TagManager` for cleaning, WebSocket snapshots, storage, and detection. Variable storage timing, target table, dynamic column, start snapshot, and change deadband are not variable attributes; they are owned by `sys_storage_routes`. Write constraint fields (`rw_mode`, `writable`, `write_source_id`, `write_path`, `write_data_type`, `write_min`, `write_max`, `write_enum`, `write_requires_audit`) are the only backend-approved basis for future API/WS variable writes. Debounce fields (`suspicious_value`, `debounce_threshold`, `debounce_ms`, `deadband`) are stored in the variable config for runtime filtering. Variable default limit fields (`default_alarm_enabled`, `default_limit_ll`, `default_limit_l`, `default_limit_h`, `default_limit_hh`, `default_limit_deadband`, `default_violation_hold_ms`, `default_recover_hold_ms`) are asset defaults; they do not live in `GlobalTagMap` and do not override detection-standard business limits.
- `sys_storage_routes`: dynamic storage route metadata for variables assigned to Projects. A route owns `storage_target`, target table, dynamic column, column type, form/query aliases, `trigger_mode`, `cycle_ms`, change `deadband`, `store_on_start`, and `enabled`. Default routes are created as disabled suggestions only; users or backend configuration must enable and tune them. The default table is `rt_project_{project_id}_data`, but a user-provided backend-validated table name is also allowed. A variable may have multiple routes, and all route table and column identifiers must be backend-validated and registered here before writes use them.
- `sys_task_rules`: task trigger rules. Rules are indexed by `trigger_var_id` in memory (`tag_id -> rule_ids`) so data-change handling evaluates only rules that depend on the changed variable. The first implementation supports numeric single-variable conditions and records the matched action type; action execution is intentionally kept out of the MQTT hot path and must call services in later iterations.
- `sys_task_flows`: developer-authored conditional event tasks. A task flow belongs to one project, declares `trigger_type`, optional JavaScript condition, `action_type`, script/action payload, optional `steps_json`, timeout, cooldown, hold time, and priority. `steps_json` is the preferred platform model for multi-step built-in modules; old `action_type/action_payload/action_script` remains a single-step compatibility path. The API contract recommends an ordered array, and the executor also accepts a single step object as one-step shorthand for developer tooling tolerance. Data-change tasks are not scanned globally; they are reached through `sys_task_flow_vars`. Built-in modules currently cover storage snapshot/prepare, context set, JavaScript, controlled variable write, detection start/stop/pause/resume, fixed-duration stop guard, qualified-hold stop guard, detection alarm mute, running limit update, feature refresh, report result registration, and controlled HTTP request. Formal business parameters must come from watched `STRING` virtual variables whose values are JSON objects. Step parameter bindings support `literal`, `trigger_param`, `event`, and `context` sources, plus `optional=true` and `default` so templates can represent optional parameters without failing when a key is omitted. JavaScript can read multiple realtime variables through `realtime.get`, `realtime.getMany`, `realtime.getByName`, and `realtime.project`; `realtime.get/getMany/write` accept decimal-string 64-bit variable IDs and return `var_id_text` in snapshots/write results. The JavaScript `trigger` object and task-flow run context also expose `trigger_var_id_text`. JavaScript writes and `builtin.write_variable` are limited to virtual variables and write audit rows, with `trigger=false` by default for JavaScript. Physical variable downlink must use WS/HTTP `VariableWriteService + KIOWriteService`, not the task executor's in-memory write path. STRING task parameter payloads are capped at 256 KiB; oversized or invalid JSON is rejected by the task module instead of being treated as a valid command. The 520-item custom detection payload budget is currently about 166 KiB, with millisecond-level parse/convert cost, so it remains a low-frequency command payload rather than a realtime data stream.
- `sys_task_flow_vars`: task-to-variable references. Runtime builds `var_id -> flow_ids` from rows with role `watch`, so a changed variable only evaluates tasks that depend on it. Roles `read` and `write` document script access intent for UI and review.
- `task_flow_runs`: execution records for task flows. Each run stores trigger metadata, origin/depth recursion metadata, input snapshot, result JSON, error text, script logs, duration, and status. The executor builds a per-run context and passes values between built-in modules through that context instead of using global temporary state.
- `task_flow_sql_logs`: SQL statements executed by developer scripts through the shared repository connection. Scripts may use `db.query` and `db.exec`, but they do not create their own database connection and each statement is logged with duration and error text.
- `sys_projects`: virtual Project groups. These drive auto-expanded pages and detection tasks. `name` remains the legacy/default name; UI-facing labels should use `display_name`, `display_name_en`, and `display_name_ja` with fallback to `name` or `project_code`.
- `sys_project_members`: local edge Project membership/follow records. Each row links one local `sys_users` account to one Project, stores a Project-local `member_role`, and controls whether that user receives Project-scoped notifications through `notify_enabled`. Notification targeting uses this table only when a Project has configured members; Projects without configured members temporarily keep the old all-enabled-users fallback so existing deployments do not silently lose detection notifications before the UI config is filled.
- `sys_detection_standards`: reusable detection standard headers. A standard defines which Project/mode it applies to, carries a version, and may reference a default Excel report template through `report_template_id`.
- `sys_detection_standard_favorites`: per-user favorite detection-standard links for the configuration UI. Favorites are query/configuration state, not a live control action.
- `sys_detection_standard_recents`: recently used detection standards. Task-system detection start records recent usage when it starts from a `standard_id`; `user_id=0` represents system/task-flow usage when no local UI user is attached.
- `sys_detection_standard_items`: reusable per-variable detection rules under one standard. `check_enabled` controls pass/fail evaluation participation; `alarm_enabled` controls whether a detected violation should create a limit-alarm lifecycle record; `store_enabled` controls whether the variable may use enabled storage routes during a detection run. `check_cycle_ms` controls the business evaluation cadence and is separate from route storage cadence; `0` no longer inherits variable storage fields. `check_on_start` controls whether detection start should immediately evaluate the current value. Numeric limits are `limit_ll`, `limit_l`, `limit_h`, and `limit_hh`; check semantics are described by `check_method`, `target_value`, `limit_deadband`, `violation_hold_ms`, `recover_hold_ms`, and `quality_policy`.
- `sys_detection_tasks`: one running or paused task per Project. It stores run metadata such as whether this run enables limit checking (`limit_check_enabled`), the end policy (`manual`, `fixed_duration`, or `qualified_hold`), planned duration, qualified hold duration, expected end time, pause start time, accumulated paused duration, stop type, operator note, optional custom-config JSON, and the frozen report template identity used by that run. `custom_config_json` is compact trace metadata for custom starts, not the run-time judgement source; custom item snapshots are still frozen into `detection_run_standard_items`. Task-flow starts may also freeze low-frequency process/report parameters such as `process_params.inlet_area_m2` and the matching `plc_writes` trace so the same value can be used for report calculations and for a controlled PLC write step without duplicating frontend input. `status=paused` removes the run from runtime storage/alarm evaluation until it is resumed; it does not delete the run snapshot. Paused time does not count toward accumulated detection duration: resume shifts `expected_end_at` by the pause delta, and `detection_run_summaries.duration_ms` subtracts `paused_duration_ms` plus the current open pause segment. Running tasks loaded on backend startup recover fixed-duration and qualified-hold guards from this row.
- `detection_run_standard_items`: immutable standard item snapshots copied when a detection run starts. It freezes the check/alarm/store flags, `check_cycle_ms`, `check_on_start`, limit config, check semantics, the variable default limit snapshot (`variable_default_*`) for later judgement. Task-flow custom detection starts can also freeze `custom_items` directly into this table with `standard_id=0/standard_item_id=0`; this is still the single run-time source of truth for judgement. Later judgement should read this table, not the latest standard or variable configuration. `PATCH /variables/{id}` may sync only the `variable_default_*` snapshot for currently running tasks when `apply_to_running=true`; it must not overwrite the business limit fields.
- `detection_run_storage_routes`: immutable storage route snapshots copied when a detection run starts. Startup still ensures the disabled default route suggestion exists for each eligible variable, but the run snapshot freezes every enabled route for that variable, including user-created custom routes. It freezes each enabled route's target, Project table, dynamic column, column type, form/query aliases, trigger mode, cycle, deadband, and start-storage flag. Detection start prepares the Project-wide table and missing dynamic columns from this snapshot before the run becomes available to the storage hot path. `ActiveTask` and `StoreTask` carry these route snapshots so the store worker can write without querying route metadata on every sample.
- `detection_run_notes`: multiple memo/remark rows attached to one detection run.
- `detection_run_events`: lightweight event timeline rows for one detection run. It records run lifecycle events such as start, normal stop, and abnormal stop. Alarm details stay in `detection_limit_alarms` to avoid duplicating high-volume per-variable alarm rows.
- `detection_run_summaries`: one aggregate row per detection run. It stores result status, duration, history row count, alarm totals, active/recovered counts, above/below limit counts, first/last alarm time, and the last refresh time.
- `detection_run_features`: per-run numeric feature rows grouped by variable. The first slice stores `sample_count`, `avg_value`, `min_value`, `max_value`, `first_sample_time`, and `last_sample_time`, calculated from persisted history rows after a run stops or when explicitly refreshed. API responses include `var_id_text` for exact browser round-tripping.
- `detection_limit_alarms`: unified limit alarm lifecycle records. `scope=detection` rows represent one variable's violation inside a detection run and keep task, standard item, variable display, limit, quality, status, and duration snapshots. `scope=default` rows represent variable-asset default alarms, use `task_id=0`, do not use detection mute, and are driven from `sys_tags.default_*` fields. Both scopes share the `Alarm` queue and notification path; detection summaries continue to aggregate by `task_id`, so default rows do not affect run OK/NG summaries. `GET /api/v1/limit-alarms` is the unified read model for both scopes and returns `var_id_text`.
- `sys_notifications`: edge notification bodies persisted from `Channels.Notify`. Each row stores a stable event UID, type, level, target scope (`all`, `user`, `role`, or `project`), Project/task/variable context, message, JSON payload, and occurrence time. These rows support UI notification history but do not replace domain source tables such as `detection_run_events`, `detection_run_summaries`, or `detection_limit_alarms`.
- `sys_notification_recipients`: per-user read state for persisted notifications. Recipient generation now honors `target_type=all|user|role|project`: `all` fans out to all enabled local edge users, `user` targets one enabled local user ID, `role` targets enabled local users with that role, and `project` targets enabled users listed in `sys_project_members` with `notify_enabled=true`. If a Project has no configured member rows, project notifications fall back to all enabled local users for transition safety. Each recipient row stores its own `read_at`.
- Runtime notifications: edge events carried by `Channels.Notify`, persisted by `NotificationDispatcher`, and fanned out online by `NotificationHub`. Current notification types include `alarm.limit.enter`, `alarm.limit.recover`, `alarm.limit.level_change`, `detection.run_started`, `detection.run_stopped`, `detection.run_abnormal_stop`, `detection.run_paused`, `detection.run_resumed`, `detection.result_ok`, `detection.result_ng`, and `detection.features_updated`.
- `sys_report_templates`: registered Excel report templates. The database stores metadata and file references, not file binary content.
- `detection_run_reports`: generated report file records attached to one detection run.
- `rt_history_data`: legacy/compatibility narrow history table with `project_id`, `task_id`, and `test_no`.
- `rt_project_{project_id}_data`: generated Project-wide tables for this project. Fixed columns include `task_id`, `test_no`, `project_id`, `project_code`, `sample_time`, and `sample_bucket_ms`; dynamic columns are added from `detection_run_storage_routes`. DDL is a low-frequency startup/config action and must not run in the MQTT write hot path.
- `sys_users`: local edge users with bcrypt password hashes, role, enabled flag, and `permissions_version`.
- `sys_service_clients`: main-server service credentials. Only token hashes are stored; scopes control service calls.
- `sys_sso_tickets`: one-time SSO handoff tickets. Only ticket hashes are stored and successful verification marks `used_at`.
- `sys_audit_logs`: security and write-action audit records. Sensitive token/password values must not be stored in audit detail.

## Basic API Surface

API naming rules for this project:

- Prefix business APIs with `/api/v1`.
- Use short business nouns: `variables`, `projects`, `gateways`, `detection-runs`.
- Use `PATCH` for assignment/configuration changes.
- Use explicit command endpoints only for industrial control transitions such as stopping a run.

Current endpoints:

- `POST /api/v1/auth/login`: public local-user login, returns `access_token`, user, and permissions.
- `GET /api/v1/auth/me`: authenticated local-user profile and permissions.
- `POST /api/v1/auth/logout`: authenticated local-user logout acknowledgement.
- `POST /api/v1/auth/sso-ticket`: authenticated local-user SSO ticket creation, requires `sso_handoff`.
- `POST /api/v1/auth/sso-ticket/verify`: main-server service verification endpoint, requires `service_sso_verify` service scope.
- `GET /api/v1/variables`: all configured and discovered variables.
- `POST /api/v1/variables`: create a virtual variable (`source_type=virtual`) or a manually mapped variable (`source_type=manual`). Virtual variables support `INT`, `FLOAT`, `BOOL`, and `STRING`, use `gateway_id=0`, and become visible in realtime snapshots only after they are enabled and assigned to a Project.
- `PATCH /api/v1/variables/{variable_id}`: update variable attributes such as display name, type, unit, scale, cleaning fields, write constraints, default limit/alarm fields, and enabled state. Variable storage cadence and table/column routing are not changed here. Optional `apply_to_running=true` syncs the variable default limit snapshot into current running task items for the same variable without changing business standard limits. A missing variable returns `404`.
- `POST /api/v1/variables/bulk-remap/kio-projects`: batch helper for KingIOT/KIO local commissioning. It ensures project codes such as `AC-01` through `AC-12`, maps raw names like `台1_39` to project 1, keeps `raw_name/source_path` unchanged, writes business `var_name` such as `kio_01_39`, fills `display_name/display_name_en/display_name_ja`, enables matched variables by default, creates disabled default storage-route suggestions, reloads `TagManager`, and returns per-variable remap evidence including `var_id_text`. If an existing default storage route is still disabled, its generated column metadata is synchronized to the new business `var_name`; enabled routes are left unchanged. Request options include `project_count`, `project_code_prefix`, display prefixes, `raw_project_prefix`, `var_group`, `var_name_prefix`, `remap_var_name`, `enable`, and `dry_run`.
- `GET/POST/PATCH/DELETE /api/v1/storage-routes`: list and manage route-owned storage configuration. Routes are where users choose the target table, column, trigger mode, cycle, change deadband, start snapshot behavior, and enabled state. Request `var_id` may be a JSON string when the frontend needs exact 64-bit IDs, and responses include `var_id_text` for exact reuse. Missing routes return `404`, and routes already frozen into a detection run return `409 Conflict` on delete.
- `GET /api/v1/task-modules`: list backend-supported task modules for the frontend dynamic task editor. Requires `system_settings`. Current modules include detection start/stop/pause/resume, fixed-duration guard, qualified-hold guard, alarm mute, running limit update, feature refresh, storage snapshot/prepare, controlled variable write, report registration, controlled HTTP request, context set, and JavaScript. Detection start declares `custom_items`, `limit_check_enabled`, `end_policy`, `duration_sec`, `qualified_hold_ms`, `process_params`, and `plc_writes` so future editors can bind the full watched STRING task request variable: detection config, process/report parameters, and controlled PLC write traces. `builtin.refresh_features` refreshes `detection_run_features`, writes a `features_updated` detection event, and publishes `detection.features_updated` through the notification bus. The JavaScript module schema also exposes its runtime API names so the frontend can render developer help without hard-coding backend capabilities; runtime variable helpers accept string IDs and return exact `var_id_text` fields.
- `GET /api/v1/task-flow-templates`: list backend-provided task-flow templates for the frontend task editor. Requires `system_settings`. Templates now cover variable-request start/stop, fixed-duration detection, qualified-hold detection, pause, resume, storage snapshot, alarm mute, running limit update, feature refresh, report registration, and controlled write-variable command. The start, fixed-duration, and qualified-hold detection templates all place `builtin.write_control_variables` before `builtin.start_detection_run` when `plc_writes` is present, so process parameters such as inlet area can be written to PLC before the run snapshot is created. These templates also pass `operator_note` and `report_template_id` through to the detection start module.
- `GET/POST/PATCH /api/v1/task-flows`: developer task-flow configuration. Requires `system_settings`. Task flows support data-change/manual/project lifecycle/schedule triggers. Automatic execution currently covers data-change, project lifecycle events, and schedule flows. A detection run start emits `project_start`; normal, task-flow, or abnormal stop emits `project_end`. `schedule_interval_ms` controls scheduled execution frequency; if it is omitted but `cooldown_ms` is set on a schedule flow, the backend temporarily uses `cooldown_ms` as the interval for compatibility. A `schedule` flow must have either `schedule_interval_ms > 0` or `cooldown_ms > 0`; otherwise the schedule scanner would never execute it. `timeout_ms` may be omitted on create to use the backend default, but negative timing values are invalid, and patching `timeout_ms` requires a value greater than 0. Manual run remains developer debugging. Payload may include `steps_json`, normally an ordered array of `{code,module,params,script}` steps; the backend also accepts a single step object and treats it as a one-step flow, but an explicit empty array is invalid because it would create a no-op task. Explicit step `code` values must be unique because step results are stored in the run context by code. Formal business parameters must come from a watched `STRING` virtual variable whose changed value is a JSON object. `data_change` flows must bind at least one `watch` variable because only watch variables enter the `var_id -> flow_ids` runtime index. Parameter bindings support only `literal`, `trigger_param`, `event`, and `context` sources; `event` keys are limited to `trigger_type`, `project_id`, `trigger_var_id`, `trigger_var_id_text`, `trigger_value`, `gateway_id`, `topic`, `origin_flow_id`, `origin_run_id`, `depth`, `request_id`, and `at`. Create requires `project_id > 0`, non-empty `flow_code`, and non-empty `name`; patch rejects zero `project_id` and blank `flow_code/name` when those fields are supplied. Create, patch, and list reject invalid task-flow enums early: `trigger_type`, `action_type`, invalid timing fields, `vars.role`, `vars.var_id <= 0`, `data_change` without a watch var, `schedule` without an effective interval, empty `steps_json` arrays, duplicate explicit `steps_json[].code`, every `steps_json[].module`, every explicit `steps_json[].params.*.source`, and every explicit `event` key must use backend-supported values, otherwise the API returns `400`.
- `GET /api/v1/task-flows/{id}`: get one task-flow configuration with variable bindings for editor detail/refresh.
- `DELETE /api/v1/task-flows/{id}`: delete one task-flow configuration and its variable bindings. Existing `task_flow_runs` and `task_flow_sql_logs` are retained for audit/history, and the in-memory task-flow index is reloaded after deletion.
- `POST /api/v1/task-flows/{id}/run`: manually enqueue one task flow for developer debugging only. It does not carry formal business parameters; normal business entry should write a `STRING` virtual variable and let data-change trigger the task flow. Execution is asynchronous and recorded in `task_flow_runs`.
- `GET /api/v1/task-flow-runs`: list task-flow execution records. Requires `system_settings`; supports project, flow, trigger, status, origin, time, limit, and offset filters. `status` is restricted to `pending|running|success|failed|timeout|skipped`, `trigger_type` is restricted to `manual|data_change|schedule|project_start|project_end`, `project_id`/`flow_id`/`trigger_var_id`/`origin_flow_id` must be positive when present, `limit` must be positive, `offset` must be non-negative, and `from` must be before or equal to `to`; invalid enum, id, time-range, or paging values return `400` instead of an empty list.
- `GET /api/v1/task-flow-runs/{id}`: get one task-flow execution record. Missing runs return `404`.
- `GET /api/v1/task-flow-runs/{id}/sql-logs`: list SQL statements executed by one task-flow run through the shared repository connection. Optional `limit` must be positive; invalid values return `400`, and a missing run returns `404` instead of an empty log list.
- `DELETE /api/v1/variables/{variable_id}`: delete a wrongly discovered variable. A missing variable returns `404`.
- `PATCH /api/v1/variables/{variable_id}/assignment`: assign a variable to a virtual Project and enable it. A missing variable or Project returns `404`.
- `GET /api/v1/realtime/variables`: current values for enabled variables. It supports `source_type`, `gateway_id`, `project_id`, legacy alias `device_id`, and repeated or comma-separated `var_id`. No filter means an intentional full snapshot. Project pages should send `project_id`; point inspectors should send one or more `var_id` values.
- `GET /api/v1/ws`: WebSocket realtime, notification, and command channel. It requires an Edge user JWT with `view_realtime`; browser clients may pass the token as `access_token` query parameter because the native WebSocket API cannot set `Authorization`. Query filters are `topic=realtime.variables|detection.runs|notifications`, `source_type`, `gateway_id`, `project_id`, and repeated or comma-separated `var_id`. Runtime `subscribe` messages also accept `var_ids` as JSON numbers or decimal strings; `connection.ready` and `subscription.updated` include `var_id_texts` beside numeric `var_ids` so browser clients can keep exact 64-bit IDs. No `project_id` or `var_id` means an intentional full realtime snapshot; UI pages should avoid that unless they are global dashboards. Server messages use an envelope with `type`, `request_id`, `command_id`, `at`, `payload`, and `error`; read messages include `connection.ready`, `subscription.updated`, `realtime.variables.snapshot`, `detection.runs.snapshot`, `notification.event`, `heartbeat`, and `error`. `notification.event` payload is a `RuntimeNotification` with `id/type/level/target_type/target_id/project_id/task_id/var_id/var_id_text/message/payload/occurred_at`; it is emitted for limit-alarm enter/recover/level_change and detection lifecycle/result events. The realtime snapshot path uses the project index for `project_id` subscriptions and direct tag lookup for `var_id` subscriptions, so single/multi-point views do not first assemble the full tag list. The server sets a 32 KiB read limit, read/write deadlines, ping/pong keepalive, and treats reconnects as stateless: clients should reconnect and resubscribe, then expect a fresh ready message and snapshot. Supported commands are `command.detection.start`, `command.detection.stop`, `command.detection.abnormal_stop`, and `command.write_variable`; each command must include `request_id`, `command_id`, and a `payload`, and writes `sys_audit_logs`. Detection commands run through `DetectionRunsService`. Variable writes require `kio_write` permission, run through `VariableWriteService`, update virtual variables in memory, and publish physical writes through `KIOWriteService` only when `writable=true`, `rw_mode=W|RW`, and `write_path` are configured; `command.write_variable` acks include `result.var_id_text`. Unsupported command types return `error.code=unsupported_command`.
- `GET /api/v1/history/data`: persisted history samples, requires `view_history`, supports `project_id`, `task_id`, `project_code`, `test_no`, `start`, `end`, and `limit` query filters. When `task_id`, `project_id`, or `test_no` matches `detection_run_storage_routes`, the repository first reads `rt_project_{project_id}_data` and reconstructs the existing `HistoryData` DTO; otherwise it falls back to the compatibility table `rt_history_data`. Response rows include `var_id_text`.
- `GET /api/v1/projects`: virtual Project groups, including `display_name`, `display_name_en`, and `display_name_ja`.
- `POST /api/v1/projects`: create a virtual Project group. Request accepts `name` for compatibility and the UI-facing `display_name`, `display_name_en`, `display_name_ja`; at least `name` or `display_name` is required.
- `PATCH /api/v1/projects/{id}`: update a virtual Project group, including UI-facing display names. A missing Project returns `404`.
- `GET /api/v1/projects/{id}/members`: list local edge users attached to one Project for notification targeting. Requires `manage_users`; response is `{items,count}` with `user_id`, `username`, `user_role`, Project-local `member_role`, and `notify_enabled`.
- `PUT /api/v1/projects/{id}/members`: replace one Project's member list. Requires `manage_users`; request body is `{members:[{user_id,member_role,notify_enabled}]}`. Duplicate `user_id` returns `400`; missing Project or user returns `404`.
- `GET /api/v1/detection-standards`: list reusable detection standards.
- `GET /api/v1/detection-standards/favorites`: list the current user's favorite detection standards.
- `GET /api/v1/detection-standards/recent`: list recently used detection standards; supports `project_id` and `limit`.
- `POST /api/v1/detection-standards/{id}/favorite`: favorite one detection standard for the current user. A missing standard returns `404`.
- `DELETE /api/v1/detection-standards/{id}/favorite`: remove the current user's favorite link. A missing standard returns `404`; removing a non-favorited existing standard is idempotent.
- `POST /api/v1/detection-standards`: create a detection standard, optionally with items. Item `var_id` may be a JSON string when the frontend needs exact 64-bit IDs. Item payloads support `alarm_enabled`, `check_cycle_ms`, `check_on_start`, `check_method`, `target_value`, `limit_deadband`, `violation_hold_ms`, `recover_hold_ms`, and `quality_policy`; defaults are `alarm_enabled=true`, `check_cycle_ms=0`, `check_on_start=true`, `numeric_range`, and `ignore_bad`.
- `GET /api/v1/detection-standards/{id}`: get one detection standard with items. Items include `var_id_text` so browser clients can round-trip exact IDs without relying on JavaScript number precision.
- `PATCH /api/v1/detection-standards/{id}`: update one detection standard header. A missing standard returns `404`.
- `PUT /api/v1/detection-standards/{id}/items`: replace all items under a standard and increment its version. A missing standard returns `404`; invalid `check_method`, invalid `quality_policy`, negative deadband, or negative hold times return `400`.
- `DELETE /api/v1/detection-standards/{id}`: delete one unused detection standard and its config items. A missing standard returns `404`; referenced standards return `409 Conflict` when they have run snapshots.
- `GET /api/v1/report-templates`: list registered Excel report templates.
- `POST /api/v1/report-templates`: create one Excel report template metadata record.
- `PATCH /api/v1/report-templates/{id}`: update one Excel report template metadata record. A missing template returns `404`.
- `DELETE /api/v1/report-templates/{id}`: delete one unused template. A missing template returns `404`; referenced templates return `409 Conflict` when referenced by standards, tasks, or generated reports.
- `GET /api/v1/gateways`: MQTT gateway status.
- `GET /api/v1/gateway-configs`: all MQTT/KIO source configurations.
- `GET /api/v1/gateway-configs/{gateway_id}`: one MQTT/KIO source configuration.
- `POST /api/v1/gateway-configs`: create one MQTT/KIO source and start it when enabled.
- `PATCH /api/v1/gateway-configs/{gateway_id}`: update one MQTT/KIO source and restart/stop it according to `enabled`. A missing gateway config returns `404`.
- `DELETE /api/v1/gateway-configs/{gateway_id}`: delete one MQTT/KIO source and stop it. A missing gateway config returns `404`.
- `POST /api/v1/gateway-configs/{gateway_id}/discover`: request a full upstream variable push.
- `POST /api/v1/gateways/{gateway_id}/publish`: publish raw MQTT payload through one connected gateway.
- `POST /api/v1/gateways/{gateway_id}/subscribe`: subscribe an extra MQTT topic through one connected gateway.
- `POST /api/v1/gateways/{gateway_id}/kio/write`: build and publish a KingIOT/KIO write payload.
- `POST /api/v1/gateways/{gateway_id}/kio/query-all`: publish a KingIOT/KIO all-tag query request.
- `GET /api/v1/runtime/channels`: worker queue depth.
- `GET /api/v1/system/database-config`: read editable MySQL history database configuration from the backend config file. Requires `system_settings`; password is redacted and represented by `password_set`.
- `PATCH /api/v1/system/database-config`: save MySQL history database configuration back to the backend config file. Requires `system_settings`; current running DB connection is not hot-swapped and the response includes `restart_required=true`.
- `POST /api/v1/system/database-config/test`: test a MySQL configuration before saving. Requires `system_settings`.
- `GET /api/v1/audit-logs`: list backend audit records from `sys_audit_logs`. Requires `system_settings`; supports `actor_type`, `actor_id`, `action`, `target_type`, `target_id`, `result`, `from`/`created_from`, `to`/`created_to`, `limit`, and `offset`. The response is `{items,total,limit,offset}` and never adds request/response bodies to audit details.
- `GET /api/v1/notifications`: list the current user's persisted edge notifications. Requires `view_realtime`; supports `unread`, `type`, `level`, `project_id`, `limit`, and `offset`, and returns `{items,total,limit,offset}`. `items[].payload` is returned as a JSON object, and variable-scoped notifications include `var_id_text`.
- `GET /api/v1/notifications/unread-count`: return `{unread}` for the current local user. Requires `view_realtime`.
- `POST /api/v1/notifications/{id}/read`: mark one notification read for the current user. Requires `view_realtime`; returns `404` if that notification is not addressed to the user.
- `POST /api/v1/notifications/read-all`: mark all unread notifications read for the current user. Requires `view_realtime`.
- `GET /api/v1/limit-alarms`: list unified limit alarm lifecycle rows. Requires `view_realtime`; supports `scope=default|detection`, `project_id`, `task_id`, `test_no`, `var_id`, `status`, `alarm_type`, `level` or `alarm_level`, `from`, `to`, `limit`, and `offset`; `from/to` filter `first_seen_at`, and the response is `{items,total,limit,offset}` with `items[].var_id_text`.
- `GET /api/v1/detection-runs`: list detection runs by `project_id`, `status`, `test_no`, `start`, `end`, and `limit`.
- `GET /api/v1/detection-runs/current?project_id=`: get the current running or paused run for one Project, including frozen `standard_items`, `storage_routes`, recent notes, and report records. This is the preferred HTTP companion for frontend realtime OK/NG rendering because it returns the run snapshot instead of the latest editable standard.
- `GET /api/v1/detection-runs/{id}`: get one run with standard item snapshot, recent notes, and report records.
- `GET /api/v1/detection-runs/{id}/summary`: get or refresh the aggregate summary for one run. The backend derives it from the task row, history rows, and `detection_limit_alarms`.
- `GET /api/v1/detection-runs/{id}/features`: get or refresh numeric feature rows for one run. The backend derives average/min/max/sample-count values from persisted history rows grouped by variable and returns `var_id_text`.
- `GET /api/v1/detection-runs/{id}/events`: list lifecycle events for one run. This is intentionally lightweight; per-variable limit alarm details remain in `detection_limit_alarms`.
- `GET /api/v1/detection-runs/{id}/storage-routes`: list the immutable storage route snapshot for one run. The response is `{items,count}` and each item shows the actual target table, dynamic column, trigger mode, cycle, deadband, `store_on_start`, and `var_id_text` used by that run.
- `POST /api/v1/detection-runs`: legacy/direct HTTP start endpoint for one Project. The preferred live business entry is a STRING virtual variable that triggers `builtin.start_detection_run`; HTTP remains useful for compatibility and developer smoke. Optional `standard_id` freezes that standard's items and each item's current variable storage mapping into `detection_run_standard_items`; optional `duration_sec`, `operator_note`, and `report_template_id` are stored on the run. The backend locks the Project row and returns `409 Conflict` when the same Project already has a running task.
- `POST /api/v1/detection-runs/{id}/stop`: stop storage for one Project with `end_type=manual_stop`.
- `POST /api/v1/detection-runs/{id}/abnormal-stop`: stop storage for one Project with `end_type=abnormal_stop`; `reason` is required.
- `POST /api/v1/detection-runs/{id}/pause`: pause one running detection run. Runtime storage and limit-alarm evaluation stop because the run is removed from `TaskManager`; persisted snapshots remain. The backend records `pause_started_at`.
- `POST /api/v1/detection-runs/{id}/resume`: resume one paused detection run and reload its standard/storage snapshots into `TaskManager`. The backend accumulates the pause segment into `paused_duration_ms` and shifts `expected_end_at` so fixed-duration runs do not count paused time.
- `GET /api/v1/detection-runs/{id}/notes`: list run notes.
- `POST /api/v1/detection-runs/{id}/notes`: append a run memo/remark.
- `GET /api/v1/detection-runs/active`: active Project runs.

`/health` remains public. Other `/api/v1` business endpoints require an Edge user JWT and route-level capability checks. Main-server service identity uses Bearer service tokens and service scopes, not local user roles.

Shared service-backed handlers map missing database rows to `404`, reference conflicts and duplicate running Project tasks to `409`, and validation errors to `400`. Handlers that need narrower semantics may still override this mapping locally.

## Audit And Runtime Logs

- Auth flows write explicit audit rows for login, logout, SSO ticket creation, and SSO verification.
- Protected HTTP write requests (`POST`, `PATCH`, `PUT`, `DELETE`) are audited by runtime middleware after route handling.
- HTTP write audit rows use `action=http.<method>`, `target_type=http_endpoint`, and `target_id` as the Gin route pattern, for example `/api/v1/projects`.
- Audit detail stores only metadata: `request_id`, optional `command_id`, method, path, route, status, client IP, user agent, latency, actor display name, and a short error summary. Request/response bodies are intentionally not stored, so passwords, KIO credentials, database credentials, tokens, and report file contents are not persisted in `sys_audit_logs`.
- `GET /api/v1/audit-logs` provides a paged read model for the settings/audit UI. It is restricted to `system_settings` and caps `limit` at 200.
- `X-Request-ID` is echoed for audited write requests. If the caller does not provide one, the backend generates a request ID. WebSocket write commands must carry `request_id`/`command_id` and reuse the same audit model.

## KingIOT/KIO Parser

The JGHJ KingIOT/KIO rule has been moved into `internal/protocol/kio`.

- Read: variables are discovered from the first full snapshot's `Objs[].N`; values default to `Objs[].1`, and quality defaults to `Objs[].3`.
- Fallback: if an object exists but does not contain the requested property, `PVs.{property}` is used as the shared value.
- Quality: raw quality code `192` maps to internal quality `1`; other raw values map to `0`.
- Data change topic: `datachange_{client_id}`. The project may configure this topic on the upper-computer side.
- Write topic: `setdata_{client_id}`. `KIOWriteService` builds the KIO write payload with `Writer`, `WriteTime`, `Qid`, `PNs`, `PVs`, and `Objs`; `Username` and `Password` are optional compatibility fields only. HTTP KIO write and WS `command.write_variable` reuse this service so publish/ack handling stays behind the backend.
- Write result topic: `setdata_result_{client_id}_{writer}`. A write is fully successful only when a matching `Qid` returns `ProcessStep=100` and `Result=ok`; `received`, `doing`, `some ok`, and `all error` are not treated as full success.
- Generated write `Qid` values stay below `1_000_000_000`. KingIO developer tests showed very large nanosecond IDs can publish successfully but fail to match the returned ack, while small explicit/generated IDs return matching `ProcessStep=100` acks.
- Query-all topic: `Query_AllKIOTags_{client_id}`. It is used to actively request the full tag list; the response is pushed through the configured data-change topic.

KIO source configuration belongs in `sys_gateways`, not in each write request:

- MQTT connection: `broker`, `client_id`, `username`, `password`, `topic`, `qos`.
- KIO identity and topics: `kio_client_id`, `kio_writer`, `setdata_topic`, `write_result_topic`, `query_all_topic`.
- KIO write credentials: `kio_write_username`, `kio_write_password`.

The local KIO test proved that MQTT connection credentials and KIO write credentials are different concerns. The EMQX broker accepted empty credentials and `Admin/admin`, but KIO down-write confirmation only returned when the write payload used `kio_write_username=sa` and `kio_write_password=C12E01F2A13FF5587E1E9E4AEDB8242D`.

Tested KIO write results on 2026-05-28:

- `台1_39` IOFloat: wrote `13.39`, ack `ProcessStep=100`, `Result=ok`.
- `台1_40` IODisc: wrote `true`, ack `ProcessStep=100`, `Result=ok`.
- `台1_41` IOFloat: wrote `41.66`, ack `ProcessStep=100`, `Result=ok`.
- `台1_42` IOString: wrote `edge-string-sa`, ack `ProcessStep=100`, `Result=ok`.

Therefore `ProcessStep=100` plus `Result=ok` means KIO completed the down-write. It is stronger than broker publish success and stronger than merely receiving the MQTT command.

Follow-up write verification on 2026-05-30:

- Direct KIO write without explicit `Qid`: `台1_39=13.42`, generated `Qid=798217001`, ack `ProcessStep=100`, `Result=ok`.
- WS `command.write_variable`: `var_id=5826452156569908253` (`台1_39`) wrote `13.43`, generated `Qid=994994001`, ack `ProcessStep=100`, `Result=ok`, and realtime memory later showed `13.430000305175781`.
- HTTP KIO multi-type write: `台1_40=true`, `台1_41=41.67`, and `台1_42=edge-ws-string-1450` all returned `ProcessStep=100` and `Result=ok`.
- WS `command.write_variable` multi-type write: `台1_40=false`, `台1_41=41.68`, and `台1_42=edge-ws-string-1452` all returned confirmed KIO acks. Realtime memory later showed the BOOL, FLOAT, and STRING values.
- Discovery refresh must not overwrite manually configured write fields (`rw_mode`, `writable`, `write_path`, `write_data_type`, write range, and audit flags) on already assigned business variables. It only refreshes source metadata for existing tags and keeps new unassigned candidates read-only.
- After a `query-all` refresh, `台1_39` through `台1_42` kept their manually configured write fields. A WS write outside the configured range (`台1_41=150`, `write_max=100`) returned `command_failed` before KIO publish, wrote a failed `ws.command.write_variable` audit row, and left the realtime value unchanged at `41.68000030517578`.

Project initialization now seeds this KingIOT/KIO source as the default gateway:

- `broker=tcp://127.0.0.1:1883`
- MQTT `client_id=edge-local-kio`, `username=Admin`, `password=admin`
- subscribe topic `datachange_S_KIO_Project`, `qos=2`
- `parser_type=kingiot_kio`
- `kio_client_id=S_KIO_Project`, `kio_writer=edge-test`
- `kio_write_username=sa`, `kio_write_password=C12E01F2A13FF5587E1E9E4AEDB8242D`
- `setdata_topic=setdata_S_KIO_Project`
- `write_result_topic=setdata_result_S_KIO_Project_edge-test`
- `query_all_topic=Query_AllKIOTags_S_KIO_Project`

## Local Run

1. Create MySQL database and tables:

```sql
SOURCE deploy/schema.sql;
```

2. Adjust `configs/config.json`.

3. Run the backend:

```powershell
go mod tidy
go run ./cmd/edge-backend
```

4. Health check:

```powershell
Invoke-RestMethod http://127.0.0.1:18080/health
```

## Backend Smoke Tools

These smoke tools require a running backend, valid MySQL config, and the local test account from `AI_BOARD.md`.

```powershell
go run -tags smoke_tools ./tools/smokehealth
go test -tags smoke_tools ./tools/smokehealth
go run -tags smoke_tools ./tools/smokeeb017
go run -tags smoke_tools ./tools/smokeeb045
go test ./internal/pipeline -run TestProcessMessageKIOPayloadUpdatesOnlyIndexedKnownTags -count=1
```

- `smokehealth` verifies backend readiness for frontend testing: `/health`, login, `auth/me`, Projects, Project members, Variables, Runtime Channels, Gateway Configs, task modules/templates/runs, notification unread count, active detection runs, and the intentionally removed legacy `/api/v1/devices=404` contract.
- `smokeeb017` verifies multi-Project detection isolation, virtual variable writes, and history ownership.
- `smokeeb045` verifies the formal watched `STRING` task request path: WS writes JSON into a request variable, task-flow data-change triggers run, `custom_items` are frozen into run standard snapshots, `process_params/plc_writes` are frozen, storage routes are prepared through `builtin.storage_prepare`, running limits are updated through `builtin.update_detection_limits`, active detection alarms are muted through `builtin.mute_detection_alarms` with `muted=1`, report outputs are registered through `builtin.register_report`, controlled HTTP calls run through `builtin.http_request`, pause/resume work, manual task-flow stop works, feature refresh writes a `features_updated` event and publishes `detection.features_updated` through both WS notification events and persisted HTTP notifications, fixed-duration and qualified-hold runs stop automatically with the expected end types, run lifecycle notifications (`detection.run_started`, `detection.run_paused`, `detection.run_resumed`, `detection.run_stopped`, and `detection.run_abnormal_stop`) are visible through the same WS/HTTP unread/read path, `detection.result_ok` notifications are visible through that path, and a start-frame above-H run produces `alarm.limit.enter`, the current active alarm can be muted, a later above-HH value still produces `alarm.limit.level_change`, a return-to-qualified value produces `alarm.limit.recover`, then stopping the run still produces `detection.result_ng` because the run had a limit alarm. The same smoke also creates an independent default-alarm variable, enables `sys_tags.default_*` limits through `PATCH /api/v1/variables/{id}`, writes `43 -> 50 -> 25`, verifies default `alarm.limit.enter/level_change/recover` through WS and persisted HTTP notifications with `task_id=0`, then confirms the recovered row through `GET /api/v1/limit-alarms?scope=default`. It also asserts the write-audit path by reading `/api/v1/audit-logs`: the formal STRING request must create a `ws.command.write_variable` audit with the same `command_id`, WS `command.detection.abnormal_stop` must create a success audit with the same `command_id`, and the control-variable downset inside `builtin.write_control_variables` must create a `task_flow.write_variable` audit for the target variable.
- `TestProcessMessageKIOPayloadUpdatesOnlyIndexedKnownTags` verifies the MQTT/KIO hot path with a real KingIOT/KIO `Objs/PVs` payload: remapped business `var_name` still reads through the raw KIO `JSONPath`, `PVs` fallback works for objects without a local value, quality codes are kept in runtime snapshots, and the `gateway/topic` index prevents updates from leaking to tags on other topics.

## Windows Build Output

```powershell
.\deploy\build-windows.ps1
```

The output is:

- `dist/edge-backend.exe`
- `dist/configs/config.json`
- `dist/schema.sql`

## MQTT Test Payload

Publish to `spindle/PLC01/data`:

```json
{
  "supply_air": {
    "temp": 23.5,
    "humidity": 47.2
  },
  "status": {
    "running": true
  }
}
```

Before starting a detection task, data updates memory only. Start a task with:

```powershell
Invoke-RestMethod http://127.0.0.1:18080/api/v1/detection-runs `
  -Method Post `
  -ContentType application/json `
  -Body '{"project_id":1,"test_no":"TEST-001","mode":"standard_2h"}'
```
