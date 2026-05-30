---
applyTo: "backend/**/*_test.go,tests/**/*,scripts/**/*,*.ps1,*.bat,*.md"
---

# Testing Smoke

Read first:

- `AGENTS.md`
- `MEMORY.md`
- `AI_BOARD.md`
- `.ai/instructions/testing-smoke.md`

Hard rules:

- Record commands and pass/fail/skipped reasons in `AI_BOARD.md`.
- Do not skip or delete tests to force a pass.
- Verify schema/model alignment for database changes.
- Realtime/control-channel changes require at least manual smoke evidence.
