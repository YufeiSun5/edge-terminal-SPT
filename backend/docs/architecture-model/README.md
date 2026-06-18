# 后端架构模型

最后更新：2026-06-18

负责人：`review-ai` 维护模型边界；当后端数据链路、控制链路、报表/计划流程发生变化时，`backend-ai` 必须同步更新这里。

这个目录是后端逻辑的第一份“架构即代码”来源。它不是第二个活动看板；所有 open / blocked 工作仍然放在根目录 `AI_BOARD.md`。

## 为什么要有这个目录

现在后端已经不是一条简单链路，而是多条路径并存：

- 边缘端现场采集和控制；
- 主服务器读取镜像库；
- service-token edge-control；
- 低频配置同步；
- 待检测计划流转；
- 报表生成和文件管理。

前端提出新需求时，不要马上新建后端接口。先在这里判断它属于哪条已有路径，能复用就复用。

## 文件说明

| File | Purpose | Audience |
| --- | --- | --- |
| `edge-terminal.c4` | LikeC4 系统/组件模型，用来看边缘后端、主服务器、同步软件、报表 worker、队列和运行态 manager 的职责边界。当前包含 `报表生成全链路` 和 `报表服务内部边界` 两个报表视图。 | 后端 AI、前端 AI、研发、审阅 AI。 |
| `processes/detection-plan-start.bpmn` | BPMN 流程图，用来看外部下发的待检测计划如何变成边缘端真实检测任务。 | 产品经理、后端 AI、前端 AI、测试。 |
| `processes/report-job-generation.bpmn` | BPMN 流程图，用来看报表 job 入队、等待同步就绪、生成 artifact 和失败状态。 | 产品经理、后端 AI、前端 AI、测试。 |

## 报表模型来源

`edge-terminal.c4` 里的报表部分按当前源码和稳定文档交叉整理，主要来源：

- `main-server/backend/internal/reports/*.go`：模板资产、计划导入、ArtifactStore、ReportPackage、Excel/PNG 生成、job worker、报表通知。
- `main-server/backend/internal/reports/download_package.go`：历史详情“数据下载”同步 zip 打包。
- `main-server/backend/internal/query/report_readiness.go`：报表生成前的同步就绪检查。
- `backend/internal/services/detection_plans_service.go`、`backend/internal/pipeline/task_flow_executor.go` 和 `backend/internal/database/detection_repo.go`：边缘端通过 watched STRING 任务请求变量开工、任务流创建真实检测任务、启动时解析 `project_group` 标准作用域并冻结 `detection_run_report_requests`。
- `backend/docs/backend-architecture.md`、`backend/docs/报表业务口径与客户模板单元格映射设计.md`、`backend/docs/报表设置计划导入与历史报表展示设计.md`、`backend/docs/边缘端全链路数据流转与分发图.md`。

## 怎么预览

推荐：

```powershell
npx likec4 serve backend/docs/architecture-model
```

用 Camunda Modeler 打开 BPMN 文件。也可以用 diagrams.net 的 BPMN import，或其他兼容 BPMN 2.0 的查看器。

## 后端复用检查门

当前端页面需要一个新能力时，先判断它应该归属哪条已有后端能力：

| Need | Reuse This Path | Do Not Do |
| --- | --- | --- |
| 查询历史任务、报警、汇总、报表请求 | 主服务器从镜像库读取同步模型 | 不要随意代理边缘端读取，也不要伪造 mock 行。 |
| 查询当前实时值或运行诊断 | 主服务器用 service-token 调边缘端 `/edge-control/*` 读取接口 | 不要从同步 MySQL 读取实时值。 |
| 启动/停止/暂停/恢复检测，写变量，应用配置 | 主服务器用户接口 -> edge-control envelope -> 边缘端 service | 不要在主服务器直接写 `sys_detection_tasks` 或运行态表。 |
| 低频共享配置 | 可同步配置表 + 节点 ID 号段 + 父表版本水位 | 不要只改镜像表就期待边缘内存更新。 |
| 外部下发的待检测工作 | `sys_detection_plans` -> 边缘端 `DetectionPlansService` -> 真实 `sys_detection_tasks` | 不要把 `pending_test` 塞进 `sys_detection_tasks`。 |
| 报表模板管理和计划导入 | 主服务器 report service 和 `ArtifactStore`；检测配置通过 `CreateDetectionStandard` 创建 | 不要把 Excel 文件塞进同步表，也不要在前端生成最终 Excel。 |
| 报表生成 | `main_report_jobs` worker，状态为 `waiting_for_sync/generating/succeeded/failed` | 同步数据缺失时不要生成空报表。 |
| 报表参数重生成 | `POST /main-server/report-jobs/:id/regenerate` 创建带 `parent_job_id/generation_type/params_override_json` 的新 job | 不要覆盖原始 `detection_run_report_requests.params_json` 或旧 artifact。 |
| 历史数据下载 | `POST /main-server/download-packages` 同步返回 zip，读取镜像库和成功报表 artifact | 不要让前端重新拼 zip，也不要为未成功报表伪造半成品。 |

如果一个需求不属于上面任何一行，先更新这个模型，明确新的后端归属，再写代码。

## 更新规则

如果代码变化影响下面内容，必须同步更新这些文件：

- 边缘端 / 主服务器职责归属；
- API、数据、控制路由；
- 队列、worker、运行态 map、任务流分发；
- 待检测计划生命周期；
- 报表 job 生命周期和 artifact 归属；
- 数据库同步假设。
