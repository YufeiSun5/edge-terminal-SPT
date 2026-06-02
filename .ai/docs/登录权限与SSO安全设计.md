# 登录权限与 SSO 安全设计

更新时间：2026-05-28 20:14 Asia/Shanghai

适用范围：Edge Terminal 本地 Electron 用户登录、Go sidecar API 鉴权、主服务器访问边缘端接口、Electron 登录后跳转主站 Web 的单点登录。

## 当前项目状态

- 后端已提供 `/health` 和 `/api/v1` 业务接口，但当前还没有登录、鉴权、用户表、服务调用凭据或 SSO 协议实现。
- 前端已预留 `desktop/src/shared/auth/tokenStore.ts`，access token 仅保存在 renderer 内存中。
- 前端 HTTP 与 SSE client 已能自动携带 `Authorization: Bearer <token>`。
- 看板 EB-004 已记录“边缘端登录与访问主服务器 Web 的单点登录能力”，当前仍 open。

本设计必须以插入式方式落地：不推倒现有前后端结构，不重写现有页面和业务 API，只在现有 API 外围增加认证中间件、权限点和少量 auth API。

## 设计目标

1. 同时支持两类访问者：
   - Electron 本地用户：现场用户从桌面端登录后操作边缘端。
   - 主服务器服务请求：主服务器后端访问边缘端实时、控制或 SSO ticket 校验接口。
2. 支持 Electron 登录后免登录打开主站 Web。
3. 权限模型足够轻，开发负担低，但能细粒度控制危险动作。
4. 所有真实安全校验放在 Go 后端；前端只负责登录态展示、菜单按钮显隐和 401/403 处理。
5. 后续新增接口和功能时，有固定维护规则，避免权限点遗漏。

## 非目标

- 不实现复杂 IAM、组织树、资源实例级 ACL 或策略语言。
- 不让 renderer 直接生成、拼接或长期保存主站 token。
- 不把主服务器服务请求伪装成本地用户请求。
- 不把数据库历史同步纳入应用层登录权限范围。

## 身份类型

### 本地用户身份

本地用户由边缘端后端认证，登录成功后拿 `edge_user_jwt`。

用途：

- 现场查看实时数据和历史数据。
- 启停检测任务。
- 管理变量、网关、系统设置。
- 申请 SSO ticket 跳转主站 Web。

### 主站服务身份

主站服务请求使用 `main_service_token`，代表主服务器后端，不代表某个现场用户。

用途：

- 主站后端读取边缘端开放的实时数据。
- 主站后端调用已确认的控制或方法接口。
- 主站后端校验 Electron 发起的 SSO ticket。

主站服务 token 不下发给浏览器，不进入 Electron renderer。

## 令牌模型

| 令牌 | 持有方 | 生命周期 | 用途 |
| --- | --- | --- | --- |
| `edge_user_jwt` | Electron renderer 内存 | 建议 30 分钟 | 本地用户调用边缘端 API |
| `main_service_token` | 主站后端和边缘端配置 | 可轮换，长期但受控 | 主站服务调用边缘端服务接口 |
| `sso_ticket` | Electron 临时拿到，主站后端一次性校验 | 建议 60 秒，单次使用 | Electron 登录态换取主站 Web session |

### edge_user_jwt

JWT 负责认证，不直接承担完整权限存储。推荐 claim：

```json
{
  "iss": "spindle-edge",
  "aud": "edge-terminal",
  "sub": "1",
  "username": "admin",
  "role": "admin",
  "permissions_version": 1,
  "iat": 1710000000,
  "nbf": 1710000000,
  "exp": 1710001800,
  "jti": "uuid"
}
```

规则：

- access token 只放 renderer 内存，沿用现有 `tokenStore.ts`。
- 后端每次请求校验签名、`iss`、`aud`、`exp`、`nbf`、用户启用状态、`permissions_version`。
- 完整权限点由后端根据 `role` 计算；前端通过 `/api/v1/auth/me` 获取当前 `permissions`。
- 权限变更时递增用户 `permissions_version`，旧 token 自动失效。

