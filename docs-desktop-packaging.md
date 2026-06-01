# Desktop Packaging Direction

目标交付形态：

- Electron + React 桌面壳
- Go 后端编译为 `edge-backend.exe`
- 配置文件随包放在 `resources/configs/config.json`
- Electron 主进程启动 sidecar 后端，然后 React 通过 `http://127.0.0.1:18080` 调用

建议目录：

```text
edge-terminal/
  backend/
  desktop/
    electron/
    src/
    package.json
```

后续打包时，Electron Builder/Forge 需要把以下文件放入安装包资源目录：

- `backend/dist/edge-backend.exe`
- `backend/dist/configs/config.json`
- `backend/dist/schema.sql`

首版不把 Redis 做成强依赖。实时数据先由 Go 进程内 map 承担，只有出现多进程共享、跨服务订阅或大屏扇出需求时再接 Redis。

## Project/Variable Boundary

- MQTT 网关是采集站点，只负责连接、订阅和发现变量。
- 项目是业务虚拟分组，不是网关下级；旧 `device` 只作为过渡期兼容别名。
- 同一项目可以绑定来自多个 MQTT 站点的变量。
- 页面按项目扩容；检测任务也按项目启动/停止。
- 历史存储只在项目检测任务运行期间打开，实时显示不受影响。
