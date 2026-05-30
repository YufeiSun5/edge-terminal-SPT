---
applyTo: "desktop/**/*,frontend/**/*,package.json,*.config.js,*.config.ts"
---

# Frontend Electron React

Read first:

- `AGENTS.md`
- `MEMORY.md`
- `AI_BOARD.md`
- `.ai/instructions/frontend-electron-react.md`

Hard rules:

- Frontend does not directly access MySQL or RabbitMQ.
- Use a typed client layer for backend HTTP/SSE/WS.
- Login, SSO, startup, tray, page scope, and DTO changes must update `AI_BOARD.md`.
- User-facing copy should be ready for Chinese, English, and Japanese.
