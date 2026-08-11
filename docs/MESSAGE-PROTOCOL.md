# Tinode 消息协议与机制总结

## 0. WebSocket 传输层协议

### 连接建立

```
HTTP GET / → WebSocket Upgrade (gorilla/websocket)
  → readLoop()  +  writeLoop()  两个 goroutine
```

### 帧格式

所有数据通过 **WebSocket Text Message** 帧传输（`websocket.TextMessage`），payload 为 **JSON 字符串**。

唯一的例外：心跳探测时客户端发送单字节 `0x31`（字符 `'1'`），服务端回复单字节 `0x30`（字符 `'0'`）。

### 输入（客户端→服务端）

```
WebSocket Text Frame
  → raw []byte
  → json.Unmarshal → ClientComMessage
  → dispatch() → 路由到对应 handler
```

**队列限制**：`sendQueueLimit = 128`，队列满则丢弃并断开连接。

### 输出（服务端→客户端）

```
ServerComMessage
  → json.Marshal → []byte
  → 写入 sess.send channel (buffered, size=128)
  → writeLoop() select 消费
  → ws.WriteMessage(websocket.TextMessage, data)
```

**WriteLoop 调度**（优先级从高到低）：
1. `sess.send` — 消息队列（`*ServerComMessage` 或 `[]*ServerComMessage` 批量）
2. `sess.bkgTimer` — 后台超时定时器
3. `sess.stop` — 停止信号
4. `sess.detach` — 取消订阅
5. `ticker` — Ping 帧（每 `pongWait * 9/10` 发送一次）

### Keep-Alive

| 参数 | 值 | 说明 |
|------|-----|------|
| `writeWait` | 10s | 写超时 |
| `pongWait` | `idleSessionTimeout` | 期望收到 Pong 的时间（默认 180s） |
| `pingPeriod` | `pongWait * 9/10` | Ping 间隔（约 162s） |
| `pingPeriod` | `pongWait * 9/10` | Ping 间隔（约 162s） |
| 心跳探测 | `0x31` / `0x30` | 网络探针（单字节） |

### 缓冲区设置

| 参数 | 值 | 说明 |
|------|-----|------|
| ReadBufferSize | 1024 | 读缓冲区 |
| WriteBufferSize | 1024 | 写缓冲区 |
| Compression | 可配置 | RFC 7692.4 压缩 |
| CheckOrigin | `true` | 允许任何 Origin |

---

## 1. 消息传输协议

### 默认传输：WebSocket (WS)

```
客户端 ←→ WebSocket Server (:6060) ←→ Hub ←→ Topic ←→ Sessions
```

**备用传输**：
- Long Polling (HTTP) — 同一套消息结构，不同传输层
- gRPC — 内部服务间通信（`hdl_grpc.go`，端口 `:16060`）

### 消息格式

所有消息通过 **JSON** 序列化，在 WebSocket 中以文本帧传输。

### C2S（客户端→服务端）消息类型

| 消息 | 结构体 | 说明 |
|------|--------|------|
| `{hi}` | `MsgClientHi` | 握手，上报必填企业码及 UA/版本/设备ID/语言 |
| `{acc}` | `MsgClientAcc` | 创建/更新用户账号 |
| `{login}` | `MsgClientLogin` | 登录认证 |
| `{sub}` | `MsgClientSub` | 订阅 Topic（加入群/私聊） |
| `{leave}` | `MsgClientLeave` | 取消订阅 |
| `{pub}` | `MsgClientPub` | **发布消息**（发送聊天内容） |
| `{get}` | `MsgClientGet` | 查询 Topic 状态/历史消息 |
| `{set}` | `MsgClientSet` | 更新 Topic 元数据/订阅权限 |
| `{del}` | `MsgClientDel` | 删除消息/Topic/订阅 |
| `{note}` | `MsgClientNote` | 客户端通知（已读/已收/打字） |

### S2C（服务端→客户端）消息类型

| 消息 | 结构体 | 说明 |
|------|--------|------|
| `{ctrl}` | `MsgServerCtrl` | 控制响应（确认/错误，含 code/text） |
| `{data}` | `MsgServerData` | **消息数据**（聊天内容投递） |
| `{meta}` | `MsgServerMeta` | 元数据更新（描述/订阅/标签/凭证） |
| `{pres}` | `MsgServerPres` | **Presence 通知**（在线/离线/权限变更） |
| `{info}` | `MsgServerInfo` | 非权威通知（已收/已读/打字/通话） |

