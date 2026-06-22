# 后端日志、变量属性、WebSocket 与检测事件实施顺序

## 目标

把 EB-019、EB-020、EB-021 的后端需求整理成可实施、可测试的顺序。当前优先级调整为：先补全日志和审计能力，再补变量属性与检测配置结构，最后实施 WebSocket 写和检测超限事件。

## 总体顺序

```text
P0 日志/审计地基
  -> P1 变量属性、存储映射与写入约束
  -> P2 检测配置语义补齐
  -> P3 WebSocket 实时读
  -> P3.5 运行态清洗、topic 索引与项目/设备实时视图
  -> P4 WebSocket 写命令
  -> P5 检测事件与超限记录
  -> P6 联调和回归
```

理由：WS 写、检测开始/停止、变量控制和超限判断都属于可追责操作。日志和审计不先落地，后续很难解释“谁写了什么、是否成功、失败原因是什么”。

## P0 日志/审计地基

### 需求

- 保留现有 stdout/stderr 运行日志，由 Electron sidecar 继续捕获到本地文件。
- 补齐数据库操作审计，覆盖所有 HTTP/WS 写操作。
- 审计日志必须记录 actor、source、request_id、command_id、resource、action、result、error、detail。
- detail 必须脱敏，不能记录密码、token、MQTT 密钥、KIO 写密码。
- 写操作至少覆盖：
  - 检测 start/stop/abnormal-stop。
  - 变量 create/update/assign/delete。
  - 虚变量 create。
  - 网关配置 create/update/delete。
  - KIO write/query-all。
  - 检测标准 create/update/delete/replace-items。
  - 报表模板 create/update/disable/delete。
  - 后续 WS command accepted/success/failed/timeout。

### 实施建议

- 新增 `internal/services/audit_service.go`，统一审计入口。
- 复用 `sys_audit_logs`，短期不新建第二张通用审计表。
- `auth.Service` 当前已有私有 audit 逻辑，应改为复用新的 `AuditService` 或至少保持同一写入模型。
- handler 不直接拼审计 SQL；由 service 或 handler 调用 `AuditService.Record`。
- 对于已有简单 CRUD，如果短期不引入业务 service，可以在 handler 完成 repo 调用后记录审计，但审计 DTO 要统一。

### 测试

- 单元测试：
  - 审计 detail 脱敏。
  - success/failed 两种结果写入。
  - request_id/command_id 为空时可正常写入。
- Handler/repository 测试：
  - 检测 start/stop/abnormal-stop 产生审计。
  - 变量创建/修改/分配/删除产生审计。
  - 标准 replace-items 产生审计。
  - KIO write 失败也产生 failed 审计。
- 必跑：
  - `go test ./...`
  - `go test ./... -coverprofile all-coverage.out`
  - `go vet ./...`
  - `go build ./cmd/edge-backend`
  - 本地 `/health` smoke。

## P1 变量属性、存储映射与写入约束

状态：2026-05-29 已完成后端 schema/model/API 基础落地。`sys_tags` 和 `TagConfig` 已补齐存储映射、读写约束、防抖、启动快照、变量默认上下限和默认报警开关字段；`POST/PATCH /api/v1/variables` 可读写这些字段；服务层已校验 `writable=true` 时必须有 `rw_mode=W|RW`、`write_path` 和 `write_data_type`。

### 需求

自动发现变量只能成为候选来源变量，不自动参与检测、不自动可写、不自动绑定设备。

建议补齐或冻结以下字段语义：

- `storage_name`: 入库字段业务名。
- `storage_target`: 入库目标类型，例如 `history_eav/detection_form/report_field/wide_table/none`。
- `storage_table`: 目标表名。
- `storage_value_column`: 目标值列。
- `storage_key_column`: 目标变量键列。
- `storage_time_column`: 目标时间列。
- `form_field_key`: 动态表单字段 key。
- `query_alias`: 查询返回别名。
- `rw_mode`: `R/W/RW`。
- `writable`: 是否允许 API/WS 写。
- `write_path`: 后端写入目标路径。
- `write_data_type`: 写入类型。
- `write_min/write_max`: 数值写入安全范围。
- `write_enum`: 枚举允许值。
- `write_requires_audit`: 控制变量默认 true。
- `suspicious_value/debounce_threshold/startup_snapshot_enable`: 采集防抖和冷启动快照策略，按变量需要启用。
- `default_alarm_enabled/default_limit_ll/default_limit_l/default_limit_h/default_limit_hh/default_limit_deadband/default_violation_hold_ms/default_recover_hold_ms`: 变量资产默认检测属性，默认不直接覆盖检测标准。

