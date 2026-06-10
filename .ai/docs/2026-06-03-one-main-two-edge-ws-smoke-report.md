# 一主二边缘 WS 真实联调测试报告

## 基本信息

- 时间：2026-06-03 11:16 Asia/Shanghai
- 执行身份：`test-ai`
- 目标：验证现有系统是否支持“一主服务器 + 两边缘节点 + 共用一个 MySQL + 共用一个 MQTT broker”的实时 WS 链路。
- 结论：未通过。当前系统不能按单主多边缘 WS 验收。
- 证据目录：`C:\Users\SunYufei\AppData\Local\Temp\edge-ws-smoke-20260603110913`

## 测试拓扑

本轮先停止旧进程，再启动三进程测试拓扑：

| 角色 | 地址 | edge_instance_id | 进程 |
| --- | --- | --- | --- |
| 边缘 1 | `127.0.0.1:18080` | `edge-1` | PID `25012` |
| 边缘 2 | `127.0.0.1:18081` | `edge-2` | PID `34624` |
| 主服务器 | `127.0.0.1:19080` | 当前配置仅 `edge-1` | PID `20004` |

共享资源：

- MySQL：`spindle_edge`
- MQTT broker：`tcp://127.0.0.1:1883`
- service token：`EDGE_MAIN_SERVICE_TOKEN=edge-main-dev-token-20260603`

临时测试数据：

| 项目 | project_id | edge_instance_id | 变量 | gateway | topic |
| --- | --- | --- | --- | --- | --- |
| `WS-SMOKE-E1-20260603111009` | `128` | `edge-1` | `910100111009` | `9101` | `datachange_WS_SMOKE_E1` |
| `WS-SMOKE-E2-20260603111009` | `129` | `edge-2` | `910200111009` | `9102` | `datachange_WS_SMOKE_E2` |

## 已验证通过的基础项

- `backend go test ./...` 通过。
- `main-server/backend go test ./...` 通过。
- `backend go build ./cmd/edge-backend` 通过。
- `main-server/backend go build ./cmd/main-server` 通过。
- 两个边缘 `/health` 可用。
- 两个边缘 `/runtime/channels/detail` 在测试后均为 `dropped=0`、`pressure=false`。
- 核心 workers `health=ok`。
- 慢客户端 7 秒未读未造成运行队列异常。

## 失败现象

### 1. 主服务器仍是单 edge bridge

`GET /api/v1/main-server/status` 只返回 `edge-1`。

主服务器 WS 请求：

```text
GET /api/v1/ws?topic=realtime.variables&project_id=129&edge_instance_id=edge-2
```

返回：

```json
{
  "code": "edge_instance_not_found",
  "edge_instance_id": "edge-2",
  "error": "edge instance is not available on this main-server bridge"
}
```

说明当前 `main-server` 没有多 edge registry，也没有按 `edge_instance_id` 选择目标边缘的能力。

### 2. 主服务器 HTTP realtime 未按 edge_instance_id 路由

请求：

```text
GET /api/v1/realtime/variables?project_id=129&edge_instance_id=edge-2
```

仍转发到 `edge-1`，返回 `edge-2` 项目变量旧值 `0`。同一时间 `edge-2` 直连 WS 已收到 MQTT 更新，变量值推进到 `3`。

这说明 HTTP realtime 当前忽略或未正确使用请求中的 `edge_instance_id`，仍走默认单边缘。

### 3. 主服务器 WS 缺少 project_id 与 edge_instance_id 一致性校验

请求：

```text
GET /api/v1/ws?topic=realtime.variables&project_id=129&edge_instance_id=edge-1
```

可以返回属于 `edge-2` 项目的变量，并在桥接消息顶层注入：

```json
{
  "edge_instance_id": "edge-1"
}
```

这是高风险错误。它会把跨边缘数据错误盖章为另一个 edge，后续前端、告警、控制链路都会被误导。

### 4. 边缘端共库运行态没有按 edge_instance_id 隔离

`edge-2 /health` 显示加载了网关 `1/9101/9102` 和 531 个 tags。

实际结果：

- `edge-2` 能看到并更新 `edge-1` 项目变量。
- `edge-2 /realtime/variables?project_id=128` 中，`edge-1` 变量随 `datachange_WS_SMOKE_E1` 更新到 `3`。
- `edge-1` 能看到 `edge-2` 项目变量，但因为未订阅 `9102`，值保持旧值。

当前代码事实：

