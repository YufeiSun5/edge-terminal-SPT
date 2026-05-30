# Project Guidelines

## Project Summary

Edge Terminal is the edge-side desktop and backend project for the precision air-conditioning performance test system. It runs on the现场边缘服务器, handles local acquisition, realtime display, detection task control, local persistence, login, startup integration, and exposes controlled interfaces to the main server.

The main server is a separate system. Realtime data is consumed from the edge side through SSE or WebSocket; historical database synchronization is handled by external database sync software, not by this application layer.

## Technology Stack

| Area | Stack |
| --- | --- |
| Backend | Go 1.24, Gin, GORM, MySQL, Eclipse Paho MQTT, gjson |
| Current runtime | Go sidecar backend listening on `127.0.0.1:18080` |
| Desktop plan | Electron + React desktop shell |
| Data ingress | MQTT multi-gateway input |
| Realtime state | Go in-memory `TagManager` map guarded by locks |
| History storage | Local MySQL tables under `backend/deploy/schema.sql` |
| Main-server integration | SSE/WebSocket for realtime data; RabbitMQ or HTTP for method/control calls <!-- 待确认 --> |
| Historical sync | External database synchronization software, outside application code |

## Frontend Technical Selection

The edge desktop frontend is initialized under `desktop/` and uses this AI-friendly, constrained stack:

| Layer | Decision |
| --- | --- |
| Desktop shell | Electron main process + secure preload bridge; renderer has no direct Node/MySQL/RabbitMQ access. |
| Renderer | React 19 + TypeScript 6 + Vite 8. |
| UI | Ant Design React 6, with compact operational layouts; use `lucide-react` icons for toolbar/action buttons. |
| Routing | React Router 7 with a centralized router under `src/app/router.tsx`. |
| Server state | TanStack Query 5 for backend HTTP polling, caching, retry, and invalidation. |
| Local state | Zustand 5 only for client-only session/UI state. |
| State machines | XState 5 only for explicit workflows such as sidecar lifecycle, import flow, or realtime connection flow. |
| Forms and validation | React Hook Form + Zod. |
| HTTP client | `axios` through `src/shared/api/http.ts`; pages must not call `fetch` or `axios` directly. |
| API typing | Prefer generated `openapi-fetch` once the Go backend exposes OpenAPI; until then keep DTOs in `src/shared/api/types.ts`. |
| Realtime | SSE first through `@microsoft/fetch-event-source` when JWT headers are required; use WebSocket only for true bidirectional control. |
| JWT | Access token stays in memory via `src/shared/auth/tokenStore.ts`; renderer must not scatter token reads/writes. |
| i18n | `i18next` + `react-i18next`; user-facing desktop copy must include Chinese, English, and Japanese keys. |
| Files/images | Use Ant Design Upload/Image plus `ExcelJS`, `PapaParse`, and `file-saver` through feature adapters. |
| Luckysheet | Treat as a legacy spreadsheet engine. It may only be mounted behind `src/features/spreadsheet/luckysheetAdapter.ts`; do not let global CSS, jQuery, or Luckysheet APIs leak into pages. |
| Testing | Vitest + Testing Library for logic/components; Playwright for desktop/web smoke when UI flows stabilize. |

Frontend hard rules:

1. Pages do not directly call backend endpoints; use `src/shared/api` or feature API modules.
2. Pages do not directly read/write JWT tokens.
3. Renderer does not access filesystem, MySQL, RabbitMQ, or Go sidecar processes directly.
4. Electron-only capabilities go through `electron/preload.cjs` and typed wrappers in `src/shared/desktop`.
5. Luckysheet and file import/export remain isolated feature adapters.
6. Backend-visible API, DTO, login, SSO, startup, realtime, and control-channel changes must update `AI_BOARD.md`.

## Core Modules

| Module | Responsibility |
| --- | --- |
| `backend/cmd/edge-backend/main.go` | Load config, connect database, start kernel and HTTP server. |
| `backend/internal/runtime/` | Wire repositories, workers, MQTT, routes, health and API surface. |
| `backend/internal/mqttx/` | Connect MQTT gateways and feed messages into processing channels. |
| `backend/internal/discovery/` | Discover JSON leaf variables by `gateway_id + source_path`. |
| `backend/internal/pipeline/` | Maintain realtime tag snapshots, active detection tasks, and buffered queues. |
| `backend/internal/storage/` | Batch detection history into MySQL only during running detection tasks. |
| `backend/internal/database/` | GORM connection and repository operations. |
| `backend/internal/models/` | Database models, DTO-like JSON structs, and tag runtime state. |
| `backend/configs/` | Local backend runtime configuration. |
| `backend/deploy/` | MySQL schema and Windows backend build script. |
| `desktop/electron/` | Electron main/preload code that starts and monitors the Go sidecar. |
| `desktop/src/` | React renderer, providers, typed clients, feature modules, and i18n resources. |
| `docs-desktop-packaging.md` | Electron + React + Go sidecar packaging direction. |

