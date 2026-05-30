---
description: "Use when: 创建或修改 Electron + React 边缘端 UI、sidecar、登录、SSO、自启动"
agent: "frontend-ai"
tools: [read, edit, search]
---

# Electron Sidecar UI Prompt

先声明身份：`frontend-ai`。

必须先读：

- [AI workflow](../instructions/ai-workflow.md)
- [Frontend Electron React rules](../instructions/frontend-electron-react.md)
- [Root board](../../AI_BOARD.md)
- [Desktop packaging direction](../../docs-desktop-packaging.md)

任务要求：

1. 明确是否影响 sidecar 启动、健康检查、登录、SSO、开机自启动、托盘或三语文案。
2. 前端通过后端 HTTP/SSE/WS client 层访问数据，不直接访问 MySQL 或 RabbitMQ。
3. 页面范围或 DTO 变化必须更新 `AI_BOARD.md`。
4. 若创建桌面项目，补充实际 dev/build 命令到 `MEMORY.md` 或稳定文档。
5. 最终回复说明身份、处理的 Board 项、open/blocked 项和验证结果。
