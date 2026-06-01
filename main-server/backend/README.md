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
- `ANY /api/v1/edge-proxy/*path`
- `ANY /api/v1/*path`

`/api/v1/*path` forwards write methods to the edge backend. During early migration, `edge.query_proxy_enabled=true` can also forward read methods so the copied UI remains usable before local query handlers are ported.

Run:

```powershell
Copy-Item configs/config.example.json configs/config.json
go run ./cmd/main-server
```
