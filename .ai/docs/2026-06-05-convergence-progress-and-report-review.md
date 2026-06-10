# 2026-06-05 收敛进度与报表链路审阅

本文是当天阶段快照，不作为第二个活跃看板。仍以根目录 `AI_BOARD.md` 作为唯一 open/blocked 任务来源。

## 当前收敛判断

项目主线已经从“架构补洞”进入“业务闭环验收”阶段：

- 一主多边缘的主服务器/边缘端职责已经收敛：主服务器负责统一登录、权限、边缘路由、WS/HTTP facade、同步库查询和报表副本；边缘端仍是现场采集与执行权威。
- 多边缘核心边界已经收敛：`edge_instance_id` 已贯穿项目、网关、变量、实时、控制、运行诊断和 station view，不能再把单边缘成功或 fallback 当作多边缘通过。
- PID 页面最小闭环已经关闭：前端 UI、变量清单、WS 订阅、edge/main_server 两种入口、真实 KIO confirmed、统一提交、结构化失败和 no-write 负向都已有证据；动态小数点、更大批量、更多现场组合和压测另拆。
- 报表链路已进入收口阶段。主服务器 job/readiness/xlsx/通知闭环、客户业务公式口径、时间窗、上下限、复合报表包、默认模板和模板单元格映射均已有后端/测试证据；报表页也已接真实主服务器 job/request/artifact。当前只剩客户原始 workbook 现场复核尚未完成。

## 今天的实施顺序

1. EB-068 PID 真实受控下设最小闭环已关闭；不再继续追加同类页面 smoke。
2. `review-ai/backend-ai` 立即启动 EB-069 报表业务口径核查项：明确数据来源、计算公式、时间窗、上下限来源、复合报表包、模板单元格落点和验收样本。
3. `backend-ai/test-ai` 按报表口径补后端聚合/填充验证：用同步库真实 `task_id/test_no/project_id/edge_instance_id`、历史数据、特征值、检测标准快照和报表请求参数计算，支持一张或多张复合报表，不用前端 mock 公式作为业务依据。
4. `frontend-ai/test-ai` 已把报表页从样例 Luckysheet 体验收敛到真实 job/request/template/artifact 页面，包括报表请求列表、生成状态、失败原因、下载和模板参数展示。
5. 下一步转入 EB-046 任务请求报表入口、EB-050 通知 WS warning、project/device 语义清理；压测当前不做。

## EB-068 关闭证据

关闭范围：PID 页面 WS 直显与实时下设最小闭环。

关键证据：

- no-write 页面 smoke：`desktop/output/playwright/pid-page-smoke-20260605034525.json`。
- no-write 负向：`desktop/output/playwright/pid-negative-smoke-20260605035518.json`。
- edge 模式真实 KIO 单值 `SP2-WD=0.0` confirmed：`desktop/output/playwright/pid-page-smoke-20260605062346.json`。
- edge 模式 2 变量统一提交 `SP2-WD/SP2-SD=0.0` confirmed：`desktop/output/playwright/pid-page-smoke-20260605062518.json`。
- main-server 入口只连 `19080` 并由后端路由到真实 KIO edge `18082` confirmed：`desktop/output/playwright/pid-page-smoke-20260605064634.json`。
- KIO timeout/unmatched 结构化失败证据：`desktop/output/playwright/pid-page-smoke-20260605052050.json`、`desktop/output/playwright/pid-page-smoke-20260605052133.json`。

不在关闭范围：动态小数点变量、更大批量策略、不可写变量现场负向细分、更多 PID 组合和现场压测。

## 报表链路现状

已经具备：

