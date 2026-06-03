# Spindle Main Server Backend

This is the first split-out backend skeleton for the main-server side.

Responsibilities:

- Read synchronized MySQL data from the main-server database.
- Keep query DTOs aligned with the edge backend while gradually porting read handlers.
- Forward control/write requests to the configured edge backend.
- Host future report template management, report generation, report files, and regeneration jobs.

Current scaffold:

- `GET /health`
- `GET /api/v1/main-server/status`
- `GET /api/v1/main-server/sync-diagnostics`
- `GET /api/v1/main-server/report-readiness?task_id=...`
- `POST /api/v1/auth/login`
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
- `POST /api/v1/edge-control/variables/write`
- `ANY /api/v1/edge-proxy/*path`
- unimplemented `ANY /api/v1/*path`

`/api/v1/main-server/status` exposes the configured edge context as `edge_nodes[]`, including `edge_instance_id`, `base_url`, `sync_database`, `service_token_ref`, and `enabled`. `query_proxy_enabled` is always reported as `false`; the old migration query proxy is disabled at runtime.

`/api/v1/main-server/sync-diagnostics` is a protected synchronization diagnostics route. It reads the configured mirror database and reports key synchronized table status, row counts, latest timestamp columns, missing tables, and an `overall_status` of `ok` or `degraded`. Missing synchronized tables return `200` with table-level `status=missing` so the UI can show exactly which mirror data is not ready; this route does not seed data or write the mirror database.

`/api/v1/main-server/report-readiness?task_id=...` is a protected report data readiness route. It reads synchronized detection tasks, report request snapshots, summaries, features, history rows, storage routes, and detection alarms for one run, then returns `overall_status=ready|waiting|not_requested` plus per-check and per-report-request diagnostics. It does not create a report job, generate Excel, or write synchronized edge tables.

The local read-only routes read synchronized tables directly from the main-server database and keep DTOs aligned with the edge backend routes. They do not seed data, write synchronized edge tables, or return mock samples.
Business read routes are still authenticated. Synchronized read-only data is exposed only through main-server JWT plus the matching permission; read-only does not mean public.

