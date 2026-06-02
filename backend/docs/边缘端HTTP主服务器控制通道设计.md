# 边缘端 HTTP 主服务器控制通道设计

更新时间：2026-06-02

## 结论

主服务器到边缘端的首版控制通道确定使用 HTTP。该通道不是通用代理，也不是普通前端登录接口的复用；它是服务端到服务端的受控命令入口，必须具备独立鉴权、细粒度 scope、幂等、审计、错误码和命令生命周期记录。

边缘端继续负责现场执行：检测启动/停止、变量写入、报警静音、运行中限值调整、特征值刷新和本地审计。主服务器只能通过 HTTP 控制通道提交命令，不允许直接修改同步库里的边缘端业务表来控制现场。

## 非目标

- 不在边缘端生成主服务器 Excel/PDF 报表。
- 不在边缘端维护主服务器报表任务、文件资产或重新生成 worker。
- 不提供长期万能 `/api/v1/**` 写代理。
- 不让主服务器直接写 `sys_detection_tasks`、`sys_tags`、`detection_run_standard_items` 等同步表来改变现场状态。
- 不为主服务器另写一套检测、写变量或报警逻辑。

## 推荐路由

控制路由集中在 `/api/v1/edge-control` 下，便于统一鉴权、审计和限流。

| Method | Path | Scope | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/v1/edge-control/detection/start` | `edge.detection.start` | 已实现。启动检测，复用 `DetectionRunsService`。 |
| `POST` | `/api/v1/edge-control/detection/stop` | `edge.detection.stop` | 已实现。正常停止检测，复用 `DetectionRunsService`。 |
| `POST` | `/api/v1/edge-control/detection/abnormal-stop` | `edge.detection.stop` | 已实现。异常停止检测，复用 `DetectionRunsService`。 |
| `POST` | `/api/v1/edge-control/variables/write` | `edge.variable.write` | 已实现。写变量，复用 `VariableWriteService` 和 `KIOWriteService`。 |
| `POST` | `/api/v1/edge-control/detection/mute-alarms` | `edge.alarm.mute` | 已实现。静音当前 running 检测的运行态报警状态，写命令和审计。 |
| `POST` | `/api/v1/edge-control/detection/update-limits` | `edge.detection.limit_update` | 已实现。运行中调整检测运行快照限值，并刷新 running runtime map。 |
| `POST` | `/api/v1/edge-control/detection/refresh-features` | `edge.feature.refresh` | 已实现。刷新 `detection_run_features`，写 `features_updated` 事件。 |
| `POST` | `/api/v1/edge-control/detection/report-requests` | `edge.report.request` | 已实现。为已有检测任务追加 `detection_run_report_requests` 请求快照。 |

当前首版已覆盖检测启动、正常停止、异常停止、变量写入、运行态报警静音、运行中限值调整、特征值刷新和报表请求独立登记。主服务器最小后端调用链和现场组合 smoke 仍是后续扩展。

注意：当前报警静音只静音边缘端 `TaskManager` 中的 active alarm state，并通过 `edge_control_commands`、`sys_audit_logs` 和 `detection_run_events` 留痕；`detection_limit_alarms` 表尚无 `muted` 字段，因此历史报警行不会持久化静音标记。若前端/主服务器需要查询“已静音”历史状态，后续需要独立 schema 变更。

## 服务身份鉴权

主服务器使用 service token，不使用本地用户 JWT。边缘端已有 `sys_service_clients` 和 service token 能力，应在此基础上增强。

这里要和用户单点登录区分清楚：主服务器和边缘端的用户列表应视为同一批用户，用户名、用户 ID 或外部用户 ID 要能对应；但服务端控制命令的网络认证仍然使用 service token。用户免登录和页面会话互认继续走一次性 SSO ticket，service token 只证明“这是主服务器后端”，不证明“这是某个用户”。

建议 service client 具备：

| 字段 | 说明 |
| --- | --- |
| `client_id` | 主服务器服务身份。 |
| `token_hash` | 只保存 token hash，不保存明文。 |
| `name` | 服务名称。 |
| `scopes` | 逗号或 JSON 数组形式的服务权限。 |
| `allowed_cidrs` | 允许来源 IP/CIDR。 |
| `enabled` | 是否启用。 |
| `expires_at` | 过期时间，可为空。 |
| `last_used_at` | 最近使用时间。 |

当前边缘端已在 `sys_service_clients` 保存 `allowed_cidrs` 和 `last_used_at`；`allowed_cidrs` 为空时不限制来源 IP，非空时支持逗号分隔的 IP 或 CIDR。`EDGE_MAIN_SERVICE_TOKEN` 环境变量种子会默认授予既有 `service_*` scope 和首批 `edge.*` 控制 scope，生产部署仍应按最小权限配置。

基础请求头：

```text
Authorization: Bearer <service_token>
X-Command-ID: <uuid>
X-Request-Time: <iso8601>
```

如果现场网络不可完全信任，再增加 HMAC 防重放：

```text
X-Client-ID: <client_id>
X-Timestamp: <unix_ms>
X-Nonce: <random>
X-Signature: <hmac_sha256>
```

无 token 返回 `401`；token 有效但 scope、来源 IP 或状态不满足时返回 `403`。

## 用户 SSO 与操作者映射

系统里需要同时保留两层身份：

| 身份 | 凭据 | 用途 |
| --- | --- | --- |
| 用户身份 | 本地用户 JWT、一次性 SSO ticket、主服务器 Web session | 用户登录、免登录、页面权限和用户操作归属。 |
| 服务身份 | service token 或后续 HMAC/mTLS | 主服务器后端调用边缘端控制接口。 |

主服务器和边缘端用户列表按同一批用户维护。首版可以用稳定 `username` 或 `user_id` 对齐；如果主服务器已有统一用户 ID，建议边缘端 `sys_users` 后续补 `external_user_id` 或 `main_user_id` 字段作为强映射。

HTTP 控制命令必须携带操作者信息：

```json
{
  "operator_id": "main-user-1",
  "operator_name": "张三",
  "operator_username": "zhangsan"
}
```

边缘端收到命令后应尝试把 `operator_username`、`operator_id` 或 `external_user_id` 映射到本地 `sys_users`：

- 映射成功：审计记录边缘端 `user_id`、用户名、主服务器用户 ID 和 service `client_id`。
- 映射失败：可以按策略拒绝控制命令，或只允许低风险命令并记录 `operator_unmapped=true`；生产建议拒绝会影响现场的控制命令。

禁止事项：

- 不允许用 SSO ticket 调用 `/api/v1/edge-control/*`。
- 不允许用 service token 创建用户 Web 登录态。
- 不允许把 service token 当成超级管理员用户。
- 不允许只记录 `client_id=main_server` 而不记录实际操作者。

## Scope

首版 scope 不要用一个总权限覆盖全部控制动作，至少拆为：

| Scope | 用途 |
| --- | --- |
| `edge.readonly` | 只读服务查询，控制接口不接受该 scope。 |
| `edge.detection.start` | 启动检测。 |
| `edge.detection.stop` | 停止或异常停止检测。 |
| `edge.variable.write` | 变量写入。 |
| `edge.alarm.mute` | 检测报警静音。 |
| `edge.detection.limit_update` | 运行中调整限值。 |
| `edge.feature.refresh` | 刷新特征值。 |
| `edge.report.request` | 提交或更新报表请求快照，若后续需要。 |

每个 handler 必须绑定具体 scope，不允许因为 service token 有效就执行所有命令。

## 命令幂等

主服务器 HTTP 重试不可造成重复启动、重复下设或重复静音。每个控制请求必须携带稳定 `command_id`，边缘端按 `command_id + client_id` 做幂等。

已新增命令记录表 `edge_control_commands`：

| 字段 | 说明 |
| --- | --- |
| `id` | 自增主键。 |
| `command_id` | 主服务器生成的命令 ID。 |
| `client_id` | service client。 |
| `operator_id` | 主服务器操作者 ID。 |
| `operator_name` | 主服务器操作者名称。 |
| `operator_username` | 主服务器操作者用户名，用于映射边缘端同批用户。 |
| `edge_user_id` | 映射成功后的边缘端 `sys_users.id`。 |
| `action` | `detection.start`、`variable.write` 等。 |
| `target_type` | `project`、`task`、`variable`、`alarm` 等。 |
| `target_id` | 目标 ID，字符串保存以兼容 64 位变量 ID。 |
| `request_json` | 脱敏后的请求内容。 |
| `status` | `received`、`running`、`success`、`failed`。 |
| `result_json` | 成功结果。 |
| `error_code` | 失败错误码。 |
| `error_message` | 失败摘要。 |
| `received_at` | 接收时间。 |
| `completed_at` | 完成时间。 |

同一个 `client_id + command_id` 再次请求时：

- 已成功：直接返回原成功结果。
- 已失败且不可重试：返回原失败结果。
- 正在执行：返回 `command_running` 或当前状态，避免并发重复执行。

当前实现按 `client_id + command_id` 唯一约束幂等；重复成功命令返回历史 `result_json`，重复失败命令返回历史错误，`received/running` 命令返回 `command_running` 且 `retryable=true`。

## 请求包

所有控制请求建议统一 envelope：

```json
{
  "command_id": "uuid",
  "operator_id": "main-user-1",
  "operator_name": "张三",
  "operator_username": "zhangsan",
  "reason": "manual start from main server",
  "payload": {}
}
```

`command_id` 同时允许从 `X-Command-ID` 读取，但 body 和 header 同时存在时必须一致。

首段所有控制请求必须携带 `operator_username`，边缘端会映射到本地启用的 `sys_users.username`；映射失败的现场控制命令会被拒绝。

## 响应包

成功：

```json
{
  "ok": true,
  "command_id": "uuid",
  "status": "success",
  "result": {}
}
```

失败：

```json
{
  "ok": false,
  "command_id": "uuid",
  "status": "failed",
  "error": {
    "code": "permission_denied",
    "message": "missing scope edge.variable.write",
    "retryable": false
  }
}
```

建议错误码：

| Code | Retryable | 说明 |
| --- | --- | --- |
| `unauthorized` | false | 未认证或 token 无效。 |
| `permission_denied` | false | scope、CIDR、状态不满足。 |
| `invalid_payload` | false | 请求格式或字段非法。 |
| `duplicate_command` | false | 命令重复且结果已确定。 |
| `command_running` | true | 同命令仍在执行。 |
| `project_not_found` | false | 项目不存在。 |
| `detection_conflict` | false | 同项目已有 running 检测等业务冲突。 |
| `variable_not_writable` | false | 变量不可写或写入约束不满足。 |
| `edge_busy` | true | 边缘端忙或队列拥塞。 |
| `kio_timeout` | true | 现场下设超时。 |
| `internal_error` | true | 未分类内部错误。 |

## 复用现有服务

HTTP 控制 handler 只做鉴权、幂等、参数校验、审计和响应映射。业务执行必须复用已有 service 或任务模块：

- `DetectionRunsService`
- `VariableWriteService`
- `KIOWriteService`
- 任务系统内置模块，如启动检测、停止检测、静音、运行中限值调整、特征值刷新
- 审计日志能力

禁止复制一套主服务器专用检测启动、变量写入或报警静音逻辑。

## 当前已实现 payload

`POST /api/v1/edge-control/detection/update-limits`：

```json
{
  "command_id": "uuid",
  "operator_username": "zhangsan",
  "payload": {
    "task_id": 123,
    "items": [
      {
        "var_id": "9212397624135540846",
        "limit_h": 18.5,
        "limit_hh": 20,
        "limit_deadband": 0.5,
        "alarm_enabled": true,
        "check_enabled": true
      }
    ]
  }
}
```

`POST /api/v1/edge-control/detection/report-requests`：

```json
{
  "command_id": "uuid",
  "operator_username": "zhangsan",
  "payload": {
    "task_id": 123,
    "report_request": {
      "reports": [
        {
          "template_code": "PERF-STD",
          "variables": [{"var_name": "outlet_temp"}],
          "params": {"inlet_area_m2": 1.25}
        }
      ]
    }
  }
}
```

报表请求解析复用检测启动时的 `report_request` 规则：支持 `reports[]`、`template_id/template_code`、`variables/var_ids/variable_names` 和 `params`，落表后仍由外部数据库同步软件同步到主服务器，主服务器生成自己的报表任务和文件资产。

## 审计

需要双层记录：

1. `edge_control_commands` 记录命令生命周期和幂等结果。
2. `sys_audit_logs` 记录受保护写请求和最终业务动作。

审计必须能回答：

- 哪个主服务器服务身份发起。
- 哪个主服务器用户触发，以及映射到哪个边缘端本地用户。
- 执行了什么 action。
- 操作了哪个项目、任务或变量。
- 边缘端是否执行成功。
- 失败原因。
- 对应 `command_id`。

日志和审计不得记录明文 service token、密码、KIO 凭据或完整敏感请求体。

## 安全底线

- 控制接口默认拒绝匿名访问。
- 控制接口拒绝本地用户 JWT 冒充 service token。
- 控制接口拒绝 SSO ticket 冒充 service token。
- service token 不能替代用户登录态；SSO ticket 不能替代服务端控制凭据。
- service token 只保存 hash。
- 支持禁用、过期和轮换。
- 支持 `allowed_cidrs`。
- 请求体大小限制，避免大 JSON 冲击边缘端。
- `command_id` 必填，避免无幂等写操作。
- 所有写动作进入审计。

## 验收门禁

后端实现关闭前至少验证：

- 缺 service token 返回 `401`。
- service token 无 scope 返回 `403`。
- `allowed_cidrs` 不匹配返回 `403`。
- SSO ticket 或本地用户 JWT 调用控制接口被拒绝。
- 控制命令缺少操作者或操作者无法映射到同批用户时，影响现场的命令被拒绝或明确记录策略。
- 同 `command_id` 重放返回同一结果，不重复执行。
- `start/stop/write/mute/limit_update/refresh` 至少覆盖已实现子集并写命令记录和审计。
- `write_variable` 仍走 `VariableWriteService/KIOWriteService`。
- 非 writable 变量拒绝。
- KIO 超时返回结构化错误和明确 `retryable`。
- `go test ./...`。
- `go vet ./...`。
- `golangci-lint run ./...`。
- `go build ./cmd/edge-backend`。
- `go run -tags smoke_tools ./tools/smokehealth`。
- `go run -tags smoke_tools ./tools/smokebackend`。
