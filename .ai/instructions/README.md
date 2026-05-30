# Instructions

## 用途

本目录保存分领域、短而可执行的 AI 编码规范。每个文件只处理一个关注点。

## 命名规范

- 使用小写短横线：`backend-go-edge.md`。
- 文件必须包含 frontmatter：`description` 和 `applyTo`。

## 什么时候创建新文件

- 出现新的长期稳定关注点，例如实时通道、安装器、安全、测试。
- 规则能被多个任务重复使用。
- 有源码、配置或需求文档证据支撑。

## 什么时候不要创建新文件

- 只是一次性任务说明。
- 与已有 instruction 高度重叠。
- 只是 open/blocked 工作项，应写入根目录 `AI_BOARD.md`。