### main_service_token

第一版推荐使用服务客户端表和 Bearer token：

- 主站后端请求带 `Authorization: Bearer <service_token>`。
- 边缘端只保存 token hash，不保存明文 token。
- token 对应 `client_id`、`scopes`、`enabled` 和过期时间。
- service token 只代表主服务器后端服务身份，不代表某个用户，也不能创建用户 Web 登录态。

后续如果主站和边缘端网络边界变复杂，可升级为 mTLS 或签名请求；不影响本地用户 JWT 设计。

### sso_ticket

SSO ticket 是一次性换票凭据，不是长期登录 token。

生成规则：

- Electron 本地用户已登录。
- 用户具备 `sso_handoff` 权限。
- ticket 随机生成，数据库只保存 `ticket_hash`。
- TTL 建议 60 秒。
- 只能使用一次。
- 绑定 `user_id`、`role`、`permissions_version`、`edge_instance_id`、`created_ip`。

校验规则：

- 只有主站后端服务身份可以调用校验接口。
- 校验成功后立刻写入 `used_at`。
- 返回用户基础身份、角色、权限版本和边缘端实例 ID，由主站创建自己的 Web session。
- 不把 `edge_user_jwt` 直接交给主站 Web。
- SSO ticket 只用于用户免登录，不用于主服务器后端调用边缘端控制接口。

### 同一批用户与控制审计

主服务器和边缘端的用户列表按同一批用户维护。首版可以用稳定 `username` 或双方约定的用户 ID 对齐；如果主服务器已经有统一用户 ID，建议边缘端 `sys_users` 后续补充 `external_user_id` 或 `main_user_id` 字段，避免只靠显示名或人工备注映射。

需要区分两条链路：

| 链路 | 凭据 | 作用 |
| --- | --- | --- |
| 用户登录/免登录 | 本地用户 JWT、一次性 SSO ticket、主服务器 Web session | 证明当前操作者是谁。 |
| 主服务器控制边缘端 | service token 或后续签名请求 | 证明调用方是主服务器后端。 |

主服务器后端调用边缘端控制接口时，必须同时带 service token 和操作者信息，例如 `operator_id`、`operator_username`、`operator_name`。边缘端应把操作者映射到本地 `sys_users`，审计同时记录 service `client_id` 和实际用户。这样既能保持同一批用户的操作归属，也不会把 service token 误当成超级用户登录态。

禁止：

- 用 SSO ticket 调用控制接口。
- 用 service token 登录用户界面。
- 只记录 `main_server` 服务身份而丢失实际操作者。

## 三角色模型

角色只保留三种：

| 角色 | 说明 |
| --- | --- |
| `guest` | 游客，只允许查看，不允许控制和配置 |
| `admin` | 管理员，允许现场运维、配置、控制和用户管理 |
| `developer` | 开发者，允许调试配置和查看系统状态，默认不允许危险控制写入 |

角色名进入数据库和 JWT，用户界面可翻译为中文、英文、日文。

## 权限点

第一版权限点如下。后续新增接口时只能复用这些权限点或新增明确命名的权限点，不能直接按角色硬编码。

| 权限点 | 含义 |
| --- | --- |
| `view_realtime` | 查看实时数据、网关在线状态 |
| `view_history` | 查看历史查询与检测历史 |
| `start_detection` | 启动检测任务 |
| `stop_detection` | 停止检测任务 |
| `manage_variables` | 修改变量、变量归属、变量启用状态 |
| `manage_gateways` | 新增、修改、删除 MQTT/KIO 网关配置 |
| `kio_write` | 下发 KIO 写入或其他设备写控制 |
| `manage_users` | 用户管理、重置密码、启停用户 |
| `system_settings` | 查看或修改系统级设置、运行时诊断 |
| `sso_handoff` | 申请主站 SSO ticket |
| `service_realtime_read` | 主站服务读取边缘端实时数据 |
| `service_control_call` | 主站服务调用已确认的控制/方法接口 |
| `service_sso_verify` | 主站服务校验 SSO ticket |

