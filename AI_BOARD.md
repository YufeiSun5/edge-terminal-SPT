# AI Collaboration Board

## 文件模型

`AI_BOARD.md` 是本项目唯一活跃 AI 协作看板，必须与 `MEMORY.md` 同级放在仓库根目录。

`.ai/docs/` 只保存稳定设计文档、闭合记录、阶段总结和归档材料，不承载 open/blocked 工作项，不创建第二个活跃看板。

## AI Identities

| Identity | Owner | Scope | Boundary |
| --- | --- | --- | --- |
| Frontend AI | `frontend-ai` | Electron + React UI、sidecar 调用封装、登录/SSO、开机自启动/托盘、三语文案、视觉和交互。 | 不实现采集/存储核心，不直接访问 RabbitMQ/MySQL，不绕过后端 API。 |
| Backend AI | `backend-ai` | Go 后端、MQTT、SSE/WS 实时出口、HTTP/RabbitMQ 控制通道、配置、DTO、安装器后端钩子。 | 前端可见 API/DTO/错误语义变化必须写入看板。 |
| Test AI | `test-ai` | 单元测试、smoke、E2E、lab、压测、发布门禁证据。 | 默认不改产品行为；修测试夹具必须说明范围。 |
| Review AI | `review-ai` | 架构审阅、上线评估、风险清单、跨模块微调。 | 先列风险和缺口；跨身份修改必须说明原因和影响。 |

## Active Board

| ID | Owner | Type | Status | Item | Next Action |
| --- | --- | --- | --- | --- | --- |
| EB-001 | `frontend-ai` | feature | closed | 创建 Electron + React 边缘端桌面壳，启动 Go sidecar 并展示健康状态。 | 已迁入原型的工位操作与历史查询页面，并完成登录、权限守卫、用户管理、托盘/开机自启动/守护策略、真实历史查询 API 适配和桌面包 smoke；业务页面不依赖 Electron-only API，便于后续迁移到主服务器复用。 |
| EB-002 | `backend-ai` | architecture | closed | 为主服务器提供实时数据出口，协议候选为 SSE 或 WebSocket。 | 已确认实时出口主方向使用 WebSocket；只读通道归 EB-026，写命令通道归 EB-027/EB-019 后续扩展。 |
| EB-003 | `backend-ai` | integration | open | 方法类/控制类调用需面向主服务器提供 RabbitMQ 或 HTTP 通道。 | 确认主通道、命令幂等语义和错误返回格式。 |
| EB-004 | `frontend-ai` | security | closed | 边缘端登录与访问主服务器 Web 的单点登录能力总项。 | 已完成 EB-008、EB-009、EB-010：本地用户 JWT、主站 service token 校验、一次性 SSO ticket、三角色 capability、桌面登录/权限守卫/SSO handoff 和用户管理。 |
| EB-005 | `frontend-ai` | platform | closed | Windows 开机自启动、托盘、异常重拉起、日志入口等外围能力。 | 已实现托盘菜单、关闭主窗最小化到托盘、Windows 登录自启动开关、sidecar 异常退出有限重拉起、日志入口和系统设置页控制面板；打包后 smoke 验证内置 backend 启动且 `/health` ok。 |
| EB-006 | `test-ai` | test | open | 建立 Go 后端最小测试和 smoke 验证门禁。 | `go test ./...`、`go build ./cmd/edge-backend`、`desktop/npm run lint|build|test` 当前通过；下一步补充可重复的 health/MQTT payload/桌面端 smoke 脚本。 |
| EB-007 | `frontend-ai` | feature | closed | 新增设置页面，覆盖实时数据源服务配置、历史数据源服务预留、MQTT 站点、项目/设备分组、发现变量与已入库变量列表。 | 已完成管理中心：变量列表为主工作区，实时数据源接 MQTT 站点与发现接口，项目/设备筛选联动变量，历史数据源保持预留边界说明。 |
| EB-008 | `backend-ai` | security | closed | 实现 Edge Auth Lite 后端认证、鉴权、服务身份和 SSO ticket。 | 已实现 `sys_users`、`sys_service_clients`、`sys_sso_tickets`、`sys_audit_logs`，JWT/service token middleware，`/api/v1/auth/login|me|logout|sso-ticket|sso-ticket/verify`，并为现有 `/api/v1` 路由绑定 capability/scope；`go test ./...`、`go build ./cmd/edge-backend`、HTTP smoke 通过，全包覆盖率 74.3%。 |
| EB-009 | `frontend-ai` | security | closed | 实现桌面端登录、权限守卫、SSO handoff 和 401/403 体验。 | 已完成：接入 `/api/v1/auth/login|me|logout|sso-ticket`，使用内存 tokenStore、Zustand 会话、路由守卫、capability 菜单显隐、401 自动清会话、403 空状态、主站 SSO ticket 打开入口和三语文案；renderer 不拼接主站 token。 |
| EB-010 | `frontend-ai` | feature | closed | 新增用户管理页面。 | 已补齐后端 `/api/v1/users` 列表、新增、编辑、启用/禁用、重置密码、删除接口；管理中心新增用户管理模块，支持 capability 展示、角色编辑、密码重置和当前用户防误禁用/删除。 |
| EB-011 | `frontend-ai` | review | closed | UI 风格复审不通过后重新统一页面视觉。 | 已按工位操作和历史查询为基准重审并修复：壳层由深色/白色割裂改为浅色玻璃系统，登录、设置、调试看板补齐同源背景光、面板边缘高光、胶囊按钮、表格与滚动条规则；业务页面仍保持 shared API/auth store/desktop adapter 边界。 |
| EB-012 | `backend-ai` | feature | closed | 新增检测标准配置、变量上下限项和单次检测标准快照。 | 已新增 `sys_detection_standards`、`sys_detection_standard_items`、`detection_run_standard_items`，检测任务记录 `standard_id/code/version`；`GET/POST/PATCH/PUT/DELETE /api/v1/detection-standards` 可维护标准和标准项，`POST /api/v1/detection-runs` 支持 `standard_id` 并冻结标准项快照，检测期间历史存储按快照 `store_enabled` 过滤变量。 |
| EB-013 | `frontend-ai` | feature | closed | 修正变量管理编辑模型。 | 未分配变量已降级为只读基础属性小卡片池与分配入口，不再允许直接启用/编辑配置；未知变量视图支持现有搜索/筛选、当前结果全选、清空选择和批量分配到项目设备；已分配变量提供完整配置编辑，包括三语显示名、变量名、类型、单位、小数位、缩放、偏移、分组、存储触发、存储模式、周期、死区和启用状态；设备展示已按当前语言读取 display 字段并 fallback。 |
| EB-014 | `frontend-ai` | feature | closed | 接入检测标准前端管理页面。 | 已在管理中心增加检测标准模块，支持标准列表、新增/编辑/删除、设备筛选、标准项选择、上下限、`check_enabled`/`store_enabled`；启动检测时选择 `standard_id` 归 EB-016 工位闭环继续接入。 |
| EB-015 | `backend-ai` | feature | closed | 多设备并行检测任务后端硬化：虚变量创建、同设备单 running、不同设备并行、任务详情、异常停止、任务历史查询和恢复语义。 | 已完成后端模块化增量：`sys_tags.source_type`、`POST /api/v1/variables`、报表模板元数据、检测任务备注/报表关联、设备行事务锁、同设备 running 返回 409、任务列表/详情/notes/异常停止、`history/data?task_id` 和标准/报表模板删除引用保护；恢复 interrupted 语义和检测事件审计留给后续 EB-017/P4 验证与扩展。 |
| EB-016 | `frontend-ai` | feature | open | 工位页补齐检测任务操作闭环、多设备运行总览和虚变量创建入口。 | typed API、管理中心新建虚变量入口、工位页开始检测弹窗、标准/报表模板选择、start/stop/abnormal-stop mutation、权限显隐、按任务进入历史曲线已完成；下一步补更清晰的多设备运行总览和报表模板管理 UI。 |
| EB-017 | `test-ai` | test | open | 多项目并行检测任务与虚变量测试 smoke。 | 覆盖 int/string 虚变量创建、至少两个项目同时启动不同标准、同项目二次启动返回 409、停止单项目不影响其他项目、历史样本归属正确、重启恢复或 interrupted 标记；记录 Go 测试和桌面 smoke 证据。 |
| EB-018 | `backend-ai` | refactor | closed | 后端模块化边界收敛，沿用 `runtime/handlers + services + database.Repository`，避免过细拆包和继续扩大上帝文件。 | 已完成行为保持型重构：`kernel.go` 收敛为启动装配、middleware、健康检查和模块路由注册；用户、设备、变量、历史、检测标准/任务、网关/KIO、报表模板路由迁入 `runtime/handlers`；`repository.go` 和 `models.go` 已按同 package 多文件拆分。 |
| EB-019 | `backend-ai` | feature | open | WebSocket 实时与命令通道：实时变量、检测任务状态、通知和写操作统一经后端分发。 | `/api/v1/ws` 已支持实时变量/检测任务快照、通知、检测任务写命令和 `command.write_variable`；实时变量快照支持全量、按 `project_id`、按单个/多个 `var_id` 和 `source_type/gateway_id` 过滤，`var_id` 订阅走直取避免先组全量。`topic=notifications` 推送 `notification.event`，同时 `Channels.Notify` 已经通过 `NotificationDispatcher` 写入 `sys_notifications/sys_notification_recipients`，HTTP 提供通知列表、未读数和标记已读；recipient 生成已支持 `target_type=all/user/role/project`，其中 `project` 因缺项目成员表暂时仍发给全部启用本地用户。变量写入统一走 `VariableWriteService`，虚拟变量更新内存并可触发 task-flow，物理变量必须满足写入约束后经 KIO service 发布，不绕过后端直写 MQTT。剩余工作是主站控制通道、项目成员通知收敛和更多现场联调 smoke。 |
| EB-020 | `backend-ai` | feature | open | 写操作审计和运行日志完善。 | P0 已完成受保护 HTTP 写请求统一审计：`POST/PATCH/PUT/DELETE` 通过 runtime middleware 写入 `sys_audit_logs`，并新增 `GET /api/v1/audit-logs` 只读分页查询接口供设置页/审计页使用；审计 detail 只保留请求 ID、命令 ID、路由、状态、actor、latency 和错误摘要，不保存请求/响应正文。后续运行日志继续覆盖 MQTT/WS/worker/存储异常，KIO WS 写命令接入时必须复用同一审计模型。 |
| EB-021 | `backend-ai` | feature | closed | 检测超限记录、检测事件流水和结果摘要。 | 检测业务超限 enter/recover、`Alarm` 队列批量入库、`detection_run_events`、`detection_run_summaries`、summary/events API 已完成；剩余静音、level_change、运行中标准调整、默认报警统一事件语义分别归 EB-039/EB-040/EB-045 后续模块。 |
| EB-022 | `backend-ai`/`frontend-ai` | feature | closed | 历史数据源 MySQL 配置实装：系统设置可读取、测试并保存后端配置文件里的数据库连接。 | 已完成：后端新增受 `system_settings` 权限保护的 `/api/v1/system/database-config` 读取/保存接口和 `/api/v1/system/database-config/test` 测试连接接口；GET 不返回明文密码，PATCH 保存到 `backend/configs/config.json`，运行中连接不热切换并返回 `restart_required=true`；前端历史数据源设置页从占位改为 MySQL 配置表单。 |
| EB-023 | `frontend-ai` | feature | closed | 历史数据源 MySQL 配置前端适配验收。 | 已完成：历史数据源页通过 shared API 调用 `/api/v1/system/database-config`，密码不回显并用 `password_set` 呈现状态；测试连接会识别 `ok=false` 并展示后端错误；保存后显示 `restart_required` 重启提示；三语文案与 Playwright smoke 已补齐。 |
| EB-024 | `backend-ai` | feature | closed | 后端变量属性和写入约束补齐。 | 已完成 P1 基础并追加变量默认检测属性：`sys_tags`/`TagConfig`/变量 API 保留采集、清洗、读写约束、默认报警字段；变量级存储映射已在 EB-042 删除，存储目标/周期/列统一归 `storage-routes`；服务层默认自动发现/虚变量不可写，`writable=true` 必须有 `rw_mode=W|RW`、`write_path`、`write_data_type`；默认上下限校验 `LL <= L <= H <= HH`。 |
| EB-025 | `backend-ai` | feature | closed | 检测配置语义补齐和运行快照收敛。 | 已完成并追加 `alarm_enabled/check_cycle_ms/check_on_start`：检测标准项补齐 `check_method/target_value/limit_deadband/violation_hold_ms/recover_hold_ms/quality_policy/alarm_enabled/check_cycle_ms/check_on_start`，启动检测时冻结检测语义和变量默认属性快照 `variable_default_*` 到 `detection_run_standard_items`；检测开始首帧存储由 enabled storage route 的 `store_on_start` 控制，检测项 `check_on_start` 控制检测开始首帧判断。 |
| EB-026 | `backend-ai` | feature | closed | WebSocket 实时读通道。 | 已完成 `/api/v1/ws` 只读通道：连接需 Edge 用户 JWT 和 `view_realtime` 权限，浏览器可用 `access_token` query 鉴权；支持 `topic/source_type/gateway_id/project_id/var_id` 订阅过滤，推送 `connection.ready`、`realtime.variables.snapshot`、`detection.runs.snapshot`、`heartbeat`；`command.*` 或带 `command_id` 的消息返回 `read_only`，写命令留给 EB-027。 |
| EB-027 | `backend-ai` | feature | closed | WebSocket 写命令通道。 | 已完成：`command.detection.start`、`command.detection.stop`、`command.detection.abnormal_stop` 复用 `DetectionRunsService`；`command.write_variable` 复用 `VariableWriteService` 和 `KIOWriteService`；所有 WS 命令要求 `request_id/command_id/payload`，返回 `command.ack` 或结构化 error，并写 `sys_audit_logs`。 |
| EB-028 | `backend-ai` | feature | closed | 检测超限、事件流水和结果摘要落地。 | 已完成超限记录、任务生命周期事件和摘要首段：`GET /api/v1/detection-runs/:id/summary`、`GET /api/v1/detection-runs/:id/events` 已可用；剩余项已拆到 EB-039、EB-040 和前端摘要展示任务，不再作为后端独立待办。 |
| EB-029 | `test-ai` | test | open | 后端分阶段联调和回归门禁。 | 每阶段跑 `gofmt`、`go test ./...`、覆盖率不低于 70%、`go vet ./...`、`go build ./cmd/edge-backend`；补 HTTP/WS smoke、审计日志断言、storage routes 断言、多项目检测和超限事件回归；完成后保持 `127.0.0.1:18080` 后端运行给前端测试。 |
| EB-030 | `frontend-ai` | feature | closed | 变量属性和写入约束前端适配。 | 已完成旧版变量编辑适配；但 EB-041/042 已改变契约，前端需重新迁移：变量编辑不再展示或提交变量级存储字段，存储目标/周期/列改由 storage routes UI 管理，设备字段改为项目字段。写入变量前端校验仍应保持 `writable=true` 必须有 `rw_mode=W|RW`、`write_path`、`write_data_type`。 |
| EB-031 | `backend-ai` | hardening | closed | WebSocket 断线重连和连接保护补强。 | 已完成：服务端设置 32 KiB read limit、read/write deadline、15 秒 ping/heartbeat、pong 延长读 deadline；断开后读 goroutine 和连接循环退出，重连会重新鉴权、订阅并收到 ready/snapshot；测试覆盖未鉴权失败、正常连接、写命令只读保护、主动断开和重连。 |
| EB-032 | `frontend-ai` | feature | closed | 检测配置独立页面与左侧菜单入口。 | 已完成：左侧新增“检测配置”一级菜单，新增 `/#/detection-config` 页面；页面复用检测标准 API，按标准展示上下限、检测方法、目标值、超限/恢复保持时间、质量策略和存储策略，并保留旧 Vue 前端 19 个检测项作为参考池；标准项选择改为从所有变量按 `var_name` 去重，变量显示复用变量列表三语字段；三语文案、lint/build/test 和 Playwright smoke 已通过。 |
| EB-033 | `backend-ai` | feature | closed | 运行态清洗、topic 索引、项目实时视图和变量下设上下行链路设计与实时底座首段实施。 | 已完成首段：新增 `TagManager.ForMessage(gateway_id, topic)` 与 `gateway/topic -> tags` 索引、`project_id -> tags` 项目实时视图、数值清洗滤网的系数偏移/可疑值/防抖/运行态死区/首帧策略；自动发现变量默认只作为 `enabled=false/project_id=null` 候选入库，运行态只加载 `enabled=true AND project_id IS NOT NULL` 的已知业务变量；`logic_worker` 改为按 topic 索引取变量，WS 项目过滤改用项目实时视图；报警状态机、运行中配置即时生效、KIO/WS 下设完整闭环、多变量组合触发任务保留站位，后续拆独立项。 |
| EB-034 | `frontend-ai` | feature | open | 前端适配变量默认上下限、默认报警开关和运行中同步选项。 | 后端已在 `VariableConfig`/`VariableCreatePayload`/`VariablePatchPayload`/检测标准项 DTO 中同步字段：`default_alarm_enabled/default_limit_ll/default_limit_l/default_limit_h/default_limit_hh/default_limit_deadband/default_violation_hold_ms/default_recover_hold_ms`、`apply_to_running`、`alarm_enabled/check_cycle_ms/check_on_start`、运行快照 `variable_default_*`。前端需要在变量高级编辑、虚变量创建、检测配置标准项和任务详情快照里展示/编辑，并明确 `apply_to_running` 的风险提示。 |
| EB-035 | `frontend-ai` | feature | closed | 系统设置日志诊断区增强。 | 已完成系统设置日志诊断区：Electron preload 新增只读 `readLogs`，设置页系统模块支持刷新并预览 Go sidecar 日志文件尾部，同时保留打开日志文件位置；后端审计日志已接入 `GET /api/v1/audit-logs`，展示最近 80 条登录、HTTP 写操作和 WS 命令审计记录。浏览器预览态会明确显示本机日志不可读取，但后端审计列表可正常通过 API 加载。 |
| EB-036 | `backend-ai` | architecture | closed | 任务触发、动态存储与双报警总体计划。 | 总体设计已沉淀到 `backend/docs/任务触发动态存储与双报警实施计划.md`，后续实施已拆到 EB-037/039/040/045；其中任务触发主线已从旧 `sys_task_rules` 转为 EB-045 `sys_task_flows + STRING 虚拟变量`。 |
| EB-037 | `backend-ai`/`frontend-ai` | feature | open | 项目动态宽表存储：storage routes、检测运行快照、自动建表扩列和宽表 writer。 | 后端已完成 route 驱动存储主路径，并补齐运行快照查询：`GET /api/v1/detection-runs/{id}/storage-routes` 返回当次冻结的 enabled routes；检测启动会先保证 disabled 默认 route 建议存在，但实际冻结同一变量的所有 enabled route，包括用户自定义 route。前端下一步接入运行任务“存储路由快照”展示，并补端到端创建/启用 smoke。 |
| EB-038 | `backend-ai` | feature | closed | 任务系统倒排索引和变量触发检测任务。 | 旧 `sys_task_rules` 路线已被 EB-045 `sys_task_flows + var_id -> flow_ids + STRING 虚拟变量 task_params` 取代；本项关闭，后续任务编排统一在 EB-045 推进。 |
| EB-039 | `backend-ai` | feature | closed | 检测业务报警增强：静音、level_change、运行中标准调整事件。 | 已完成后端首段：TaskManager 支持当前检测报警静音，静音只作用于已处于报警的业务项；H/HH/L/LL 更严重方向变化写 `level_change` 并继续使用同一张 `detection_limit_alarms`；任务模块 `builtin.update_detection_limits` 可调整 running 快照并刷新运行态。 |
| EB-040 | `backend-ai` | feature | open | 变量默认报警完整闭环。 | 旧“独立默认报警表”设计废弃，避免系统超限表和检测任务超限表冗余；后续应扩展统一超限事件语义，用 `scope=default|detection` 或等价上下文区分变量默认报警和检测业务报警。默认报警不带静音，检测静音仍归 EB-039。 |
| EB-041 | `backend-ai`/`frontend-ai` | breaking-change | open | 将旧“设备”领域统一改为“项目”，不保留 `/devices` 兼容入口。 | 后端已改用 `projects` API/DTO/文档和项目动态宽表命名；`/api/v1/devices` 已返回 404。前端首轮兼容适配已完成，但页面和 shared type 仍保留大量 `device/device_id/device_code` 别名；前端迁移闭合拆到 EB-047，完成后 EB-041 才能关闭。 |
| EB-042 | `backend-ai`/`frontend-ai` | breaking-change | open | 彻底删除变量属性里的存储频率、触发、首帧快照、目标表列字段。 | 后端已从变量 API 中移除存储属性；前端变量编辑、虚变量创建和变量 PATCH/POST 已停止展示/提交 `store_*`、`storage_*`、`startup_snapshot_enable`，并新增 storage routes UI 管理目标表列、触发、周期、死区和首帧存储。下一步补 storage route 权限/筛选和运行快照展示。 |
| EB-043 | `backend-ai` | feature | closed | 任务系统倒排索引首段：变量触发规则驱动检测和存储业务。 | 旧 `sys_task_rules` 首段已被 EB-045 任务流系统替代；后续只维护 `sys_task_flows/sys_task_flow_vars` 的 `var_id -> flow_ids` 倒排索引。 |
| EB-044 | `backend-ai` | feature | closed | 存储总线首段：统一入口、项目/表 bucket、批量合并 flush。 | StorageBus 首段已完成：按 `project_id + table_name` 分 bucket、支持多项目/多表并发、宽表 route 避免重复窄历史写入；后续存储扩展归 EB-037。 |
| EB-045 | `backend-ai`/`frontend-ai` | feature | open | 条件事件脚本任务系统：多变量条件、内置业务动作、JS 脚本和执行日志。 | 后端已补 P0/P1：`GET /api/v1/task-flow-runs`、`GET /api/v1/task-flow-runs/:id`、`GET /api/v1/task-flow-runs/:id/sql-logs`；`task_flow_runs` 记录 `origin_flow_id/origin_run_id/depth`；新增 `builtin.write_variable`，只允许写虚拟变量，写审计并默认阻断自递归；JavaScript runtime API 已支持 `realtime.get/getMany/getByName/project/write`，其中 `write` 同样只写虚拟变量、复用审计且默认不触发递归；物理变量下设继续走 WS/HTTP 的 `VariableWriteService + KIOWriteService`，不让任务执行器假写现场；新增任务模板、静音、运行中限值调整、storage prepare、报表结果登记和 HTTP 请求模块。任务模板已补固定时长检测、合格持续检测、暂停/恢复、限值调整、特征值刷新、报表登记，并支持 `optional/default` 参数绑定。最新迭代明确“HTTP 配置/查询，现场动作走 STRING 虚拟变量触发任务系统”：`builtin.start_detection_run` 支持 `custom_items/limit_check_enabled/end_policy/qualified_hold_ms`，运行中限值支持 `items` 批量调整，固定时长和合格持续守护支持后端启动恢复；检测启动/结束已派发 `project_start/project_end` 生命周期任务流；`schedule` 已接入轻量扫描器，`schedule_interval_ms` 控制定时间隔。 |
| EB-046 | `frontend-ai` | feature | open | 侧边菜单新增“任务”模块，承载后端业务编排和 EB-045 task-flows。 | 前端首段已完成；shared API 已补 `getTaskModules/getTaskFlowTemplates/getTaskFlowRuns/getTaskFlowRun/getTaskFlowSqlLogs` 类型与调用。下一步 UI 需把模板实例化到表单、展示执行记录/SQL 日志，并支持 `builtin.write_variable`、固定时长、合格持续、暂停/恢复、运行中限值、storage_prepare、register_report、http_request、schedule_interval_ms 等动态参数表单。后端已补 `GET /task-flows/:id` 单条详情，允许 `PATCH /task-flows/:id` 修改 `project_id`，并新增 `DELETE /task-flows/:id` 删除任务流配置；删除不会删除执行历史。正式业务参数仍应写入 STRING 虚拟变量触发，不放在手动运行弹窗。 |
| EB-047 | `backend-ai`/`frontend-ai` | feature | open | 12 项目批量初始化、KIO 变量归属和业务名重映射，为检测业务端到端测试准备真实项目变量。 | 后端 `POST /api/v1/variables/bulk-remap/kio-projects` 已完成并在本地库执行：确保 `AC-01..AC-12` 项目，把 `台N_xx` 原名变量分到项目 N，保留 `raw_name/source_path` 和写入配置，写入业务 `var_name`、三语显示名、变量分组、启用状态和 disabled 默认 storage route，并重载运行态；未启用默认 route 会随业务名同步列名，已启用 route 不自动改。前端需在变量管理或调试/初始化区提供 dry-run 和执行入口。 |
| EB-047 | `frontend-ai` | cleanup | open | 前端“设备”到“项目”迁移闭合，移除页面层旧 device 命名和测试缺口。 | 优先处理：`desktop/src/shared/api/types.ts`、`desktop/src/features/edge-status/api.ts`、工位页、历史页、设置页、检测配置页、任务页。要求页面文案、变量名、query key、URL 参数、表单字段、DTO 主字段统一使用 `project/project_id/project_code`；仅在 shared API 兼容层短期保留 `device_id` alias，并标注 TODO/关闭条件。测试必须覆盖 lint/build/test、`/api/v1/projects` 成功、`/api/v1/devices` 404、变量分配到项目、检测启动/停止、历史按 `project_id/task_id` 查询、storage routes 项目筛选、任务页项目筛选。 |
| EB-048 | `frontend-ai` | feature | open | 变量属性设定和任务系统二次适配闭合。 | 按下方 EB-048 清单接入变量资产属性、storage routes、任务模块/模板、执行记录和正式 STRING 虚拟变量触发链。重点变化：变量不再承载存储策略，检测业务限值不等同变量默认限值，任务正式参数不走手动运行弹窗，页面不得直接调用 KIO 下设绕过后端变量写 service。后端已补 `GET /api/v1/detection-runs/current?project_id=...`，前端可用它拿当前 running/paused run snapshot 来拼实时 OK/NG；WS shared type 已支持 `topic=notifications` 和 `notification.event`，并新增 HTTP 通知中心接口，前端需要接入全局通知/工位通知、未读数、通知列表和标记已读。 |
| EB-049 | `backend-ai`/`review-ai` | future-plan | open | 边缘端与主服务器协同：数据库同步只负责只读镜像，控制命令回到边缘端执行，主服务器基于同步到位的数据执行 Excel 报表任务和通知。 | 已新增 `backend/docs/边缘端与主服务器协同计划.md` 并同步到全链路图。未来实施前需要确认命令通道 HTTP/RabbitMQ、命令幂等、边缘端 `detection_report_requests` 数据就绪表、主服务器报表任务表、主服务器通知中心和数据到位检查策略；当前不允许主服务器直接改同步库里的边缘端业务表来控制现场。 |

