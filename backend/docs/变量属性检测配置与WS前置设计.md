# 变量属性、检测配置与 WebSocket 前置设计

## 背景

本设计用于约束 EB-019 WebSocket 通道、EB-020 写操作审计和 EB-021 检测超限记录的实施顺序。

结论：WebSocket 可以作为实时读写通道，但不能先于变量属性和检测配置边界盲目实现写操作。自动发现变量只代表“系统看见了一个来源点位”，不代表它已经可以参与检测、存储、超限判断或控制写入。

## JGHJ 旧后端参考

已审阅 `SPD_JGHJ 数据采集与任务` 旧后端，里面的变量模型比当前项目更完整：

- `sql/spd_jghj.sql` 的 `sys_variables` 包含 `data_type`、`rw_mode`、`json_path`、`scale_factor`、`offset_val`、`alarm_enable`、`limit_hh/h/l/ll`、`deadband`、`store_mode`、`store_cycle`、`store_deadband`、`suspicious_value`、`debounce_threshold`、`startup_snapshot_enable`、`source_type`、`calc_rule`。
- `models/tag.go` 的运行态 `Tag` 同时保存配置和内存状态，包括 `CurrentValue`、`LastValue`、`CurrentStrValue`、`LastStrValue`、`Quality`、`LastQuality`、防抖观察窗、报警状态和当前报警记录 ID。
- `workers/logic_worker.go` 使用双循环：Pass 1 只解析和更新内存快照，Pass 2 基于同一批快照触发任务、存储、报警和 SSE，避免业务判断读到一半旧值一半新值。
- 质量码按 KIO/SCADA 的 `192` 识别 Good，非 `192` 记为 Bad，但旧后端仍会更新值并把质量状态带给业务。

本项目当前已经有 `sys_tags.source_type`、`gateway_id + source_path`、三语 display、缩放偏移、存储策略、运行态 current/last value、quality 和检测标准快照；但尚未完整覆盖读写属性、防抖配置、检测判定策略、超限事件和 WS 写审计。

## 核心边界

变量必须拆成四层理解：

| 层 | 作用 | 是否可自动生成 |
| --- | --- | --- |
| 来源变量 | 表示从哪个数据源、哪个路径发现了一个原始点位。 | 可以自动发现。 |
| 业务变量 | 表示这个点位被确认后属于哪个设备、怎样展示、怎样转换、能否存储、能否写。 | 不能完全自动，需要配置确认。 |
| 运行态变量 | 表示当前值、上一值、质量、更新时间、防抖状态、是否过期。 | 由采集链路实时维护。 |
| 检测配置项 | 表示在某个检测标准下是否检测、是否存储、上下限和判定规则。 | 不能自动，需要检测标准配置。 |

自动发现只允许落到“来源变量”层。后续必须经过业务变量确认，才能进入 WS 订阅展示、检测标准选择、历史存储和写命令。

## 变量身份

WS、历史、检测、写命令必须统一使用以下标识组合：

```text
source_type + gateway_id/source_id + source_path + var_id + project_id
```

当前 MQTT/KIO 可以继续使用 `gateway_id + source_path` 作为来源唯一键。未来非 MQTT 数据源不应伪装成 MQTT gateway，而应扩展为统一 `source_id/source_type` 概念；在此之前，`source_type` 先承担协议区分：

- `mqtt`
- `virtual`
- `manual`
- 未来可扩展：`modbus`、`opcua`、`http`、`computed`

前端和主站业务侧优先使用 `var_id`。底层写入由后端通过 `var_id` 解析到真实数据源、写入路径和协议适配器。

## 变量属性建议

当前 `sys_tags` 已有基础字段，但实施 WS 写和超限判断前建议补齐或确认以下语义。

### 来源和发现

| 字段 | 说明 |
| --- | --- |
| `source_type` | 数据源类型。 |
| `gateway_id` / future `source_id` | 来源实例。当前 MQTT 使用 `gateway_id`。 |
| `source_topic` | MQTT topic 或来源通道名。 |
| `source_path` | 上游唯一路径。 |
| `raw_name` | 上游原始名称。 |
| `json_path` | 当前解析路径。非 JSON 数据源后续应由 adapter 转成统一 path。 |
| `discovered` | 是否自动发现。 |
| `placeholder` | 是否占位或虚变量。 |

约束：自动发现变量默认不能写、不能检测、不能历史存储，除非被分配到设备并配置启用。

### 展示和业务归属

