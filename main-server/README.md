# Spindle Main Server

`main-server/` is the split-out host for the future main-server application. Existing edge-side `backend/` and `desktop/` directories are intentionally not changed by this split.

## Target Boundary

The main server and edge server each connect to their own MySQL instance. External database synchronization keeps business data aligned. The main server reads synchronized data locally; detection control and other现场 write requests are routed to the edge backend.

## Directory Layout

```text
main-server/
  backend/   Go main-server API, local query routes, edge-control proxy, future report generation.
  desktop/   Copied Electron + React shell; can run as desktop or LAN web UI.
```

## Frontend

The frontend is copied from the edge desktop so all current pages are present. It uses:

- Electron desktop shell.
- Vite web server bound to `0.0.0.0:5273` for LAN access during development.
- `VITE_MAIN_API_BASE_URL`, defaulting to `http://127.0.0.1:19080`.

The Electron shell does not start an edge sidecar. It only checks the configured main-server backend.

## Backend

The backend starts on `0.0.0.0:19080` by default. It connects to the local synchronized MySQL database and forwards write/control requests to the configured edge backend.

Early migration mode can set:

```json
"edge": {
  "base_url": "http://127.0.0.1:18080",
  "query_proxy_enabled": true
}
```

When `query_proxy_enabled` is `true`, read requests are temporarily proxied to the edge backend so the copied UI can be exercised before local query handlers are ported. Before production split, query routes should be implemented against the main-server MySQL and this flag should be disabled.

## Report Ownership

Report template management, report file storage, image assets, regeneration jobs, and export/download routes should move here. The edge side should keep only detection execution and report-request snapshot creation.