### EB-019/020/021 前置设计文档

- `backend/docs/变量属性检测配置与WS前置设计.md`
- `backend/docs/后端日志WS检测实施顺序.md`
- `backend/docs/运行态清洗与实时业务视图设计.md`
- `backend/docs/变量默认上下限与业务报警装配设计.md`
- `backend/docs/任务触发动态存储与双报警实施计划.md`
- `backend/docs/任务变量流转与内置模块设计.md`
- `backend/docs/边缘端全链路数据流转与分发图.md`
- `backend/docs/边缘端与主服务器协同计划.md`
- 结论：WebSocket 读写合一可行，但 WS 写和检测超限必须以完整变量属性和检测配置为前置。
- 当前实施顺序：P0 日志/审计地基 -> P1 变量属性与写入约束 -> P2 检测配置语义补齐 -> P3 WS 实时读 -> P3.5 运行态清洗/topic 索引/项目设备视图 -> P4 WS 写命令 -> P5 检测事件与超限记录 -> P6 联调回归。
- 后端任务拆分：P0=EB-020 日志/审计地基；P1=EB-024 变量属性、存储映射、写入约束；P2=EB-025 检测配置和运行快照；P3=EB-026 WS 只读实时；P3.5=EB-033 运行态清洗、topic 索引和项目设备实时视图；P4=EB-027 WS 写命令；P5=EB-028 检测事件/超限/摘要；P6=EB-029 测试联调。P3 之前不做 WS 写，P4 之前所有写操作必须先有审计。
- P1 旧存储映射结论已废弃：变量属性不再描述入库业务名、存储目标、目标表/列、动态表单字段 key、查询别名、存储频率或首帧存储；这些全部归 `sys_storage_routes`。
- 自动发现变量只生成来源候选；只有完成项目绑定、业务属性、写入属性和检测标准配置后，才允许参与检测、历史存储、超限判断或 WS 写命令。
- JGHJ 旧后端参考已纳入设计：旧 `sys_variables` 包含读写模式、提取路径、缩放偏移、报警上下限、存储策略、防抖和计算来源；旧运行态 `Tag` 保存 current/last value、字符串值、质量码、防抖窗口和报警状态。
- EB-041 后端破坏性改名后，项目统一使用 `sys_projects.project_id/project_code` 语义；旧 `/devices`、`device_id`、`device_code` 后端契约不再保留。`site_no` 只作为站点/项目编号或辅助分组。
- EB-033/041 追加固定：自动发现变量不是运行态变量。发现入库默认 `enabled=false`、`project_id=null`；只有分配项目并启用后的变量才进入 `TagManager` 清洗、WS、历史存储或检测后续业务。
- EB-033 旧暂缓项已拆分：报警 enter/recover 首段已完成，level_change/静音/运行中标准调整归 EB-039；KIO/WS 变量下设完整闭环归 EB-019/027；多变量组合触发任务和倒排索引归 EB-045 `sys_task_flows + var_id -> flow_ids`。
- 变量默认上下限属于变量资产属性，检测业务实际判定仍以检测标准/运行快照为准；运行中修改变量默认属性只有显式 `apply_to_running=true` 时才同步 running 任务的 `variable_default_*` 快照，不覆盖业务标准 `limit_*`。
- 检测启动首帧存储和首帧判断分开：`storage_routes.store_on_start` 决定检测开始是否投递当前值到存储队列，检测标准项 `check_on_start` 决定检测开始是否立刻用当前值做一次业务判断；`check_cycle_ms` 独立于存储 route 周期。
- 新总计划固定后续拆分：任务触发系统只通过倒排索引触发 service 动作；动态存储使用按项目宽表和 `sys_storage_routes/detection_run_storage_routes`；检测业务报警负责业务上下限、静音和等级变化；变量默认报警独立闭环且不带静音。
- EB-045 修正任务流转结论：变量是一切任务业务的根基。普通业务优先做内置模块，前端填写的参数必须写入 STRING 虚拟变量；任务系统由变量变化触发，解析 `trigger_value` JSON 为 `task_params`，`steps_json` 承载多步骤链，执行期间通过 run context 传递中间变量；JavaScript 只作为开发者高级模块。
- 2026-05-30 审查结论：`sys_task_rules` 相关 EB-038/043 已过时并关闭，统一由 EB-045 `sys_task_flows + var_id -> flow_ids` 承载；“变量默认报警独立表”设计已废弃，EB-040 改为统一超限事件语义；EB-021/028/044 后端首段已完成并关闭。

### EB-047 前端项目迁移闭合要求