## Core Conventions

1. Keep MQTT gateways as data sources; do not model them as device parents.
2. Devices are virtual business groups used by pages and detection tasks.
3. MQTT callbacks must not write MySQL directly; they enqueue into buffered channels.
4. Realtime values are memory-first; history is written only for variables assigned to a device with a running detection task.
5. Business APIs use `/api/v1`; use short nouns and stable JSON field names.
6. Historical sync to the main server is not implemented in this app; keep local database schema and timestamps stable for external database sync software.
7. Edge-to-main realtime export should be SSE or WebSocket; method/control calls should use RabbitMQ or HTTP once the channel is confirmed.
8. Edge desktop work must include login, SSO handoff to the main-server Web app, Windows autostart, sidecar health, logs, and offline status UX.

## Required Reading

- `MEMORY.md`: current project memory, known gaps, and recent AI changes.
- `AI_BOARD.md`: the only active AI collaboration board.
- `.ai/instructions/ai-workflow.md`: mandatory AI workflow.
- `.ai/instructions/backend-go-edge.md`: Go backend and edge data path rules.
- `.ai/instructions/frontend-electron-react.md`: Electron + React desktop shell rules.
- `.ai/instructions/testing-smoke.md`: verification and smoke-test rules.

## On-Demand Resources

| Resource | Use When |
| --- | --- |
| `.ai/docs/README.md` | Need stable architecture notes, closed records, or archive guidance. |
| `backend/docs/backend-architecture.md` | Need current backend flow, data model, and endpoint evidence. |
| `docs-desktop-packaging.md` | Need desktop packaging and sidecar deployment direction. |
| `.ai/skills/README.md` | Considering a reusable project-specific workflow. |
| `.ai/agents/read-only-review.agent.md` | Need a read-only architecture or risk review. |
| `.ai/prompts/` | Starting common backend, frontend, or verification tasks. |

## AI Identity Model

| Identity | Owner | Scope | Boundary |
| --- | --- | --- | --- |
| Frontend AI | `frontend-ai` | Electron + React UI, sidecar calls, login/SSO UX, startup/tray UX, three-language copy. | Does not implement acquisition, storage core, RabbitMQ consumers/producers, or direct MySQL access. |
| Backend AI | `backend-ai` | Go backend, MQTT, realtime SSE/WS, control APIs, RabbitMQ/HTTP integration, config, DTOs, installer hooks. | Frontend-visible API/DTO/error changes must be added to `AI_BOARD.md`; data path, bus, queue, runtime map, storage, alarm, task-flow, notification, WS, or frontend/backend handoff changes must also update `backend/docs/边缘端全链路数据流转与分发图.md`. |
| Test AI | `test-ai` | Go tests, smoke checks, desktop smoke, API health, E2E/lab evidence, release gates. | Defaults to not changing product behavior; fixture changes must describe scope. |
| Review AI | `review-ai` | Architecture review, risk list, cross-module consistency, release readiness. | Lists risks and gaps first; cross-identity edits must explain reason and impact. |

## Mandatory Workflow

1. Before editing, read `MEMORY.md` and `AI_BOARD.md`.
2. Before making changes, declare one identity: `frontend-ai`, `backend-ai`, `test-ai`, or `review-ai`.
3. Check the relevant `.ai/instructions/*.md` file before touching that area.
4. For API, DTO, error semantics, page scope, realtime channel, control channel, login, SSO, or startup changes, update `AI_BOARD.md` first or in the same change.
5. For backend data-flow changes, update `backend/docs/边缘端全链路数据流转与分发图.md` first or in the same change.
6. After material changes, update `MEMORY.md` with a short change log entry.
7. Do not create another active board under `.ai/docs/`.

## Test Gate

- Backend format: `gofmt` on changed Go files.
- Backend unit/build: from `backend/`, run `go test ./...` and `go build ./cmd/edge-backend`.
- Backend smoke: start backend with a valid MySQL config, then check `GET http://127.0.0.1:18080/health`.
- Schema changes: verify `backend/deploy/schema.sql` and GORM models remain aligned.
- Realtime/control changes: document manual SSE/WS, RabbitMQ, or HTTP verification in `AI_BOARD.md`.
- Desktop changes: run the Electron/React dev command once the frontend exists <!-- 待确认 -->.
- If a gate cannot run, record the reason in `AI_BOARD.md` and final response.

## Language

- Code, config keys, API fields, database fields, logs, and protocol identifiers use English.
- User-facing edge desktop copy must support Chinese, English, and Japanese when the frontend exists.
- Project collaboration documents may use Chinese, with stable identifiers and commands kept in English.
- Do not translate existing API field names or database column names for display convenience.
