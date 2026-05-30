---
applyTo: "backend/**/*.go,backend/**/*.sql,backend/**/*.json,backend/**/*.ps1,backend/docs/**/*.md"
---

# Backend Go Edge

Read first:

- `AGENTS.md`
- `MEMORY.md`
- `AI_BOARD.md`
- `.ai/instructions/backend-go-edge.md`

Hard rules:

- MQTT callbacks enqueue; they do not write MySQL directly.
- Do not implement application-layer historical sync unless `AI_BOARD.md` confirms scope change.
- API/DTO/schema/realtime/control changes must update `AI_BOARD.md`.
- Verify with `go test ./...` and `go build ./cmd/edge-backend` when possible.
