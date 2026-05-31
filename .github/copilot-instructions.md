# Copilot Instructions

1. Start by reading root `AGENTS.md`.
2. Then read `MEMORY.md` and `AI_BOARD.md`.
3. `AI_BOARD.md` is the only active collaboration board.
4. Use `.ai/instructions/` as the source of detailed rules.
5. For Go backend work, read `.ai/instructions/backend-go-edge.md`.
6. For Electron + React work, read `.ai/instructions/frontend-electron-react.md`.
7. For tests or smoke checks, read `.ai/instructions/testing-smoke.md`.
8. Keep every open or blocked work item in `AI_BOARD.md`; do not create active work items under `.ai/docs/`.
9. Move closed conclusions, implementation notes, and test evidence to `.ai/docs/`, module docs, or `.ai/docs/archive/`.
10. Historical database sync is external database-sync-software scope, not application code.
11. Realtime edge-to-main data uses SSE or WebSocket after confirmation.
12. Method/control calls use RabbitMQ or HTTP after confirmation.
13. Update `MEMORY.md` after material project changes.
14. Update `AI_BOARD.md` for API, DTO, page, realtime, control, login, SSO, startup, or test status changes.