- 后端契约已经破坏性切换为项目语义：前端页面层不得继续新增 `device/device_id/device_code` 命名。
- shared API 兼容层可以短期把旧页面别名映射到 `project_id/project_code`，但必须集中在 `desktop/src/features/edge-status/api.ts` 或类型兼容段，不允许散落在页面组件。
- 必改范围：`desktop/src/shared/api/types.ts`、`desktop/src/features/edge-status/api.ts`、`StationOperationPage`、`HistoryQueryPage`、`SettingsPage`、`DetectionConfigPage`、`TaskFlowsPage` 和三语 i18n 文案。
- URL/query 参数应优先使用 `project_id`；旧 `device_id` 查询参数只允许做一次性读取兼容，页面内部状态必须转成 `project_id`。
- 表单字段统一用 `project_id`，包括变量分配、虚变量创建、检测启动、检测配置筛选、storage route、task-flow 项目筛选。
- 文案统一为“项目”，不再把项目称作设备；真正物理设备/采集站点另用 gateway/source 表达。
- 关闭 EB-047 前必须跑：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test`。
- smoke 必须记录：登录后 `GET /api/v1/projects` 200、`GET /api/v1/devices` 404、变量分配到项目、创建虚变量、检测启动/停止、历史按 `project_id/task_id` 查询、storage routes 项目筛选、任务页项目筛选和任务创建。

### EB-048 前端变量属性与任务系统交付清单

#### 变量属性设定

- `GET /api/v1/variables` 已实现变量列表，支持 `gateway_id/enabled/discovered/source_type/keyword` 过滤。该接口返回配置属性，不用于高频实时刷新；项目页实时值使用 `GET /api/v1/realtime/variables?project_id=...` 或 `var_id=...`。
- `POST /api/v1/variables` 已支持创建 `source_type=virtual` 或 `manual` 变量。虚拟变量支持 `INT/FLOAT/BOOL/STRING`，可传 `project_id/project_code/var_group/var_name/display_name/display_name_en/display_name_ja/data_type/unit/decimal_places/scale_factor/offset_val`。
- `PATCH /api/v1/variables/{variable_id}` 已支持编辑变量资产属性：三语显示名、业务名、类型、单位、小数位、缩放/偏移、清洗防抖、写入约束、默认上下限/默认报警和启用状态。`apply_to_running=true` 只同步当前 running 任务里的 `variable_default_*` 快照，不覆盖检测标准业务限值，前端必须给二次确认。
- `PATCH /api/v1/variables/{variable_id}/assignment` 用于把候选变量分配到项目并启用，字段为 `project_id/project_code/var_group/enabled`。
- `POST /api/v1/variables/bulk-remap/kio-projects` 是现场初始化工具，支持 `dry_run`，用于把 `台N_xx` KIO 原名批量归属到 `AC-01..AC-12`，并生成 disabled 默认 storage route。前端可放在调试/初始化入口，不应放成普通用户日常操作。
- 写入约束 UI 必须保留：`writable=true` 时要求 `rw_mode=W|RW`、`write_path`、`write_data_type`，可选 `write_min/write_max/write_enum/write_requires_audit`。普通变量写入必须走 WS `command.write_variable` 或后端内置 `builtin.write_variable`，不允许页面直接调用 MQTT/KIO。
- 变量默认报警字段是资产默认值：`default_alarm_enabled/default_limit_ll/default_limit_l/default_limit_h/default_limit_hh/default_limit_deadband/default_violation_hold_ms/default_recover_hold_ms`。它们不等同检测业务运行限值；检测业务实际判定仍看检测标准项和运行快照。
- 原变量级存储字段已废弃并从后端变量 API 删除：前端不得展示或提交 `store_*`、`storage_table`、`storage_value_column`、`storage_target`、`startup_snapshot_enable` 等旧字段。

#### Storage Routes

- `GET/POST/PATCH/DELETE /api/v1/storage-routes` 已实现存储路由 CRUD。字段为 `project_id/var_id/route_code/storage_target/table_name/column_name/column_type/form_field_key/query_alias/trigger_mode/cycle_ms/deadband/store_on_start/enabled`。
- `GET /api/v1/storage-routes` 支持 `project_id/var_id/enabled` 过滤。前端变量详情可以显示该变量的路由列表，存储配置页必须按项目筛选。
- `GET /api/v1/detection-runs/{id}/storage-routes` 已实现运行任务冻结路由查询，返回 `{items,count}`；前端任务详情可用它展示当次检测实际使用的表、列、触发方式、周期、死区和首帧存储配置。
- `storage_routes.store_on_start` 控制检测开始时是否做首帧存储；检测首帧判断由检测标准项 `check_on_start` 控制，二者不能在 UI 上绑成同一个开关。
- 创建/启用 route 后，后端会在检测启动或 schema prepare 阶段准备项目宽表和缺失列；移除变量不删除历史宽表列，前端文案不要暗示会删列。

#### 检测配置和任务运行接口

- 前端选择/编辑“检测配置”时走 HTTP CRUD，不直接查数据库：`GET /api/v1/detection-standards?project_id=...&enabled=true` 列表，`GET /api/v1/detection-standards/{id}` 取标准项，`POST/PATCH/PUT /api/v1/detection-standards` 增删改标准和标准项。选择当前检测配置本质上是在启动检测时传 `standard_id`，或由 task-flow 的 `builtin.start_detection_run` 参数传入 `standard_id`。
- 前端展示“当前运行检测配置”时不要再读最新标准表，要读运行快照：优先 `GET /api/v1/detection-runs/current?project_id=...`，它会返回该项目当前 running 或 paused 的检测详情；也可用 `GET /api/v1/detection-runs?project_id=...&status=running&limit=1` 或先 `GET /api/v1/detection-runs/active` 找到 `task_id`，再 `GET /api/v1/detection-runs/{id}`。返回体里的 `standard_items` 是本次检测冻结快照，字段包含 `check_enabled/alarm_enabled/store_enabled/check_cycle_ms/check_on_start/limit_*/limit_deadband/violation_hold_ms/recover_hold_ms/quality_policy/variable_default_*`。
- 实时 OK/NG 展示推荐组合：WS 或 `GET /api/v1/realtime/variables?project_id=...` 提供实时值；HTTP `GET /api/v1/detection-runs/{id}` 提供当前 run 配置快照；HTTP `GET /api/v1/detection-runs/{id}/summary` 和 `/events` 提供任务级结果/事件。不要把上下限配置塞进实时变量 WS 消息里，否则通用实时通道会被某一类业务污染。
- 即时通知推荐接 WS：`topic=notifications` 会收到 `notification.event`，payload 为 `RuntimeNotification`。当前类型包括 `alarm.limit.enter/recover/level_change`、`detection.run_started/stopped/abnormal_stop/paused/resumed`、`detection.result_ok/result_ng`、`detection.features_updated`。断线补历史和未读状态走 HTTP 通知中心接口：`GET /api/v1/notifications`、`GET /api/v1/notifications/unread-count`、`POST /api/v1/notifications/{id}/read`、`POST /api/v1/notifications/read-all`。通知列表支持 `unread/type/level/project_id/limit/offset`，返回 `{items,total,limit,offset}`，`payload` 是 JSON 对象。
- 检测标准项已支持业务报警和判断周期：`alarm_enabled/check_cycle_ms/check_on_start/check_method/target_value/limit_deadband/violation_hold_ms/recover_hold_ms/quality_policy/limit_ll/limit_l/limit_h/limit_hh/store_enabled/check_enabled`。
- 运行任务接口已支持暂停/恢复和结果查询：`POST /api/v1/detection-runs/{id}/pause`、`POST /api/v1/detection-runs/{id}/resume`、`GET /api/v1/detection-runs/{id}/summary`、`GET /api/v1/detection-runs/{id}/events`、`GET /api/v1/detection-runs/{id}/features`。
- `duration_ms`/固定时长结果会扣除暂停时长；前端展示累计检测时长时应使用后端 summary 或任务字段，不要自行用 wall-clock 简算。

#### 任务系统接口

- `GET /api/v1/task-modules` 已返回后端内置模块及 `params_schema`。前端任务编辑器应按 schema 动态渲染参数表单，当前模块包括检测启动/结束/暂停/恢复、固定时长守护、合格持续守护、报警静音、运行中限值调整、特征值刷新、storage snapshot/prepare、受控写变量、报表登记、HTTP 请求、context set、JavaScript。
- `GET /api/v1/task-flow-templates` 已返回 12 个后端模板，覆盖变量请求启动/停止检测、固定时长检测、合格持续检测、暂停、恢复、存储快照、静音、运行中限值调整、特征值刷新、报表登记和受控写变量。前端应提供“从模板创建任务”，再允许用户编辑参数绑定。
- `GET/POST/PATCH /api/v1/task-flows` 已实现任务流配置，`GET /api/v1/task-flows/{id}` 已实现单条详情。核心字段：`project_id/flow_code/name/enabled/trigger_type/condition_script/action_type/action_script/action_payload/steps_json/timeout_ms/cooldown_ms/hold_ms/schedule_interval_ms/priority/remark/vars`。`trigger_type=schedule` 时前端应填写 `schedule_interval_ms`，单位毫秒；过渡期若未填写但有 `cooldown_ms`，后端会用 `cooldown_ms` 作为定时间隔。`PATCH` 已支持修改 `project_id`，替换 `vars` 时后端会按新项目同步变量绑定。
- `DELETE /api/v1/task-flows/{id}` 已实现删除任务流配置和变量绑定；执行历史 `task_flow_runs/task_flow_sql_logs` 保留。
- `steps_json` 推荐由前端发送 JSON 字符串化后的 step 数组：`[{code,module,params,script}]`。后端为了调试容错也接受单个 step object，但前端正式生成必须用数组。
- `vars` 用于声明变量角色，`watch` 变量进入 `var_id -> flow_ids` 倒排索引，`read/write` 用于 UI 和审阅意图。正式业务入口要求 `watch` 一个 `STRING` 虚拟变量。
- 正式业务参数不再来自 `POST /task-flows/{id}/run` 弹窗。前端应把用户填写的参数 JSON 写入 watched `STRING` 虚拟变量，例如 `{"command":"start_detection","project_id":1,"standard_id":5,"duration_sec":3600}`，由数据变化触发任务系统并解析为 `task_params`。
- `POST /api/v1/task-flows/{id}/run` 只保留开发者调试触发，不承载正式业务参数。
- `GET /api/v1/task-flow-runs`、`GET /api/v1/task-flow-runs/{id}`、`GET /api/v1/task-flow-runs/{id}/sql-logs` 已实现执行记录和 SQL 日志查询，列表支持 `project_id/flow_id/flow_code/status/trigger_type/trigger_var_id/origin_flow_id/from/to/limit/offset`。
- 任务内 JavaScript 可用，但普通业务优先选择内置模块。允许开发者脚本通过后端受控能力访问 realtime、storage、db，但前端要把它标记为高风险开发者功能，并展示 SQL 日志。

#### 前端验收

- 必跑：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test -- --run`。
- HTTP smoke：登录后验证 `GET /api/v1/variables`、`GET /api/v1/storage-routes?project_id=1`、`GET /api/v1/task-modules`、`GET /api/v1/task-flow-templates`、`GET /api/v1/task-flow-runs?limit=10`。
- 业务 smoke：创建 STRING 请求虚拟变量，按模板创建 task-flow，WS `command.write_variable` 写入 JSON，确认 `task_flow_runs` 成功，并能看到检测启动/停止、可选存储、可选报警、暂停/恢复、特征值刷新至少一条链路；另开一个 `topic=notifications` WS 连接验证报警通知和检测 OK/NG 通知，再用 HTTP 通知接口验证通知列表、未读数和标记已读。

## EB-018 施工流程

文档位置：

- 主设计文档：`后端模块化审阅与演进计划.md`
- 业务约束文档：`多设备并行检测任务设计计划.md`
- 后端现状文档：`backend/docs/backend-architecture.md`
- 交互看板：`AI_BOARD.md`

施工原则：

1. 只做中等粒度拆分，不做形式化过度分层。
2. 不新增 `internal/detection` 包，避免和 `internal/services/detection_runs_service.go` 形成双入口。
3. 新增后端接口必须走 `runtime/handlers + services`，不要继续直接塞进 `runtime/kernel.go`。
4. `database.Repository` 可以按文件拆分，但先不拆 package，对外方法名保持兼容。
5. `models.go` 可以按文件拆分，但先不拆 package，GORM table name、JSON 字段和 API DTO 不变。
6. 前端可见 API、DTO、错误码、权限语义变化，必须同步 `AI_BOARD.md`，并追平 `desktop/src/shared/api/types.ts` 和 feature API。

施工顺序：

1. P0 冻结边界：确认 `runtime/handlers` 只处理 HTTP 入参、权限注册、状态码和响应；`services` 处理业务不变量、状态转换、repo/cache 协调；`database` 处理持久化；`pipeline` 保持实时热路径。
2. P1 新业务不进旧大文件：虚变量、检测任务、报表模板、后续主站控制入口都继续使用 handler/service；新增错误使用明确 error，并在 handler 映射 HTTP 状态码。
3. P2 拆旧 `kernel.go`：按 `users_handler.go`、`devices_handler.go`、`history_handler.go`、`detection_standards_handler.go`、`gateways_handler.go`、`kio_handler.go` 顺序迁移旧路由，迁移期间保持 URL、权限、DTO、错误语义不变。
4. P3 拆 `repository.go` 文件：拆成 `auth_repo.go`、`gateways_repo.go`、`variables_repo.go`、`devices_repo.go`、`history_repo.go`、`detection_repo.go`、`reports_repo.go`；仍保留同一个 `database` package 和同一个 `Repository` 类型。
5. P4 拆 `models.go` 文件：拆成 `auth.go`、`gateway.go`、`device.go`、`variable.go`、`history.go`、`detection.go`、`report.go`、`runtime_tag.go`；仍保留同一个 `models` package。
6. P5 前端契约追平：补 `source_type`、`createVariable`、`DetectionRun` 列表/详情 DTO、`ReportTemplate` DTO、start/stop/abnormal-stop mutation、`history/data?task_id` 查询入口。

每阶段验收：

- 后端改动后至少运行 `gofmt`、`backend/go test ./...`、`backend/go build ./cmd/edge-backend`。
- API/DTO/错误语义变化必须在本看板追加 Activity Log。
- 迁移旧路由时不得改变现有 URL、权限 capability、JSON 字段和状态码。
- 前端可见变更必须由 `frontend-ai` 在 EB-016 或新任务中接入 typed API。
- 如果某阶段无法跑测试，必须在 Activity Log 写明未跑原因。

明确禁止：

- 禁止把检测运行再拆出第二套 `internal/detection` 入口。
- 禁止为每个 endpoint 建一个 service。
- 禁止为普通 CRUD 引入无实际收益的 interface。
- 禁止让 handler 直接操作 `TagManager` 热路径；需要通过 service 协调。
- 禁止把报表模板混进检测标准，二者是相关但独立的业务概念。

## Local Test Credentials

以下凭据只用于本地联调和实验环境，生产交付前必须替换。

| 用途 | 位置/接口 | 账号/标识 | 密码/Token | 备注 |
| --- | --- | --- | --- | --- |
| Edge 本地登录 | `POST /api/v1/auth/login` | `admin` | `Admin@12345` | 默认 bootstrap 管理员，拥有 `admin` 角色全部本地 capability。 |
| 本地 MySQL | `backend/configs/config.json` | `root` | `root` | 数据库名 `spindle_edge`。 |
| 默认 MQTT broker | `backend/configs/config.json` gateway `default-kingiot-kio` | `Admin` | `admin` | broker `tcp://127.0.0.1:1883`，topic `datachange_S_KIO_Project`。 |
| KIO 写入凭据 | `backend/configs/config.json` gateway `default-kingiot-kio` | `sa` | `C12E01F2A13FF5587E1E9E4AEDB8242D` | 用于 KIO write payload，不是 MQTT broker 登录。 |
| 主站 service token | `auth.service_clients` 或 `EDGE_MAIN_SERVICE_TOKEN` | 待配置 | 待配置 | 当前运行配置为空；单元测试中的 `main-token` 仅测试用，不是本地运行凭据。 |

## Board Rules