## 角色权限矩阵

| 权限点 | guest | admin | developer |
| --- | --- | --- | --- |
| `view_realtime` | 是 | 是 | 是 |
| `view_history` | 是 | 是 | 是 |
| `start_detection` | 否 | 是 | 否 |
| `stop_detection` | 否 | 是 | 否 |
| `manage_variables` | 否 | 是 | 是 |
| `manage_gateways` | 否 | 是 | 是 |
| `kio_write` | 否 | 是 | 否 |
| `manage_users` | 否 | 是 | 否 |
| `system_settings` | 否 | 是 | 是 |
| `sso_handoff` | 否 | 是 | 是 |

说明：

- `developer` 默认不拥有 `kio_write`，避免调试期间误写设备。
- 主站服务身份不使用上述用户角色，使用 service scope：`service_realtime_read`、`service_control_call`、`service_sso_verify`。
- 如确需开发者临时写设备，应通过单独开关或临时角色调整，并记录审计。

## API 鉴权规则

`/health` 保持公开，用于 Electron 主进程和安装环境健康检查。

本地用户 API：

| API | 权限 |
| --- | --- |
| `POST /api/v1/auth/login` | 公开，但需要登录限流 |
| `POST /api/v1/auth/logout` | 已登录 |
| `GET /api/v1/auth/me` | 已登录 |
| `POST /api/v1/auth/sso-ticket` | `sso_handoff` |
| `GET /api/v1/realtime/variables` | `view_realtime` |
| `GET /api/v1/gateways` | `view_realtime` |
| `GET /api/v1/runtime/channels` | `system_settings` |
| `GET /api/v1/devices` | `view_realtime` |
| `POST /api/v1/devices` | `manage_variables` |
| `GET /api/v1/variables` | `view_realtime` |
| `PATCH /api/v1/variables/:id` | `manage_variables` |
| `PATCH /api/v1/variables/:id/assignment` | `manage_variables` |
| `DELETE /api/v1/variables/:id` | `manage_variables` |
| `GET /api/v1/gateway-configs` | `manage_gateways` |
| `GET /api/v1/gateway-configs/:id` | `manage_gateways` |
| `POST /api/v1/gateway-configs` | `manage_gateways` |
| `PATCH /api/v1/gateway-configs/:id` | `manage_gateways` |
| `DELETE /api/v1/gateway-configs/:id` | `manage_gateways` |
| `POST /api/v1/gateway-configs/:id/discover` | `manage_gateways` |
| `POST /api/v1/gateways/:id/publish` | `kio_write` |
| `POST /api/v1/gateways/:id/subscribe` | `manage_gateways` |
| `POST /api/v1/gateways/:id/kio/write` | `kio_write` |
| `POST /api/v1/gateways/:id/kio/query-all` | `manage_gateways` |
| `GET /api/v1/detection-runs/active` | `view_realtime` |
| `POST /api/v1/detection-runs` | `start_detection` |
| `POST /api/v1/detection-runs/:id/stop` | `stop_detection` |

主站服务 API：

| API | Scope |
| --- | --- |
| `POST /api/v1/auth/sso-ticket/verify` | `service_sso_verify` |
| 主站实时读取出口 SSE/WS | `service_realtime_read` |
| 主站控制/方法调用接口 | `service_control_call` |

后续新增主站接口时，必须明确是“服务身份”还是“本地用户身份”，不能共用同一个中间件入口。

## 插入式后端落地

建议新增包：

```text
backend/internal/auth/
  claims.go
  jwt.go
  password.go
  permissions.go
  middleware.go
  service_token.go
  sso_ticket.go
```

建议最小数据表：