- `/api/v1/auth/login` reads synchronized `sys_users`, verifies the same bcrypt password hash used by the edge backend, and issues a main-server JWT. The main-server JWT is separate from the edge JWT.
- `/api/v1/auth/me` validates the main-server JWT and rechecks synchronized `sys_users` so disabled users or permission-version changes take effect after synchronization.
- `/api/v1/auth/sso-ticket/verify` verifies an edge-created one-time SSO ticket by calling the configured edge backend `/api/v1/auth/sso-ticket/verify` with the service token from `edge.service_token_ref`, then issues a main-server JWT for the synchronized user. The main server does not update synchronized `sys_sso_tickets`.
- `/api/v1/auth/sso-ticket` currently returns `501 main_server_sso_ticket_unsupported`; one-time tickets are created on the edge side for handoff to the main server.
- `/api/v1/users` is a protected read-only view of synchronized `sys_users`. User create/update/reset/delete on the main server remains blocked by the raw write guard and must not write the synchronized edge user table directly.
- `/api/v1/gateway-configs` and `/api/v1/gateway-configs/:gateway_id` are protected read-only views of synchronized `sys_gateways`. They keep the edge gateway configuration DTO, omit password fields, and let the main-server settings page inspect MQTT/KIO configuration that has already synchronized. Gateway create/update/delete/discover/publish/subscribe/KIO write calls remain blocked or edge-controlled.
- `/api/v1/gateways` returns `501 main_server_runtime_diagnostic_unsupported`. Gateway online/offline status is MQTT runtime state in edge memory, not synchronized table data.
- `/api/v1/projects/:id/members` is a protected read-only view of synchronized `sys_project_members` joined with synchronized `sys_users`. It returns `{items,count}` with member usernames, roles, and notification switches, and filters explicit foreign-edge projects by `edge_instance_id`. Project-member writes remain blocked on the main server; change project notification recipients through the edge backend or a future controlled edge command.
- `/api/v1/system/database-config` is a protected local read-only view of the main-server backend database config. It returns the configured mirror database host, port, user, name, `password_set`, `read_only=true`, and `source=main_server_config`; it never proxies to the edge backend and never returns the password. `PATCH /api/v1/system/database-config` and `POST /api/v1/system/database-config/test` return `501 main_server_database_config_read_only`; edit the main-server config file and restart the process instead of changing synchronized edge database settings from the main-server UI.
- `/api/v1/runtime/channels`, `/api/v1/runtime/channels/detail`, `/api/v1/runtime/notifications`, `/api/v1/runtime/workers`, and `/api/v1/task-flows/runtime` are protected live diagnostic mirror routes. The main server calls the edge backend `GET /api/v1/edge-control/runtime/*` or `/api/v1/edge-control/task-flows/runtime` with the service token from `edge.service_token_ref` and returns the edge runtime diagnostics unchanged. These routes do not read synchronized tables and do not return mock queue state. Missing token returns `503 edge_runtime_token_missing`; disabled edge access returns `503 edge_runtime_disabled`; network failure returns `502 edge_runtime_unavailable`.
- `/api/v1/task-modules` and `/api/v1/task-flow-templates` are protected task metadata mirror routes. The main server calls the edge backend `GET /api/v1/edge-control/task-modules` and `/api/v1/edge-control/task-flow-templates` with the service token from `edge.service_token_ref` and returns the edge task module/template definitions unchanged. Missing token returns `503 edge_metadata_token_missing`; disabled edge access returns `503 edge_metadata_disabled`; network failure returns `502 edge_metadata_unavailable`.
- `/api/v1/ws` returns `501 main_server_realtime_ws_unsupported`. The edge WebSocket carries live memory snapshots and write/control commands under the edge user/auth model; the main server must not raw-proxy it with a main-server user JWT. Use `/api/v1/realtime/variables` for the current read-only mirror, or add a service-token WebSocket bridge before enabling main-server live subscriptions.
- `/api/v1/audit-logs` is a protected read-only view of synchronized `sys_audit_logs`. It supports `actor_type/actor_id/action/target_type/target_id/result/from/to/limit/offset`.
- `/api/v1/notifications` and `/api/v1/notifications/unread-count` are protected read-only views of synchronized `sys_notifications/sys_notification_recipients` for the logged-in synchronized user. They support `unread/type/level/project_id/from/to/keyword/limit/offset`, return JSON `payload`, and include `var_id_text`.
- `/api/v1/notifications/:id/read` and `/api/v1/notifications/read-all` return `501 main_server_notification_read_unsupported`; the main server must not update the synchronized edge notification read state until a main-server-owned read-state table is designed.
- `/api/v1/realtime/variables` is a protected live mirror route. Realtime values are not available in the synchronized MySQL mirror, so the main server calls the edge backend `GET /api/v1/edge-control/realtime/variables` with the service token from `edge.service_token_ref`, preserves query filters, and returns the edge `TagSnapshot[]` response unchanged. Missing token returns `503 edge_realtime_token_missing`; disabled edge access returns `503 edge_realtime_disabled`; network failure returns `502 edge_realtime_unavailable`.
- `/api/v1/report-templates` is a protected read-only view of synchronized `sys_report_templates`. It supports `enabled` and `keyword`, and returns template metadata including `params_schema_json` for report-request forms and future main-server report generation. Main-server template writes remain disabled until a main-server-owned publish/version strategy is designed.
- `/api/v1/storage-routes` is a protected read-only view of synchronized current `sys_storage_routes`. It supports `project_id`, `var_id`, and `enabled`, returns edge-compatible storage route DTOs with `var_id_text`, and filters explicit foreign-edge projects by `edge_instance_id`. Main-server storage-route writes remain blocked; historical run pages should keep using `/api/v1/detection-runs/:id/storage-routes` because that endpoint reads immutable run snapshots.
- `/api/v1/detection-standards` and `/api/v1/detection-standards/:id` are protected read-only views of synchronized `sys_detection_standards/sys_detection_standard_items`. The list supports `project_id`, `project_code`, `mode`, `enabled`, and `keyword`; detail includes standard items and `var_id_text`. Explicit foreign-edge project standards are hidden by `edge_instance_id`.
- `/api/v1/detection-standards/favorites` and `/api/v1/detection-standards/recent` are protected read-only views of synchronized `sys_detection_standard_favorites/sys_detection_standard_recents` for the logged-in synchronized user. Main-server favorite/recent writes remain disabled because those tables are owned by the edge side until a main-server-owned preference model is designed.
- `/api/v1/main-server/report-readiness` is a protected read-only diagnostic for future report workers and pages. It requires `task_id`, checks the run edge context, and reports whether the synchronized mirror has the stopped run row, report requests, summary, requested variable history, requested variable features, and alarm rows needed before the main server should generate Excel.
- `/api/v1/projects` reads `sys_projects` and filters explicit foreign-edge projects by `edge_instance_id`.
- `/api/v1/variables` reads synchronized `sys_tags`, returns the edge-compatible variable DTO including `var_id_text`, and filters explicit foreign-edge project variables by `edge_instance_id`.
- `/api/v1/history/data` reads synchronized detection history. It keeps the edge behavior of preferring project wide tables through `detection_run_storage_routes`, then falling back to `rt_history_data`.
- `/api/v1/limit-alarms` reads synchronized `detection_limit_alarms`, returns the edge-compatible alarm DTO including `var_id_text`, and filters explicit foreign-edge project alarms by `edge_instance_id`.
- `/api/v1/task-flows` and `/api/v1/task-flows/:id` are protected read-only views of synchronized `sys_task_flows/sys_task_flow_vars`. The list supports `project_id`, `trigger_type`, and `enabled`, returns `vars[].var_id_text`, and filters explicit foreign-edge project flows by `edge_instance_id`. Main-server task-flow create/update/delete/run stays blocked; runtime execution belongs to the edge backend.
- `/api/v1/task-flows/runtime` reads the edge task-flow execution queue through the runtime diagnostic mirror. The main server has no local edge task-flow execution queue and does not synthesize queue state.
- `/api/v1/task-flow-runs*` reads synchronized `task_flow_runs/task_flow_sql_logs`, returns `trigger_var_id_text`, and filters run records by edge project context.
- `/api/v1/detection-runs/active` reads synchronized running and paused detection tasks and filters explicit foreign-edge projects by `edge_instance_id`.
- `/api/v1/detection-runs/current` reads the current running or paused detection task plus run snapshots. No current task returns `404 not_found`.
- `/api/v1/detection-runs*` reads detection task records, run standard snapshots, storage route snapshots, summaries, features, events, notes, and report request snapshots from synchronized tables. Notes are read-only on the main server; `POST /api/v1/detection-runs/:id/notes` returns `501 main_server_detection_note_write_unsupported` until a controlled edge command or main-server-owned note model is designed.
- `/api/v1/station-view/effective` reads station-view templates and bindings. Missing synchronized station-view configuration returns `503 sync_not_ready`.

