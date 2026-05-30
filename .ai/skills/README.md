# Skills

## 用途

本目录保存项目特有、可复用、步骤稳定的操作技能。

## 命名规范

- 每个技能一个目录：`.ai/skills/<skill-name>/SKILL.md`。
- `SKILL.md` 必须包含 `name`、`description`、`argument-hint` frontmatter。
- 技能目录名必须与 frontmatter `name` 一致。

## 什么时候创建新技能

- 同类操作在项目中出现 3 次以上。
- 输入和输出明确。
- 步骤稳定可复现。
- 包含本项目特有约束。

## 什么时候不要创建新技能

- 不创建“读代码”“修 bug”这类通用技能。
- 不为尚未实际重复的流程提前创建技能。

当前暂不创建具体 skill。边缘端 Go 后端 smoke、Electron sidecar 打包、登录/SSO 联调等流程稳定重复后再创建。