| 字段 | 说明 |
| --- | --- |
| `project_id` / `project_code` | 业务设备归属。 |
| `var_name` | 系统内部稳定变量名。 |
| `display_name/display_name_en/display_name_ja` | 三语显示。 |
| `var_group` | 低频展示分组，不承担设备归属含义。 |
| `data_type` | 业务数据类型：`BOOL/INT/FLOAT/DOUBLE/STRING` 等。 |
| `unit` / `decimal_places` | 展示单位和精度。 |
| `enabled` | 是否作为业务变量启用。 |

约束：未绑定 `project_id` 的变量不能参与检测任务。

### 清洗和存储

| 字段 | 说明 |
| --- | --- |
| `scale_factor` / `offset_val` | 原始值到业务值的线性转换。 |
| `store_trigger` | `always` 或 `on_detection`。 |
| `store_mode` | 不存、变化、定时、混合。 |
| `store_cycle_sec` | 定时存储周期。 |
| `store_deadband` | 变化存储死区。 |
| `storage_name` | 入库字段业务名，面向动态表单、报表和查询展示。 |
| `storage_target` | 入库目标类型，例如 `history_eav`、`detection_form`、`report_field`、`wide_table`、`none`。 |
| `storage_table` | 目标表名。默认历史趋势仍是 `rt_history_data`。 |
| `storage_value_column` | 目标值列。通用历史表按类型使用 `value` 或 `str_value`。 |
| `storage_key_column` | 目标变量键列。通用历史表为 `var_id`。 |
| `storage_time_column` | 目标时间列。通用历史表为 `source_time`。 |
| `form_field_key` | 动态表单字段 key，用于检测表单、报表模板和前端配置对齐。 |
| `query_alias` | 查询返回时使用的稳定别名，避免前端直接依赖数据库列名。 |
| `suspicious_value` | 可疑值，例如计数器假 0。 |
| `debounce_threshold` | 上一有效值超过该阈值才启用可疑值拦截。 |
| `startup_snapshot_enable` | 冷启动首帧是否允许作为快照存储。 |

本项目当前缺少最后三个防抖/启动快照字段。是否引入需要结合精密空调变量特征判断；对计数器、状态量和控制反馈值比较有价值。

存储字段的边界：

- `store_mode/store_trigger/store_cycle_sec/store_deadband` 表示“什么时候允许存”。
- `storage_target/storage_table/storage_value_column/storage_key_column/storage_time_column` 表示“存到哪里、查询时从哪里取”。
- `form_field_key/query_alias/storage_name` 表示“动态表单、报表和前端查询怎么识别这个字段”。
- `sys_detection_standard_items.store_enabled` 表示“这个变量在某个检测标准/某次检测里是否存”，它是检测配置里的开关，不替代变量自己的存储映射。

默认历史趋势不建议为每个变量动态建列。当前 `rt_history_data` 是按 `var_id + source_time + value/str_value` 的 EAV/窄表模型，适合高频采集和动态变量。只有当检测结果表、动态表单或报表模板明确需要宽表字段时，才通过 `storage_target=detection_form/report_field/wide_table` 配置字段映射。

### 写入属性

| 字段 | 说明 |
| --- | --- |
| `rw_mode` | `R`、`W`、`RW`。 |
| `writable` | 是否允许从 API/WS 发起写。可由 `rw_mode` 派生，但建议显式字段便于权限过滤。 |
| `write_source_id` | 写入目标数据源；默认同读取来源。 |
| `write_path` | 后端 adapter 使用的写入路径。KIO 可映射到点名或 setdata payload。 |
| `write_data_type` | 写入值类型，避免前端传错类型。 |
| `write_min/write_max` | 写入值安全范围，仅对数值有效。 |
| `write_enum` | 枚举或布尔值允许集。 |
| `write_requires_audit` | 写操作是否必须审计；控制量默认 true。 |

约束：WS 写命令必须以 `var_id` 为目标，只允许写 `enabled=true` 且 `writable=true` 的变量，并且后端必须能解析出明确 `write_path`。客户端不得直接传 MQTT topic 作为最终写入目标。

## 运行态建议

运行态不应全部写入数据库主表。`TagManager` 内存对象应保存：

| 字段 | 说明 |
| --- | --- |
| `current_value/current_str_value` | 当前业务值。 |
| `last_value/last_str_value` | 上一次已确认业务值。 |
| `raw_value` | 最近原始值，便于排查清洗问题。 |
| `quality` / `last_quality` | 当前和上一质量。KIO `192` 映射为 Good。 |
| `source_time` | 来源时间。 |
| `last_update` | 后端接收更新时间。 |
| `last_change` | 值变化时间。 |
| `last_store` | 最后入库时间。 |
| `initialized` | 是否收到过有效样本。 |
| `stale` / `stale_reason` | 是否过期或来源断开。 |
| `debounce_pending` | 是否存在待确认可疑值。 |