### 实施建议

- 先扩 `sys_tags` 和 `TagConfig`，保持旧字段兼容。
- 默认历史趋势继续使用 `rt_history_data` 的窄表/EAV 模型，即通过 `var_id + source_time + value/str_value` 存储，不为每个变量动态建列。
- 动态表单、检测结果表和报表模板需要固定字段时，通过 `storage_target/storage_table/storage_value_column/form_field_key/query_alias` 做映射。
- `POST /api/v1/variables` 创建虚变量时默认：
  - `source_type=virtual`
  - `writable=false`
  - `enabled=true` 仅表示配置启用；仍需绑定 `project_id` 才进入运行态清洗
  - `discovered=false`
  - `placeholder=true`
- 自动发现 MQTT 变量默认：
  - `writable=false`
  - `enabled=false`
  - `project_id=null`
  - 只作为候选来源变量入库，不进入 `TagManager` 清洗。
- 变量列表和快照返回新增字段时同步前端 DTO。
- 变量默认上下限修改默认只影响未来任务；`PATCH /api/v1/variables/{id}` 显式传 `apply_to_running=true` 时，只同步当前 running 任务的 `variable_default_*` 快照，不覆盖业务标准上下限。
- 查询服务不能让前端直接猜表名列名；后端根据变量存储映射解析数据来源，并返回稳定 `query_alias`。

### 测试

- 自动发现变量不可写。
- 自动发现变量默认禁用且不进入 `LoadTags()`/`TagManager`。
- 虚变量默认不可写。
- 未绑定虚变量不进入 `TagManager`，绑定项目/设备后才参与清洗和 WS。
- 默认历史变量写入 `rt_history_data.value` 或 `rt_history_data.str_value`。
- 配置动态表单字段后，查询接口能按 `query_alias` 返回。
- 手工指定 `writable=true` 但缺少 `write_path` 时写命令必须拒绝。
- 写入值超出 `write_min/write_max` 返回 400/422。
- 默认上下限需要校验 `LL <= L <= H <= HH`，默认回差不能为负数。
- 变量级默认报警保持时间不能为负数。
- 未绑定设备变量不能被加入检测标准或启动检测时应被忽略/拒绝，具体语义实施前冻结。

## P2 检测配置语义补齐

### 需求

`check_enabled`、`alarm_enabled` 和 `store_enabled` 必须分开：

- `store_enabled`: 检测期间是否写历史。
- `check_enabled`: 检测期间是否参与超限/合格判定。
- `alarm_enabled`: 检测期间是否生成超限报警生命周期记录。

已补充并冻结：

- `check_method`: 支持 `numeric_range`、`bool_equals`、`string_equals`、`regex`，当前超限实现仍以后续事件阶段为准。
- `limit_deadband`: 恢复死区。
- `quality_policy`: Bad quality 时忽略、记录无效或判异常。
- `target_value`: BOOL/STRING 后续判断目标。
- `violation_hold_ms`/`recover_hold_ms`: 超限进入和恢复保持时间。
- `check_cycle_ms`: 检测业务判断周期；不参与历史入库周期计算，也不能作为存储 route 的兜底。历史入库周期只读取 `sys_storage_routes.cycle_ms`。
- `check_on_start`: 检测启动时是否立刻对当前值做一次判断，和变量属性里的首帧存储分开。
- 运行快照额外冻结变量默认检测属性和存储映射：`variable_default_*`、`storage_*`、`form_field_key`、`query_alias`。

### 实施建议

- v1 先冻结完整语义，不急着实现 BOOL/STRING/regex 判定。
- 检测启动时继续冻结标准项快照、变量默认属性快照和变量存储映射，后续超限判断、报表和历史读取只读 `detection_run_standard_items`。
- 变量主表可以保存默认上下限，但业务检测实际使用 `sys_detection_standard_items` 和 `detection_run_standard_items.limit_*`。
- 检测启动首帧存储由变量 `startup_snapshot_enable` 控制；检测启动首帧判断由检测项 `check_on_start` 控制，二者不绑定。

