# Docs

## 用途

本目录保存稳定设计文档、架构说明、接口契约、阶段总结、闭合记录和归档材料。

实时交互看板只在根目录 `AI_BOARD.md`。本目录不是活跃看板位置，只承接已经稳定或闭合的内容。

历史内容查找规则：

- 当前 open/blocked 工作项：看根目录 `AI_BOARD.md`。
- 已闭合但仍是当前事实来源的设计、接口、架构说明：看 `.ai/docs/` 或对应模块文档，例如 `backend/docs/`。
- 已闭合且只用于追溯的看板记录、旧阶段总结、被替代方案：看 `.ai/docs/archive/`。

## 命名规范

- 使用小写短横线或清晰中文标题。
- 稳定契约建议使用：`api-contract.md`、`realtime-channel.md`、`control-channel.md`。
- 阶段总结建议使用：`phase-YYYY-MM-DD.md`。

## 什么时候创建新文件

- 设计已经稳定，需要被多个 Agent 或编辑器引用。
- API/DTO/数据库/实时通道/控制通道形成稳定契约。
- `AI_BOARD.md` 中的历史已闭合，需要移出活跃看板。

## 什么时候不要创建新文件

- 不要创建活跃看板。
- 不要维护 open/blocked 工作项。
- 不要复制 `AGENTS.md` 或 `.ai/instructions/` 的大段规则。

## 从看板迁移闭合内容

- `AI_BOARD.md` 必须保留所有 open/blocked 项。
- closed 项的结论、施工说明、测试证据和相关解释应迁移到对应稳定文档。
- 找不到对应文档时，迁移到 `.ai/docs/archive/` 的简短归档文件。
- 活跃看板里只保留必要的短引用，避免后续协作被历史噪声干扰。

当前稳定项目文档优先参考：

- `../../backend/docs/backend-architecture.md`
- `../../docs-desktop-packaging.md`
