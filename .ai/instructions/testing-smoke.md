---
description: "Use when: adding tests, smoke checks, release gates, lab validation, or recording verification evidence"
applyTo: "backend/**/*_test.go,tests/**/*,scripts/**/*,*.ps1,*.bat,*.md"
---

# Testing And Smoke Rules

## Required Reading

- Root `MEMORY.md`
- Root `AI_BOARD.md`
- `.ai/instructions/backend-go-edge.md` for backend tests
- `.ai/instructions/frontend-electron-react.md` for desktop tests

## Test Rules

1. Prefer focused tests for changed backend logic before broad refactors.
2. Do not delete or skip tests to manufacture a pass.
3. Record commands, pass/fail result, and skipped reasons in `AI_BOARD.md`.
4. Schema-affecting changes require checking GORM models and `backend/deploy/schema.sql`.
5. Realtime or control-channel changes require at least a manual smoke path.
6. Field-facing machine/project tests are constrained to real test machines `AC-01` through `AC-12`. Do not create or use `AC-13+` or unrelated smoke Projects as evidence for KIO/PID/realtime acceptance; temporary non-field fixtures require explicit cleanup and must be labeled as non-field coverage.

## Current Minimum Gates

From `backend/`:

```powershell
go test ./...
go build ./cmd/edge-backend
```

Runtime smoke when MySQL/MQTT are available:

```powershell
Invoke-RestMethod http://127.0.0.1:18080/health
Invoke-RestMethod http://127.0.0.1:18080/api/v1/runtime/channels
```

MQTT payload smoke can use the sample in `backend/docs/backend-architecture.md`.

## Evidence Format

Use root `AI_BOARD.md` Activity Log:

```text
- YYYY-MM-DD HH:mm | test-ai | test | <scope> | <open/closed/blocked>
```