### 测试

- `store_enabled=false` 的变量不入历史，但如果 `check_enabled=true` 后续仍可判定。
- `check_enabled=false` 的变量不产生超限记录。
- 任务启动后修改标准，不影响运行中任务快照。

## P3 WebSocket 实时读

### 需求

- 新增 `/api/v1/ws`。
- 必须鉴权。
- 支持订阅变量、检测任务快照和运行态通知；变量/任务仍用 1 秒周期 snapshot，报警和检测结果通过 `notification.event` 即时推送。
- 消息必须携带 `source_type + gateway_id/source_id + source_path + var_id + project_id`。
- P3 首段只做读；后续 EB-027 已接入受控写命令，写操作必须经过后端 service 和审计。

### 实施建议

- 已新增 `runtime/handlers/ws_handler.go`。
- 已新增 `services/realtime_ws_service.go`，避免塞回 `kernel.go`。
- 浏览器客户端可用 `access_token` query 参数鉴权；非浏览器客户端仍可用 `Authorization: Bearer`。
- 客户端可发送 `subscribe` 调整 `topics/source_type/gateway_id/project_id/var_ids`，topic 支持 `realtime.variables`、`detection.runs`、`notifications`。
- `command.detection.*` 和 `command.write_variable` 已接入后端 service；未知命令返回结构化 error。
- 连接保护已补齐：32 KiB read limit、read/write deadline、15 秒 ping/heartbeat、pong 延长读 deadline；断线后服务端 goroutine 退出，客户端重连后按新连接重新鉴权、订阅并收到 ready/snapshot。
- `Channels.Notify` + `NotificationHub` 已用于报警进入/恢复/等级变化、检测生命周期和检测 OK/NG 结果通知；通知队列满时丢弃通知并记录日志，不能阻塞采集热路径。

### 测试

- 未登录连接失败。
- 登录后可收到 snapshot。
- 收到 `connection.ready`、`realtime.variables.snapshot`、`detection.runs.snapshot`、`notification.event` 和 `heartbeat`。
- 写命令成功返回 `command.ack`，失败返回 `error`，并写入 `sys_audit_logs`。
- 未带 token 的 WS 连接返回 401。
- 主动断开后重连，可重新收到 ready/snapshot。
- 慢客户端不阻塞 logic/store channel。
- 多设备订阅过滤准确。

## P3.5 运行态清洗、topic 索引与项目/设备实时视图

### 需求

- 运行态清洗必须位于 MQTT/KIO 解析之后、WebSocket/存储/检测业务之前。
- 清洗层处理类型转换、`scale_factor/offset_val`、质量归一、可疑值、防抖、运行态死区和首帧策略。
- 清洗层只更新 `Tag` 运行态，不查数据库、不计算上下限、不写 MySQL。
- `TagManager` 增加 `gateway_id + topic -> []*Tag` 索引，热路径通过 `ForMessage(gateway_id, topic)` 取变量，避免每条消息全量扫描所有 tag。
- 当前“项目/设备”统一使用 `sys_projects.project_id`；`site_no` 不作为实时 map key。
- `GlobalTagMap` 保存唯一稳定实时值；`ProjectRealtimeView` 按 `project_id` 装配页面和 WS 变量视图；`ProjectRunContext` 为后续检测判定和报警状态机预留当前任务上下文。

### 实施建议

- 本轮实现 topic 级索引，source path 级倒排索引后续再优化。
- 索引未命中时 fallback 到 `All()`，保证兼容旧配置和异常数据。
- 防抖 pending 值不推 WS、不入库、不参与检测。
- 定时存储遇到 pending 时直接读取最后稳定值，不等待防抖结束。
- 保留现有 1 秒 WS snapshot 机制，snapshot 必须来自清洗后的稳定值。

### 暂缓但站位

- 报警 enter/recover/level_change 状态机。
- 检测配置运行中即时生效。
- KIO/WS 变量下设完整闭环。
- 多变量组合触发任务和 `var_id -> rule_ids` 倒排索引。

这些能力需要在 `backend/docs/运行态清洗与实时业务视图设计.md` 中保留边界，后续拆独立事项实施。

### 测试