### 核心消息结构

**发布消息 (`{pub}`)**：
```json
{
  "pub": {
    "id": "client_msg_id",
    "topic": "grp_ABC123",
    "head": {"key": "value"},
    "content": {"txt": "Hello", "fmt": "..."}
  }
}
```

**服务端投递 (`{data}`)**：
```json
{
  "data": {
    "topic": "grp_ABC123",
    "from": "usr_XYZ",
    "ts": "2026-08-04T00:00:00Z",
    "seq": 12345,
    "head": {},
    "content": {"txt": "Hello"}
  }
}
```

**控制响应 (`{ctrl}`)**：
```json
{
  "ctrl": {
    "id": "client_msg_id",
    "code": 200,
    "text": "OK",
    "ts": "2026-08-04T00:00:00Z"
  }
}
```

---

## 2. 消息类型（按内容语义）

Tinode 的消息内容（`content` 字段）是 **Drafty** 格式 — 一种轻量级富文本协议。

### Drafty 消息类型

| 类型 | 说明 |
|------|------|
| `text` | 纯文本消息 |
| `image` | 图片（含缩略图、文件引用） |
| `video` | 视频 |
| `audio` | 音频 |
| `file` | 通用文件附件 |
| `contact` | 联系人卡片 |
| `location` | 位置信息 |
| `payment` | 支付信息 |
| `url` | URL 预览 |
| `exif` | 媒体元信息（EXIF） |

### 系统消息

| 类型 | 说明 |
|------|------|
| 通知 | 用户被加入/踢出/权限变更 |
| 邀请 | 用户被邀请加入群聊 |
| 标题变更 | Topic 标题修改 |

### Access Mode 权限位

消息操作受 8 个权限位控制（`store/types/types.go`）：

| 位 | 值 | 名称 | 说明 |
|----|-----|------|------|
| J | 1 | ModeJoin | 允许加入/订阅 |
| R | 2 | ModeRead | 接收广播消息 (`{data}`, `{info}`) |
| W | 4 | ModeWrite | 发送消息 (`{pub}`) |
| P | 8 | ModePres | 接收 Presence 通知 |
| A | 16 | ModeApprove | 批准/踢出成员 |
| S | 32 | ModeShare | 邀请新成员 |
| D | 64 | ModeDelete | 硬删除消息 |
| O | 128 | ModeOwner | 完全访问权 |

有效权限 = `modeWant & modeGiven`，两者取交集。

---

## 3. 消息状态

消息状态通过 **seqID / recvID / readID / delID** 四个序列号跟踪：

### 核心状态字段

| 字段 | 说明 | 存储位置 |
|------|------|----------|
| `seqId` | 消息全局递增 ID（Topic 级别） | `Topic.lastId` + DB |
| `recvID` | 客户端确认"已收到"的最大 seqId | `perUserData.recvID` |
| `readID` | 客户端确认"已读"的最大 seqId | `perUserData.readID` |
| `delID` | 最新删除操作的 ID | `perUserData.delID` |

### 状态流转

```
发送方 pub ─→ 服务端分配 seqId ─→ 写 DB ─→ broadcast to sessions
                                              │
                                              ▼
                                      客户端收到 {data}
                                              │
                                    发送 {note: "recv"} ─→ 更新 recvID
                                              │
                                    用户打开消息 → 发送 {note: "read"} ─→ 更新 readID
```

### {note} 通知

客户端通过 `{note}` 消息上报状态：

```json
{"note": {"topic": "grp_X", "what": "recv", "seq": 1234}}    // 已收到
{"note": {"topic": "grp_X", "what": "read", "seq": 1234}}    // 已读
{"note": {"topic": "grp_X", "what": "kp"}}                   // 打字中 (key pressed)
```

服务端收到后更新 `perUserData.recvID` / `readID`，并通过 `{pres}` 广播给其他订阅者。

### {pres} Presence 通知

```json
{
  "pres": {
    "topic": "grp_X",
    "what": "read|recv|on|off|msg|del|acs",
    "src": "usr_XYZ",
    "seq": 1234,
    "clear": 56
  }
}
```

| `what` 值 | 说明 |
|-----------|------|
| `on` / `off` | 用户上线/下线 |
| `msg` | 新消息到达（seqId） |
| `recv` | 用户已收到消息（seqId） |
| `read` | 用户已读消息（seqId） |
| `del` | 消息被删除（clear/delSeq） |
| `acs` | 权限变更 |

