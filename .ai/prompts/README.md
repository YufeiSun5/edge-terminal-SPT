# Prompts

## 用途

本目录保存项目常用任务 Prompt 模板，作为跨编辑器入口。

## 命名规范

- 文件名：`<task-name>.prompt.md`。
- 必须包含 `description`、`agent`、`tools` frontmatter。
- 正文用 Markdown 链接引用相关 instructions、skills 或 agents。

## 什么时候创建新 Prompt

- 任务会反复出现，例如后端 API 变更、桌面 sidecar 开发、smoke 验证。
- Prompt 能明确身份、输入、输出和必须读取的文档。

## 什么时候不要创建新 Prompt

- 不为一次性讨论创建。
- 不复制完整规范正文。