- `ForMessage(gateway_id, topic)` 只返回该网关/topic 下变量，reload/upsert 后索引同步更新。
- scale/offset、suspicious、debounce、deadband、startup snapshot 行为正确。
- pending 值不进入 Store 队列，定时存储读取最后稳定值。
- WS snapshot 读取清洗后的稳定值。
- 订阅过滤和断线重连保持通过。

## P4 WebSocket 写命令

### 需求

- WS 写必须走后端 service，不得绕过后端直写 MQTT。
- 所有 command 必须有 `request_id` 和 `command_id`。
- 支持幂等：重复 `command_id` 返回同一结果或明确 duplicate。
- 当前已完成检测任务写命令首段：`command.detection.start`、`command.detection.stop`、`command.detection.abnormal_stop` 复用 `DetectionRunsService` 并写 `sys_audit_logs`。
- KIO 写命令暂不直接接入 WS；需先把现有 `GatewaysHandler` 内的 KIO 拼包/发布逻辑拆到 service，再由 HTTP 和 WS 共用，避免 WS 绕过后端边界或复制 MQTT 细节。
- 支持：
  - `command.variable.write`
  - `command.detection.start`
  - `command.detection.stop`
  - `command.detection.abnormal_stop`

### 实施建议

- 先复用 HTTP service：检测命令走 `DetectionRunsService`。
- KIO 写入封装为 `GatewayCommandService`，HTTP KIO handler 和 WS command 共用。
- 每个命令写两段审计：
  - accepted/rejected
  - success/failed/timeout
- WS command 错误格式与 HTTP 错误语义对齐。

### 测试

- 不可写变量拒绝。
- 无 `write_path` 拒绝。
- 越界值拒绝。
- 同设备重复 start 返回冲突。
- command 重复提交幂等。
- KIO publish 失败也记录审计。

## P5 检测事件与超限记录

### 需求

新增业务事件，不复用通用审计表承载检测业务查询。

当前先落地的模型：

- `detection_limit_alarms`
  - 一条记录表示某任务内某变量的一次超限报警生命周期。
  - `first_seen_at/last_seen_at/recovered_at/status` 记录开始、持续和恢复。
  - `alarm_type/alarm_level/limit_value/start_value/peak_value/recover_value` 记录 LL/L/H/HH 等超限方向、等级和值。
  - 表随后端 `AutoMigrate` 初始化，`backend/deploy/schema.sql` 同步。

后续可追加模型：

- `detection_run_events`
  - start、manual_stop、abnormal_stop、timeout、violation_enter、violation_recover、summary_generated。

### 实施建议

- pipeline 中新增 `DetectionEvaluator`，基于 active task 和运行标准快照判定。
- 超限判断不阻塞采集热路径，进入独立 buffered queue。
- v1 先只做 numeric range。
- 检测开始/停止/异常停止由 `DetectionRunsService` 写事件。

### 测试

- 超过 HH/H/L/LL 分别产生正确记录。
- 回到范围内写 recovered。
- `check_enabled=false` 不产生记录。
- Bad quality 按冻结策略处理。
- 停止检测后不再产生该任务超限。

## P6 联调和回归

### 后端门禁

- `gofmt` changed Go files。
- `go test ./...`。
- `go test ./... -coverprofile all-coverage.out`，总覆盖率不低于 70%。
- `go vet ./...`。
- `go build ./cmd/edge-backend`。
- 本地 `/health` smoke。
- 登录 smoke。
- 抽查 users、Projects、variables、detection-standards、detection-runs、history/data、report-templates、runtime/channels。

### 前端/联调门禁

- typed API 同步。
- 管理中心变量属性字段显示/编辑。
- 检测标准字段显示/编辑。
- WS 连接状态、断线重连、订阅过滤。
- 写命令成功/失败/权限不足/冲突提示。
- 操作日志可查询或至少数据库可验证。

## 当前决策

1. 先做 EB-020 日志/审计，不先做 WS。
2. WS 实时读可以早于 WS 写，但 WS 写必须等变量写入约束和审计完成。
3. 超限记录必须等检测配置语义冻结后实施；上下限放在检测配置/运行快照，存储映射放在变量属性。
4. 自动发现变量不是业务变量；业务变量必须经过设备绑定和属性确认。
5. 所有后续实施不得把逻辑重新堆回 `kernel.go`。
