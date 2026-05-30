# Edge Terminal

边缘端项目初始化目录。

当前先落地 Go 后端，后续 Electron + React 可以把 `backend` 编译成随安装包携带的 sidecar exe，由 Electron 主进程启动并通过 `http://127.0.0.1:18080` 调用。

## 已初始化

- `backend/`: Go 后端
- `backend/configs/config.json`: 本地运行配置
- `backend/deploy/schema.sql`: MySQL 建库和示例测点
- `backend/docs/backend-architecture.md`: 后端链路说明

## 后端能力

- MQTT 多网关入口
- MQTT 变量发现：按 `gateway_id + source_path` 写入变量库
- 虚拟设备分组：变量可人工分配到设备，一个设备可包含多个 MQTT 站点来的变量
- 检测任务按设备启动：未开始检测时只更新实时值，不写历史
- Go 内存大 map 保存实时测点快照
- buffered channel 解耦 MQTT、逻辑处理和 MySQL 写入
- MySQL 批量历史入库
- `/health`
- `/api/v1/realtime/variables`
- `/api/v1/variables`
- `/api/v1/gateways`
- `/api/v1/runtime/channels`
- `/api/v1/detection-runs`