- `sys_projects` 有 `edge_instance_id`。
- `sys_tags` 没有直接 `edge_instance_id`，通过 `project_id` 间接归属项目。
- `sys_gateways` 没有 `edge_instance_id`。
- `LoadGateways()` 只按 `enabled=true` 加载全部网关。
- `LoadTags()` 只按 `enabled=true AND project_id IS NOT NULL` 加载全部已绑定变量。

所以同库多边缘下，边缘端无法天然知道“本机只应启动哪些 MQTT gateway”和“本机只应维护哪些变量运行态”。

### 5. 大订阅 WS 快照存在桥接限制风险

主服务器 WS 全量订阅 531 tags 时，只收到 `connection.ready`，7 秒内没有收到 `realtime.variables.snapshot`。

当前判断：疑似主服务器 WS bridge 读取边缘大 snapshot 时触发 `256 KiB` read limit。该问题不是多边缘主因，但在多边缘聚合后会放大。

## 根因判断

本轮测试不说明“单主多边缘设计错误”，而说明当前实现只完成了单边缘主站桥接和部分主站查询过滤，尚未把 `edge_instance_id` 贯穿到运行态和路由层。

关键缺口：

1. 网关没有节点归属字段，边缘无法按本机身份过滤 MQTT gateway。
2. 边缘启动加载 gateway/tag 时没有使用当前 `auth.edge_instance_id`。
3. 主服务器配置是单 `edge`，不是 `edges[]` registry。
4. 主服务器 WS/HTTP realtime 没有根据 `project_id -> sys_projects.edge_instance_id` 解析目标 edge。
5. 主服务器缺少 `project_id + edge_instance_id` 强一致性校验。
6. 大 snapshot 读限制需要在多边缘聚合前处理。

## P0 修复计划

该问题应作为后续最高优先级处理。建议顺序：

1. 在 `sys_gateways` 增加 `edge_instance_id`，并同步 GORM model、schema、迁移、网关配置 API 和 seed。
2. 边缘端 `LoadGateways(edgeInstanceID)` 只加载本机网关。
3. 边缘端 `LoadTags(edgeInstanceID)` 通过 `sys_tags.project_id -> sys_projects.edge_instance_id` 只加载本机项目变量。
4. 变量创建、变量分配、网关配置更新时校验 project 与 gateway 的 `edge_instance_id` 一致。
5. 主服务器配置从单 `edge` 升级为 `edges[]` registry，同时保留旧配置兼容转换。
6. 主服务器 HTTP realtime 和 WS bridge 按 `edge_instance_id` 路由；只传 `project_id` 时必须能唯一解析目标 edge。
7. 主服务器对 `project_id + edge_instance_id` 不匹配返回明确错误，禁止错误盖章。
8. 处理大 snapshot：配置化 read limit，或改为分批 snapshot/增量首帧。
9. 建立一主二边缘自动化 smoke，作为主服务器实时链路发布门禁。

## 验收标准

修复后至少需要通过以下检查：

- `/api/v1/main-server/status` 能表达 `edge-1` 和 `edge-2`。
- `edge-1 /health` 只加载 `edge-1` 网关和变量。
- `edge-2 /health` 只加载 `edge-2` 网关和变量。
- 主服务器 WS `project_id=E1&edge_instance_id=edge-1` 成功。
- 主服务器 WS `project_id=E2&edge_instance_id=edge-2` 成功。
- 主服务器 WS `project_id=E2&edge_instance_id=edge-1` 明确失败。
- 主服务器 HTTP realtime `project_id=E2&edge_instance_id=edge-2` 返回来自 `edge-2` 的实时值。
- 停止 `edge-2` 不影响 `edge-1` 实时链路。
- 全量或大订阅不会只停留在 `connection.ready`。

## 证据文件

- `verify-evidence.json`：主流程、负向路由和 WS 收包摘要。
- `pressure-evidence.json`：慢客户端和大订阅补测。
- `final-health-evidence.json`：最终端口、health、channels、workers 状态。
- `edge-1.json`、`edge-2.json`、`main.json`：本轮临时启动配置。

本报告作为 EB-061/P0 多边缘地基修复的测试依据。后续不能把单边缘 WS 成功、fallback、或错误路由后返回数据视为多边缘通过。

## 2026-06-03 13:36 复验记录

后端完成 EB-061 首段修复后，`test-ai` 使用最新源码重新启动：

- `edge-1`: `127.0.0.1:18080`, PID `39220`
- `edge-2`: `127.0.0.1:18081`, PID `53672`
- `main-server`: `127.0.0.1:19080`, PID `23376`

复验配置和证据目录：

