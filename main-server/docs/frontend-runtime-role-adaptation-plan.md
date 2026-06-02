# 前端一套代码适配 Edge/Main Server 计划

## 背景与结论

当前前端业务 API 基本已经集中在 `src/shared/api/http.ts`，页面大多通过 `/api/v1/...` 相对路径访问后端，因此后端路由可以承担 edge/main-server 的执行差异。阻塞一套前端复用的主要问题不是业务 API 路径，而是以下运行态假设：

- Electron 壳硬编码启动 `edge-backend.exe` sidecar，并检查 `127.0.0.1:18080`。
- Renderer bridge 和状态页使用 `SidecarStatus`、`EdgeDesktopApi`、`edgeDesktop` 等边缘端语义。
- 状态页和设置页默认展示边缘端 sidecar、MQTT 网关、KIO 初始化、网关配置、sidecar 重启等现场能力。
- i18n 文案中有大量 `Edge`、`sidecar`、`127.0.0.1:18080`、`edge-backend.exe` 固定描述。

目标是保留一套业务前端页面，通过运行角色配置控制显示、能力和后端路由语义。`main-server/desktop` 当前副本只作为拆分过渡，不作为长期分叉目标。

## 目标运行模型

新增统一运行态配置：

```ts
type RuntimeRole = "edge" | "main_server";

type RuntimeFeatures = {
  sidecar: boolean;
  gatewayManage: boolean;
  kioManage: boolean;
  detectionControl: boolean;
  reportGeneration: boolean;
  lanWeb: boolean;
};
```

默认行为：

- `edge`：连接边缘端后端，Electron 可启动/监控 sidecar，显示 MQTT/KIO/网关配置和现场检测控制。
- `main_server`：连接主服务器后端，Electron 不启动 edge sidecar，GET 查询由主服务器本地同步 MySQL 提供，控制类请求由主服务器后端转发边缘端，报表生成/文件资产/重新生成归主服务器。

前端继续调用相同 API 路径，例如 `/api/v1/detection-runs`、`/api/v1/history/data`、`/api/v1/report-templates`。执行差异由当前连接的后端负责。

## Frontend-AI 实施顺序

1. 抽象环境配置
   - 在 `src/shared/config/env.ts` 新增中性 `apiBaseUrl`。
   - 兼容现有 `VITE_EDGE_API_BASE_URL`，新增 `VITE_MAIN_API_BASE_URL` 和 `VITE_APP_ROLE=edge|main_server`。
   - 保留旧 `edgeApiBaseUrl` 作为兼容别名，避免一次性修改所有调用点。
   - 可选：启动后读取 `/api/v1/main-server/status` 推导主服务器能力，但本阶段以 `VITE_APP_ROLE` 为准。

2. 抽象 Electron bridge
   - 将 renderer 侧 `SidecarStatus` 语义升级为 `BackendRuntimeStatus`，旧类型名可先保留为 alias。
   - Edge 模式 Electron main process 才启动 `edge-backend.exe`。
   - Main Server 模式 Electron main process 只检查 `MAIN_SERVER_URL` 或 `apiBaseUrl` 的 `/health`，不启动 edge sidecar。
   - `restartSidecar` 在 renderer 侧改为中性 `restartBackend`；旧函数名可保留兼容，但 main_server 模式只做刷新或调用主服务器后端提供的受控重启接口 <!-- 待确认 -->。

3. 状态页按角色显示
   - Edge 模式保持现有边缘端概览：sidecar、后端健康、MQTT 网关、实时变量、运行检测。
   - Main Server 模式显示：主服务器后端健康、本地同步 MySQL 状态、边缘控制目标、查询代理状态、报表服务状态。
   - 不在 Main Server 模式显示 `edge-backend.exe missing`、`127.0.0.1:18080` 或 sidecar watchdog 文案。

4. 设置页能力 gating
   - Edge 模式保留网关配置、KIO 初始化、sidecar 重启、数据库配置等现场维护入口。
   - Main Server 模式隐藏或只读化网关配置、KIO 初始化、sidecar 重启、边缘端数据库热切换等现场入口。
   - Main Server 模式如需触发检测控制、变量写入等现场动作，仍调用原 API，由主服务器后端负责转发、幂等和审计。
   - 报表模板管理、报表生成、文件资产、重新生成入口在 Main Server 模式启用；Edge 模式只保留报表请求登记和查看。

5. i18n 文案中性化
   - 将用户可见的固定 `Edge`、`sidecar`、`edge-backend.exe`、`127.0.0.1:18080` 文案改为按角色选择。
   - 推荐中性词：`后端服务`、`运行节点`、`数据源站点`、`控制目标`、`主服务器`、`边缘端`。
   - 三语资源必须同步更新中文、英文、日文。

6. 保持业务 API 不分叉
   - 页面层不新增 `if main_server then call another endpoint` 的业务分叉。
   - 如确需新增状态/能力接口，集中在 runtime config 或 backend status 查询中，不散落到各业务页面。

## 后端路由配合要求

- GET 查询类：主服务器后端优先查本地同步 MySQL。
- POST/PATCH/PUT/DELETE 控制类：主服务器后端不得直接改边缘业务表，应转发到边缘端受控 HTTP 命令通道，并保留审计。
- 报表类：主服务器本地处理模板、公式参数、图片资产、Excel/PDF 文件、预览和重新生成。
- 迁移期可用 `edge.query_proxy_enabled=true` 代理读请求，但关闭条件是主服务器 GET 查询路由完成本地 MySQL 移植。

## 验证计划

### 静态验证

```powershell
cd desktop
npm run build
npm run test -- --run
```

如 `npm run lint` 被既有规则阻断，需要记录阻断文件、规则和是否与本轮改动相关。

### Edge 模式 smoke

- `VITE_APP_ROLE=edge`
- API 指向 `http://127.0.0.1:18080`
- 状态页仍显示 sidecar、网关、实时变量和运行检测。
- 设置页仍可看到边缘端网关、KIO、sidecar 管理能力。
- 检测开始、停止、异常停止等现场控制仍走边缘端后端。

### Main Server 模式 smoke

- `VITE_APP_ROLE=main_server`
- API 指向 `http://127.0.0.1:19080`
- 页面不显示 `edge-backend.exe 缺失`、`sidecar 重启`、`127.0.0.1:18080 无响应` 等边缘端错误语义。
- 状态页显示主服务器后端、本地同步数据库、边缘控制目标和查询代理/报表服务状态。
- 设置页不允许直接执行边缘专属采集配置写操作，除非主服务器后端明确提供代理控制能力。

## 明确假设

- 不维护两套业务前端；`main-server/desktop` 是拆分过渡，不是长期分叉。
- 主服务器 Web 与 Electron 共用 renderer；Electron main process 可以按角色存在启动行为差异。
- 同一路径 API 由当前连接后端决定本地处理、查询本地 MySQL 或转发边缘端。
- 报表生成、文件资产、图片、PDF、重新生成 worker 最终归主服务器。
