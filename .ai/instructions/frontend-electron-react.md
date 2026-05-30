---
description: "Use when: creating or editing Electron + React edge desktop UI, sidecar launch, login, SSO, autostart, tray, i18n, or page interactions"
applyTo: "desktop/**/*,frontend/**/*,package.json,*.config.js,*.config.ts"
---

# Frontend Electron React Rules

## Required Reading

- Root `MEMORY.md`
- Root `AI_BOARD.md`
- `docs-desktop-packaging.md`
- `.ai/instructions/ai-workflow.md`

## Edge Desktop Scope

The edge desktop shell should cover:

- Start and monitor the Go sidecar backend.
- Show backend health and main-server connection status.
- Provide local login.
- Support SSO handoff from edge login to the main-server Web app.
- Support Windows autostart, tray/minimize behavior, logs, and basic recovery UX.
- Provide local realtime/detection pages for现场操作.
- Use Chinese, English, and Japanese copy when user-facing text is introduced.

## Boundaries

1. Do not access MySQL directly from the frontend.
2. Do not implement RabbitMQ logic in the renderer.
3. Call backend HTTP/SSE/WS interfaces through a small typed client layer.
4. Keep Excel report generation outside the edge desktop transfer path unless a new confirmed requirement is added.
5. Any page scope, DTO, error, login, SSO, or startup behavior change must update root `AI_BOARD.md`.

## UX Rules

- Edge pages should be operational and compact, not marketing-style.
- Show offline, blocked, running, alarm, and sync-software status distinctly when those states exist.
- Do not hide sidecar startup failure; present actionable status and log entry points.
- Keep i18n keys stable and do not hard-code user-facing text once an i18n structure exists.

## Verification

Once the desktop project exists, run the project-specific dev/build commands and verify:

- Sidecar starts or reports failure clearly.
- Health check reaches `127.0.0.1:18080`.
- Login and SSO handoff do not expose unauthorized access.
- Autostart/tray behavior is documented if not fully automatable in local tests.
