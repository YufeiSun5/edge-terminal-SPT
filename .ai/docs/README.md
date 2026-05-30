# Docs

## 用途

本目录保存稳定设计文档、架构说明、接口契约、阶段总结、闭合记录和归档材料。

## 命名规范

- 使用小写短横线或清晰中文标题。
- 稳定契约建议使用：`api-contract.md`、`realtime-channel.md`、`control-channel.md`。
- 阶段总结建议使用：`phase-YYYY-MM-DD.md`。

## 什么时候创建新文件

- 设计已经稳定，需要被多个 Agent 或编辑器引用。
- API/DTO/数据库/实时通道/控制通道形成稳定契约。
- `AI_BOARD.md` 中的历史已闭合，需要归档。

## 什么时候不要创建新文件

- 不要创建活跃看板。
- 不要维护 open/blocked 工作项。
- 不要复制 `AGENTS.md` 或 `.ai/instructions/` 的大段规则。

当前稳定项目文档优先参考：

- `../../backend/docs/backend-architecture.md`
- `../../docs-desktop-packaging.md`
