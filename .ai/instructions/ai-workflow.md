---
description: "Use when: any AI agent starts, edits, reviews, tests, or coordinates work in this project"
applyTo: "**/*"
---

# AI Workflow

## Required Sequence

1. Read root `MEMORY.md`.
2. Read root `AI_BOARD.md`.
3. Declare one identity: `frontend-ai`, `backend-ai`, `test-ai`, or `review-ai`.
4. Read the matching `.ai/instructions/*.md` before editing.
5. Update `AI_BOARD.md` when work changes APIs, DTOs, errors, pages, realtime channels, control channels, login, SSO, startup, tests, or risk status.
6. Update `MEMORY.md` after material changes.

## Board Discipline

- `AI_BOARD.md` is the only active board.
- Do not put open or blocked items under `.ai/docs/`.
- If a decision becomes stable, move the decision summary into `.ai/docs/` and keep only active follow-up in `AI_BOARD.md`.

## Project Boundary Rules

- Edge side owns local acquisition, local control, realtime display, local database writes, login, SSO handoff, startup integration, and sidecar operation.
- Main server is separate.
- Realtime data to the main server should use SSE or WebSocket.
- Historical database synchronization is handled by external database sync software, not by this application layer.
- Method/control calls from the main server should use RabbitMQ or HTTP after the channel is confirmed.
- Excel files are not a data transfer mechanism between edge side and main server.

## Evidence Sources

Use current source, config, build scripts, schema, and existing docs before historical notes. If requirements and source conflict, state the conflict and mark unresolved content with `<!-- 待确认 -->`.