- 边缘端能在检测任务中冻结 `report_request`，形成 `detection_run_report_requests`，包含 `task_id/test_no/project_id/project_code/template_* / variables_json / params_json / ext_*`。
- 生成一张或多张报表是原生设计：推荐入口是 `report_request.reports[]`，一次检测可选择多个报表模板/变量组/参数；落库后 `detection_run_report_requests` 的语义是一行一份报表请求，不再是一行一个变量。
- 边缘端能生成 `detection_run_summaries`，包含 `duration_ms/history_rows/alarm_* / result_status`。
- 边缘端能刷新 `detection_run_features`，当前按 `rt_history_data` 的全部 numeric rows 聚合 `sample_count/avg_value/min_value/max_value/first_sample_time/last_sample_time`。
- 主服务器 `ReportReadiness` 会检查任务已停止、报表请求、summary、history、features、alarm rows，并等待同步库数据齐全。
- 主服务器 report worker 能生成 `.xlsx`，追加 `Report_Run`、`Report_Request`、`Readiness_Checks`、`Features`、`Manifest_JSON` 数据页，并提供 job events、artifact 下载和主服务器报表通知。
- 2026-06-05 15:16 起，主服务器 report worker 首段已补 `ReportPackage` 契约、manifest `report_package`、Excel `Report_Package` 追溯页和 `params_json.cell_mapping` 指定单元格填充；`required=true` 缺失 source 会让 job 失败。该首段已通过 `main-server/backend go test ./internal/reports` 和 `go test ./...`。
- 2026-06-05 15:31 起，主服务器 report worker 已补真正连续合格两小时窗口扫描：每个报表变量按 `var_id` 读取同步历史，优先项目宽表、无数据回退 `rt_history_data`，按运行标准快照、质量策略和上下限判断连续合格，窗口不足输出 `status=insufficient`，窗口达到两小时输出该窗口 `avg/min/max/sample_count/first_sample_time/last_sample_time/status=available`。该段已通过 `main-server/backend go test ./internal/reports`、`go test ./...`、`go vet ./...`、`go build ./cmd/main-server`、`golangci-lint run ./...`。
- 2026-06-05 15:45 起，测试侧首轮已覆盖同一受控检测任务下两条报表请求：两个 job、xlsx artifact、manifest、`Report_Package`、变量集合、全检测指标、连续两小时指标、上下限和 `cell_mapping` 可见单元格互相独立；错误 edge、缺 history、缺 feature、窗口不足、单份 required mapping 失败不影响另一份成功等边界已有测试。

尚未闭合：

- 还没有用真实客户 workbook 做现场验收。必须证明客户可见 sheet 的任务编号、时间范围、平均值、上下限、两小时合格窗口指标和判定结果，与 `Report_Package`/manifest 中的来源一致。
- 缺默认模板语义已由用户确认并转入实现：业务上应有系统默认报表模板，不能用 `generated_default` workbook 当客户报表通过证据。主服务器 report worker 已落地默认模板标识 `SPINDLE_DEFAULT_REPORT v1`、`file_ref=templates/default-report-template.xlsx`、启动 schema 检查 seed 和默认模板文件创建；请求模板缺失或不可达时先使用该默认模板，`generated_default_missing_template` 只保留为默认模板本身无法创建或打开时的防御兜底。
- 默认模板 workbook/cell mapping smoke 已通过：`test-ai` 用 `SPINDLE_DEFAULT_REPORT v1` 和默认模板生成 xlsx，验证 `Default_Report` 客户可见 sheet 与 `Report_Package`/manifest 的任务身份、时间范围、全检测平均值、连续合格两小时指标、上下限、单位和备注一致，且 `Report_Run.template_source` 命中真实默认模板路径。
- 前端 `desktop/src/features/reports/ReportsPage.tsx` 已移除样例模板、样例任务和前端本地公式，改为真实主服务器报表 job 控制台；main_server 页面 smoke 已验证真实 job/readiness/events/request snapshot、模板列表和 xlsx 下载均走 `19080`。

当前实施顺序：

1. `test-ai/review-ai` 处理 EB-069 最后一项：拿到客户原始 workbook 后做现场复核，验证客户原始模板指定单元格和完整追溯页一致；若客户 workbook 暂不可得，EB-069 标记 `field blocked`。
2. `frontend-ai/test-ai` 收口 EB-046：任务请求弹窗从旧“本次报表变量/ext_*”升级为原生 `report_request.reports[]` 多报表列表，每项选择模板、变量组并按 `params_schema_json` 填 `params` 或必要 `cell_mapping`。
3. `test-ai` 给 EB-046 补 main_server/edge 页面 smoke，证明“任务请求生成多条 `detection_run_report_requests` -> 主服务器 report job/readiness/artifact -> 报表页可见”闭环。
4. `backend-ai/frontend-ai/test-ai` 将报表页 smoke 暴露的全局通知 WS warning 归入 EB-050，先复现来源，再补通知 WS facade 或前端订阅问题。
5. `review-ai` 在 EB-069 field 状态、EB-046 报表请求入口和 EB-050 通知 warning 三项明确后，再安排 project/device 命名清理和更多桌面业务流 smoke。

