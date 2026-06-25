# Spindle Main Server Backend

This is the first split-out backend skeleton for the main-server side.

Responsibilities:

- Read synchronized MySQL data from the main-server database.
- Keep query DTOs aligned with the edge backend while gradually porting read handlers.
- Forward control/write requests to the configured edge backend.
- Host report template reads, report generation, report files, default template seeding, and regeneration jobs.

Current scaffold:

- `GET /health`
- `GET /api/v1/main-server/status`
- `GET /api/v1/main-server/sync-diagnostics`
- `GET /api/v1/main-server/report-readiness?task_id=...`
- `POST /api/v1/main-server/report-jobs/enqueue`
- `GET /api/v1/main-server/report-jobs`
- `GET /api/v1/main-server/report-jobs/:id`
- `GET /api/v1/main-server/report-jobs/:id/events`
- `GET /api/v1/main-server/report-jobs/:id/artifact`
- `POST /api/v1/main-server/report-jobs/:id/retry`
- `POST /api/v1/main-server/report-jobs/:id/regenerate`
- `GET /api/v1/main-server/report-notifications`
- `GET /api/v1/main-server/report-notifications/unread-count`
- `POST /api/v1/main-server/report-notifications/:id/read`
- `POST /api/v1/main-server/report-notifications/read-all`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `GET /api/v1/auth/me`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/sso-ticket`
- `POST /api/v1/auth/sso-ticket/verify`
- `GET /api/v1/users`
- `GET /api/v1/gateways`
- `GET /api/v1/gateway-configs`
- `GET /api/v1/gateway-configs/:gateway_id`
- `GET /api/v1/projects/:id/members`
- `GET /api/v1/system/database-config`
- `PATCH /api/v1/system/database-config`
- `POST /api/v1/system/database-config/test`
- `GET /api/v1/runtime/channels`
- `GET /api/v1/runtime/channels/detail`
- `GET /api/v1/runtime/notifications`
- `GET /api/v1/runtime/workers`
- `GET /api/v1/task-modules`
- `GET /api/v1/task-flow-templates`
- `GET /api/v1/ws`
- `GET /api/v1/audit-logs`
- `GET /api/v1/notifications`
- `GET /api/v1/notifications/unread-count`
- `POST /api/v1/notifications/:id/read`
- `POST /api/v1/notifications/read-all`
- `GET /api/v1/realtime/variables`
- `GET /api/v1/report-templates`
- `GET /api/v1/storage-routes`
- `GET /api/v1/detection-standards`
- `GET /api/v1/detection-standards/:id`
- `GET /api/v1/detection-standards/favorites`
- `GET /api/v1/detection-standards/recent`
- `GET /api/v1/projects`
- `GET /api/v1/variables`
- `GET /api/v1/history/data`
- `GET /api/v1/limit-alarms`
- `GET /api/v1/task-flows`
- `GET /api/v1/task-flows/:id`
- `GET /api/v1/task-flows/runtime`
- `GET /api/v1/task-flow-runs`
- `GET /api/v1/task-flow-runs/:id`
- `GET /api/v1/task-flow-runs/:id/sql-logs`
- `GET /api/v1/detection-runs/active`
- `GET /api/v1/detection-runs/current?project_id=...&edge_instance_id=...`
- `GET /api/v1/detection-runs`
- `GET /api/v1/detection-runs/:id`
- `GET /api/v1/detection-runs/:id/summary`
- `GET /api/v1/detection-runs/:id/features`
- `GET /api/v1/detection-runs/:id/events`
- `GET /api/v1/detection-runs/:id/storage-routes`
- `GET /api/v1/detection-runs/:id/report-requests`
- `GET /api/v1/detection-runs/:id/notes`
- `GET /api/v1/detection-plans`
- `GET /api/v1/detection-plans/:id`
- `POST /api/v1/detection-plans/:id/start`
- `GET /api/v1/station-view/effective?project_id=...&edge_instance_id=...`
- `POST /api/v1/edge-control/detection/start`
- `POST /api/v1/edge-control/detection/stop`
- `POST /api/v1/edge-control/detection/abnormal-stop`
- `POST /api/v1/edge-control/detection/pause`
- `POST /api/v1/edge-control/detection/resume`
- `POST /api/v1/edge-control/detection/mute-alarms`
- `POST /api/v1/edge-control/detection/update-limits`
- `POST /api/v1/edge-control/detection/refresh-features`
- `POST /api/v1/edge-control/detection/report-requests`
- `POST /api/v1/edge-control/detection-plans/:id/start`
- `POST /api/v1/edge-control/variables/write`
- `ANY /api/v1/edge-proxy/*path`
- unimplemented `ANY /api/v1/*path`

`/api/v1/main-server/status` exposes the configured edge context as `edge_nodes[]`, including `edge_instance_id`, `base_url`, `sync_database`, `service_token_ref`, and `enabled`. `query_proxy_enabled` is always reported as `false`; the old migration query proxy is disabled at runtime.

`/api/v1/main-server/sync-diagnostics` is a protected synchronization diagnostics route. It reads the configured mirror database and reports key synchronized table status, row counts, latest timestamp columns, missing tables, and an `overall_status` of `ok` or `degraded`. Missing synchronized tables return `200` with table-level `status=missing` so the UI can show exactly which mirror data is not ready; this route does not seed data or write the mirror database.

`/api/v1/main-server/report-readiness?task_id=...` is a protected report data readiness route. It reads synchronized detection tasks, report request snapshots, summaries, features, history rows, storage routes, and detection alarms for one run, then returns `overall_status=ready|waiting|not_requested` plus per-check and per-report-request diagnostics. It does not create a report job, generate Excel, or write synchronized edge tables.

`/api/v1/main-server/report-jobs*` are main-server-owned report worker routes. `POST /enqueue` reads the same real synchronized readiness data and creates idempotent `main_report_jobs` rows, one row per synchronized `detection_run_report_requests` item. The background worker rechecks readiness, keeps jobs in `waiting_for_sync` when mirror data is incomplete, retries transient failures, writes lifecycle rows to `main_report_job_events`, writes a JSON manifest through the local report `ArtifactStore`, and generates a downloadable `.xlsx` artifact. When a synchronized `sys_report_templates.file_ref` points to a reachable workbook, the worker appends data sheets to that workbook. If the requested template is missing or unreachable, the job fails; it does not silently fall back to the system default template. The default template `SPINDLE_DEFAULT_REPORT` version `1`, file ref `templates/default-report-template.xlsx`, is seeded during `EnsureSchema`, stores its `params_schema_json` cell mapping metadata in `sys_report_templates`, and is used only when explicitly selected by the report request. `GET /:id/events` returns the job event timeline. `GET /:id/artifact` downloads the successful job's `.xlsx` artifact by resolving the stored `artifact_key` and rejects not-ready jobs or unavailable artifacts. `POST /:id/regenerate` creates a new `main_report_jobs` row with `generation_type=params_override`, `parent_job_id`, and `params_override_json`; it uses the same synchronized run/request snapshots but overrides report parameters for the new generation only, without updating `detection_run_report_requests` or overwriting old artifacts. Started, succeeded, and failed report events also create rows in main-server-owned `main_report_notifications/main_report_notification_recipients`; these are not synchronized edge notifications. This worker stage never writes synchronized edge business tables; customer-specific template cell mapping is driven by `detection_run_report_requests.params_json` or the per-generation override.

The local read-only routes read synchronized tables directly from the main-server database and keep DTOs aligned with the edge backend routes. They do not seed data, write synchronized edge tables, or return mock samples.
Business read routes are still authenticated. Synchronized read-only data is exposed only through main-server JWT plus the matching permission; read-only does not mean public.

- `/api/v1/auth/login` reads synchronized `sys_users`, verifies the same bcrypt password hash used by the edge backend, and issues a main-server JWT. The main-server JWT is separate from the edge JWT.
- `/api/v1/auth/refresh` requires the current main-server JWT to still be valid, rechecks synchronized `sys_users`, and returns the same token response shape as login. Expired or invalid tokens return `401`, so the shared frontend can keep long-running WS sessions alive without persisting tokens.
- `/api/v1/auth/me` validates the main-server JWT and rechecks synchronized `sys_users` so disabled users or permission-version changes take effect after synchronization.
- `/api/v1/auth/sso-ticket/verify` verifies an edge-created one-time SSO ticket by calling the configured edge backend `/api/v1/auth/sso-ticket/verify` with the service token from `edge.service_token_ref`, then issues a main-server JWT for the synchronized user. The main server does not update synchronized `sys_sso_tickets`.
- `/api/v1/auth/sso-ticket` currently returns `501 main_server_sso_ticket_unsupported`; one-time tickets are created on the edge side for handoff to the main server.
- `/api/v1/users` is a protected read-only view of synchronized `sys_users`. User create/update/reset/delete on the main server remains blocked by the raw write guard and must not write the synchronized edge user table directly.
- `/api/v1/gateway-configs` and `/api/v1/gateway-configs/:gateway_id` are protected read-only views of synchronized `sys_gateways`. They keep the edge gateway configuration DTO including `edge_instance_id`, omit password fields, and let the main-server settings page inspect MQTT/KIO configuration that has already synchronized. Gateway create/update/delete/discover/publish/subscribe/KIO write calls remain blocked or edge-controlled.
- `/api/v1/gateways` is a protected live gateway runtime mirror. The main server resolves the target edge through `edges[]`, calls the edge backend `GET /api/v1/edge-control/gateways` with that edge service token, and returns the edge MQTT/KIO runtime status unchanged. It does not read synchronized `sys_gateways` for online/offline state and does not return mock gateway health. Missing token returns `503 edge_runtime_token_missing`; disabled edge access returns `503 edge_runtime_disabled`; network failure returns `502 edge_runtime_unavailable`.
- `/api/v1/projects/:id/members` is a protected read-only view of synchronized `sys_project_members` joined with synchronized `sys_users`. It returns `{items,count}` with member usernames, roles, and notification switches, and filters explicit foreign-edge projects by `edge_instance_id`. Project-member writes remain blocked on the main server; change project notification recipients through the edge backend or a future controlled edge command.
- `/api/v1/system/database-config` is a protected local read-only view of the main-server backend database config. It returns the configured mirror database host, port, user, name, `password_set`, `read_only=true`, and `source=main_server_config`; it never proxies to the edge backend and never returns the password. `PATCH /api/v1/system/database-config` and `POST /api/v1/system/database-config/test` return `501 main_server_database_config_read_only`; edit the main-server config file and restart the process instead of changing synchronized edge database settings from the main-server UI.
- `/api/v1/runtime/channels`, `/api/v1/runtime/channels/detail`, `/api/v1/runtime/notifications`, `/api/v1/runtime/workers`, and `/api/v1/task-flows/runtime` are protected live diagnostic mirror routes. The main server resolves the target edge through `edges[]` using explicit `edge_instance_id` or available project/task context, then calls the edge backend `GET /api/v1/edge-control/runtime/*` or `/api/v1/edge-control/task-flows/runtime` with that edge service token and returns the edge runtime diagnostics unchanged. These routes do not read synchronized tables and do not return mock queue state. Missing token returns `503 edge_runtime_token_missing`; disabled edge access returns `503 edge_runtime_disabled`; network failure returns `502 edge_runtime_unavailable`.
- `/api/v1/task-modules` and `/api/v1/task-flow-templates` are protected task metadata mirror routes. The main server resolves the target edge through `edges[]`, calls the edge backend `GET /api/v1/edge-control/task-modules` and `/api/v1/edge-control/task-flow-templates` with that edge service token, and returns the edge task module/template definitions unchanged. Missing token returns `503 edge_metadata_token_missing`; disabled edge access returns `503 edge_metadata_disabled`; network failure returns `502 edge_metadata_unavailable`.
- `/api/v1/ws` is the main-server WebSocket facade. It requires a main-server JWT with `view_realtime` and also accepts browser `access_token` query auth. Realtime subscriptions resolve the target edge through `edges[]` by URL `project_id`, `task_id`, or explicit `edge_instance_id`, reject mismatched project/edge combinations, connect to the selected edge backend `GET /api/v1/edge-control/ws` with that edge service token, remove the browser token before dialing the edge, forward realtime/detection/notification messages, and inject top-level `edge_instance_id` into bridged messages. Supported `command.detection.start/stop/abnormal_stop/pause/resume` and `command.write_variable` messages are not executed on the main server; each command resolves its edge from explicit `edge_instance_id` or payload `project_id/task_id/var_id/project_code/var_name`, then is converted into the existing edge-control HTTP command with `operator_id/operator_name/operator_username/command_id/payload`, so edge-side command lifecycle, idempotency, and audit remain authoritative. Command-only clients may connect without any `topic` query and send a single `command.*`; the bridge must keep the client side open until it returns `command.ack` or `error`, even if the edge realtime WS read side closes. For failed variable writes with partial edge results, the bridge preserves `payload.result` from the edge response, including `var_id_text`, `broker_accepted`, `project_confirmed`, and nested KIO status fields for PID/operator feedback. Missing token returns `503 edge_realtime_token_missing` before upgrade; edge WS failure returns `502 edge_realtime_ws_unavailable`. The bridge read limit is 4 MiB for large initial snapshots.
- `/api/v1/audit-logs` is a protected read-only view of synchronized `sys_audit_logs`. It supports `actor_type/actor_id/action/target_type/target_id/result/from/to/limit/offset`.
- `/api/v1/notifications` and `/api/v1/notifications/unread-count` are protected views of synchronized `sys_notifications/sys_notification_recipients` for the logged-in synchronized user. They support `unread/type/level/project_id/from/to/keyword/limit/offset`, return JSON `payload`, and include `var_id_text`. Read state is calculated with the main-server-owned `main_notification_reads` overlay first, then the synchronized recipient row.
- `/api/v1/notifications/:id/read` and `/api/v1/notifications/read-all` write only `main_notification_reads`. The main server does not update synchronized edge `sys_notification_recipients.read_at`; filtered read-all uses the same category filters as the list/unread-count routes.
- `/api/v1/main-server/report-notifications` and `/api/v1/main-server/report-notifications/unread-count` are protected main-server-owned report notification routes. They read `main_report_notifications/main_report_notification_recipients`, support `unread/job_id/level/limit/offset`, and are generated from report worker `started/succeeded/failed` events. `POST /api/v1/main-server/report-notifications/:id/read` and `/read-all` update only the main-server-owned recipient table, never synchronized edge notification rows.
- `/api/v1/realtime/variables` is a protected live mirror route. Realtime values are not available in the synchronized MySQL mirror, so the main server resolves the target edge through `edges[]` by `project_id` or explicit `edge_instance_id`, rejects mismatched project/edge combinations, calls the selected edge backend `GET /api/v1/edge-control/realtime/variables` with that edge service token, and returns the edge `TagSnapshot[]` response unchanged. Missing token returns `503 edge_realtime_token_missing`; disabled edge access returns `503 edge_realtime_disabled`; network failure returns `502 edge_realtime_unavailable`.
- `/api/v1/report-templates` is a protected read-only view of synchronized/current `sys_report_templates`. It supports `enabled` and `keyword`, and returns template metadata including `params_schema_json` for report-request forms and report generation. `/api/v1/main-server/report-templates*` is the main-server-owned template asset management surface: `GET` lists templates, `POST /upload` accepts multipart `file + template_code/name/display_name/params_schema_json`, stores the workbook through the local report `ArtifactStore`, writes `file_ref` as an `artifact_key`, and records `file_sha256/file_size`; `PATCH /:id/mapping` updates template mapping JSON; `GET /:id/artifact` downloads the stored template copy. Current rows keep `template_code` unique, so uploading the same code updates the current row and increments or uses the supplied `version`; immutable historical template-version records are still a later extension.
- `/api/v1/main-server/report-plan-imports/parse` is the first plan-import route for report settings. It accepts a multipart `.xlsx` file, stores the source workbook under `plan-imports/{year}/{sha}/...` in the local report `ArtifactStore`, parses recognized Chinese/English headers, attempts to match projects, variables, and report templates, and returns a draft with row-level `issues`, normalized limits, confidence, and `needs_confirmation`. Supported limit expressions include `10~20%`, `10±5`, `10±5%`, `>=10`, `<=20`, and `10 +5/-3`.
- `/api/v1/main-server/report-plan-imports/confirm` accepts the reviewed rows from the parse draft and creates synchronized detection configurations in `sys_detection_standards/sys_detection_standard_items`, grouped by project and test number, through the same `StationViewQuery.CreateDetectionStandard` path used by main-server detection-standard writes. That path validates `project_id/project_code/edge_instance_id`, requires enabled target variables that belong to the project, hydrates item display fields from `sys_tags`, computes `config_hash` with the same stable payload algorithm used by the edge backend, and advances the parent `version/config_hash/updated_at` after item replacement. After the standard is created, the route creates a synchronized `sys_detection_plans(status=pending)` row with the generated `standard_code` and `report_request_json` built from imported template/report/variable/param columns; the response includes `created_plans`, `plans`, and `plan_creation_status=created`. Rows with unmatched project/variable, invalid limits, or one-sided limits without explicit confirmation return `409 plan_import_not_ready`. The main server must not fake a pending plan by writing a non-runtime status into `sys_detection_tasks`.
- `/api/v1/storage-routes` is a protected read-only view of synchronized current `sys_storage_routes`. It supports `project_id`, `var_id`, and `enabled`, returns edge-compatible storage route DTOs with `var_id_text`, and filters explicit foreign-edge projects by `edge_instance_id`. Main-server storage-route writes remain blocked; historical run pages should keep using `/api/v1/detection-runs/:id/storage-routes` because that endpoint reads immutable run snapshots.
- `/api/v1/detection-standards` and `/api/v1/detection-standards/:id` are protected read-only views of synchronized `sys_detection_standards/sys_detection_standard_items`. Detection standards may be true global, `project_group` batch scoped, or single-project scoped. `project_id` in the list is a runtime context filter and includes exact project standards, same-`project_group` standards, and true global standards (`project_id IS NULL` with empty `project_group`); `sync_scope` is only synchronization scope, not business applicability. Detail includes standard items and `var_id_text`. Explicit foreign-edge project standards are hidden by `edge_instance_id`. When a concrete edge starts a run, it remaps standard items to that project's enabled variables by business `var_name` before freezing the run snapshot.
- `/api/v1/detection-standards/favorites` and `/api/v1/detection-standards/recent` are protected read-only views of synchronized `sys_detection_standard_favorites/sys_detection_standard_recents` for the logged-in synchronized user. Main-server favorite/recent writes remain disabled because those tables are owned by the edge side until a main-server-owned preference model is designed.
- `/api/v1/main-server/report-readiness` is a protected read-only diagnostic for future report workers and pages. It requires `task_id`, checks the run edge context, and reports whether the synchronized mirror has the stopped run row, report requests, summary, requested variable history, requested variable features, and alarm rows needed before the main server should generate Excel.
- `/api/v1/main-server/report-jobs/enqueue` creates or returns idempotent main-server report jobs for one task. Body: `{"task_id":123,"force":false,"edge_instance_id":"edge-a"}`; `edge_instance_id` is optional when the configured/default edge or task context is enough. A task with no synchronized report request returns `409 report_not_requested`. `GET /api/v1/main-server/report-jobs` lists jobs by `status/task_id/edge_instance_id/limit/offset`; `GET /api/v1/main-server/report-jobs/:id` returns one job; `GET /api/v1/main-server/report-jobs/:id/events` returns `{items,count,limit}` from main-server-owned `main_report_job_events`, including `event_type`, `level`, `message`, JSON `payload`, and `created_at`; `GET /api/v1/main-server/report-jobs/:id/artifact` downloads the generated `.xlsx` artifact for `succeeded` jobs, returns `409 report_artifact_not_ready` before success, and returns `404 report_artifact_unavailable` when the artifact key cannot be resolved. `POST /api/v1/main-server/report-jobs/:id/retry` resets failed or waiting jobs for another worker attempt. `POST /api/v1/main-server/report-jobs/:id/regenerate` accepts `{"params":{...},"reason":"..."}` or `{"params_json":"{...}"}` and creates a new generation job with parameter overrides, leaving the parent job and old artifact available. Successful jobs expose `artifact_ref` as the workbook `artifact_key` plus `artifact_name`; the `succeeded` event payload also includes manifest `artifact_key` and `manifest_name`. Report worker `started/succeeded/failed` events create main-server-owned report notifications for enabled users; report notification read state is updated through `/api/v1/main-server/report-notifications*`.
- `/api/v1/main-server/download-packages` is the History / Data Download backend package route. It requires `view_history` and accepts `{"task_id":123,"keys":["data-raw","config-standard","report-job-45"],"edge_instance_id":"edge-a"}`. The first implementation generates and returns the zip synchronously instead of creating a package table. The zip always includes `download-manifest.json` and `snapshots/task.json`; selected keys add history CSV, standard/limit/storage/report-request snapshots, task events, alarm records, generated report `.xlsx` artifacts, and report job event JSON. A requested report artifact must already be `succeeded`; missing or not-ready artifacts return the existing `report_artifact_*` errors. Standalone curve PNG export is not yet a separate artifact; when requested, the package records it in `manifest.skipped` while the official chart images remain embedded in the report workbook.
- `/api/v1/projects` reads `sys_projects` and filters explicit foreign-edge projects by `edge_instance_id`.
- `/api/v1/variables` reads synchronized `sys_tags`, supports `gateway_id`, `project_id`, `project_code`, `var_group`, `writable`, `enabled`, `discovered`, `source_type`, and `keyword`, returns the edge-compatible variable DTO including `var_id_text`, and filters explicit foreign-edge project variables by `edge_instance_id`. `device_id` is no longer an active query alias and is rejected as `400 unsupported_query_param`; callers must use `project_id`.
- `/api/v1/history/data` reads synchronized detection history. It keeps the edge behavior of preferring project wide tables through `detection_run_storage_routes`, then falling back to `rt_history_data`.
- `/api/v1/limit-alarms` reads synchronized `detection_limit_alarms`, returns the edge-compatible alarm DTO including `var_id_text`, and filters explicit foreign-edge project alarms by `edge_instance_id`.
- `/api/v1/task-flows` and `/api/v1/task-flows/:id` are protected read-only views of synchronized `sys_task_flows/sys_task_flow_vars`. The list supports `project_id`, `trigger_type`, and `enabled`, returns `vars[].var_id_text`, and filters explicit foreign-edge project flows by `edge_instance_id`. Main-server task-flow create/update/delete/run stays blocked; runtime execution belongs to the edge backend.
- `/api/v1/task-flows/runtime` reads the edge task-flow execution queue through the runtime diagnostic mirror. The main server has no local edge task-flow execution queue and does not synthesize queue state.
- `/api/v1/task-flow-runs*` reads synchronized `task_flow_runs/task_flow_sql_logs`, returns `trigger_var_id_text`, and filters run records by edge project context.
- `/api/v1/detection-runs/active` reads synchronized running and paused detection tasks and filters explicit foreign-edge projects by `edge_instance_id`.
- `/api/v1/detection-runs/current` reads the current running or paused detection task plus run snapshots. No current task returns `404 not_found`.
- `/api/v1/detection-runs*` reads detection task records, run standard snapshots, storage route snapshots, summaries, features, events, notes, and report request snapshots from synchronized tables. Notes are read-only on the main server; `POST /api/v1/detection-runs/:id/notes` returns `501 main_server_detection_note_write_unsupported` until a controlled edge command or main-server-owned note model is designed.
- `/api/v1/detection-plans` and `/api/v1/detection-plans/:id` are protected read-only views of synchronized `sys_detection_plans`. The list supports `status`, `factory_no`, `keyword`, `limit`, and `offset`; it intentionally allows the same `factory_no` to return multiple planned tests.
- `POST /api/v1/detection-plans/:id/start` is a user-facing main-server bridge. It wraps the request in the same edge-control command envelope used by detection-run writes and forwards it to the resolved edge endpoint `POST /api/v1/edge-control/detection-plans/:id/start`; `project_id` in the payload remains required so the main server can route to the correct edge, and callers may pass `request_var_id` or `request_var_name` when the watched STRING task request variable is not the default `task_request`. The edge endpoint writes that task request variable and waits for the configured task flow to create `sys_detection_tasks`; the main server does not update synchronized plan or task tables directly.
- `/api/v1/station-view/effective` reads station-view templates and bindings. When `edge_instance_id` is provided, the route validates `project_id + edge_instance_id`; when only `project_id` is provided, it resolves the target from `sys_projects.edge_instance_id` and rejects unresolved or ambiguous projects with explicit error codes. Missing synchronized station-view configuration returns `503 sync_not_ready`.

The `POST /api/v1/edge-control/*` routes are controlled server-to-server calls to the selected edge backend. The main server resolves the target through `edges[]` from explicit `edge_instance_id` query/header, envelope top-level fields, or nested `payload.project_id/task_id/var_id/project_code/var_name`; explicit edge mismatch returns `404 control_edge_instance_mismatch`, and multi-edge requests without a resolvable target return `409 control_edge_instance_unresolved`. After resolving the edge, the main server reads the token from that edge's `service_token_ref` environment variable, sends `Authorization: Bearer <token>` and `X-Command-ID`, and preserves the edge response status/body. Missing token returns `503 edge_control_token_missing`; disabled edge control returns `503 edge_control_disabled`; network failure returns `502 edge_backend_unavailable`. The live realtime mirror uses the same service-token client for a read-only `GET` request but does not create an edge command lifecycle row because no field state changes.

For the shared frontend, the main server also exposes controlled aliases for the common detection write paths: `POST /api/v1/detection-runs`, `POST /api/v1/detection-runs/:id/stop`, `POST /api/v1/detection-runs/:id/abnormal-stop`, `POST /api/v1/detection-runs/:id/pause`, and `POST /api/v1/detection-runs/:id/resume`. These routes require a main-server user JWT and `start_detection` or `stop_detection`, wrap the user payload into the edge-control envelope with `operator_username` and `command_id`, then call the edge backend detection control endpoints. They do not write synchronized detection tables directly.

Raw query and write proxying is disabled. `POST/PATCH/PUT/DELETE /api/v1/*path` and write calls through `/api/v1/edge-proxy/*path` return `501 edge_control_required`. Unimplemented read calls under `/api/v1/*path` and read calls through `/api/v1/edge-proxy/*path` return `501 main_server_query_route_not_implemented`; port the matching handler to `main-server/backend/internal/query` or add an explicit service-token mirror endpoint instead of relying on the old migration proxy. The legacy `edge.query_proxy_enabled` config key is kept only for config compatibility and is ignored.

Run:

```powershell
Copy-Item configs/config.example.json configs/config.json
go run ./cmd/main-server
```