WS 实时消息可以暴露 current、last、quality、timestamps、source identity，但防抖内部状态默认不暴露给普通前端，只在调试接口或日志中可见。

## 检测配置建议

检测标准项不应该被变量主表替代。同一个变量在不同模式、不同标准下可能有不同上下限。

当前 `sys_detection_standard_items` 和 `detection_run_standard_items` 已有：

- `check_enabled`
- `store_enabled`
- `required`
- `limit_ll`
- `limit_l`
- `limit_h`
- `limit_hh`
- `unit`
- `decimal_places`

建议补充或确认以下语义：

| 字段 | 说明 |
| --- | --- |
| `check_method` | `numeric_range`、`bool_equals`、`string_equals`、`regex`，v1 可只做 numeric。 |
| `target_value` | BOOL/STRING/枚举检测目标值。 |
| `limit_deadband` | 超限恢复死区，避免边界抖动反复产生事件。 |
| `violation_hold_ms` | 超限持续多久才记录，可选。 |
| `recover_hold_ms` | 恢复持续多久才关闭超限，可选。 |
| `quality_policy` | Bad quality 时忽略、记录无效、或直接判异常。 |

v1 可以先不实现所有字段，但设计上必须确认：`check_enabled` 控制是否参与超限判定，`store_enabled` 只控制检测期间是否写历史，两者不能混用。

上下限必须留在检测配置/检测运行快照里，不放到变量主表作为最终判定依据。同一个变量可以在不同检测标准、不同模式、不同产品型号下使用不同上下限。

存储映射不属于检测上下限配置。检测标准只决定“本次检测是否存这个变量”；变量属性决定“如果要存，后端应该写到哪个目标、哪个字段，以及查询时用哪个字段别名返回”。

## WS 消息约束

实时推送示例：

```json
{
  "type": "realtime.variable.delta",
  "request_id": "",
  "data": {
    "source_type": "mqtt",
    "gateway_id": 1,
    "source_path": "Objs.#(N==\"Temp\").1",
    "var_id": 1001,
    "project_id": 1,
    "var_name": "supply_air_temp",
    "value": 23.5,
    "last_value": 23.4,
    "quality": 1,
    "source_time": "2026-05-29T13:30:00+08:00"
  }
}
```

写命令示例：

```json
{
  "type": "command.variable.write",
  "request_id": "req-001",
  "command_id": "cmd-001",
  "data": {
    "var_id": 2001,
    "value": 1
  }
}
```

后端处理顺序：

1. 鉴权。
2. 校验 `command_id` 幂等。
3. 按 `var_id` 读取变量属性。
4. 校验 `enabled/writable/rw_mode/write_path/write_data_type/write_min/write_max/write_enum`。
5. 写入审计日志 accepted。
6. 调用对应 `SourceAdapter.Write` 或现有 KIO 写服务。
7. 写入审计日志 success/failed/timeout。
8. 返回 `command.result`。

## 检测超限处理顺序

检测超限不应直接使用变量主表的报警上下限，而应使用检测运行快照。

建议流程：

```text
SourceAdapter -> NormalizedTagEvent -> TagManager Update -> DetectionEvaluator -> Store/Violation/Event queues
```

`DetectionEvaluator` 只处理：

- 有 active detection task 的 `project_id`
- 当前任务标准快照里存在该 `var_id`
- `check_enabled=true`
- 类型和质量符合标准项策略

超限记录应进入独立表，例如 `detection_run_violations`，检测开始/停止/异常停止/超限进入/恢复进入 `detection_run_events`。这比复用通用 `sys_audit_logs` 更适合业务查询；`sys_audit_logs` 仍记录“谁发起了操作”。

## 实施顺序

1. 完成变量属性模型设计冻结：读写模式、写入映射、存储映射、动态表单字段、防抖字段、运行态快照字段。
2. 完成检测标准项语义冻结：`check_enabled`、`store_enabled`、上下限、质量策略、非数值类型策略。
3. 实施变量属性 schema/API/DTO，确保自动发现变量默认不可写、不可检测。
4. 实施 WS 实时读，只推送已启用或订阅允许的变量。
5. 实施 WS 写命令，强制走后端 service 和审计日志。
6. 实施检测事件和超限记录。

## 不做的事

- 不把自动发现变量直接变成检测项。
- 不让 WS 客户端直接传 MQTT topic 作为最终写目标。
- 不把检测上下限长期写死在变量主表里。
- 不把所有运行态字段持久化到 `sys_tags`。
- 不把通用审计日志当成检测业务事件表。