```text
C:\Users\SunYufei\AppData\Local\Temp\edge-ws-smoke-retest-20260603132415
```

关键证据：

- `verify-evidence.json`
- `smoke-data.json`
- `initial-health.json`
- `final-health-retest.json`
- `edge-1.json`
- `edge-2.json`
- `main.json`

通过项：

- `backend go test ./...` 通过。
- `main-server/backend go test ./...` 通过。
- `backend go build ./cmd/edge-backend` 通过。
- `main-server/backend go build ./cmd/main-server` 通过。
- 主服务器 `/api/v1/main-server/status` 返回 `edge-1` 和 `edge-2` 两个 edge nodes。
- `edge-1` 只启动 gateway `9101`，`edge-2` 只启动 gateway `9102`。
- MQTT 变量通过 discovery 生成候选，再 assignment 到各自项目；直接创建 `source_type=mqtt` 已被后端拒绝，这是符合当前变量发现边界的行为。
- 主服务器 HTTP realtime：
  - `project_id=134` 返回 `edge-1/gateway 9101` 实时值。
  - `project_id=135` 返回 `edge-2/gateway 9102` 实时值。
  - `project_id=135&edge_instance_id=edge-2` 返回 `edge-2` 实时值。
- 主服务器 WS：
  - `project_id=134&edge_instance_id=edge-1` 收到 `edge_instance_id=edge-1` 且 `gateway_id=9101` 的 snapshot。
  - `project_id=135&edge_instance_id=edge-2` 收到 `edge_instance_id=edge-2` 且 `gateway_id=9102` 的 snapshot。
- 负向校验通过：
  - `project_id=135&edge_instance_id=edge-1` 的 HTTP realtime 返回 `404 project_edge_instance_mismatch`。
  - 同样 mismatch 的 WS 建连前返回 `404 project_edge_instance_mismatch`。
  - `edge_instance_id=edge-X` 的 WS 返回 `404`。
- 主服务器全量 WS 按 `edge_instance_id=edge-1/edge-2` 均能返回 `realtime.variables.snapshot`，不再只停留在 `connection.ready`。
- 两个边缘最终 `/runtime/channels/detail` 均 `dropped=0`、`pressure=false`，workers `health=ok`。

剩余失败：

- `GET /api/v1/station-view/effective?project_id=135` 在不传 `edge_instance_id` 时返回 `404 not_found`。
- 显式请求 `GET /api/v1/station-view/effective?project_id=135&edge_instance_id=edge-2` 成功，说明 station view 数据本身可读，失败点是主服务器该接口未按 `project_id -> edge_instance_id` 自动解析目标 edge。

复验结论：

核心一主二边缘 WS/HTTP realtime 链路已经通过，前一轮发现的“主服务器单 edge bridge、WS 错误盖章、边缘启动跨节点网关混载、大 snapshot 无 snapshot”已被修复。但 EB-061 仍不能关闭，因为主服务器同名 station-view 接口仍存在 project-only 自动 edge 路由缺口。该缺口会影响前端只传 `project_id` 的工位页入口，应作为 EB-061 的剩余修复项继续处理。

## 2026-06-03 14:11 最终复验记录

后端补完主服务器 `GET /api/v1/station-view/effective` 的 `project_id -> sys_projects.edge_instance_id` 自动解析后，`test-ai` 重启真实主服务器并复跑完整一主二边缘 smoke。

运行拓扑：

- `edge-1`: `127.0.0.1:18080`, PID `39220`
- `edge-2`: `127.0.0.1:18081`, PID `53672`
- `main-server`: `127.0.0.1:19080`, PID `12788`

证据目录仍为：

```text
C:\Users\SunYufei\AppData\Local\Temp\edge-ws-smoke-retest-20260603132415
```

关键证据：

- `verify-evidence.json`
- `smoke-data.json`
- `final-health-1410.json`
- `main-retest-1358.out.log`
- `main-retest-1358.err.log`

验证结果：

- `main-server/backend go test ./internal/server -run "TestStationViewEffectiveRoute" -count=1` 通过。
- 重启 `main-server:19080` 后 `/health` 返回 `edge-1/edge-2` 两个 edge nodes。
- 完整 `verify-ws-retest.mjs` 返回 `failed_count=0`。
- `edge-1` 只启动 gateway `9101`，`edge-2` 只启动 gateway `9102`。
- 主服务器 station-view project-only 请求已经能解析 edge：
  - `project_id=136` 自动解析到 `edge-1`。
  - `project_id=137` 自动解析到 `edge-2`。
