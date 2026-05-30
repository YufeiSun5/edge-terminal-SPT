---
description: "Use when: 执行 Go 后端、实时通道、控制通道或桌面端 smoke 验证"
agent: "test-ai"
tools: [read, edit, search]
---

# Smoke Verification Prompt

先声明身份：`test-ai`。

必须先读：

- [AI workflow](../instructions/ai-workflow.md)
- [Testing and smoke rules](../instructions/testing-smoke.md)
- [Root board](../../AI_BOARD.md)

任务要求：

1. 列出要验证的范围和命令。
2. 优先运行 `go test ./...` 和 `go build ./cmd/edge-backend`。
3. 有 MySQL/MQTT 环境时执行 health、runtime channels 和 MQTT payload smoke。
4. 没有环境时记录未跑原因，不伪造通过。
5. 把结果写入 `AI_BOARD.md` Activity Log。
