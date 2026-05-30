---
description: "Use when: 修改 Go 后端 API、DTO、实时出口或控制通道"
agent: "backend-ai"
tools: [read, edit, search]
---

# Backend API Change Prompt

先声明身份：`backend-ai`。

必须先读：

- [AI workflow](../instructions/ai-workflow.md)
- [Backend Go edge rules](../instructions/backend-go-edge.md)
- [Root board](../../AI_BOARD.md)
- [Root memory](../../MEMORY.md)

任务要求：

1. 明确变更涉及 API、DTO、数据库、SSE/WS、RabbitMQ/HTTP 中的哪一类。
2. 先更新或同步 `AI_BOARD.md` 中对应项。
3. 保持历史数据库同步在外部同步软件边界内。
4. 修改后运行或记录未运行原因：`go test ./...`、`go build ./cmd/edge-backend`。
5. 最终回复说明身份、处理的 Board 项、open/blocked 项和验证结果。