- 主服务器 HTTP realtime 分别返回 `edge-1/gateway 9101` 和 `edge-2/gateway 9102` 的实时值。
- 主服务器 WS 分别收到正确 `edge_instance_id` 的 snapshot：
  - `project_id=136&edge_instance_id=edge-1` 返回 `edge-1/gateway 9101`。
  - `project_id=137&edge_instance_id=edge-2` 返回 `edge-2/gateway 9102`。
- 负向校验保持正确：
  - `project_id=137&edge_instance_id=edge-1` 的 HTTP realtime 返回 `404 project_edge_instance_mismatch`。
  - 同样 mismatch 的 WS 建连前返回 `404 project_edge_instance_mismatch`。
- 全量 WS 按 `edge_instance_id=edge-1/edge-2` 均返回 snapshot。
- 最终两边缘 `/runtime/channels/detail` 均 `dropped=0`、`pressure=false`，workers `health=ok`。

最终结论：

EB-061/P0 的一主二边缘地基已经通过真实复验。后续可以继续推进主服务器报表、station view 模板发布和前端显式携带 `edge_instance_id` 的增强，但不得重新引入前端直连边缘、主服务器裸代理、跨边缘错误盖章或 fallback 当验收通过。

## 2026-06-03 14:25 控制指令路由补测

用户进一步确认目标：主服务器指令前端不区分边缘，由后端自动路由 WS 和 HTTP；变量表共享，但各边缘变量按节点归属隔离。`test-ai` 沿用 14:11 的三进程拓扑补测控制链路。

运行拓扑：

- `edge-1`: `127.0.0.1:18080`, PID `39220`
- `edge-2`: `127.0.0.1:18081`, PID `53672`
- `main-server`: `127.0.0.1:19080`, PID `12788`

证据文件：

```text
C:\Users\SunYufei\AppData\Local\Temp\edge-ws-smoke-retest-20260603132415\command-route-evidence.json
```

临时控制变量：

- project: `WS-SMOKE-E2-20260603061022`
- project_id: `137`
- edge_instance_id: `edge-2`
- var_name: `ws_cmd_route_20260603062518`
- var_id_text: `9212397624135540852`
- source_type: `virtual`

验证通过：

- `backend go test ./...` 通过。
- `main-server/backend go test ./...` 通过。
- `edge-1 /health`、`edge-2 /health`、`main-server /health` 均为 `ok`。
- 主服务器 HTTP edge-control 包体顶层带 `project_id=137` 时，可自动路由到 `edge-2` 并成功写入 virtual 变量。
- 主服务器 HTTP edge-control 显式错传 `edge_instance_id=edge-1` 且 `project_id=137` 时，返回 `404 control_edge_instance_mismatch`。
- 主服务器 WS URL 带 `project_id=137` 时，`command.write_variable` 正确路由到 `edge-2`，返回 `command.ack`，主服务器 realtime 后续可读到写入值。

发现的问题：

1. HTTP 显式 edge-control 只有 envelope 顶层 `project_id` 能触发主服务器自动路由；如果请求体只在 `payload.project_id` 里携带项目，主服务器不会解析 payload，最终落到默认 `edge-1` 并失败。
2. WS `command.write_variable` 在命令专用连接不带 URL `project_id/edge_instance_id` 时，不会从每条命令 payload 的 `project_id=137` 重新解析目标 edge，而是使用建连时默认 edge，当前为 `edge-1`，结果返回 `error edge_command_failed`。
3. 同一个 WS 命令在 URL 带 `project_id=137` 时能成功，说明边缘端 `VariableWriteService`、共享变量表隔离和 edge-2 命令执行链路本身可用；失败点在主服务器 WS 命令路由选择时机。

补测结论：

EB-061/P0 的 realtime WS/HTTP 和共享变量表隔离仍然通过；但“前端/调用方完全不区分边缘，由主服务器后端按指令 payload 自动路由 WS/HTTP 控制命令”尚未完成。该问题已登记为 EB-062。修复要求：

- 主服务器 WS 每条 `command.*` 消息都应按 payload 中的 `project_id`、`task_id` 或显式 edge 解析目标边缘，并校验 mismatch。
- 主服务器 HTTP `/api/v1/edge-control/*` 应同时支持 envelope 顶层和 `payload.project_id/task_id` 解析，避免调用方重复携带字段。
- 修复后必须补一主二边缘 command smoke：HTTP payload-only、WS command-only payload project、WS URL project、错误 edge mismatch、停 edge-2 负向场景。
