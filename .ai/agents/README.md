# Agents

## 用途

本目录保存自定义 AI Agent 定义，供跨编辑器复用。

## 命名规范

- 文件名：`<name>.agent.md`。
- 必须包含 `description`、`name`、`tools` frontmatter。
- 审查类 Agent 默认只读，tools 使用最小必要集。

## 什么时候创建新 Agent

- 需要长期复用的角色边界。
- 需要明确“不做什么”的工作类型。
- 与 `frontend-ai`、`backend-ai`、`test-ai`、`review-ai` 身份模型一致。

## 什么时候不要创建新 Agent

- 不为一次性任务创建 Agent。
- 不创建拥有过宽工具权限的审查 Agent。