```sql
sys_users (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  username VARCHAR(64) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  role VARCHAR(32) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  permissions_version BIGINT NOT NULL DEFAULT 1,
  last_login_at DATETIME(3) NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL
)

sys_sso_tickets (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  ticket_hash VARCHAR(255) NOT NULL UNIQUE,
  user_id BIGINT UNSIGNED NOT NULL,
  edge_instance_id VARCHAR(128) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  used_at DATETIME(3) NULL,
  created_at DATETIME(3) NULL
)

sys_service_clients (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  client_id VARCHAR(128) NOT NULL UNIQUE,
  secret_hash VARCHAR(255) NOT NULL,
  scopes TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  expires_at DATETIME(3) NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL
)

sys_audit_logs (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  actor_type VARCHAR(32) NOT NULL,
  actor_id VARCHAR(128) NOT NULL,
  action VARCHAR(128) NOT NULL,
  target_type VARCHAR(64) DEFAULT '',
  target_id VARCHAR(128) DEFAULT '',
  result VARCHAR(32) NOT NULL,
  detail JSON NULL,
  created_at DATETIME(3) NULL
)
```

第一版可把角色到权限点映射写在 Go 常量里，不做权限配置页面。

路由改造方式：

```go
v1 := k.router.Group("/api/v1")

v1.POST("/auth/login", k.auth.Login)
v1.POST("/auth/sso-ticket/verify", serviceAuth.Require("service_sso_verify"), k.auth.VerifySSOTicket)

protected := v1.Group("")
protected.Use(userAuth.RequireLogin())

protected.GET("/auth/me", k.auth.Me)
protected.POST("/auth/logout", k.auth.Logout)
protected.POST("/auth/sso-ticket", userAuth.Require("sso_handoff"), k.auth.CreateSSOTicket)

protected.GET("/realtime/variables", userAuth.Require("view_realtime"), k.handleRealtimeVariables)
protected.POST("/gateways/:gateway_id/kio/write", userAuth.Require("kio_write"), k.handleKIOWrite)
```

现有 handler 可以先从匿名函数逐步提取成方法，但不要求一次性重构。第一阶段可保留现有 handler 结构，只在每段路由前插入 `Require(...)`。

## 插入式前端落地

沿用现有结构：

- `desktop/src/shared/auth/tokenStore.ts` 继续只保存 access token。
- `desktop/src/features/auth/api.ts` 保持 typed client，补 `/login`、`/logout`、`/me`、`/sso-ticket`。
- `desktop/src/features/auth/authStore.ts` 存储 `user`、`role`、`permissions`、`authenticated`。
- router 增加登录页和受保护布局，不改业务页面主体。
- 菜单、按钮、危险操作按 `permissions` 显隐。
- axios 统一处理：
  - `401`：清理 token，跳转登录。
  - `403`：保持登录态，显示无权限提示。

前端不得：

- 直接读取或持久化主站 token。
- 从 URL 解析并保存长期 token。
- 只靠按钮隐藏实现安全控制。
- 自行构造 SSO ticket。

## SSO 流程

Electron 用户点击“打开主站”：

1. Electron renderer 调用边缘端：

```http
POST /api/v1/auth/sso-ticket
Authorization: Bearer <edge_user_jwt>
```

2. 边缘端校验 `sso_handoff` 权限，生成一次性 ticket，返回：

```json
{
  "ticket": "random-ticket",
  "expires_in": 60,
  "main_site_url": "https://main.example.com/sso/edge?ticket=random-ticket&edge_id=edge-001"
}
```

3. Electron 通过主进程 `shell.openExternal` 打开主站 URL。

4. 主站后端收到 ticket 后调用边缘端：

```http
POST /api/v1/auth/sso-ticket/verify
Authorization: Bearer <main_service_token>
Content-Type: application/json

{
  "ticket": "random-ticket",
  "edge_id": "edge-001"
}
```

5. 边缘端校验成功后返回：

```json
{
  "edge_instance_id": "edge-001",
  "user": {
    "id": 1,
    "username": "admin",
    "role": "admin",
    "permissions_version": 1
  }
}
```

6. 主站创建自己的 Web session。边缘端 ticket 标记为已使用。

## 敏感数据与审计

必须审计：

