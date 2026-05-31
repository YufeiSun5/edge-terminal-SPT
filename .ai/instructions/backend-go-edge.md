---
description: "Use when: editing Go backend, MQTT ingestion, Gin APIs, GORM models, realtime data, history writes, RabbitMQ/HTTP control, or edge-main boundaries"
applyTo: "backend/**/*.go,backend/**/*.sql,backend/**/*.json,backend/**/*.ps1,backend/docs/**/*.md"
---

# Backend Go Edge Rules

## Required Reading

- Root `MEMORY.md`
- Root `AI_BOARD.md`
- `backend/docs/backend-architecture.md`
- `backend/docs/边缘端全链路数据流转与分发图.md` when changing acquisition, runtime maps, storage, alarms, task flows, notifications, WS, or HTTP data distribution.
- `backend/deploy/schema.sql` for schema-affecting changes

## Hard Rules

1. Keep MQTT callbacks lightweight; enqueue into channels and do not write MySQL directly.
2. Keep `gateway_id + source_path` as the discovered variable identity.
3. Keep projects as virtual business groups; do not make MQTT gateways project parents. Legacy `device` names are historical aliases only and must not expand outside explicit compatibility layers.
4. Realtime state remains memory-first through `TagManager`; persist history only during running detection tasks.
5. Business APIs stay under `/api/v1` and preserve stable JSON field names.
6. Do not implement application-layer historical sync to the main server unless `AI_BOARD.md` records a confirmed scope change.
7. For main-server realtime export, prefer an explicit SSE or WebSocket endpoint with documented payloads.
8. For method/control calls, design idempotent HTTP or RabbitMQ command semantics and document errors.

## Change Coordination

Update `AI_BOARD.md` before or during changes to:

- API paths, methods, payloads, or error bodies.
- Database schema or model fields.
- MQTT payload parsing, variable discovery, or storage timing.
- SSE/WS realtime export.
- RabbitMQ/HTTP control path.

Also update `backend/docs/边缘端全链路数据流转与分发图.md` when a change affects any data path, bus, queue, runtime map, worker, storage route, alarm flow, task-flow trigger, notification distribution, WS topic, or frontend/backend data handoff.

## Verification

Run from `backend/` when possible:

```powershell
go test ./...
go build ./cmd/edge-backend
```

For runtime smoke, use a valid MySQL config and check:

```powershell
Invoke-RestMethod http://127.0.0.1:18080/health
```
