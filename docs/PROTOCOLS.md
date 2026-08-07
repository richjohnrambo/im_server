# Tinode 协议详解

> 本文档详细介绍 Tinode 支持的所有通信协议，包括协议原理、消息格式、实现细节和使用方法。

---

## 目录

- [协议概述](#协议概述)
- [WebSocket 协议](#websocket-协议)
- [gRPC 协议](#grpc-协议)
- [长轮询协议](#长轮询协议)
- [HTTP 文件传输协议](#http-文件传输协议)
- [消息格式](#消息格式)
- [协议对比](#协议对比)

---

## 协议概述

Tinode 支持四种主要的通信协议：

| 协议 | 端点 | 用途 | 特点 |
|------|------|------|------|
| **WebSocket** | `/v0/channels` | 实时消息推送 | 双向通信、低延迟 |
| **gRPC** | `grpc_listen` 端口 | 高性能客户端通信 | 强类型、流式传输 |
| **长轮询** | `/v0/channels/lp` | 兼容性客户端 | HTTP 兼容、防火墙友好 |
| **HTTP** | `/v0/file/*` | 文件上传下载 | RESTful、大文件支持 |

### 协议架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              客户端层                                        │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐               │
│  │ Web App   │  │  Android  │  │    iOS    │  │  CLI/Bot  │               │
│  │ (WS/LP)   │  │ (WS/gRPC) │  │ (WS/gRPC) │  │  (gRPC)   │               │
│  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘               │
└────────┼──────────────┼──────────────┼──────────────┼──────────────────────┘
         │              │              │              │
         ▼              ▼              ▼              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              协议层                                          │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐               │
│  │ WebSocket │  │   gRPC    │  │Long Polling│  │   HTTP    │               │
│  │JSON/Text │  │ Protobuf  │  │ JSON/Text │  │Multipart │               │
│  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘               │
└────────┼──────────────┼──────────────┼──────────────┼──────────────────────┘
         │              │              │              │
         ▼              ▼              ▼              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              会话层                                          │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                           Session Store                                 │ │
│  │                    (统一会话管理，协议无关)                              │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              业务层                                          │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐               │
│  │    Hub    │  │   Topic   │  │   User    │  │   Auth    │               │
│  │ 连接中心  │  │  消息路由 │  │  用户管理 │  │  认证模块 │               │
│  └───────────┘  └───────────┘  └───────────┘  └───────────┘               │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## WebSocket 协议

### 概述

WebSocket 是 Tinode 主要的实时通信协议，用于 Web 应用和移动应用的实时消息推送。

WebSocket 是一种在单个 TCP 连接上进行全双工通信的协议，它通过 HTTP 升级握手建立连接，然后切换到 WebSocket 协议进行双向数据传输。

### 端点

```
ws://host:port/v0/channels
wss://host:port/v0/channels  (TLS)
```

---

### 协议层交互详解

#### 1. 握手阶段（HTTP Upgrade）

WebSocket 连接通过 HTTP Upgrade 机制建立：

```
客户端请求：
GET /v0/channels?apikey=xxx HTTP/1.1
Host: server.example.com
Upgrade: websocket                    ← 请求升级协议
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZQ==   ← 随机 Base64 值
Sec-WebSocket-Version: 13             ← 协议版本
Origin: https://example.com

服务器响应：
HTTP/1.1 101 Switching Protocols      ← 同意切换
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ... ← 根据Key计算出的验证值
```

**验证值计算**：

```
Sec-WebSocket-Accept = base64(sha1(Key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
```

#### 2. 数据帧格式

WebSocket 数据帧格式（RFC 6455）：

```
  0                   1                   2                   3
  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
 +-+-+-+-+-------+-+-------------+-------------------------------+
 |F|R|R|R| opcode|M| Payload len |    Extended payload length    |
 |I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
 |N|V|V|V|       |S|             |   (if payload len==126/127)   |
 |1|2|3|4|       |K|             |                               |
 +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
 |     Extended payload length continued...                       |
 +-------------------------------+-------------------------------+
 |                               |Masking-key (if MASK set)      |
 +-------------------------------+-------------------------------+
 | Masking-key (continued)       |          Payload Data         |
 +-------------------------------- - - - - - - - - - - - - - - - +
```

**关键字段**：

| 字段 | 位数 | 说明 |
|------|------|------|
| FIN | 1 | 是否为最后一帧（消息可能分多帧） |
| RSV1-3 | 3 | 保留位（用于扩展） |
| opcode | 4 | 帧类型 |
| MASK | 1 | 是否掩码（客户端必须掩码） |
| Payload len | 7+ | 数据长度 |

**Opcode 类型**：

| Opcode | 类型 | 说明 |
|--------|------|------|
| 0x0 | Continuation | 续帧（分片消息的后续部分） |
| 0x1 | Text | 文本帧（Tinode 使用此类型传输 JSON） |
| 0x2 | Binary | 二进制帧 |
| 0x8 | Close | 连接关闭 |
| 0x9 | Ping | 心跳请求 |
| 0xA | Pong | 心跳响应 |

#### 3. 心跳机制

WebSocket 协议层的 Ping/Pong 心跳：

```
客户端                                     服务器
    │                                         │
    │────── Ping 帧 (opcode=0x9) ────────────>│
    │                                         │
    │<───── Pong 帧 (opcode=0xA) ─────────────│
    │                                         │
```

Tinode 的心跳配置：

```go
const (
    writeWait   = 10 * time.Second              // 写操作超时
    pongWait    = idleSessionTimeout             // 等待 Pong 超时（默认 60 秒）
    pingPeriod  = (pongWait * 9) / 10           // 发送 Ping 周期（54 秒）
)
```

**心跳流程**：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           WebSocket 心跳流程                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  服务器 writeLoop:                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ for {                                                               │   │
│  │   select {                                                          │   │
│  │     case <-ticker.C:        // 每 54 秒                             │   │
│  │       ws.WriteMessage(PingMessage, nil)  // 发送 Ping               │   │
│  │     case msg <- sess.send:  // 有消息待发送                          │   │
│  │       ws.WriteMessage(TextMessage, msg)   // 发送消息                │   │
│  │   }                                                                 │   │
│  │ }                                                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  服务器 readLoop:                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ws.SetReadDeadline(time.Now().Add(pongWait))  // 设置读取超时        │   │
│  │                                                                     │   │
│  │ ws.SetPongHandler(func(string) error {      // 收到 Pong 时          │   │
│  │   ws.SetReadDeadline(time.Now().Add(pongWait))  // 重置超时          │   │
│  │   return nil                                                        │   │
│  │ })                                                                  │   │
│  │                                                                     │   │
│  │ for {                                                               │   │
│  │   _, raw, err := ws.ReadMessage()  // 读取消息                      │   │
│  │   sess.dispatchRaw(raw)            // 处理消息                      │   │
│  │ }                                                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 4. 消息分片

大消息会被分成多帧传输：

```
帧1: FIN=0, opcode=0x1, payload="Hello "
帧2: FIN=0, opcode=0x0 (continuation), payload="World "
帧3: FIN=1, opcode=0x0 (continuation), payload="!"
```

接收方根据 FIN 位判断消息是否完整。

#### 5. 连接关闭

正常关闭流程：

```
客户端                                     服务器
    │                                         │
    │────── Close 帧 (opcode=0x8) ───────────>│
    │      payload: 1000 (正常关闭)            │
    │                                         │
    │<───── Close 帧 (opcode=0x8) ────────────│
    │      payload: 1000                       │
    │                                         │
    │====== TCP 连接断开 =====================│
```

**状态码**：

| 状态码 | 说明 |
|--------|------|
| 1000 | 正常关闭 |
| 1001 | 端点离开 |
| 1002 | 协议错误 |
| 1003 | 不支持的数据类型 |
| 1008 | 策略违规 |
| 1011 | 服务器错误 |

---

### WebSocket vs 应用层

Tinode 在 WebSocket 之上有一套应用层协议：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           协议层次结构                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    应用层 (Tinode 协议)                              │   │
│  │                                                                     │   │
│  │  消息类型: {hi}, {login}, {sub}, {pub}, {get}, {set}, {del}...     │   │
│  │  数据格式: JSON 文本                                                 │   │
│  │  业务逻辑: 认证、订阅、消息路由、权限控制                             │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    WebSocket 协议层                                  │   │
│  │                                                                     │   │
│  │  帧类型: Text (0x1), Binary (0x2), Ping (0x9), Pong (0xA)          │   │
│  │  传输: 建立连接、心跳保活、分片传输、关闭连接                         │   │
│  │  特性: 掩码、压缩扩展                                                │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    TCP 传输层                                        │   │
│  │                                                                     │   │
│  │  可靠传输、流控制、拥塞控制                                          │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**示例：发送一条聊天消息**

```
应用层 JSON:
{"pub": {"id": "msg123", "topic": "grpTechGroup", "content": "Hello!"}}

        │
        ▼
WebSocket 帧:
┌────────────────────────────────────────────────┐
│ FIN=1 | opcode=0x1 | MASK=1 | payload='...'   │
│ payload = {"pub": {"id": "msg123", ...}}       │
└────────────────────────────────────────────────┘

        │
        ▼
TCP 数据包:
[WebSocket 握手 + TLS 加密 + TCP 头 + IP 头]
```

---

### 消息流转完整流程

#### 前端发送消息

```javascript
// 1. 构造 JSON 消息
const message = {
  pub: {
    id: "msg123",
    topic: "grpTechGroup",
    content: "Hello World"
  }
};

// 2. 通过 WebSocket 发送
websocket.send(JSON.stringify(message));
```

#### 后端接收处理流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              消息流转全景图                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ① WebSocket 帧到达                                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ TCP → WebSocket 帧 → JSON 文本                                       │   │
│  │ opcode=0x1, payload='{"pub":{"id":"msg123",...}}'                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ② hdl_websock.go: serveWebSocket()                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ • HTTP Upgrade 握手                                                  │   │
│  │ • 创建 Session                                                       │   │
│  │ • 启动 readLoop() + writeLoop()                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ③ readLoop() 读取消息                                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ _, raw, err := sess.ws.ReadMessage()                                 │   │
│  │ // raw = []byte('{"pub":{"id":"msg123",...}}')                      │   │
│  │ sess.dispatchRaw(raw)                                                │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ④ dispatchRaw() 解析 JSON                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ var msg ClientComMessage                                             │   │
│  │ json.Unmarshal(raw, &msg)                                            │   │
│  │ // msg.Pub = {Id: "msg123", Topic: "grpTechGroup", ...}             │   │
│  │ sess.dispatch(&msg)                                                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ⑤ dispatch() 路由到处理器                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ switch {                                                             │   │
│  │   case msg.Pub != nil:                                               │   │
│  │     handler = checkVers(checkUser(s.publish))                        │   │
│  │     msg.Id = msg.Pub.Id                                              │   │
│  │     msg.Original = msg.Pub.Topic                                     │   │
│  │ }                                                                    │   │
│  │ handler(msg)                                                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ⑥ publish() 业务处理                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ • 验证用户权限 (W)                                                    │   │
│  │ • 获取 Topic: hub.topicGet("grpTechGroup")                           │   │
│  │ • 发送到 Topic 消息通道: topic.clientMsg <- msg                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ⑦ Topic 处理并广播                                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ • 生成消息 ID (lastID++)                                              │   │
│  │ • 存储到数据库                                                        │   │
│  │ • 遍历所有订阅者:                                                     │   │
│  │   for session := range topic.sessions {                              │   │
│  │     session.send <- ServerComMessage{Data: {...}}                    │   │
│  │   }                                                                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ⑧ writeLoop() 发送给客户端                                                 │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ case msg := <-sess.send:                                             │   │
│  │   bits := json.Marshal(msg)                                          │   │
│  │   ws.WriteMessage(TextMessage, bits)                                 │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### 连接流程

```
┌─────────┐                    ┌─────────┐
│ Client  │                    │ Server  │
└────┬────┘                    └────┬────┘
     │                              │
     │──── HTTP GET /v0/channels ──>│  WebSocket 握手
     │<─── HTTP 101 Switching ─────│
     │                              │
     │<─────── {ctrl} ──────────────│  服务器响应 (含 sid, ver)
     │                              │
     │─────── {hi} ────────────────>│  客户端握手
     │<─────── {ctrl} ──────────────│  握手确认
     │                              │
     │─────── {login} ─────────────>│  用户认证
     │<─────── {ctrl} ──────────────│  认证结果
     │                              │
     │─────── {sub} ───────────────>│  订阅话题
     │<─────── {ctrl} ──────────────│
     │<─────── {meta} ──────────────│  话题元数据
     │                              │
     │─────── {pub} ───────────────>│  发送消息
     │<─────── {ctrl} ──────────────│
     │<─────── {data} ──────────────│  消息推送
     │                              │
```

### 核心实现

**文件**: `server/hdl_websock.go`

#### 1. 连接升级

```go
// WebSocket 升级器配置
var upgrader = websocket.Upgrader{
    ReadBufferSize:    1024,
    WriteBufferSize:   1024,
    EnableCompression: globals.wsCompression,
    // 允许来自任意 Origin 的连接
    CheckOrigin: func(r *http.Request) bool { return true },
}

func serveWebSocket(wrt http.ResponseWriter, req *http.Request) {
    // 1. 验证 API Key
    if isValid, _ := checkAPIKey(getAPIKey(req)); !isValid {
        wrt.WriteHeader(http.StatusForbidden)
        return
    }

    // 2. 升级 HTTP 连接为 WebSocket
    ws, err := upgrader.Upgrade(wrt, req, nil)

    // 3. 创建新会话
    sess, count := globals.sessionStore.NewSession(ws, "")

    // 4. 启动读写循环
    go sess.writeLoop()  // 发送消息
    go sess.readLoop()   // 接收消息
}
```

#### 2. 读循环（接收消息）

```go
func (sess *Session) readLoop() {
    defer func() {
        sess.closeWS()
        sess.cleanUp(false)
    }()

    // 设置读取限制和超时
    sess.ws.SetReadLimit(globals.maxMessageSize)
    sess.ws.SetReadDeadline(time.Now().Add(pongWait))

    // Pong 处理器：收到 Pong 后重置超时
    sess.ws.SetPongHandler(func(string) error {
        sess.ws.SetReadDeadline(time.Now().Add(pongWait))
        return nil
    })

    for {
        // 读取客户端消息
        _, raw, err := sess.ws.ReadMessage()
        if err != nil {
            return
        }

        // 统计
        statsInc("IncomingMessagesWebsockTotal", 1)

        // 分发消息到业务层
        sess.dispatchRaw(raw)
    }
}
```

#### 3. 写循环（发送消息）

```go
func (sess *Session) writeLoop() {
    ticker := time.NewTicker(pingPeriod)  // 定时发送 Ping
    defer ticker.Stop()

    for {
        select {
        case msg := <-sess.send:
            // 发送消息到客户端
            if !sess.sendMessage(msg) {
                return
            }

        case <-ticker.C:
            // 发送 Ping 保持连接
            if err := wsWrite(sess.ws, websocket.PingMessage, nil); err != nil {
                return
            }

        case msg := <-sess.stop:
            // 关闭会话
            if msg != nil {
                wsWrite(sess.ws, websocket.TextMessage, msg)
            }
            return

        case topic := <-sess.detach:
            // 从话题分离
            sess.delSub(topic)
        }
    }
}
```

### 心跳机制

| 参数 | 值 | 说明 |
|------|-----|------|
| `writeWait` | 10秒 | 写操作超时 |
| `pongWait` | 配置项 | 等待 Pong 超时（默认 60 秒） |
| `pingPeriod` | `pongWait * 0.9` | 发送 Ping 周期 |

### 消息格式

WebSocket 消息使用 **JSON 文本格式**：

```json
{
  "sub": {
    "id": "unique-id",
    "topic": "me",
    "get": {
      "what": "desc sub data"
    }
  }
}
```

### 配置项

| 配置项 | 说明 |
|--------|------|
| `listen` | WebSocket 监听地址 |
| `ws_compression` | 是否启用压缩 |
| `max_message_size` | 最大消息大小 |

---

## gRPC 协议

### 概述

gRPC 提供高性能、强类型的客户端通信，适合移动应用、CLI 工具和插件系统。

### 端点

```
host:grpc_listen_port
```

默认端口：`16060`

### 服务定义

**文件**: `pbx/model.proto`

#### Node 服务（客户端实现）

```protobuf
service Node {
    // 消息循环：双向流
    rpc MessageLoop(stream ClientMsg) returns (stream ServerMsg) {}

    // 大文件上传：客户端流
    rpc LargeFileReceive(stream FileUpReq) returns (FileUpResp) {}

    // 大文件下载：服务端流
    rpc LargeFileServe(FileDownReq) returns (stream FileDownResp) {}
}
```

#### Plugin 服务（插件实现）

```protobuf
service Plugin {
    // 消息管道：处理所有客户端消息
    rpc FireHose(ClientReq) returns (ServerResp) {}

    // 用户发现
    rpc Find(SearchQuery) returns (SearchFound) {}

    // 事件通知
    rpc Account(AccountEvent) returns (Unused) {}
    rpc Topic(TopicEvent) returns (Unused) {}
    rpc Subscription(SubscriptionEvent) returns (Unused) {}
    rpc Message(MessageEvent) returns (Unused) {}
}
```

### 核心实现

**文件**: `server/hdl_grpc.go`

#### 1. 消息循环

```go
func (*grpcNodeServer) MessageLoop(stream pbx.Node_MessageLoopServer) error {
    // 1. 创建新会话
    sess, count := globals.sessionStore.NewSession(stream, "")
    if p, ok := peer.FromContext(stream.Context()); ok {
        sess.remoteAddr = p.Addr.String()
    }

    defer func() {
        sess.closeGrpc()
        sess.cleanUp(false)
    }()

    // 2. 启动写循环
    go sess.writeGrpcLoop()

    // 3. 读循环
    for {
        in, err := stream.Recv()
        if err == io.EOF {
            return nil
        }
        if err != nil {
            return err
        }

        // 统计
        statsInc("IncomingMessagesGrpcTotal", 1)

        // 分发消息
        sess.dispatch(pbCliDeserialize(in))
    }
}
```

#### 2. 服务器启动

```go
func serveGrpc(addr string, kaEnabled bool, tlsConf *tls.Config) (*grpc.Server, error) {
    lis, err := netListener(addr)

    var opts []grpc.ServerOption
    opts = append(opts, grpc.MaxRecvMsgSize(int(globals.maxMessageSize)))

    // TLS 配置
    if tlsConf != nil {
        opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConf)))
    }

    // Keepalive 配置
    if kaEnabled {
        kepConfig := keepalive.EnforcementPolicy{
            MinTime:             1 * time.Second,
            PermitWithoutStream: true,
        }
        opts = append(opts, grpc.KeepaliveEnforcementPolicy(kepConfig))

        kpConfig := keepalive.ServerParameters{
            Time:    60 * time.Second,
            Timeout: 20 * time.Second,
        }
        opts = append(opts, grpc.KeepaliveParams(kpConfig))
    }

    srv := grpc.NewServer(opts...)
    pbx.RegisterNodeServer(srv, &grpcNodeServer{})

    go srv.Serve(lis)
    return srv, nil
}
```

### 消息类型

#### 客户端消息 (ClientMsg)

```protobuf
message ClientMsg {
    oneof Message {
        ClientHi hi = 1;         // 握手
        ClientAcc acc = 2;       // 账户
        ClientLogin login = 3;   // 登录
        ClientSub sub = 4;       // 订阅
        ClientLeave leave = 5;   // 离开
        ClientPub pub = 6;       // 发布
        ClientGet get = 7;       // 查询
        ClientSet set = 8;       // 更新
        ClientDel del = 9;       // 删除
        ClientNote note = 10;    // 通知
    }
    ClientExtra extra = 13;      // 额外参数
}
```

#### 服务器消息 (ServerMsg)

```protobuf
message ServerMsg {
    oneof Message {
        ServerCtrl ctrl = 1;     // 控制响应
        ServerData data = 2;     // 数据消息
        ServerPres pres = 3;     // 在线状态
        ServerMeta meta = 4;     // 元数据
        ServerInfo info = 5;     // 信息通知
    }
}
```

### 文件传输

#### 上传（客户端流）

```protobuf
message FileUpReq {
    string id = 1;              // 请求 ID
    Auth auth = 2;              // 认证信息
    string topic = 3;           // 所属话题
    FileMeta meta = 4;          // 文件元数据
    bytes content = 5;          // 文件内容（分块）
}

message FileUpResp {
    string id = 1;
    int32 code = 2;             // 状态码
    string text = 3;
    FileMeta meta = 4;
    string redir_url = 5;       // 重定向 URL
}
```

#### 下载（服务端流）

```protobuf
message FileDownReq {
    string id = 1;
    Auth auth = 2;
    string uri = 3;             // 文件 URI
    string if_modified = 4;     // ETag
}

message FileDownResp {
    string id = 1;
    int32 code = 2;
    string text = 3;
    FileMeta meta = 4;
    string redir_url = 5;
    bytes content = 6;          // 文件内容（分块）
}
```

### Keepalive 配置

| 参数 | 值 | 说明 |
|------|-----|------|
| `MinTime` | 1秒 | 客户端最小 Ping 间隔 |
| `PermitWithoutStream` | true | 允许无活动流时 Ping |
| `Time` | 60秒 | 服务端 Ping 间隔 |
| `Timeout` | 20秒 | Ping 超时时间 |

### 特殊功能

gRPC 协议支持 WebSocket 不支持的功能：

| 功能 | 说明 |
|------|------|
| **代理发送** | root 用户可代表其他用户发送消息 |
| **用户删除** | 完整删除用户账户 |
| **文件流式传输** | 高效的大文件传输 |

---

## 长轮询协议

### 概述

长轮询是 WebSocket 的兼容替代方案，适用于不支持 WebSocket 的环境（如老旧浏览器、严格防火墙）。

### 端点

```
POST/GET http://host:port/v0/channels/lp
```

### 工作原理

```
┌─────────┐                    ┌─────────┐
│ Client  │                    │ Server  │
└────┬────┘                    └────┬────┘
     │                              │
     │── POST /v0/channels/lp ────>│  创建会话
     │<── 201 Created + sid ───────│
     │                              │
     │── POST /v0/channels/lp ────>│  登录 (sid + {login})
     │<── {ctrl} ──────────────────│
     │                              │
     │── GET /v0/channels/lp?sid=X │  长轮询（等待消息）
     │                              │  (阻塞等待)
     │<── {data} ──────────────────│  有消息时返回
     │                              │
     │── GET /v0/channels/lp?sid=X │  继续轮询
     │<── (超时返回空) ─────────────│
     │                              │
```

### 核心实现

**文件**: `server/hdl_longpoll.go`

#### 1. 会话创建

```go
func serveLongPoll(wrt http.ResponseWriter, req *http.Request) {
    // 获取会话 ID
    sid := req.FormValue("sid")

    if sid == "" {
        // 创建新会话
        sess, count := globals.sessionStore.NewSession(wrt, "")
        sess.remoteAddr = getRemoteAddr(req)

        // 返回会话 ID
        wrt.WriteHeader(http.StatusCreated)
        pkt := NoErrCreated(req.FormValue("id"), "", now)
        pkt.Ctrl.Params = map[string]string{"sid": sess.sid}
        enc.Encode(pkt)
        return
    }

    // 获取已有会话
    sess = globals.sessionStore.Get(sid)
    if sess == nil {
        wrt.WriteHeader(http.StatusForbidden)
        enc.Encode(ErrSessionNotFound(now))
        return
    }

    // 处理请求
    if req.ContentLength != 0 {
        // 有消息体：读取并处理
        sess.readOnce(wrt, req)
        return
    }

    // 无消息体：长轮询等待
    sess.writeOnce(wrt, req)
}
```

#### 2. 消息读取

```go
func (sess *Session) readOnce(wrt http.ResponseWriter, req *http.Request) (int, error) {
    // 检查消息大小
    if req.ContentLength > globals.maxMessageSize {
        return http.StatusExpectationFailed, errors.New("request too large")
    }

    // 读取消息体
    raw, err := io.ReadAll(req.Body)
    if err == nil {
        sess.lock.Lock()
        statsInc("IncomingMessagesLongpollTotal", 1)
        sess.dispatchRaw(raw)
        sess.lock.Unlock()
        return 0, nil
    }
    return 0, err
}
```

#### 3. 长轮询等待

```go
func (sess *Session) writeOnce(wrt http.ResponseWriter, req *http.Request) {
    for {
        select {
        case msg := <-sess.send:
            // 有消息待发送
            sess.sendMessageLp(wrt, msg)
            return

        case <-time.After(pingPeriod):
            // 超时：返回空包
            wrt.Write([]byte{})
            return

        case msg := <-sess.stop:
            // 会话关闭
            globals.sessionStore.Delete(sess)
            if msg != nil {
                lpWrite(wrt, msg)
            }
            return

        case <-req.Context().Done():
            // HTTP 请求取消
            return
        }
    }
}
```

### HTTP 头设置

```go
// CORS 支持
wrt.Header().Set("Access-Control-Allow-Origin", "*")

// 禁用缓存
wrt.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
wrt.Header().Set("Pragma", "no-cache")
wrt.Header().Set("Expires", "0")

// 内容类型
wrt.Header().Set("Content-Type", "text/plain")
```

### 请求参数

| 参数 | 方法 | 说明 |
|------|------|------|
| `sid` | GET/POST | 会话 ID（创建后必需） |
| `id` | GET/POST | 客户端消息 ID |
| `apikey` | GET/POST/Header | API Key |

### 消息格式

与 WebSocket 相同，使用 JSON 文本格式。

---

## HTTP 文件传输协议

### 概述

HTTP 文件传输协议用于大文件的带外上传和下载，独立于主消息通道。

### 端点

| 端点 | 方法 | 用途 |
|------|------|------|
| `/v0/file/u` | POST | 文件上传 |
| `/v0/file/s/[id]` | GET | 文件下载 |

### 核心实现

**文件**: `server/hdl_files.go`

#### 1. 文件上传

```go
func largeFileReceiveHTTP(wrt http.ResponseWriter, req *http.Request) {
    // 1. 验证 API Key
    if isValid, _ := checkAPIKey(getAPIKey(req)); !isValid {
        writeHttpResponse(ErrAPIKeyRequired(now), nil)
        return
    }

    // 2. 认证检查
    authMethod, secret := getHttpAuth(req)
    uid, challenge, err := authFileRequest(authMethod, secret, req.FormValue("sid"), getRemoteAddr(req))

    // 3. 读取文件
    file, header, err := req.FormFile("file")

    // 4. 检测 MIME 类型
    buff := make([]byte, 512)
    file.Read(buff)
    mimeType := http.DetectContentType(buff)

    // 5. 创建文件定义
    fdef := &types.FileDef{
        Id:       store.Store.GetUidString(),
        User:     uid.String(),
        MimeType: mimeType,
    }

    // 6. 调用媒体处理器上传
    url, size, err := mh.Upload(fdef, file)

    // 7. 完成上传
    fdef, err = store.Files.FinishUpload(fdef, true, size)

    // 8. 返回结果
    params := map[string]string{"url": url}
    writeHttpResponse(NoErrParams(msgID, "", now, params), nil)
}
```

#### 2. 文件下载

```go
func largeFileServeHTTP(wrt http.ResponseWriter, req *http.Request) {
    // 1. 验证 API Key
    if isValid, _ := checkAPIKey(getAPIKey(req)); !isValid {
        writeHttpResponse(ErrAPIKeyRequired(now), err)
        return
    }

    // 2. 认证检查
    authMethod, secret := getHttpAuth(req)
    uid, challenge, err := authFileRequest(authMethod, secret, req.FormValue("sid"), getRemoteAddr(req))

    // 3. 调用媒体处理器下载
    fd, rsc, err := mh.Download(req.URL.String())
    defer rsc.Close()

    // 4. 设置响应头
    wrt.Header().Set("Content-Type", fd.MimeType)
    if asAttachment {
        wrt.Header().Set("Content-Disposition", "attachment")
    }

    // 5. 流式传输文件
    http.ServeContent(wrt, req, "", fd.UpdatedAt, rsc)
}
```

#### 3. gRPC 文件传输

```go
// 文件上传（gRPC）
func (*grpcNodeServer) LargeFileReceive(stream pbx.Node_LargeFileReceiveServer) error {
    // 接收第一个请求（包含元数据）
    req, err := stream.Recv()

    // 认证
    authMethod, secret := req.Auth.Scheme, req.Auth.Secret
    uid, challenge, err := authFileRequest(authMethod, secret, "", remoteAddr)

    // 创建管道用于流式读取
    reader, writer := io.Pipe()

    // 后台协程读取分块数据
    go func() {
        defer writer.Close()
        for {
            if req, err := stream.Recv(); err == nil {
                chunk := req.GetContent()
                writer.Write(chunk)
            }
        }
    }()

    // 上传到媒体处理器
    url, size, err := mh.Upload(fdef, reader)

    // 关闭并返回
    stream.SendAndClose(&pbx.FileUpResp{...})
}

// 文件下载（gRPC）
func (*grpcNodeServer) LargeFileServe(req *pbx.FileDownReq, stream pbx.Node_LargeFileServeServer) error {
    // 认证
    uid, challenge, err := authFileRequest(...)

    // 下载文件
    fd, rsc, err := mh.Download(req.GetUri())
    defer rsc.Close()

    // 分块发送
    resp.Content = make([]byte, 1024*1024*2)  // 2MB 缓冲
    for {
        n, err := rsc.Read(resp.Content)
        if err == io.EOF {
            break
        }
        resp.Content = resp.Content[:n]
        stream.Send(&resp)
    }
}
```

### 认证方式

HTTP 文件传输支持多种认证方式（按优先级）：

| 优先级 | 方式 | 格式 |
|--------|------|------|
| 1 | HTTP 头 | `Authorization: Basic base64(scheme:secret)` |
| 2 | URL 参数 | `?auth=basic&secret=xxx` |
| 3 | 表单值 | `auth=basic&secret=xxx` |
| 4 | Cookie | `auth=basic; secret=xxx` |
| 5 | 会话 ID | `sid=xxx`（已认证会话） |

### MIME 类型限制

允许的 MIME 类型前缀：

```go
var allowedMimeTypes = []string{
    "application/",
    "audio/",
    "font/",
    "image/",
    "text/",
    "video/",
}
```

其他类型自动转换为 `application/octet-stream`。

### 响应格式

#### 上传成功

```json
{
    "ctrl": {
        "code": 200,
        "text": "ok",
        "params": {
            "url": "/v0/file/s/abcdef12345.jpg",
            "expires": "2024-01-02T00:00:00.000Z"
        }
    }
}
```

#### 重定向（307）

某些情况下媒体处理器返回重定向：

```json
{
    "ctrl": {
        "code": 307,
        "text": "Temporary Redirect",
        "params": {
            "url": "https://s3.amazonaws.com/bucket/file.jpg"
        }
    }
}
```

### 文件垃圾回收

```go
func largeFileRunGarbageCollection(period time.Duration, blockSize int) chan<- bool {
    stop := make(chan bool)
    go func() {
        // 添加随机抖动避免集群节点同时运行
        period = (period >> 1) + (period >> 2) + time.Duration(rand.Intn(int(period>>1)))
        gcTicker := time.Tick(period)
        for {
            select {
            case <-gcTicker:
                // 删除未使用的文件
                store.Files.DeleteUnused(time.Now().Add(-time.Hour), blockSize)
            case <-stop:
                return
            }
        }
    }()
    return stop
}
```

---

## 消息格式

### 通用消息结构

所有协议共享相同的消息语义，只是序列化格式不同：

| 协议 | 序列化格式 | 特点 |
|------|-----------|------|
| WebSocket | JSON | 文本、易调试 |
| 长轮询 | JSON | 与 WebSocket 相同 |
| gRPC | Protobuf | 二进制、高效 |

### 消息类型总览

#### 客户端到服务端

| 消息 | 说明 | 用途 |
|------|------|------|
| `{hi}` | 握手 | 建立连接 |
| `{acc}` | 账户 | 创建/修改账户 |
| `{login}` | 登录 | 认证 |
| `{sub}` | 订阅 | 订阅话题 |
| `{leave}` | 离开 | 离开话题 |
| `{pub}` | 发布 | 发送消息 |
| `{get}` | 查询 | 获取数据 |
| `{set}` | 更新 | 修改数据 |
| `{del}` | 删除 | 删除数据 |
| `{note}` | 通知 | 临时通知 |

#### 服务端到客户端

| 消息 | 说明 | 用途 |
|------|------|------|
| `{ctrl}` | 控制响应 | 请求响应 |
| `{data}` | 数据消息 | 消息内容 |
| `{meta}` | 元数据 | 话题信息 |
| `{pres}` | 在线状态 | 状态变更 |
| `{info}` | 信息 | 转发的通知 |

---

## 协议对比

### 功能对比

| 功能 | WebSocket | gRPC | 长轮询 | HTTP |
|------|-----------|------|--------|------|
| 实时消息 | ✅ | ✅ | ⚠️ | ❌ |
| 双向通信 | ✅ | ✅ | ❌ | ❌ |
| 流式传输 | ❌ | ✅ | ❌ | ⚠️ |
| 文件传输 | ❌ | ✅ | ❌ | ✅ |
| 浏览器支持 | ✅ | ❌ | ✅ | ✅ |
| 防火墙友好 | ⚠️ | ❌ | ✅ | ✅ |
| 性能 | 高 | 最高 | 低 | 中 |
| 带宽效率 | 中 | 高 | 低 | 中 |

### 使用场景

| 场景 | 推荐协议 | 原因 |
|------|----------|------|
| Web 应用 | WebSocket | 实时性、浏览器原生支持 |
| 移动应用 | gRPC | 高性能、省流量 |
| CLI 工具 | gRPC | 强类型、易开发 |
| 聊天机器人 | gRPC | Plugin API、流式事件 |
| 老旧浏览器 | 长轮询 | 兼容性 |
| 严格防火墙环境 | 长轮询 | 纯 HTTP |
| 文件上传下载 | HTTP | 大文件支持 |

### 性能对比

| 指标 | WebSocket | gRPC | 长轮询 |
|------|-----------|------|--------|
| 连接建立 | ~50ms | ~30ms | ~100ms |
| 消息延迟 | ~10ms | ~5ms | ~100ms |
| CPU 占用 | 中 | 低 | 高 |
| 内存占用 | 中 | 低 | 中 |
| 带宽占用 | 中 | 低 | 高 |

---

## 附录

### 配置示例

```json
{
    "listen": ":6060",
    "grpc_listen": ":16060",
    "max_message_size": 262144,
    "idle_session_timeout": 60000,
    "ws_compression": true,

    "api_key_salt": "your-hmac-salt",
    "expvar": "/debug/vars"
}
```

### 调试技巧

#### 1. WebSocket 调试

使用浏览器控制台：

```javascript
const ws = new WebSocket('ws://localhost:6060/v0/channels');
ws.onopen = () => console.log('Connected');
ws.onmessage = (e) => console.log('Received:', e.data);
ws.send(JSON.stringify({hi: {ver: "0.22", ua: "test"}}));
```

#### 2. gRPC 调试

使用 `grpcurl` 工具：

```bash
# 列出服务
grpcurl -plaintext localhost:16060 list

# 调用方法
grpcurl -plaintext -d '{"hi": {"ver": "0.22"}}' localhost:16060 pbx.Node/MessageLoop
```

#### 3. 长轮询调试

使用 `curl`：

```bash
# 创建会话
curl -X POST http://localhost:6060/v0/channels/lp

# 发送消息
curl -X POST "http://localhost:6060/v0/channels/lp?sid=SESSION_ID" \
    -d '{"login": {"scheme": "basic", "secret": "dGVzdDp0ZXN0"}}'

# 长轮询
curl "http://localhost:6060/v0/channels/lp?sid=SESSION_ID"
```

### 相关文件

| 文件 | 说明 |
|------|------|
| `server/hdl_websock.go` | WebSocket 处理 |
| `server/hdl_grpc.go` | gRPC 处理 |
| `server/hdl_longpoll.go` | 长轮询处理 |
| `server/hdl_files.go` | 文件传输处理 |
| `pbx/model.proto` | gRPC 协议定义 |
| `server/http.go` | HTTP 服务器 |
| `server/session.go` | 会话管理 |
| `server/datamodel.go` | 消息结构定义 |