- 登录成功和失败。
- 创建 SSO ticket。
- 校验 SSO ticket 成功和失败。
- 主服务器控制命令中的 service `client_id`、`operator_id/operator_username`、映射到的边缘端用户、`command_id` 和执行结果。
- 启动/停止检测任务。
- 修改变量、变量归属、网关配置。
- KIO write、MQTT publish 等写设备动作。
- 用户管理、服务 token 轮换。

敏感字段规则：

- 用户密码只保存 hash。
- service token 只保存 hash。
- SSO ticket 只保存 hash。
- MQTT/KIO 密码字段 API 写入时可接收，读取时默认不返回明文。
- 日志和审计不得记录明文密码、JWT、service token、SSO ticket。

## 后续接口和功能维护规则

新增 API 时必须在同一 PR 或同一变更中完成：

1. 在本设计文档或后续 API 契约中登记接口权限。
2. 明确访问身份：本地用户、主站服务、公开健康检查三选一。
3. 为本地用户接口绑定一个或多个 capability。
4. 为主站服务接口绑定 service scope。
5. 如果接口会改变设备、网关、变量、检测任务或用户状态，增加审计事件。
6. 如果返回配置或凭据，检查是否需要脱敏。
7. 在 `AI_BOARD.md` 记录 API/DTO/权限影响。
8. 至少补 smoke：未登录返回 `401`、无权限返回 `403`、有权限返回预期结果。

新增页面或按钮时必须：

1. 前端使用 `/api/v1/auth/me` 的 `permissions` 控制入口显隐。
2. 页面不得直接判断角色名，除非只是展示角色标签。
3. 操作失败时区分 `401` 和 `403`。
4. 用户可见文案补中文、英文、日文。

新增角色时必须：

1. 先说明为什么三角色不能覆盖。
2. 更新角色权限矩阵。
3. 更新后端角色常量和测试。
4. 更新前端角色展示文案。

新增权限点时必须：

1. 权限点命名使用动词加资源，例如 `manage_reports`、`export_history`。
2. 默认不授予 `guest`。
3. 明确是否允许 `developer`。
4. 更新权限矩阵和 route permission map。

## 分阶段落地

### 第一阶段：本地登录和 API 鉴权

- 新增 `sys_users`。
- 实现密码 hash、JWT 签发、`/auth/login`、`/auth/me`、`/auth/logout`。
- 给现有 `/api/v1` 接口加用户鉴权和权限中间件。
- 前端增加登录页、路由守卫、401/403 处理。

### 第二阶段：主站服务身份

- 新增 `sys_service_clients`。
- 实现 service token 校验。
- 将主站实时出口和控制接口绑定 service scope。
- 明确主站服务调用的审计字段。

### 第三阶段：SSO handoff

- 新增 `sys_sso_tickets`。
- 实现 `/auth/sso-ticket` 和 `/auth/sso-ticket/verify`。
- Electron 增加“打开主站”入口，通过主进程打开外部 URL。
- 主站接入 ticket verify。

### 第四阶段：审计和凭据治理

- 新增 `sys_audit_logs`。
- 网关/KIO 密码 API 返回脱敏。
- service token 轮换流程。
- 登录失败限流、账号锁定、密码修改。

## 自审结论

本设计满足当前约束：

- 不推倒现有前端：沿用 tokenStore、typed API 和现有页面，只加 auth store、登录页、路由守卫和按钮权限。
- 不推倒现有后端：沿用 Gin `/api/v1`，通过中间件和少量 auth endpoint 插入。
- 同时区分本地用户和主站服务身份，避免服务调用冒充现场人员。
- SSO 使用一次性 ticket，避免把边缘端 JWT 暴露给主站 Web。
- 三角色保持简单，危险动作通过 capability 控制。
- 后续新增 API、页面、权限点、角色都有维护规则。

仍需确认：

- 主站 Web 的正式域名、回调路径和边缘端实例 ID 来源。<!-- 待确认 -->
- service token 的初始下发和轮换流程由安装器、配置文件还是主站注册流程承担。<!-- 待确认 -->
- `developer` 是否在实验室环境可临时拥有 `kio_write`。<!-- 待确认 -->