## 报表核查清单

必须明确并测试以下口径：

- 复合报表包：沿用原生 `report_request.reports[]` 和 `detection_run_report_requests`，定义主服务器侧 `ReportPackage` 输出契约。一个检测任务可生成一张或多张复合报表；每个 report 可对应一个 workbook、一个 sheet 或一个 workbook 内的 section，但必须有稳定 `report_code/template_code/version/sheet/section/cell_mapping_version`。
- 全量测试数据 manifest：每张复合报表必须声明并携带生成该报表所需的全部测试数据信息，包括任务身份、项目/边缘、变量 `var_id_text/var_name/unit/source`、样本时间窗、统计样本数、上下限、标准快照、`params_json`、报警/判定、计算版本和数据来源。客户可见 sheet 可以只展示摘要，但 artifact 中必须有 `Manifest_JSON` 或数据页可追溯。
- 任务身份：`task_id`、`test_no`、`project_id/project_code`、`edge_instance_id` 必须同时进入 manifest 和 xlsx。
- 检测时间窗：默认使用检测任务 `started_at` 到 `ended_at`，并说明暂停时间是否从统计样本中剔除；如果依赖 `task_id` 写入历史，应验证历史样本没有越界。
- 两小时合格窗口：定义从哪一刻开始计时，是否要求所有检测项合格，窗口不足两小时如何处理，窗口内平均值如何取样。
- 全检测时长平均值：按每个报表变量计算 `avg/min/max/sample_count/first_sample_time/last_sample_time`，同时覆盖 EAV 与宽表来源。
- 上下限来源优先级：建议优先 `report_request.params_json` 显式参数，其次检测运行标准快照 `detection_run_standard_items`，最后变量默认限值；缺失时必须在 readiness 或 artifact warnings 中暴露。
- 模板填充：定义 `template_code/version` 对应的 cell mapping，例如任务编号、项目、起止时间、平均值、上下限、合格判定、备注字段分别写入哪些单元格。
- 多报表关系：同一 `report_request` 下多个 report 可以共享同一任务事实和样本数据，但每张报表必须独立记录自己使用的变量集合、指标集合、窗口和 cell mapping；不能因为第一个报表成功就把整个 package 标记为成功。
- 失败语义：缺 history、缺 features、缺上下限、两小时合格窗口不足、模板缺失、单元格映射缺失，必须返回明确 waiting/failed/warning，不生成看似成功的假报表。

## 建议关闭条件

报表业务口径项只有在以下证据齐全后才能关闭：

- 至少一个真实或受控 smoke 任务覆盖 2 个以上 numeric 变量、明确上下限、`params_json` 参数和 report request，并至少包含两个 report 定义或一个 workbook 内两个复合 report section。
- 主服务器 readiness 能说明 summary/history/features/limits/template mapping 是否齐全。
- 生成的 xlsx 不只是追加数据页，而是把任务编号、时间范围、变量平均值、上下限和判定结果写入约定单元格，同时保留完整测试数据 manifest 或数据页。
- 反向测试已覆盖缺 history、缺 feature、错 edge、缺模板映射、复合报表中某一张 report 失败、两小时合格窗口不足；关闭前仍需用系统默认模板跑 workbook/cell mapping smoke，并确认客户可见单元格与 `Report_Package`/manifest 追溯来源一致；`generated_default_missing_template` 只能记录为默认模板不可用诊断，不能计入客户报表通过。
- 前端报表页读取真实 job/request/artifact，不再把本地 mock 公式显示为业务结果，并通过 `main_server` 页面 smoke 证明真实 job/status/events/download 链路可用。
- 客户原始 workbook 现场复核完成，或在客户 workbook 暂不可得时明确登记为 `field blocked`，不再阻塞 EB-046/EB-050 工程收口。
