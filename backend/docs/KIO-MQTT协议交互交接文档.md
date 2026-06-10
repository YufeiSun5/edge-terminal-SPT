# Edge 后端与 KIO MQTT 交互速查

> 以 `kio_client_id = S_KIO_Project`，`kio_writer = edge-test` 为例。

---

## 一句话总结

| 操作 | Topic | 方向 | 发什么 | 得到什么 |
|------|-------|------|--------|---------|
| 获取所有变量 | `Query_AllKIOTags_S_KIO_Project` | Edge → 网关 | 空消息 | 网关通过 `datachange_` topic 推回全量数据 |
| 获取实时数据 | `datachange_S_KIO_Project` | 网关 → Edge | （订阅等待） | 变量名+值+时间+质量码 |
| 下设实时数据 | `setdata_S_KIO_Project` | Edge → 网关 | Writer+Qid+变量名+值 | 网关往 `setdata_result_` topic 发回执 |
| 获取下设结果 | `setdata_result_S_KIO_Project_edge-test` | 网关 → Edge | （订阅等待） | Qid+ProcessStep+Result，按 Qid 匹配 |

---

## 一、Edge 订阅的 Topic（被动接收）

### 1. `datachange_S_KIO_Project` — 实时数据上报

**谁发的**：KIO 网关
**什么时候发**：变量值有变化时持续推送
**Edge 收到后做什么**：更新内存中的实时值快照，匹配检测任务规则，触发报警评估，运行中的检测任务会写入历史数据库

**消息格式**：
```json
{
  "Objs": [
    {"N": "supply_air_temp", "1": 25.3, "2": 150, "3": 192},
    {"N": "running_status",  "1": true,  "2": 150, "3": 192}
  ],
  "PVs": {"2": "2026-06-08 10:30:00.000 +0800"}
}
```

| 字段 | 含义 |
|------|------|
| `N` | 变量名 |
| `1` | 值 |
| `2` | 时间偏移（毫秒），基于 PVs.2 |
| `3` | 质量码，192=好 |

---

### 2. `setdata_result_S_KIO_Project_edge-test` — 下设结果回执

**谁发的**：KIO 网关
**什么时候发**：Edge 发了写指令之后，网关处理完毕回复
**Edge 收到后做什么**：按 Qid 匹配之前发出的写请求，返回执行结果

**消息格式**：
```json
{
  "Qid": 86400123001,
  "ProcessStep": 100,
  "Result": "OK",
  "Time": "2026-06-08 10:30:00.500 +0800"
}
```

| 字段 | 含义 |
|------|------|
| `Qid` | 和写指令里的 Qid 一一对应 |
| `ProcessStep` | 100 = 处理完成 |
| `Result` | "OK" = 成功，其他 = 失败 |

---

## 二、Edge 发布的 Topic（主动发送）

### 3. `Query_AllKIOTags_S_KIO_Project` — 获取所有变量

**什么时候发**：每次 MQTT 连接成功（含断线重连）时自动发一次
**发什么**：空消息（payload 为空）
**得到什么**：KIO 网关收到后，把当前所有变量的完整数据通过 `datachange_S_KIO_Project` 推回来，Edge 从中自动发现所有变量名并记录到数据库

---

### 4. `setdata_S_KIO_Project` — 下设实时数据

**什么时候发**：需要往 KIO 网关写入变量值时
**发什么**：

```json
{
  "Writer": "edge-test",
  "WriteTime": "2026-06-08 10:30:00.000 +0800",
  "Username": "sa",
  "Password": "C12E01F2A13FF5587E1E9E4AEDB8242D",
  "Qid": 86400123001,
  "Objs": [
    {"N": "SP", "1": 25.5}
  ]
}
```

| 字段 | 含义 |
|------|------|
| `Writer` | 写入者身份，对应配置里的 `kio_writer` |
| `Username`/`Password` | KIO 写入认证 |
| `Qid` | 唯一请求ID，用来匹配回执 |
| `Objs[].N` | 要写的变量名 |
| `Objs[].1` | 要写的值 |

**得到什么**：KIO 网关处理后，往 `setdata_result_S_KIO_Project_edge-test` 发一条回执，Qid 对应

---

## 三、Topic 命名规则

把 `S_KIO_Project` 换成你的 `kio_client_id`，`edge-test` 换成你的 `kio_writer`：

| 用途 | 规则 |
|------|------|
| 实时数据 | `datachange_{kio_client_id}` |
| 全量查询 | `Query_AllKIOTags_{kio_client_id}` |
| 下设数据 | `setdata_{kio_client_id}` |
| 下设回执 | `setdata_result_{kio_client_id}_{kio_writer}` |

这些值配在 `backend/configs/config.json` 的 `gateways[]` 里，也同步存在数据库 `sys_gateways` 表中。