1. 开工前先声明身份，再读 Active Board。
2. 新问题必须加到 Active Board，分配合法 `Owner`。
3. 解决后改为 `closed`，并追加 Activity Log。
4. 无法推进时改为 `blocked`，写清缺什么。
5. API、DTO、错误语义、页面范围变化，先更新 Active Board，再同步稳定契约文档。
6. `test-ai` 必须记录验证命令、通过/失败结果、未跑原因。
7. `review-ai` 必须优先列风险、缺口和阻塞。
8. 最终回复必须说明身份、处理的 Board 项、仍 open/blocked 的项。
9. 闭合历史过长时，由 `review-ai` 归档到 `.ai/docs/archive/` 或阶段总结。

## Activity Log Format

```text
- YYYY-MM-DD HH:mm | <frontend-ai/backend-ai/test-ai/review-ai> | <question/decision/answer/blocker/review/test> | <影响范围> | <open/closed/blocked>
```

## Activity Log

- 2026-05-30 23:39 | backend-ai | api | EB-037 补齐检测运行存储快照查询：新增 `GET /api/v1/detection-runs/:id/storage-routes`，权限 `view_realtime`，返回 `{items,count}`；同时修正检测启动冻结逻辑，默认 route 仍作为 disabled 建议自动保证存在，但运行快照会冻结同一变量的所有 enabled storage routes，包括用户自定义 route | open
- 2026-05-30 23:41 | backend-ai/test-ai | test | EB-037 存储快照接口门禁通过：新增 handler 测试覆盖自定义 enabled storage route 在检测启动时被冻结并可由 `/detection-runs/:id/storage-routes` 读回；`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 71.1%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` 通过；后端已重启 PID 10012，smoke 验证 `/health`、admin 登录和 `GET /api/v1/detection-runs/14/storage-routes` 返回 `route_count=2` | closed
- 2026-05-30 23:34 | backend-ai | api | EB-045/046 继续补齐任务流编辑闭环：新增 `GET /api/v1/task-flows/:id`，返回单条任务流配置和变量绑定，供前端编辑器直达、刷新和删除后确认使用；权限仍为 `system_settings` | open
- 2026-05-30 23:34 | backend-ai/test-ai | test | EB-045/046 任务流详情接口门禁通过：测试覆盖创建后按 ID 读取配置和变量绑定、DELETE 后详情不可读且执行历史仍保留；`go test ./... -coverprofile all-coverage.out` 总覆盖率 71.1%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` 通过；后端已重启 PID 38180，smoke 验证 `/health`、admin 登录、创建任务流、`GET /task-flows/:id` 和 DELETE 均通过 | closed
- 2026-05-30 21:18 | backend-ai | api | EB-045/046 继续补齐任务流管理闭环：`PATCH /api/v1/task-flows/:id` 支持修改 `project_id`，替换 `vars` 时后端同步变量绑定项目；新增 `DELETE /api/v1/task-flows/:id` 删除任务流配置和变量绑定，执行历史 `task_flow_runs/task_flow_sql_logs` 保留用于审计；前端任务页可以取消“编辑已有任务时项目不可改”的限制并增加删除动作 | open
- 2026-05-30 21:22 | backend-ai/test-ai | test | EB-045/046 任务流管理门禁通过：新增测试覆盖 PATCH 修改 `project_id` 并同步 `sys_task_flow_vars.project_id`、DELETE 删除任务流配置和变量绑定、执行历史保留；`go test ./... -coverprofile all-coverage.out` 总覆盖率 71.1%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` 通过；后端已重启 PID 12648，smoke 验证 `/health`、admin 登录、创建任务流、PATCH 改项目、DELETE 删除均通过 | closed
- 2026-05-30 21:04 | backend-ai | api | EB-045 继续迭代定时任务：`sys_task_flows` 新增 `schedule_interval_ms`，`POST/PATCH /api/v1/task-flows` 读写该字段；`TaskFlowExecutor` 新增轻量 schedule scanner，每秒扫描 `trigger_type=schedule` 的启用任务，到期后投递现有任务流队列并写 `task_flow_runs`；兼容期若 schedule flow 未填 `schedule_interval_ms` 但填了 `cooldown_ms`，后端用 `cooldown_ms` 作为定时间隔；前端任务表单需新增定时间隔字段 | open
- 2026-05-30 21:08 | backend-ai/test-ai | test | EB-045 定时任务门禁通过：新增测试覆盖 `schedule_interval_ms` 到期触发、`cooldown_ms` 兼容兜底、无间隔 schedule 不执行，以及 schedule scanner 产生 `task_flow_runs` 成功记录；`go test ./... -coverprofile all-coverage.out` 总覆盖率 71.2%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` 通过；后端已重启 PID 13528，smoke 验证 `/health`、admin 登录、`task-modules=16`、`task-flow-templates=12`、`task-flows?trigger_type=schedule` 和 JavaScript 模块支持 `schedule` 均通过 | closed
- 2026-05-30 20:48 | backend-ai | api | EB-045 继续迭代项目生命周期任务流：检测启动成功后派发 `TaskFlowEvent(project_start)`，检测正常结束、任务流结束或异常结束后派发 `TaskFlowEvent(project_end)`；触发源覆盖任务流内置检测模块和 HTTP/WS 复用的 `DetectionRunsService`；生命周期 `task_params` 携带 `task_id/project_id/project_code/test_no/status/end_type/trigger_type`，可用于存储准备、特征值刷新、报表登记和通知；`schedule` 触发类型仍为配置元数据，尚未接定时调度 worker | open
- 2026-05-30 20:52 | backend-ai/test-ai | test | EB-045 生命周期任务流门禁通过：新增测试覆盖任务流内置检测启动/结束触发 `project_start/project_end`，以及 `DetectionRunsService` 启动/停止同样触发生命周期任务；`go test ./... -coverprofile all-coverage.out` 总覆盖率 71.2%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` 通过；后端已重启 PID 35820，smoke 验证 `/health`、admin 登录、`task-modules=16`、`task-flow-templates=12`、JavaScript runtime API 和生命周期 trigger 暴露均通过 | closed
- 2026-05-30 20:30 | backend-ai | api | EB-045 继续迭代：JavaScript 任务模块补齐多变量实时读取 API：`realtime.getMany([var_id])`、`realtime.getByName(var_name, project_id?)`、`realtime.project(project_id?)`；`realtime.write(var_id,value,options?)` 改为复用任务流写变量审计，只允许写虚拟变量，默认 `trigger=false`，需要继续触发任务时必须显式传 `trigger:true/max_depth`；`builtin.write_variable` 同步收紧为只写虚拟变量，物理变量下设必须走 WS/HTTP 的 KIO 写服务；`GET /api/v1/task-modules` 的 JavaScript 模块返回 `runtime_api`，前端无需硬编码脚本能力说明 | open
- 2026-05-30 20:35 | backend-ai/test-ai | test | EB-045 单功能和全量门禁通过：新增测试覆盖多变量 JS 条件读取、按项目读取变量、JS 写 STRING 虚拟变量、写审计落库、默认不触发递归，以及任务流拒绝物理变量假写；门禁通过 `go test ./... -coverprofile all-coverage.out` 总覆盖率 71.0%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`；后端已重启 PID 9892，smoke 验证 `/health`、admin 登录、`task-modules=16`、`task-flow-templates=12` 和 JavaScript `runtime_api` 均通过 | closed
- 2026-05-30 20:21 | backend-ai | decision | EB-045 性能复核后做轻量优化：520 个 `custom_items` 的 STRING 任务参数 payload 实测 166453 bytes，低于 256 KiB 上限，JSON 解析约 3.25ms、转换运行快照约 0.56ms；该参数只用于低频业务动作，不进入 MQTT 热路径，因此暂不需要继续扩大 WS 限制或改成实时通道配置流。已将 `sys_detection_tasks.custom_config_json` 改为紧凑追踪 JSON，只保留必要自定义项字段，运行判断唯一事实仍是 `detection_run_standard_items` 快照 | closed
- 2026-05-30 20:21 | backend-ai/test-ai | test | EB-045 回归门禁通过：新增/更新测试覆盖 520 项 STRING 参数性能预算和紧凑 `custom_config_json` 不落 `created_at` 等膨胀字段；`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.6%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` 通过；后端已重启 PID 13280，smoke 验证 `/health`、admin 登录、`task-modules=16`、`task-flow-templates=12`、`detection-standards/favorites`、`detection-standards/recent` 均通过 | closed
- 2026-05-30 19:35 | backend-ai | api | EB-045 继续迭代：检测业务正式入口保持 STRING 虚拟变量触发任务系统；`sys_detection_tasks` 新增 `limit_check_enabled/end_policy/qualified_hold_ms/custom_config_json`；`builtin.start_detection_run` 支持 `custom_items` 自定义检测项、`limit_check_enabled` 是否启用上下限、`end_policy=manual|fixed_duration|qualified_hold` 和 `qualified_hold_ms`；`builtin.update_detection_limits` 支持 `items` 批量调整运行中检测项并刷新 active task；WS read limit 从 32 KiB 提升到 256 KiB 以承载低频业务配置 payload；新增 `GET /api/v1/detection-standards/favorites|recent`、`POST/DELETE /api/v1/detection-standards/:id/favorite` | open
- 2026-05-30 19:35 | backend-ai | decision | 前端边界确认：检测配置模板、收藏、最近使用、任务历史和当前运行快照可走 HTTP；开始/停止/暂停/恢复/消音/运行中改上下限等改变现场业务状态的动作必须写入 STRING 虚拟变量，由 `sys_task_flows` data_change 触发内置业务模块执行；本轮不做拖拽式前端，只给未来拖拽编辑器保留模块 schema 和参数来源 | open
- 2026-05-30 19:36 | backend-ai/test-ai | test | EB-045 新增单测覆盖 STRING 虚拟变量 JSON 启动自定义检测配置、首帧存储、首帧超限报警、`custom_items` 运行快照冻结、520 个自定义项 payload 解析、超限 payload 拦截、检测配置收藏/最近使用和自定义运行快照；门禁通过：`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.5%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`；后端已重启 PID 37496，smoke 验证 `/health`、admin 登录、`task-modules`、`task-flow-templates`、`detection-standards/favorites`、`detection-standards/recent` 均通过 | closed
- 2026-05-30 17:15 | backend-ai | decision | 已给 frontend-ai 新增 EB-048 交付清单：明确变量属性设定、storage routes、检测配置/运行接口、任务模块/模板/执行记录 API、正式 STRING 虚拟变量触发链、`steps_json` 数组契约、`apply_to_running` 风险提示和旧变量级存储字段废弃范围 | open
- 2026-05-30 17:18 | backend-ai | test | EB-048 交付接口轻量 smoke 通过：`/health` ok 且 KIO gateway active、队列全 0；登录后 `GET /api/v1/variables?source_type=virtual` 返回 5 条、`GET /api/v1/storage-routes?project_id=1` 返回 50 条、`GET /api/v1/task-modules` 返回 16 个模块、`GET /api/v1/task-flow-templates` 返回 12 个模板、`GET /api/v1/task-flow-runs?limit=5` 返回 5 条；后端 PID 22656 继续运行 | closed
- 2026-05-30 17:35 | backend-ai | api | EB-019 通知总线首段完成：新增非持久运行态 `Channels.Notify` 和 `NotificationHub`；WS 新增 `topic=notifications` 和 `notification.event`，payload 为 `RuntimeNotification`，字段包括 `id/type/level/project_id/project_code/task_id/test_no/var_id/var_name/display_name/message/payload/occurred_at`；当前通知类型覆盖 `alarm.limit.enter/recover/level_change`、`detection.run_started/stopped/abnormal_stop/paused/resumed`、`detection.result_ok/result_ng`、`detection.features_updated`。前端 shared type 已同步，前端需接入全局通知/工位通知展示 | open
- 2026-05-30 17:36 | backend-ai/test-ai | test | EB-019 通知总线验证通过：`gofmt`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.3%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test -- --run` 均通过；后端监听 PID 25436，`/health` ok 且队列含 `notify=0`，WS `topic=notifications` smoke 新建并停止 `SMOKE-NOTIFY-20260530173551`，收到 `detection.run_started` 和 `detection.run_stopped` | closed
- 2026-05-30 18:35 | backend-ai | api | EB-019 通知中心持久化落地：新增 `sys_notifications`、`sys_notification_recipients`，`Channels.Notify` 由 `NotificationDispatcher` 先落库再通过 `NotificationHub` 在线推送；新增 `GET /api/v1/notifications`、`GET /api/v1/notifications/unread-count`、`POST /api/v1/notifications/{id}/read`、`POST /api/v1/notifications/read-all`，权限为 `view_realtime`。前端 shared DTO/API 已同步，EB-048 需接 UI 未读数、列表和已读操作 | open
- 2026-05-30 18:35 | backend-ai/test-ai | test | 通知中心持久化验证通过：repository 测试覆盖通知落库、启用用户 recipient、未读过滤、标记单条/全部已读和重复 `event_uid` 幂等；handler 测试覆盖认证路由、列表 payload 对象、未读数、404 和 bad query；dispatcher 测试覆盖落库后 WS 发布。门禁通过 `go test ./...`、覆盖率 71.1%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test -- --run` | closed
- 2026-05-30 18:36 | backend-ai/test-ai | test | 后端已用新二进制重启并保持 `127.0.0.1:18080` PID 18164；smoke 通过 `/health`、admin 登录、通知未读数、通知列表；启动并停止 `SMOKE-NOTIFY-PERSIST-20260530183332` 后 HTTP 通知列表出现 `detection.run_started`、`detection.run_stopped`、`detection.result_ok` 3 条，随后单条已读和全部已读接口验证 unread=0 | closed
- 2026-05-30 18:52 | backend-ai | api | EB-019 通知分发对象细化：`NotificationDispatcher` 落库生成 recipient 时已按 `target_type` 分发，`all` 给全部启用本地用户，`user` 给指定用户 ID，`role` 给指定角色用户，`project` 因项目成员关系未建模暂时兜底给全部启用本地用户；无效目标只保留通知主体不生成 recipient | open
- 2026-05-30 18:55 | backend-ai/test-ai | test | 通知分发对象细化验证通过：新增 repository 测试覆盖 `role=operator` 只给 operator、`user=<id>` 只给指定用户、无效 user target 不生成 recipient；门禁通过 `go test ./...`、覆盖率 71.1%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`；后端已重启 PID 8644，`/health`、admin 登录、通知未读数和通知列表 smoke 通过 | closed
- 2026-05-30 17:30 | backend-ai | decision | 明确前端实时检测配置获取方式：配置管理走 HTTP `detection-standards` CRUD；“当前配置”不是最新标准表，而是检测启动后冻结的 `detection_run_standard_items`，前端通过 `GET /api/v1/detection-runs?project_id=...&status=running&limit=1` 或 `GET /api/v1/detection-runs/{id}` 获取；实时值仍走 WS/`realtime/variables`，由前端按 run snapshot 拼 OK/NG 展示 | open
- 2026-05-30 17:45 | backend-ai | api | 新增 `GET /api/v1/detection-runs/current?project_id=...`，返回某项目当前 running 或 paused 检测详情，包含冻结 `standard_items`、`storage_routes`、notes 和 reports；缺 `project_id` 返回 400，无当前任务返回 404。前端 shared API 新增 `getCurrentDetectionRun(projectId)` | open
- 2026-05-30 17:55 | backend-ai/test-ai | test | 当前检测配置接口验证通过：handler 测试覆盖 running/paused current run 和缺 `project_id` 400；HTTP smoke 启动 `SMOKE-CURRENT-*` 检测任务 `task_id=12`，`GET /api/v1/detection-runs/current?project_id=1` 返回同一 running 任务，随后 stop 成功。门禁通过 `go test ./...`、覆盖率 70.2%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test -- --run`；后端 PID 14700 继续运行，`/health` ok、KIO gateway active、队列全 0 | closed
- 2026-05-30 14:42 | backend-ai | decision | 现场 KingIO 写入测试暴露并修复两个问题：默认 KIO `Qid` 从纳秒级时间戳改为 `<1_000_000_000` 的小范围递增值，避免 KingIO ack 匹配失败；自动发现 upsert 不再覆盖已分配业务变量的 `rw_mode/writable/write_path/write_data_type` 等人工写入配置 | closed
- 2026-05-30 14:43 | backend-ai | test | KingIO 现场写入通过：直接 KIO 写 `台1_39=13.42` 返回 `ProcessStep=100/Result=ok`；WS `command.write_variable` 写 `var_id=5826452156569908253` 到 `13.43` 返回 `command.ack`、`Project_confirmed=true`、KIO `status=confirmed`，实时快照回刷为 `13.430000305175781`，审计日志写入 `ws.command.write_variable` success；后端保持 `127.0.0.1:18080` PID 36240 | closed
- 2026-05-30 14:44 | backend-ai | test | 修复后门禁通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.0%、`go vet ./...`、`golangci-lint run ./...`、`go build ./cmd/edge-backend` | closed
- 2026-05-30 15:30 | backend-ai | decision | 实时快照粒度控制补强：`GET /api/v1/realtime/variables` 新增 `source_type/gateway_id/project_id/device_id/var_id` 过滤；WS `var_id` query 支持逗号或重复参数，并在后端走直接 tag lookup，项目视图走 project index；无过滤仍表示有意全量快照，前端项目页应传 `project_id` 防止大快照 | closed
- 2026-05-30 15:30 | frontend-ai | decision | 工位页实时变量查询改为有选中项目时调用 `getRealtimeVariables({project_id})`，shared API 新增 `RealtimeVariableListParams`；调试/全局页面仍可不传参数获取全量 | closed
- 2026-05-30 15:32 | backend-ai/test-ai | test | 实时粒度控制验证通过：新增后端单测覆盖 HTTP realtime `project_id/device_id/var_id` 过滤和 WS comma `var_id` 订阅；门禁通过 `go test ./...`、覆盖率 70.0%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`；前端 `npm run lint`、`npm run build`、`npm run test -- --run` 通过；后端已重启 PID 36924，smoke 验证全量 528、project_id=1 返回 8、单点 1、多点 2、非法 var_id 返回 400，WS 多点快照返回 2；复测确认 WS `Authorization: Bearer <access_token>` 可握手，前次 401 是测试脚本取错 token 字段 | closed
- 2026-05-30 15:55 | backend-ai | api | EB-047 首段实施：新增 `POST /api/v1/variables/bulk-remap/kio-projects`，用于本地 KIO 调试批量确保 12 个项目并按 `台N_xx` 原名把变量归属到项目 N，重映射 `var_name/display_name*`，创建 disabled 默认 storage route，并重载 `TagManager`；前端 shared API 已补 `bulkRemapKioProjects` 类型与调用 | open
- 2026-05-30 16:01 | backend-ai/test-ai | test | EB-047 执行与验证：本地 dry-run 命中 504 个 `台N_xx` KIO 变量；真实执行创建缺失项目 6 个并更新 504 个变量，形成 `AC-01..AC-12`，项目实时快照为 AC-01 46 个、AC-02..AC-12 各 42 个，`/health` tags=1028；复测 `台1_41` 保留 `raw_name/write_path=台1_41` 和 `RW` 写入约束，业务名为 `kio_01_41`，disabled 默认 route 列同步为 `kio_01_41` | open
- 2026-05-30 16:01 | backend-ai/test-ai | test | EB-047 门禁通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.1%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`；前端 `npm run lint`、`npm run build`、`npm run test -- --run` 通过；后端已用新二进制重启并保持 `127.0.0.1:18080` PID 49904 | open
- 2026-05-30 16:28 | backend-ai | api | EB-045/046 任务模板继续迭代：`GET /api/v1/task-flow-templates` 增加固定时长检测、合格持续检测、暂停、恢复、运行中限值调整、特征值刷新和报表登记模板；任务流参数绑定支持 `optional=true` 和 `default`，前端 shared API 补任务模块、模板、执行记录和 SQL 日志 typed 调用 | open
- 2026-05-30 16:28 | backend-ai/test-ai | test | EB-045/046 验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.2%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`；前端 `npm run lint`、`npm run build`、`npm run test -- --run` 通过；后端重启 PID 60884，`/health` ok，smoke 确认 task modules=16、task-flow templates=12 | open
- 2026-05-30 14:22 | backend-ai | test | EB-019/027/045 回归补强：新增 WS `command.write_variable` 写 STRING 虚拟变量触发 task-flow 的 handler 级测试，覆盖变量内存更新、倒排索引触发、task_flow_runs 成功记录和 `ws.command.write_variable` 审计落库；本轮未启动 KingIO/IO 服务 | closed
- 2026-05-30 14:23 | backend-ai | test | 验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.0%、`go vet ./...`、`golangci-lint run ./...`、`go build ./cmd/edge-backend`；本地后端仍监听 `127.0.0.1:18080` PID 57656 | closed
- 2026-05-30 14:12 | backend-ai | decision | EB-019/027 继续迭代：拆出 `KIOWriteService`，新增 `VariableWriteService`，WS `command.write_variable` 支持写 STRING 虚拟变量触发任务流，也支持满足 `writable/rw_mode/write_path` 约束的物理变量经 KIO 下设；前端/主站不得绕过后端直写 MQTT | closed
- 2026-05-30 14:13 | backend-ai | test | EB-019/027 验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.0%、`go vet ./...`、`golangci-lint run ./...`、`go build ./cmd/edge-backend` 通过；后端已重启为 PID 57656，并通过 `/health`、登录、`task-modules` smoke | closed
- 2026-05-30 13:55 | backend-ai | decision | EB-045 任务模块继续迭代：新增 task-flow-runs/SQL 日志查询 API、受控 write_variable、origin/depth 防递归、任务模板、检测静音/level_change/运行中限值调整、storage_prepare、register_report、http_request；前端需适配 EB-046 新契约 | open
- 2026-05-30 13:58 | backend-ai | test | EB-045 验证通过：`go test ./...`、覆盖率 70.1%、`go vet ./...`、`golangci-lint run ./...`、`go build ./cmd/edge-backend`；后端已重启并通过 `/health`、登录、`task-modules/task-flow-templates/task-flow-runs` smoke | open
- 2026-05-27 09:55 | review-ai | decision | AI 协作文档体系初始化 | open
- 2026-05-27 09:55 | test-ai | test | `backend/go test ./...` 通过，当前无测试文件 | open
- 2026-05-27 12:20 | frontend-ai | decision | 前端技术选型固定为 Electron + React 19 + TS 6 + Vite 8 + Ant Design + TanStack Query + Zustand + XState；Luckysheet 仅允许走隔离 adapter | open
- 2026-05-27 12:20 | frontend-ai | decision | 初始化 `desktop/`，Electron main 负责 Go sidecar 启动、健康轮询和日志入口，renderer 只走 preload 与 typed HTTP client | open
- 2026-05-27 12:35 | test-ai | test | `desktop/npm run build`、`desktop/npm run lint`、`backend/go test ./...`、`desktop/npx electron-builder --dir`、`desktop/npm run package` 通过；Playwright 已截图验证 `http://127.0.0.1:4173` 首屏 | open
- 2026-05-28 16:57 | frontend-ai | decision | 从 `react-design-prototype` 迁入工位操作与历史查询页面，新增工作台导航；历史查询暂用前端样例数据，真实历史 API 待 backend-ai 提供 | open
- 2026-05-28 17:24 | frontend-ai | decision | 工位操作页按原型补齐左侧卡片池自适应高度、拖拽占位/overlay、胶囊按钮、动态背景与橘黄色物理光源高光 | open
- 2026-05-28 17:46 | frontend-ai | decision | 对照运行中的 `react-design-prototype` 继续还原工位页：卡片图表改回原型 ChartLine 黑线/MinMax 标签，右侧表格改回 table 结构、独立表头和 hover 滚动条 | open
- 2026-05-28 18:26 | frontend-ai | decision | 修复工位右侧固定表头下滚动条默认可见问题；历史查询页按原型拆分为图表、表格、甘特弹窗与样式组件，并新增 `.ai/skills/edge-glass-prototype-style` 风格还原 skill | open
- 2026-05-28 18:27 | frontend-ai | test | `desktop/npm run lint` 与 `desktop/npm run build` 通过；Playwright 已对 `/#/station`、`/#/history`、历史甘特弹窗和原型 `/reports` 同视口截图 | open
- 2026-05-28 18:34 | frontend-ai | decision | 设置页范围确认：实时数据源服务接 MQTT 站点、设备/项目分组、发现变量与数据库变量；历史数据源服务仅预留数据库连接来源配置位 | open
- 2026-05-28 19:38 | frontend-ai | decision | 新增 `/#/settings` 系统设置页、侧边栏入口、三语文案、typed API 扩展；实时数据源页接 gateway-configs/gateways/devices/variables/discover/assignment，历史数据源保持预留说明 | open
- 2026-05-28 19:39 | frontend-ai | test | `desktop/npm run lint` 与 `desktop/npm run build` 通过；Playwright 已截图验证 `/#/settings` 实时数据源与历史数据源两个 tab | open
- 2026-05-28 19:53 | frontend-ai | decision | 设置页信息架构调整为变量优先：项目/设备分组与变量列表为主，MQTT 站点降级为一次性采集源配置；本地后端离线时明确提示无法加载站点/分组/变量 | open
- 2026-05-28 19:54 | frontend-ai | test | `desktop/npm run lint` 与 `desktop/npm run build` 通过；确认当前 `127.0.0.1:18080` 未监听导致站点数据无法加载，Playwright 已截图验证离线提示 | open
- 2026-05-28 20:05 | frontend-ai | decision | 设置页进一步收敛为“站点概览 + 左侧变量筛选 + 右侧变量全信息表”；MQTT 站点新增/编辑/发现变量移动到弹窗，不再占主工作区 | open
- 2026-05-28 20:06 | frontend-ai | test | `desktop/npm run lint` 与 `desktop/npm run build` 通过；Playwright 已验证真实站点/变量数据渲染和站点弹窗 | open
- 2026-05-28 20:20 | frontend-ai | decision | 设置页重构为管理中心：左侧模块导航覆盖变量列表、实时数据源、历史数据源、系统设置；变量列表为默认主工作区并占主要面积 | open
- 2026-05-28 20:21 | frontend-ai | test | `desktop/npm run lint` 与 `desktop/npm run build` 通过；Playwright 已验证管理中心默认变量工作区 | open
- 2026-05-28 20:28 | frontend-ai | decision | 管理中心模块入口移动到标题右侧展示卡片，移除左侧模块导航，释放主区域宽度给变量筛选与变量表 | open
- 2026-05-28 20:29 | frontend-ai | test | `desktop/npm run lint` 与 `desktop/npm run build` 通过；Playwright 已验证卡片入口版管理中心 | open
- 2026-05-28 20:43 | frontend-ai | decision | 工位操作改为桌面首屏，边缘概览降级为调试看板；左侧工位操作按项目/设备自动扩展二级菜单，工位页按选中设备过滤实时变量与状态卡 | open
- 2026-05-28 20:44 | frontend-ai | test | `desktop/npm run lint` 与 `desktop/npm run build` 通过；Playwright 验证 `/#/` 首页工位、6 个设备二级菜单、`device_id=1` 精确高亮与状态卡切换 | open
- 2026-05-29 00:03 | frontend-ai | test | `desktop/npm run package` 完成 Go 后端与前端构建但旧 `release/win-unpacked` 文件锁导致首次安装包输出失败；改用全新输出目录验证 `electron-builder --dir` 可运行包、打包后 Electron 启动内置 `resources/backend/edge-backend.exe`，`/health` 返回 ok 且 tags=521；随后 `release-installer-smoke` 完整生成 nsis、portable 与 blockmap 产物 | open
- 2026-05-28 19:54 | backend-ai | test | 新增 `backend/tools/dbinit` 初始化 Docker 映射的本地 MySQL schema；`go test ./...`、`go build ./cmd/edge-backend` 通过；启动 `go run ./cmd/edge-backend`，`/health` 与 `/api/v1/runtime/channels` 返回 200 | open
- 2026-05-28 19:58 | backend-ai | answer | 为本地桌面/浏览器来源补充 Gin CORS 与 OPTIONS 预检；`go test ./...`、`go build ./cmd/edge-backend` 通过，Playwright 验证 `/#/settings` 无离线提示且浏览器上下文可读取 3 个变量 | open
- 2026-05-28 20:14 | review-ai | decision | 新增登录权限与 SSO 安全设计，明确本地用户 JWT、主站 service token、一次性 SSO ticket、三角色 capability 和后续 API 维护规则 | open
- 2026-05-28 23:57 | review-ai | decision | 将登录权限落地从 EB-004 总项拆分为后端认证鉴权 EB-008、前端登录权限守卫 EB-009、前端用户管理页面 EB-010 | open
- 2026-05-29 00:28 | backend-ai | test | 完成 EB-008：新增 Edge Auth Lite 后端认证鉴权、服务身份、SSO ticket、审计表和路由权限绑定；`go test ./... -coverprofile all-coverage.out` 通过且全包覆盖率 74.3%，`go build ./cmd/edge-backend` 通过，HTTP smoke 验证 `/health` 200、未登录实时变量 401、admin 登录和 `/auth/me`/`runtime/channels` 200 | closed
- 2026-05-29 00:34 | backend-ai | answer | 在看板新增 Local Test Credentials，集中记录本地登录、MySQL、MQTT、KIO 写入和 service token 配置状态，供前端联调使用 | open
- 2026-05-29 00:51 | frontend-ai | decision | 完成 EB-009：新增玻璃风格登录页、内存 token 会话恢复、路由守卫、401 自动清会话、403 权限页、capability 菜单显隐、退出登录和主站 SSO ticket 入口；Electron openExternal 保持在 desktop adapter，业务页面保持可迁移到主服务器 | closed
- 2026-05-29 00:52 | frontend-ai | decision | 以工位操作和历史查询为基准统一设置页、调试看板的浅色动态背景、玻璃面板、胶囊按钮和滚动条风格；调试看板登录/主站文案更新为当前认证状态 | open
- 2026-05-29 00:53 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build` 通过；`desktop/npm run test` 因当前无 test/spec 文件按 Vitest 默认返回 1；Playwright 验证未登录跳转登录、`admin/Admin@12345` 登录、设置/历史/调试看板/工位访问、退出登录和 SSO 未配置提示 | closed
- 2026-05-29 00:50 | backend-ai | decision | 新增根目录 `计划.md`，基于当前后端设计和 JG HJ 旧实现，明确采集与业务模块按 raw、clean、business、detection、export、验收分 9 次主迭代实现 | open
- 2026-05-29 01:01 | backend-ai | decision | 为 EB-010 补齐用户管理后端接口：`GET/POST/PATCH/DELETE /api/v1/users` 与 `POST /api/v1/users/:id/reset-password`，并限制当前用户自禁用/自删除 | closed
- 2026-05-29 01:02 | frontend-ai | decision | 管理中心新增用户管理模块，用户入口与变量/实时/历史/系统入口同属标题右侧展示卡片；业务页面只依赖 shared API 与 auth store，避免 Electron-only 逻辑，便于后续迁移到主服务器 | closed
- 2026-05-29 01:03 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build`、`backend/go test ./...`、`backend/go build ./cmd/edge-backend` 通过；`desktop/npm run test` 因无 test/spec 文件返回 1；浏览器 smoke 验证 admin 登录、用户新增、改角色、重置密码、新密码登录、删除和测试用户清理 | closed
- 2026-05-29 01:18 | frontend-ai | decision | 完成 EB-005：Electron 主进程新增托盘、最小化到托盘、Windows 开机自启动开关、sidecar 异常退出最多 5 次重拉起；设置中心系统模块提供对应控制，浏览器预览态明确降级 | closed
- 2026-05-29 01:19 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；新增 `desktopBridge` 浏览器降级单测；`backend:build` 与 `electron-builder --dir --config.directories.output=release-platform-smoke` 通过，打包后 exe 启动内置 `resources/backend/edge-backend.exe` 且 `/health` 返回 ok、tags=521 | closed
- 2026-05-29 01:29 | backend-ai | decision | 补齐 `GET /api/v1/history/data` 历史查询接口，支持 device/test/time/limit 过滤，返回 `rt_history_data` 持久化样本；历史数据仍只由检测任务入库，外部数据库同步边界不变 | closed
- 2026-05-29 01:29 | frontend-ai | decision | 历史查询页接入 shared API 的真实历史数据适配器，可将后端变量样本按时间透视为图表/表格行；接口无数据时保留样例数据标识，业务页面不依赖 Electron preload | closed
- 2026-05-29 01:29 | test-ai | test | `backend/go test ./...`、`backend/go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test`、`desktop/npm run backend:build`、`electron-builder --dir --config.directories.output=release-platform-smoke` 通过；打包 exe smoke 验证内置后端 `/health` ok、`/api/v1/history/data?limit=1` 返回 200 | closed
- 2026-05-29 01:41 | backend-ai | decision | 前端可见 DTO 变更：`GET/POST/PATCH /api/v1/devices` 增加/支持 `display_name`、`display_name_en`、`display_name_ja`；`name` 保留兼容，前端展示应按语言读取 display 字段并 fallback 到 `name`/`device_code` | open
- 2026-05-29 01:43 | backend-ai | test | 设备三语显示名后端变更验证：`backend/go test ./...`、`backend/go build ./cmd/edge-backend`、`desktop/npm run build`、`desktop/npm run lint` 通过；已重启本地后端并用 `/api/v1/devices`、`PATCH /api/v1/devices/:id` smoke 验证返回三语字段 | closed
- 2026-05-29 08:45 | review-ai | review | UI 风格复审发现壳层深色侧栏、纯白顶栏、登录/设置/调试看板玻璃质感和控件样式未完全对齐工位/历史查询基准 | closed
- 2026-05-29 08:45 | frontend-ai | decision | 完成 EB-011：统一 shell、login、settings、debug 的浅色动态背景、玻璃面板、边缘光照、胶囊按钮、表格和滚动条样式，保留页面可迁移到主服务器的架构边界 | closed
- 2026-05-29 08:45 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 截图复审 `login/station/history/settings/debug`，产物位于 `desktop/output/playwright/style-audit-after/` | closed
- 2026-05-29 09:13 | backend-ai | decision | 完成 EB-012：新增检测标准主表、标准项表和检测运行标准项快照表；变量项拆分 `check_enabled` 和 `store_enabled`，检测启动时冻结标准版本，历史存储按运行快照的 `store_enabled` 过滤 | closed
- 2026-05-29 09:13 | backend-ai | test | EB-012 验证通过：`backend/go test ./...`、`backend/go test ./... -coverprofile all-coverage.out`、`go tool cover -func all-coverage.out` 总覆盖率 74.2%、`backend/go vet ./...`、`backend/go build ./cmd/edge-backend`、`desktop/npm run build`、`desktop/npm run lint` 通过；`golangci-lint` 本机未安装 | closed
- 2026-05-29 09:14 | backend-ai | test | 已重启本地后端并 smoke 验证 EB-012：创建检测标准含 1 个标准项、带 `standard_id` 启动检测、返回 1 条运行标准快照、停止检测、删除标准配置；`/health` 返回 ok、tags=521 | closed
- 2026-05-29 09:16 | frontend-ai | decision | 按 Apple Vision Pro + Claude 时尚杂志方向微调：历史页“样例数据”从查询按钮区移到图表说明，只保留只读状态；二级菜单选中态去掉蓝色内阴影，改为轻玻璃浮层 | closed
- 2026-05-29 09:16 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 验证工具栏不再出现 `history-data-source`，状态移入图表说明，截图位于 `desktop/output/playwright/visionpro-style-fix-final/` | closed
- 2026-05-29 11:10 | review-ai | review | 新增 `多设备并行检测任务设计计划.md`，结合根目录需求、Word 需求、当前 Go 后端与 React 前端状态，明确并行检测不变量、API 增量、复杂度、Go 规范审阅和防瞎搞硬规则 | open
- 2026-05-29 11:10 | review-ai | decision | 看板新增 EB-015/EB-016/EB-017，分别留给后端、前端、测试推进多设备并行检测闭环 | open
- 2026-05-29 09:46 | frontend-ai | decision | 完成 EB-013：变量管理区分未分配发现变量和已分配设备变量；未分配变量只读基础属性并只能分配，已分配变量开放完整配置编辑；工位菜单和设置筛选按当前语言读取设备 display 字段 | closed
- 2026-05-29 09:46 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 截图验证未分配变量分配弹窗和已分配变量完整编辑弹窗，产物位于 `desktop/output/playwright/variable-edit-model/` | closed
- 2026-05-29 09:46 | frontend-ai | decision | 新增 EB-014：后端检测标准 EB-012 已完成但前端管理入口缺失，需要作为 frontend-ai 下一项处理 | open
- 2026-05-29 11:20 | frontend-ai | decision | EB-013 追加未分配变量小卡片池：未知变量视图只显示基础属性，沿用关键词和左侧筛选，支持全选当前结果、清空选择和批量分配到项目设备 | closed
- 2026-05-29 11:20 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 验证未分配卡片池、批量分配弹窗和隐藏表格态，截图位于 `desktop/output/playwright/unassigned-card-batch/` | closed
- 2026-05-29 11:25 | review-ai | review | 补充虚变量设计边界：当前模型有 `placeholder/discovered` 但没有创建 API/UI；检测开始、状态、结果类 int 变量应作为虚变量由后端服务写入，不应伪装成 PLC 发现变量 | open
- 2026-05-29 11:35 | frontend-ai | decision | EB-013 性能修正：未知变量池从一次性渲染全部玻璃卡片改为每页 48 个轻量卡片，移除隐藏表格渲染，批量按钮改为高对比深色胶囊样式 | closed
- 2026-05-29 11:35 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 验证未知变量页只渲染 48 张卡、有分页、无隐藏表格、批量按钮文字为白色，截图位于 `output/playwright/unassigned-performance-fix/` | closed
- 2026-05-29 11:15 | backend-ai | decision | 完成 EB-015 后端：新增虚变量创建、`source_type`、检测任务并发锁、任务列表/详情/notes/异常停止、报表模板元数据与任务报表关联；前端可见 API/DTO/schema 已同步后端架构文档 | closed
- 2026-05-29 11:23 | backend-ai | test | EB-015 验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out`、`go tool cover -func all-coverage.out` 总覆盖率 72.6%、`go vet ./...`、`go build ./cmd/edge-backend` 通过；已重启本地后端并 smoke 验证 `/health`、登录、`GET /detection-runs`、`GET /report-templates` | closed
- 2026-05-29 11:40 | frontend-ai | decision | 修复批量分配按钮内部文本和图标被 Ant Design 样式压暗的问题，对按钮本体、内部 span、图标、hover/active/disabled 状态统一做高对比颜色覆盖 | closed
- 2026-05-29 11:40 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 读取按钮、文字、图标计算颜色均为 `rgb(255, 255, 255)`，截图位于 `output/playwright/batch-button-readable-retry/` | closed
- 2026-05-29 11:55 | frontend-ai | decision | 批量分配工具栏按钮不再使用 Ant Design Button，改为原生 `button` + 自定义胶囊样式，彻底绕开 Ant 内部状态色覆盖 | closed
- 2026-05-29 11:55 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 验证按钮 DOM 为原生 BUTTON、无 ant-btn 类、文字/图标颜色均为白色，截图位于 `output/playwright/batch-button-native-retry/` | closed
- 2026-05-29 12:00 | frontend-ai | decision | 修正未分配变量分配概念：`var_group` 只是变量自身低频标签，不是项目/设备归属；未分配变量单个/批量分配弹窗移除变量分组输入，只保留项目/设备和启用状态，提交时 `var_group` 自动为空 | closed
- 2026-05-29 12:00 | frontend-ai | test | `desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 验证批量分配弹窗不再出现“变量分组”，只显示“选择项目/设备”和“启用”，截图位于 `output/playwright/variable-assignment-scope/` | closed
- 2026-05-29 11:45 | review-ai | review | 深度复查后发现后端已存在 `internal/services` 与 `internal/runtime/handlers`，虚变量、检测运行、报告模板后端已部分落地；新增 `后端模块化审阅与演进计划.md`，修正为沿用现有 handler/service，不再建议新增 `internal/detection` 包 | open
- 2026-05-29 11:50 | review-ai | decision | 在交互看板为 EB-018 补充详细施工流程，列明文档位置、P0-P5 执行顺序、每阶段验收和禁止事项，避免后续后端模块化按错误边界施工 | open
- 2026-05-29 12:50 | backend-ai | decision | EB-018 后端模块化实施完成：旧 `kernel.go` 内联业务路由迁入 `runtime/handlers`，变量变更统一通过 `VariablesService` 重载 TagManager，`database.Repository` 与 `models` 保持同 package 按领域拆文件；无 API/DTO/schema 变更 | closed
- 2026-05-29 12:50 | backend-ai | test | EB-018 验证通过：`gofmt`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.8%、`go vet ./...`、`go build ./cmd/edge-backend` 通过；已重启本地后端并 smoke 验证 `/health`、登录、devices、variables、history/data、detection-standards、detection-runs、report-templates、runtime/channels | closed
- 2026-05-29 13:22 | backend-ai | review | 确认实时与写操作首选 WebSocket，但必须经过后端 service，不允许客户端绕过后端直接写 MQTT；新增 EB-019/EB-020/EB-021 追踪 WS 通道、写操作审计和检测超限/事件流水 | open
- 2026-05-29 13:28 | frontend-ai | decision | 完成 EB-014：管理中心新增检测标准模块，接入标准列表、新增/编辑/删除、标准项选择、上下限、`check_enabled`/`store_enabled`；同步补齐检测运行、报表模板、虚变量创建等前端 typed API | closed
- 2026-05-29 13:28 | frontend-ai | decision | EB-016 部分完成：管理中心新增“创建虚变量”入口，支持选择项目/设备并创建 `source_type=virtual` 的 `INT/FLOAT/BOOL/STRING` 变量；工位页检测 start/stop/abnormal-stop 与任务历史入口继续 open | open
- 2026-05-29 13:28 | frontend-ai | test | EB-014/EB-016 前端验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright smoke 验证检测标准弹窗和虚变量弹窗，截图位于 `desktop/output/playwright/detection-standards-ui/` 与 `desktop/output/playwright/virtual-variable-ui/` | closed
- 2026-05-29 13:43 | frontend-ai | decision | EB-016 继续推进：工位页接入开始检测弹窗、标准/报表模板选择、start/stop/abnormal-stop mutation、任务历史跳转；历史查询页支持 `task_id`、`device_id`、`test_no` 查询参数 | open
- 2026-05-29 13:43 | frontend-ai | test | EB-016 工位闭环验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright UI smoke 启动本地测试任务 `UI-SMOKE-*` 后成功停止，确认任务历史按钮可见，截图位于 `desktop/output/playwright/station-run-ui/` | closed
- 2026-05-29 13:52 | frontend-ai | decision | 修复 EB-016 虚变量入口：新建虚变量弹窗补齐中文/英文/日文显示名，提交时英文/日文空值回退到中文显示名，避免三语显示断层 | open
- 2026-05-29 13:52 | frontend-ai | test | 虚变量三语名验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright smoke 确认弹窗出现中文显示名、英文显示名、日文显示名 | closed
- 2026-05-29 13:28 | backend-ai | decision | 补充 EB-019 前置约束：变量可以自动发现，但自动发现只生成候选来源变量；必须由变量属性/设备绑定/检测标准配置确认后，才允许参与检测、历史存储、超限判断或 WS 写命令 | open
- 2026-05-29 13:32 | backend-ai | decision | 新增 `backend/docs/变量属性检测配置与WS前置设计.md`，对照 JGHJ 旧后端变量模型，明确来源变量、业务变量、运行态变量和检测配置项的分层，以及 WS 写和检测超限的前置数据结构约束 | open
- 2026-05-29 13:41 | backend-ai | decision | 新增 `backend/docs/后端日志WS检测实施顺序.md`，明确先补全 EB-020 写操作审计和运行日志，再推进变量属性、检测配置、WS 实时读、WS 写命令和检测超限事件 | open
- 2026-05-29 13:52 | backend-ai | decision | 补充变量存储映射设计：变量属性负责 `storage_name/storage_target/storage_table/storage_value_column/form_field_key/query_alias`，上下限仍留在检测配置/运行快照，`store_enabled` 只表示某检测标准是否存该变量 | open
- 2026-05-29 14:02 | backend-ai | decision | 新增 EB-022：历史数据源 MySQL 配置从前端占位改为后端配置文件读写能力；接口必须隐藏明文密码，保存后要求重启后端生效，不在运行中热切换当前 GORM 连接 | open
- 2026-05-29 14:08 | backend-ai | test | EB-022 已完成并验证：`backend/go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.3%、`go vet ./...`、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 均通过；已重启本地后端并确认 `/health` ok、`/api/v1/system/database-config` 可读、`/api/v1/system/database-config/test` 返回 ok | closed
- 2026-05-29 14:20 | backend-ai | decision | 新增 EB-023 给前端适配验收历史数据源 MySQL 配置页；新增 EB-024 到 EB-029 将后端后续拆为日志审计、变量属性/存储映射、检测配置、WS 只读、WS 写、检测事件/超限和联调回归七个阶段 | open
- 2026-05-29 15:05 | backend-ai | decision | 已给前端留下 EB-030：适配变量属性、存储映射、写入约束、防抖和启动快照高级编辑 UI；后端继续推进 EB-025 检测配置语义和运行快照冻结 | open
- 2026-05-29 15:28 | backend-ai | decision | 完成 EB-025：检测标准项新增检查方法、目标值、恢复死区、超限/恢复保持时间和质量策略；检测运行快照冻结检测语义与变量存储映射，前端 DTO 已同步 | closed
- 2026-05-29 15:29 | backend-ai | test | EB-025 验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.7%、`go vet ./...`、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 通过；`golangci-lint` 本机未安装 | closed
- 2026-05-29 15:31 | backend-ai | test | 已重启本地后端 PID 42268 并保持 `127.0.0.1:18080` 运行；smoke 验证 `/health` 返回 ok、admin 登录成功、`detection-standards` 与 `detection-runs` 代表接口返回 200 | closed
- 2026-05-29 15:55 | backend-ai | decision | 完成 EB-026：新增 `/api/v1/ws` 只读实时通道、`RealtimeWSService`、WS handler 和前端共享 WS envelope 类型；本阶段只推变量快照、检测任务快照和 heartbeat，写命令统一返回 `read_only` | closed
- 2026-05-29 15:56 | backend-ai | test | EB-026 验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.3%、`go vet ./...`、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 通过；`golangci-lint` 本机未安装 | closed
- 2026-05-29 15:57 | backend-ai | test | 已重启本地后端 PID 20420 并保持 `127.0.0.1:18080` 运行；smoke 验证 `/health` ok、admin 登录成功、WS 连接 `/api/v1/ws?access_token=...&topic=realtime.variables` 首帧返回 `connection.ready` | closed
- 2026-05-29 16:02 | backend-ai | decision | 新增 EB-031：在 WS 写命令前先补强只读 WS 的断线重连和连接保护，避免后续写命令建立在脆弱连接模型上 | open
- 2026-05-29 16:12 | backend-ai | decision | 完成 EB-031：WS handler 增加 read limit、read/write deadline、ping/pong 保活、断开清理和重连后重新 ready/snapshot；后端架构文档与实施顺序文档已同步 | closed
- 2026-05-29 16:13 | backend-ai | test | EB-031 验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.1%、`go vet ./...`、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 通过；`golangci-lint` 本机未安装 | closed
- 2026-05-29 16:15 | backend-ai | test | 已重启本地后端 PID 39632 并保持 `127.0.0.1:18080` 运行；smoke 验证 `/health` ok、WS 首连和主动断开后重连均返回 `connection.ready` | closed
- 2026-05-29 16:35 | backend-ai | decision | EB-027 完成检测任务 WS 写命令首段：`command.detection.start/stop/abnormal_stop` 复用 `DetectionRunsService`，成功返回 `command.ack`，失败返回结构化 error，所有命令写 `sys_audit_logs`；KIO WS 写留到拆出 KIO service 后实施 | open
- 2026-05-29 16:36 | backend-ai | test | EB-027 首段验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.0%、`go vet ./...`、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 通过；`golangci-lint` 本机未安装 | open
- 2026-05-29 16:38 | backend-ai | test | 已重启本地后端 PID 70660 并保持 `127.0.0.1:18080` 运行；smoke 验证 `/health` ok、WS `command.detection.start` 返回 `cmd-smoke-start` ack、WS `command.detection.stop` 返回 `cmd-smoke-stop` ack | open
- 2026-05-29 15:08 | frontend-ai | decision | 完成 EB-023：历史数据源 MySQL 配置页确认使用 shared API，密码不回显，测试连接成功/失败均有明确反馈，保存后展示 `restart_required` 重启 sidecar 提示 | closed
- 2026-05-29 15:08 | frontend-ai | test | EB-023 验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright smoke 验证历史数据源页、密码空值、连接测试成功和错误端口失败反馈，截图位于 `desktop/output/playwright/database-config-ui/` | closed
- 2026-05-29 16:02 | frontend-ai | decision | 完成 EB-030：变量编辑补存储映射、写入约束、运行保护高级分区和前端写入校验；检测标准项同步 EB-025 的检测方法、目标值、回差、保持时间和质量策略字段 | closed
- 2026-05-29 16:02 | frontend-ai | test | EB-030 验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright smoke 验证变量高级编辑与检测标准语义字段，截图位于 `desktop/output/playwright/variable-attributes-ui/` | closed
- 2026-05-29 14:25 | backend-ai | decision | EB-020 P0 实施：新增 runtime HTTP 写操作审计 middleware，受保护 `POST/PATCH/PUT/DELETE` 自动写 `sys_audit_logs`；审计 detail 只保留 request/command/route/status/actor/latency/error 元数据，不保存请求体或响应体，避免泄露密码、token、KIO 凭据和数据库连接密码 | open
- 2026-05-29 14:28 | backend-ai | test | EB-020 P0 验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 生成 `backend/all-coverage.out` 且总覆盖率 70.5%、`go vet ./...`、`go build ./cmd/edge-backend` 通过；`golangci-lint` 本机未安装；已重启本地后端并 smoke 验证 `/health` ok、登录 ok、`POST /api/v1/system/database-config/test` 返回 `{"ok":true}` 且回传 `X-Request-ID=smoke-audit-1` | open
- 2026-05-29 15:06 | backend-ai | decision | EB-024 P1 实施：`sys_tags`、`TagConfig`、`POST/PATCH /api/v1/variables` 和 `desktop/src/shared/api/types.ts` 补齐变量存储映射、读写约束、防抖与启动快照字段；服务层增加默认值和写入约束校验，自动发现变量和虚变量默认不可写，缺少 `write_path` 的可写变量会被拒绝 | open
- 2026-05-29 15:09 | backend-ai | test | EB-024 P1 验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 生成 `backend/all-coverage.out` 且总覆盖率 70.6%、`go vet ./...`、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 均通过；`golangci-lint` 本机仍未安装；已重启本地后端并 smoke 验证虚变量默认 `writable=false/rw_mode=R`、存储映射字段可读回、缺少 `write_path` 的可写变量返回 400 | closed
- 2026-05-29 16:33 | frontend-ai | decision | 完成 EB-032：新增左侧“检测配置”一级菜单和 `/#/detection-config` 页面，复用检测标准 API 展示/维护标准项上下限、检测方法、保持时间、质量策略和存储策略，并把旧 Vue 前端 19 个检测项作为参考池 | closed
- 2026-05-29 16:33 | frontend-ai | test | EB-032 验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright smoke 登录后打开 `/#/detection-config`，确认检测配置页、旧系统检测项、19 个参考项和新增标准入口可见，截图位于 `desktop/output/playwright/detection-config-page/detection-config-page.png` | closed
- 2026-05-29 17:00 | frontend-ai | decision | EB-032 规则修正：检测标准项来源从“已分配变量”改为“所有变量按 `var_name` 去重”，检测配置页和管理中心标准弹窗保持一致；变量项显示直接复用变量列表已有中/英/日字段，不在标准配置里单独维护翻译 | closed
- 2026-05-29 17:00 | frontend-ai | test | EB-032 追加验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 打开新增标准弹窗，确认“从所有变量名中选择标准项”提示和变量名选择器可用，截图位于 `desktop/output/playwright/detection-config-variable-names/standard-variable-names.png` | closed
- 2026-05-29 18:00 | backend-ai | decision | EB-033 实时底座首段完成：`TagManager` 新增 `gateway/topic -> tags` 与 `device_id -> tags` 索引，`logic_worker` 改用 `ForMessage`，`Tag.UpdateNumeric` 实施 scale/offset 后的 suspicious/debounce/deadband/startup 清洗，WS 设备过滤改用设备实时视图；暂缓项继续只做站位 | closed
- 2026-05-29 18:00 | backend-ai | test | EB-033 验证通过：`gofmt`、`go test ./internal/models ./internal/pipeline ./internal/services`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.6%、`go vet ./...`、`go build ./cmd/edge-backend` 通过；`golangci-lint` 本机未安装；已重启后端 PID 27756 并 smoke 验证 `/health`、登录、WS ready、WS detection start/stop | closed
- 2026-05-29 18:20 | backend-ai | decision | EB-033 追加运行态准入：自动发现变量默认 `enabled=false/device_id=null`，只作为候选来源；`Repository.LoadTags()` 与 `TagManager` 统一只接收 `enabled=true AND device_id IS NOT NULL` 的已知业务变量，未知候选不清洗、不推 WS、不入库 | closed
- 2026-05-29 18:20 | backend-ai | test | EB-033 运行态准入补强验证通过：已安装 `golangci-lint v2.12.2`，`golangci-lint run ./...` 为 0 issues；`gofmt`、`go test ./internal/database ./internal/discovery ./internal/pipeline ./internal/services`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.4%、`go vet ./...`、`go build ./cmd/edge-backend` 通过；已重启后端 PID 23916，`/health` ok 且运行态 tags=4，登录、variables、runtime/channels smoke 通过 | closed
- 2026-05-29 20:24 | frontend-ai | decision | UI 色彩规则修正：设置页与检测配置页普通重点统一使用蓝色/蓝青色，移除普通卡片、选中态和玻璃高光中的橙黄色；橙黄色保留为特别重点色，不再作为普通工作区强调色 | closed
- 2026-05-29 20:24 | frontend-ai | test | 色彩修正验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 截图检查 `/#/settings` 与 `/#/detection-config`，产物位于 `desktop/output/playwright/blue-accent-audit/` | closed
- 2026-05-29 23:06 | frontend-ai | decision | 侧边菜单色彩修正：移除 shell 背景、侧栏光照、通用玻璃面板和菜单选中态中的黄色点缀，改为蓝色/蓝青色体系；工位/历史页面内部特殊光源不在本轮调整 | closed
- 2026-05-29 23:06 | frontend-ai | test | 侧边菜单色彩验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 截图验证 `/#/settings` 侧边栏，产物位于 `desktop/output/playwright/sidebar-blue-accent/settings-sidebar-blue.png` | closed
- 2026-05-29 23:38 | frontend-ai | decision | 登录页色彩修正：移除登录背景右上光球、登录浮层和面板内部玻璃高光中的黄色，统一改为蓝色/蓝青色；登录、shell、设置、检测配置均不再使用普通黄色点缀 | closed
- 2026-05-29 23:38 | frontend-ai | test | 登录页色彩验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 截图验证 `/#/login`，产物位于 `desktop/output/playwright/login-blue-accent/login-blue-accent.png` | closed
- 2026-05-29 20:40 | backend-ai | decision | EB-021 先落地超限报警生命周期表：新增 `detection_limit_alarms`，一条记录覆盖某任务内某变量从超限开始到恢复，字段包含任务/设备/标准项/变量快照、报警类型和等级、开始/峰值/恢复值、阈值、质量、开始/最后/恢复时间和持续时长；暂不实现实时判定状态机 | open
- 2026-05-29 20:40 | backend-ai | test | EB-021 表地基验证通过：`gofmt`、`go test ./internal/models ./internal/database`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.4%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` 通过；已重启后端 PID 74708，`/health` ok | closed
- 2026-05-29 23:05 | backend-ai | decision | 新增 `backend/docs/变量默认上下限与业务报警装配设计.md`；变量默认上下限作为 `sys_tags` 资产属性保留，检测标准项新增 `alarm_enabled`，检测启动时把变量默认属性冻结到运行快照 `variable_default_*`，变量 PATCH 可通过 `apply_to_running=true` 只同步 running 任务变量默认快照 | open
- 2026-05-29 23:05 | backend-ai | decision | 新增 EB-034 给前端：适配变量默认上下限、默认报警开关、检测标准项 `alarm_enabled`、任务详情运行快照 `variable_default_*` 和变量编辑 `apply_to_running` 风险提示 | open
- 2026-05-29 23:05 | backend-ai | test | 变量默认上下限/业务报警装配地基验证通过：`gofmt`、`go test ./internal/runtime/handlers ./internal/services ./internal/database`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.2%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 均通过；已重启后端并 smoke 验证 `/health`、admin 登录和 variables 默认报警字段返回 | closed
- 2026-05-29 23:48 | frontend-ai | decision | 完成 EB-035：系统设置日志诊断区新增本机运行日志预览，Electron preload 暴露只读 `readLogs` 读取 Go sidecar 日志尾部；保留打开日志文件位置。审计日志已有写入地基但暂无查询接口，前端暂不做审计列表 | closed
- 2026-05-29 23:48 | frontend-ai | test | EB-035 验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；Playwright 验证 `/#/settings` 系统设置里“运行日志预览”和“刷新日志”可见，截图位于 `desktop/output/playwright/settings-runtime-logs/runtime-logs-settings.png` | closed
- 2026-05-29 23:58 | backend-ai | decision | EB-020 追加后端审计日志查询：新增 `GET /api/v1/audit-logs`，受 `system_settings` 权限保护，支持 actor/action/target/result/time/limit/offset 过滤，响应 `{items,total,limit,offset}`；同步前端共享 DTO/API 便于设置页后续接入 | open
- 2026-05-29 23:59 | backend-ai | test | EB-020 审计日志查询验证通过：`gofmt`、`backend/go test ./...`、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 均通过；新增 kernel 测试覆盖查询成功、非法 limit、非法时间 | open
- 2026-05-30 00:07 | frontend-ai | decision | EB-035 追加前端后端审计日志列表：系统设置模块新增“后端审计日志”卡片，调用 `getAuditLogs({limit:80})`，展示时间、操作者、动作、对象、结果、request/command/status 和错误摘要，并补齐中英日文案 | closed
- 2026-05-30 00:07 | frontend-ai | test | 审计日志前端验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 通过；已重启本地后端为 PID 71332，HTTP 验证 `/api/v1/audit-logs?limit=5` 返回 145+ 条记录，浏览器验证 `/#/settings` 系统设置中“后端审计日志”显示 147 条且可见 `auth.login` 行 | closed
- 2026-05-29 23:46 | backend-ai | decision | 检测配置补 `check_cycle_ms/check_on_start` 并同步运行快照；变量属性补 `default_violation_hold_ms/default_recover_hold_ms`。检测开始首帧存储按变量 `startup_snapshot_enable`，检测开始首帧判断按业务 `check_on_start`，二者不绑定；`check_cycle_ms=0` 时快照继承变量 `store_cycle_sec` | open
- 2026-05-29 23:46 | backend-ai | test | 检测配置判断周期和首帧语义验证通过：`gofmt`、`go test ./internal/models ./internal/database ./internal/runtime/handlers ./internal/services`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.0%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 均通过；已重启后端 PID 71284 并 smoke 验证 `/health`、admin 登录和变量默认保持时间字段返回 | closed
- 2026-05-30 00:43 | backend-ai | decision | EB-021/EB-028 首段完成：新增 `Alarm` 队列和批量入库 worker，`TaskManager` 在设备运行上下文维护业务超限状态；数据变化和周期扫描按运行快照 `alarm_enabled/check_enabled/check_cycle_ms/limit_*/limit_deadband/violation_hold_ms/recover_hold_ms/quality_policy` 触发上限、下限报警进入和恢复，清洗层仍只更新实时值和质量戳 | open
- 2026-05-30 00:43 | backend-ai | test | 业务上下限报警性能验证通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.1%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`、`go build -tags perf_tools ./tools/mqttpubperf ./tools/perfsetup` 通过；520 变量 10 分钟压测 `PERF-520-20260530-003212` 发布 312000 次更新，MQTT 延迟 avg 2.8ms/p95 3.7ms/max 12.4ms，报警表 31200 条全部 recovered，其中 `above_h=15600`、`below_l=15600`，历史 322920 行；health 采样 120 次，队列最大 `alarm=520/store=120/logic=0`，结束后队列全 0，已通过 API 停止 PERF 任务并保持后端运行 | closed
- 2026-05-30 01:05 | backend-ai | decision | EB-028 继续推进：新增 `detection_run_events` 和 `detection_run_summaries`，检测开始、正常停止、异常停止写轻量事件；摘要按需从任务、历史数据和超限报警表刷新，避免把高频报警明细重复写成第二套事件表造成双倍写入压力 | open
- 2026-05-30 01:05 | backend-ai | api | 新增前端可见接口：`GET /api/v1/detection-runs/:id/summary` 返回 `result_status/history_rows/alarm_total/alarm_active/alarm_recovered/alarm_above_h/alarm_above_hh/alarm_below_l/alarm_below_ll/first_alarm_at/last_alarm_at`；`GET /api/v1/detection-runs/:id/events?limit=` 返回任务生命周期事件列表；前端共享 DTO/API 已同步 | open
- 2026-05-30 01:20 | backend-ai | test | EB-028 事件/摘要验证通过：`gofmt`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.0%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 通过；已重启后端 PID 48260，`/health` ok，API smoke 新建并停止 `SMOKE-EVENT-20260530005535`，`/summary` 返回 `result_status=ok`，`/events` 返回 `run_started,run_stopped` | closed
- 2026-05-30 01:55 | backend-ai | decision | EB-037 首段实施：新增 storage route 模型和运行快照，`sys_tags.storage_*` 保留兼容，设备宽表主路径由 `sys_storage_routes/detection_run_storage_routes` 承载；检测启动时冻结 routes 并确保 `rt_device_{device_id}_data` 和动态列存在，写入热路径暂未切到宽表 writer | open
- 2026-05-30 09:16 | backend-ai | decision | EB-037 追加宽表写入路径：`DetectionTask`/`ActiveTask` 携带运行 storage routes，`StoreTask` 携带路由快照；`InsertHistoryBatch` 保持写 `rt_history_data` 兼容表，同时对 `wide_table` routes 写入 `rt_device_{device_id}_data`，同一 `(task_id,sample_bucket_ms)` 使用 upsert 更新动态列 | open
- 2026-05-30 09:20 | backend-ai | test | EB-037 storage route/schema/wide writer 验证通过：`gofmt`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.2%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 通过；已重启后端 PID 20768，`/health` ok，API smoke 新建并停止 `SMOKE-WIDE-20260530092033-1`，验证检测启动可通过真实 MySQL AutoMigrate、冻结 storage routes 并准备设备宽表 schema | closed
- 2026-05-30 09:27 | backend-ai | decision | EB-037 补齐宽表历史读取和批量合并：宽表 writer 先按 `table/task_id/sample_bucket_ms` 聚合多变量后一次 upsert；`GET /api/v1/history/data` 在 `task_id/device_id/test_no` 命中 `detection_run_storage_routes` 时优先读取 `rt_device_{device_id}_data`，并重建旧 `HistoryData` DTO，前端接口字段不变 | open
- 2026-05-30 09:29 | backend-ai | test | EB-037 宽表合并写入和历史读取验证通过：新增单测覆盖同一采样点多变量合并成一条设备宽表记录、数值列 upsert 更新、字符串列读取和 `QueryHistoryData` 从宽表重建旧 DTO；`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.4%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` 通过；已重启后端 PID 67644，`/health` ok，登录、`history/data?limit=1`、`detection-runs?limit=1` smoke 返回 200 | closed
- 2026-05-30 09:48 | backend-ai | decision | EB-037 存储设计审查后推翻变量级存储频率：变量属性只保留采集、清洗、写入约束和默认报警；存储目标表/列、周期、变化死区、首帧存储和启用状态全部归 `sys_storage_routes`；新增 storage routes API，前端需停止把 `store_cycle_sec/store_deadband/store_trigger/store_mode/storage_table` 作为变量编辑主入口 | open
- 2026-05-30 09:52 | backend-ai | test | EB-037 route 驱动存储重构验证通过：`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.6%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`、`desktop/npm run lint`、`desktop/npm run build` 通过；已重启后端 PID 52372，`/health` ok，admin 登录和 `GET /api/v1/storage-routes?enabled=false` smoke 返回 200 | closed
- 2026-05-30 10:15 | backend-ai | decision | 用户确认本轮不保留旧 `/devices` 兼容入口；新增 EB-041/042/043/044：项目命名统一、变量存储属性删除、任务系统倒排索引、存储总线。前端需同步迁移到 `/api/v1/projects`、`project_id/project_code` 和 `storage-routes` 配置 UI | open
- 2026-05-30 10:40 | backend-ai | decision | EB-041/042/043/044 首段实施：后端只注册 `/api/v1/projects`，模型和 schema 切到 `sys_projects/project_id/project_code/rt_project_*`；启动迁移会把旧 `sys_devices/device_id/device_code` 改到项目命名并删除变量存储属性列；变量 API 不再接收/返回 `store_*`、`storage_*`、`startup_snapshot_enable`；新增 `sys_task_rules` 和 `TaskRuleIndex` 倒排索引地基；存储 worker 改为 `StorageBus` 按 `project_id + table_name` 分 bucket 批量 flush | open
- 2026-05-30 10:50 | backend-ai | test | EB-041/042/043/044 首段验证通过：`gofmt`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.2%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` 通过；已重启后端 PID 45052 并保持 `127.0.0.1:18080` 运行，smoke 验证 `/health` ok、admin 登录 ok、`GET /api/v1/projects` 200、`GET /api/v1/storage-routes?enabled=false&limit=1` 200、`GET /api/v1/history/data?limit=1` 200、`GET /api/v1/devices` 404、变量响应不再包含 `device_id/store_cycle_sec/storage_table` | closed
- 2026-05-30 11:10 | frontend-ai | decision | 适配 EB-041/042 后端破坏性契约：前端 shared DTO/API 将项目字段 `project_id/project_code` 映射到旧页面别名，`getDevices/createDevice/updateDevice` 实际调用 `/api/v1/projects`；变量创建/编辑/分配和历史查询参数会发送项目字段并过滤变量级存储字段；设置中心新增“存储路由”模块，通过 `/api/v1/storage-routes` 管理项目宽表目标、列、触发、周期、死区和启用状态 | open
- 2026-05-30 11:10 | frontend-ai | test | EB-041/042 前端适配验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 均通过；HTTP smoke 登录 admin 后验证 `GET /api/v1/projects` 返回 8 个项目、`GET /api/v1/storage-routes` 返回 4 条路由、旧 `GET /api/v1/devices` 返回 404 | closed
- 2026-05-30 11:30 | backend-ai | decision | EB-045 条件事件任务系统首段实施：新增任务流模型、任务变量引用、执行日志、SQL 日志、goja JS 引擎、异步执行队列和 priority 小批量排序；数据变化热路径只通过 `var_id -> flow_ids` 倒排索引投递任务，不直接执行脚本；内置 `builtin.storage_snapshot` 复用 StorageBus 写入，不新建数据库连接 | open
- 2026-05-30 11:38 | backend-ai | test | EB-045 首段验证通过：新增单测覆盖 `start_flag` 变量 0->1 后命中条件事件任务、执行 `builtin.storage_snapshot`、投递 StoreTask 并通过 repository 写入历史；新增 JS 任务测试覆盖 goja 脚本、`db.query`、执行日志和 SQL 日志；新增倒排索引 priority/cooldown/hold 测试。门禁通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.2%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`；后端已重启 PID 27880，`/health`、登录、`GET /api/v1/task-flows` 和 `GET /api/v1/projects` smoke 200 | closed
- 2026-05-30 11:45 | backend-ai | decision | 已给 frontend-ai 新增 EB-046：侧边菜单新增“任务”模块承载后端 task-flows，列明当前可接 API、页面字段、权限、三语、手动运行、脚本编辑器、执行记录和 SQL 日志占位要求；后端仍需后续补执行记录/SQL 日志查询 API | open
- 2026-05-30 12:06 | frontend-ai | decision | EB-046 前端首段完成：新增 `/#/tasks` 路由和侧边菜单“任务”入口，受 `system_settings` 权限保护；新增 task-flows typed API、任务列表、新建/编辑弹窗、项目筛选、变量角色绑定、条件脚本、动作脚本/参数、手动运行按钮、执行记录和 SQL 日志占位 tab；编辑已有任务时项目选择禁用，因为当前后端 PATCH 未开放 project_id 修改 | open
- 2026-05-30 12:06 | frontend-ai | test | EB-046 验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 均通过；HTTP smoke 登录 admin 后验证 `GET /api/v1/task-flows` 返回 0 条、`GET /api/v1/projects` 返回 8 个项目。本轮尝试连接 in-app Browser 验证页面，但当前 Browser 插件返回 `iab` 不可用，已改用构建和 HTTP smoke 验证 | closed
- 2026-05-30 12:42 | frontend-ai | decision | 新增 3D 驾驶舱探索页 `/#/model-cockpit`：左侧菜单新增“3D 驾驶舱”，引入 Three.js 与 `three/examples` GLTFLoader/OrbitControls，模型资源从根目录 `3d模型暂存/base_basic_pbr.glb` 复制到 `desktop/public/models/edge-air-conditioner.glb`；页面支持 360 度拖拽、内部流体粒子/流道、模型空间浮窗和射线遮挡。按用户修正要求，模型保持不透明，内部流体仍是独立 3D 对象，以诊断显影/视觉错位方式叠加到模型表面，不使用整机透明或剖切 | closed
- 2026-05-30 12:42 | frontend-ai | test | 3D 驾驶舱验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test` 均通过；Playwright 登录打开 `/#/model-cockpit`，桌面与窄视口均检测到 WebGL、非空 canvas 和 4 个浮窗，桌面拖拽旋转后检测到浮窗遮挡态；最终诊断显影截图位于 `desktop/output/playwright/model-cockpit/opaque-diagnostic-overlay-1440x920.png` | closed
- 2026-05-30 12:30 | backend-ai | decision | EB-045 初版任务数据流转设计已被废弃：HTTP 手动运行业务参数入口不符合“变量是一切根基”的平台模型；后续以 STRING 虚拟变量数据变化作为正式任务参数入口 | closed
- 2026-05-30 12:30 | backend-ai | api | 新增 `GET /api/v1/task-modules`；扩展 `POST/PATCH /api/v1/task-flows` 支持 `steps_json`；`sys_task_flows` 新增 `steps_json` 字段，旧 `action_type/action_payload/action_script` 保持单步骤兼容；正式业务参数不由手动运行接口承载 | open
- 2026-05-30 12:35 | backend-ai | test | EB-045 多步骤任务链初版验证通过，但其中 HTTP 手动运行业务参数路径已作废；保留有效部分：多步骤 `steps_json`、`builtin.context_set`、`builtin.storage_snapshot`、JS 步骤和 StorageBus 投递验证 | closed
- 2026-05-30 12:50 | backend-ai | decision | 已删除 `backend/docs/任务数据流转与内置模块设计.md`，新增 `backend/docs/任务变量流转与内置模块设计.md`；任务业务参数只从 watched STRING 虚拟变量的 JSON 字符串解析为 `task_params`，参数绑定来源改为 `trigger_param`，HTTP 手动运行只保留开发者调试触发 | open
- 2026-05-30 12:55 | backend-ai | test | EB-045 变量流转修正验证通过：单测覆盖 watched STRING 虚拟变量变更后解析 JSON 参数、条件脚本读取 `task_params.command`、步骤用 `trigger_param.project_id/duration_sec`、`builtin.storage_snapshot` 投递 StorageBus、JS 读取 context；门禁通过：`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.0%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`；后端已重启 PID 26684，smoke 验证 `/health`、admin 登录、`GET /api/v1/task-modules` 返回 source=`literal,trigger_param,event,context`、`GET /api/v1/task-flows` 200 | closed
- 2026-05-30 13:05 | backend-ai | review | 后端待办审查并清理过时项：关闭 EB-002、EB-021、EB-028、EB-036、EB-038、EB-043、EB-044；修正 EB-017 设备为项目；修正 EB-019 剩余范围为变量写入/KIO service/防递归；修正 EB-040，废弃默认报警独立表，改为统一超限事件语义；EB-045 继续作为任务编排主线 | closed
- 2026-05-30 13:20 | backend-ai | api | EB-045 新增任务流内置检测模块：`builtin.start_detection_run` 支持 `enable_storage/enable_alarm/duration_sec/standard_id/report_template_id`，新增 `stop_detection_run/pause_detection_run/resume_detection_run/fixed_duration_guard/qualified_hold_guard/refresh_features`；`sys_detection_tasks.status` 新增 `paused`，`end_type` 新增 `fixed_duration/qualified_hold/task_flow_stop`；新增 `detection_run_features`，并开放 `POST /api/v1/detection-runs/:id/pause|resume`、`GET /api/v1/detection-runs/:id/features` | open
- 2026-05-30 13:25 | backend-ai | test | EB-045 内置检测业务模块验证通过：单测覆盖可选存储首帧、可选上下限报警、暂停/恢复、手动结束、固定时长结束、合格持续时长结束、结束后平均/最小/最大特征值计算；门禁通过 `gofmt`、`go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.5%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` | closed
- 2026-05-30 13:40 | backend-ai | api | 暂停时长语义修正：`sys_detection_tasks` 新增 `pause_started_at`、`paused_duration_ms`；暂停记录起点，恢复累计暂停时长并顺延 `expected_end_at`，停止 paused 任务会把当前暂停段计入 `paused_duration_ms`，`detection_run_summaries.duration_ms` 扣除所有暂停时间 | open
- 2026-05-30 13:45 | backend-ai | test | 暂停时长不计入累计时长验证通过：repository/service 测试覆盖恢复顺延预计结束时间和累计暂停时长；门禁通过 `go test ./...`、覆盖率 70.7%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend` | closed
- 2026-05-30 13:50 | frontend-ai | decision | 3D 驾驶舱按旧 Vue `SPDShowPage00.vue` 结构复刻：复制驾驶舱背景、标题、图表、中心背景和五张卡片资产到 `desktop/public/cockpit/`；`/#/model-cockpit` 改为标题栏、五张顶部信息卡、中部实时监测表 + 3D 模型、右侧温湿度趋势图布局，同时保留不透明模型、内部 3D 流体诊断显影和模型空间遮挡浮窗 | closed
- 2026-05-30 13:50 | frontend-ai | test | Vue 结构复刻验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test -- --run` 通过；Playwright 登录打开 `/#/model-cockpit`，验证 WebGL canvas、5 张卡、8 行监测数据、2 张趋势图、4 个模型浮窗、无缺失 i18n key 且数值列不截断，截图位于 `desktop/output/playwright/model-cockpit/vue-structure-1440x920.png` | closed
- 2026-05-30 13:35 | frontend-ai | decision | 驾驶舱背景资产矢量化：按旧 Vue PNG 原始尺寸重建 `cockpit-background/title/center/chart` 与 `card-0..4` SVG，页面引用全部切到 SVG，PNG 仅保留为参考；矢量层复刻旧图的蓝色渐变、标题斜边、中心/图表折线框、卡片角线、水印圆和齿轮装饰 | closed
- 2026-05-30 13:35 | frontend-ai | test | SVG 背景验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test -- --run` 通过；Playwright 验证 `/#/model-cockpit` 的页面、标题、中心、图表、卡片背景均引用 `.svg`，WebGL/浮窗正常，截图位于 `desktop/output/playwright/model-cockpit/svg-vector-background-1440x920.png` | closed
- 2026-05-30 13:58 | frontend-ai | decision | 驾驶舱 SVG 背景改为纹理层：移除 SVG 中大面积实心蓝色填充，保留线框、高光、齿轮、水印和标题斜边纹理；新增 `.cockpit-dynamic-background`，按工位页动态光效模式提供漂移蓝/青光底，卡片、中心框、图表框改为半透明玻璃底承接动态背景 | closed
- 2026-05-30 13:58 | frontend-ai | test | 动态背景 + SVG 纹理验证通过：`desktop/npm run lint`、`desktop/npm run build`、`desktop/npm run test -- --run` 通过；Playwright 验证 `/#/model-cockpit` 存在动态背景层和 SVG 纹理层，root 不再使用静态 SVG 实心背景，WebGL/5 卡/2 图/4 浮窗正常，截图位于 `desktop/output/playwright/model-cockpit/dynamic-svg-texture-1440x920.png` | closed
- 2026-05-30 15:05 | backend-ai | test | KingIO 开发版现场写入验证继续通过：HTTP KIO 多类型写入 `台1_40=true`、`台1_41=41.67`、`台1_42=edge-ws-string-1450` 均收到 `ProcessStep=100/Result=ok`；WS `command.write_variable` 多类型写入 `台1_40=false`、`台1_41=41.68`、`台1_42=edge-ws-string-1452` 均确认成功，实时快照读回对应 BOOL/FLOAT/STRING 值；`query-all` 后 `台1_39~台1_42` 的 `rw_mode/writable/write_path/write_data_type/write_min/write_max` 保持不被发现刷新覆盖；越界 WS 写入 `台1_41=150` 被后端校验拦截，返回 `command_failed: value 150 is above write_max 100`，审计表记录 `ws.command.write_variable failed`，实时值保持 `41.68000030517578`；门禁通过 `go test ./...`、覆盖率 70.0%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`；后端 PID 36240 继续运行，`/health` ok、KIO gateway active、队列全 0 | closed
- 2026-05-30 16:58 | backend-ai | api | EB-045 任务流执行器兼容补强：`steps_json` 正式契约仍推荐多步骤数组，但后端现在也接受单个 step object 并包装为单步骤执行，避免调试脚本或 PowerShell 单元素数组序列化成对象后导致任务失败 | open
- 2026-05-30 17:00 | backend-ai | test | 重启电脑后已恢复后端并完成虚拟变量任务链 smoke：创建 FLOAT 虚拟值变量和 STRING 请求变量，任务请求变量写入 `{"command":"start_detection",...}` 后触发 task-flow，启动检测任务 `task_id=11`，首帧历史 `history_rows=1`，业务超限 `alarm_total=1`，手动停止后生成特征值 `sample_count=1 avg/min/max=35`；门禁通过 `go test ./...`、`go test ./... -coverprofile all-coverage.out` 总覆盖率 70.2%、`go vet ./...`、`golangci-lint run ./...` 0 issues、`go build ./cmd/edge-backend`；后端 PID 22656 继续运行，`/health` ok、MQTT/KIO gateway active、队列全 0 | closed