### Topic 描述中的状态

在 `{meta.desc}` 中携带 Topic 级别的状态：

```json
{
  "meta": {
    "desc": {
      "seq": 12345,       // 最大消息 ID
      "read": 12300,      // 当前用户的已读 ID
      "recv": 12340,      // 当前用户的已收 ID
      "clear": 56         // 当前用户的删除 ID
    }
  }
}
```

---

## 4. 消息加密

### 传输层加密

| 层级 | 技术 | 状态 |
|------|------|------|
| WebSocket | **TLS (WSS)** — 通过配置 `tls_enabled: true` + 证书 | ✅ 支持 |
| gRPC | **TLS** — 可选配置 | ✅ 支持 |
| HTTP Long Polling | **TLS (HTTPS)** | ✅ 支持 |

**当前默认**：明文 WebSocket（无 TLS），可通过配置开启 WSS。

### 端到端加密 (E2E)

**Tinode 当前不支持端到端加密。**

- 消息在服务端以明文存储（DB 中 `messages.content` 字段为 JSON 明文）
- 消息广播时也是明文 JSON
- 服务端可以读取和搜索所有消息内容（`im_message_index.search_text` 全文索引）

### 认证加密

- 登录认证通过 `{login}` + `secret` 字段传输，secret 在服务端做哈希存储
- WebSocket 握手后通过 `{hi}` + `{login}` 建立身份

---

## 5. 同步规则

### 5.1 订阅同步

```
客户端 {sub, topic: "grp_X", get: {what: "desc+data+sub"}}
  → 服务端查找/创建 Topic
  → 注册 Session 到 Topic.sessions
  → 返回 {meta}（desc + sub + 历史 data）
  → 后续新消息通过 {data} 实时推送
```

### 5.2 消息同步

**拉取历史**：
```
客户端 {get, topic: "grp_X", get: {what: "data", data: {since: 100, before: 200, limit: 50}}}
  → 服务端从 DB 查询 messages
  → 返回 {data} 消息列表
```

**实时推送**：
```
发送方 {pub} → 服务端写 DB (seqId++) → 广播到所有订阅 Sessions
```

**广播机制**（Redis 改造后）：
```
Pub → DB Save → Redis Publish(topic:{name}:broadcast, msg)
  → 所有节点收到 Pub/Sub
  → 每个节点只遍历本地 attached sessions 投递
```

### 5.3 消息去重

- 客户端发送消息时带 `id` 字段（客户端生成的消息 ID）
- 服务端在 `{ctrl}` 响应中原样返回 `id`，客户端据此匹配请求/响应
- 服务端内部用 `seqId` 作为唯一递增 ID

### 5.4 离线消息

- 用户断线重连后，通过 `{get}` 请求 `since` 参数拉取缺失消息
- `since` = 本地最大 `seqId`，服务端返回 `seqId > since` 的所有消息
- 支持 `ranges` 参数批量请求多个区间

### 5.5 已读/已收同步

1. 客户端收到 `{data}` 后发送 `{note: "recv"}` 确认
2. 用户查看消息后发送 `{note: "read"}` 确认
3. 服务端更新 `perUserData.recvID/readID`
4. 通过 `{pres}` 广播给其他订阅者（"what": "recv" / "read"）
5. 私聊场景下也会推送到对方 `me` topic

### 5.6 删除同步

```
客户端 {del, topic: "grp_X", what: "msg", delseq: [{low: 100, hi: 105}]}
  → 服务端软删除消息（标记 deletedAt）
  → 更新 delID
  → {pres} 广播 "what": "del" 给所有订阅者
```

### 5.7 同步时序图

```
Client A                  Server                  Client B
   │                        │                        │
   │──── {sub, get} ───────→│                        │
   │←─── {meta, data} ─────│                        │
   │                        │                        │
   │──── {pub} ────────────→│                        │
   │                        │← DB save, seqId++      │
   │←─── {ctrl, code:200} ─│                        │
   │                        │──── {pres, msg} ──────→│
   │                        │──── {data, seq:N} ────→│
   │                        │                        │
   │                        │←── {note: "recv"} ─────│
   │←── {pres: "recv"} ────│                        │
   │                        │←── {note: "read"} ─────│
   │←── {pres: "read"} ────│                        │
```