The `POST /api/v1/edge-control/*` routes are controlled server-to-server calls to the configured edge backend. The main server reads the token from the environment variable named by `edge.service_token_ref`, sends `Authorization: Bearer <token>` and `X-Command-ID`, and preserves the edge response status/body. Missing token returns `503 edge_control_token_missing`; disabled edge control returns `503 edge_control_disabled`; network failure returns `502 edge_backend_unavailable`. The live realtime mirror uses the same service-token client for a read-only `GET` request but does not create an edge command lifecycle row because no field state changes.

For the shared frontend, the main server also exposes controlled aliases for the common detection write paths: `POST /api/v1/detection-runs`, `POST /api/v1/detection-runs/:id/stop`, `POST /api/v1/detection-runs/:id/abnormal-stop`, `POST /api/v1/detection-runs/:id/pause`, and `POST /api/v1/detection-runs/:id/resume`. These routes require a main-server user JWT and `start_detection` or `stop_detection`, wrap the user payload into the edge-control envelope with `operator_username` and `command_id`, then call the edge backend detection control endpoints. They do not write synchronized detection tables directly.

Raw query and write proxying is disabled. `POST/PATCH/PUT/DELETE /api/v1/*path` and write calls through `/api/v1/edge-proxy/*path` return `501 edge_control_required`. Unimplemented read calls under `/api/v1/*path` and read calls through `/api/v1/edge-proxy/*path` return `501 main_server_query_route_not_implemented`; port the matching handler to `main-server/backend/internal/query` or add an explicit service-token mirror endpoint instead of relying on the old migration proxy. The legacy `edge.query_proxy_enabled` config key is kept only for config compatibility and is ignored.

Run:

```powershell
Copy-Item configs/config.example.json configs/config.json
go run ./cmd/main-server
```
