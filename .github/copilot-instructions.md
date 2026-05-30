# Copilot Instructions

1. Start by reading root `AGENTS.md`.
2. Then read `MEMORY.md` and `AI_BOARD.md`.
3. `AI_BOARD.md` is the only active collaboration board.
4. Use `.ai/instructions/` as the source of detailed rules.
5. For Go backend work, read `.ai/instructions/backend-go-edge.md`.
6. For Electron + React work, read `.ai/instructions/frontend-electron-react.md`.
7. For tests or smoke checks, read `.ai/instructions/testing-smoke.md`.
8. Do not create active work items under `.ai/docs/`.
9. Historical database sync is external database-sync-software scope, not application code.
10. Realtime edge-to-main data uses SSE or WebSocket after confirmation.
11. Method/control calls use RabbitMQ or HTTP after confirmation.
12. Update `MEMORY.md` after material project changes.
13. Update `AI_BOARD.md` for API, DTO, page, realtime, control, login, SSO, startup, or test status changes.
