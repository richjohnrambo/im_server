# Tinode Chat Server 代码通读指南

> 本指南帮助你快速理解 Tinode 代码架构，掌握开发流程，能够独立完成新需求开发和项目部署。

---

## 目录

1. [架构总览](#架构总览)
2. [启动流程详解](#启动流程详解)
3. [核心数据结构](#核心数据结构)
4. [消息处理流程](#消息处理流程)
5. [关键模块详解](#关键模块详解)
6. [开发实战指南](#开发实战指南)
7. [测试与调试](#测试与调试)
8. [部署指南](#部署指南)

---

## 架构总览

### 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              客户端层                                        │
│   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   │
│   │  WebSocket  │   │  Long Poll  │   │    gRPC     │   │    HTTP     │   │
│   └──────┬──────┘   └──────┬──────┘   └──────┬──────┘   └──────┬──────┘   │
└──────────┼─────────────────┼─────────────────┼─────────────────┼───────────┘
           │                 │                 │                 │
           ▼                 ▼                 ▼                 ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              协议处理层                                      │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                     hdl_websock.go / hdl_longpoll.go                │   │
│   │                        hdl_grpc.go / http.go                        │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              会话层 (Session)                                │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  Session 结构体 - 管理单个客户端连接                                │   │
│   │  - 认证状态、订阅列表、消息队列                                      │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Hub (核心路由)                                  │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  - 消息路由分发                                                      │   │
│   │  - Topic 生命周期管理                                                │   │
│   │  - 全局状态维护                                                      │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Topic 层                                        │
│   ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌─────────┐     │
│   │   me    │   │   fnd   │   │   p2p   │   │   grp   │   │   sys   │     │
│   │ (用户)  │   │ (发现)  │   │ (单聊)  │   │ (群聊)  │   │ (系统)  │     │
│   └─────────┘   └─────────┘   └─────────┘   └─────────┘   └─────────┘     │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              存储层 (Store)                                  │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  store.go - 存储抽象层                                              │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│   ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌─────────┐                   │
│   │  MySQL  │   │PostgreSQL│   │ MongoDB │   │RethinkDB│                   │
│   └─────────┘   └─────────┘   └─────────┘   └─────────┘                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 核心组件职责

| 组件 | 文件 | 职责 |
|------|------|------|
| **main.go** | 入口点 | 配置加载、组件初始化、服务启动 |
| **Hub** | hub.go | 全局消息路由、Topic 生命周期管理 |
| **Session** | session.go | 客户端连接管理、消息收发 |
| **Topic** | topic.go | 聊天室/会话逻辑、消息处理 |
| **Store** | store/store.go | 数据库操作抽象层 |
| **Plugins** | plugins.go | 插件系统、事件钩子 |

---

## 启动流程详解

### 1. 入口函数 (main.go)

```go
// main.go 启动流程
func main() {
    // 1. 解析命令行参数
    flag.Parse()

    // 2. 加载配置文件
    config := loadConfig(configFileName)

    // 3. 初始化日志系统
    logs.Init()

    // 4. 初始化数据库适配器
    store.Open(config.StoreConfig)

    // 5. 初始化认证模块
    auth.Init(config.AuthConfig)

    // 6. 初始化推送通知
    push.Init(config.PushConfig)

    // 7. 初始化媒体处理
    media.Init(config.MediaConfig)

    // 8. 创建 Hub（核心路由）
    globals.hub = newHub()

    // 9. 创建 SessionStore
    globals.sessionStore = newSessionStore()

    // 10. 初始化插件
    pluginsInit(config.PluginConfig)

    // 11. 启动 HTTP/WebSocket 服务
    listenAndServe(addr, mux, tlsConfig, stop)
}
```

### 2. 配置加载流程

配置文件 `tinode.conf` 是 JSON 格式，主要配置项：

```json
{
    "listen": ":6060",              // HTTP 监听地址
    "grpc_listen": ":16060",        // gRPC 监听地址
    "api_key_salt": "...",          // API 密钥盐值
    "max_message_size": 131072,     // 最大消息大小

    "store_config": {               // 数据库配置
        "use_adapter": "mysql",
        "adapters": {
            "mysql": { "dsn": "..." }
        }
    },

    "auth_config": {                // 认证配置
        "basic": { "add_to_tags": true }
    },

    "push_config": {                // 推送配置
        "fcm": { "credentials_file": "..." }
    }
}
```

### 3. 数据库初始化 (tinode-db)

```bash
# 初始化数据库并加载测试数据
./tinode-db -config=tinode.conf -data=data.json

# 主要步骤：
# 1. 连接数据库
# 2. 创建表/集合
# 3. 创建索引
# 4. 加载初始数据（用户、话题）
```

---

## 核心数据结构

### 1. Hub 结构体

Hub 是整个系统的核心路由器：

```go
// server/hub.go
type Hub struct {
    // Topic 缓存 (name -> *Topic)
    topics *sync.Map

    // 客户端消息路由通道
    routeCli chan *ClientComMessage

    // 服务端消息路由通道
    routeSrv chan *ServerComMessage

    // 订阅请求通道
    join chan *ClientComMessage

    // Topic 注销通道
    unreg chan *topicUnreg

    // 用户状态变更通道
    userStatus chan *userStatusReq

    // 关闭通道
    shutdown chan chan<- bool
}
```

**Hub 的核心方法**：

```go
// hub.go - 主事件循环
func (h *Hub) run() {
    for {
        select {
        case msg := <-h.routeCli:
            // 路由客户端消息到对应 Topic
            h.routeClientMessage(msg)

        case msg := <-h.routeSrv:
            // 路由服务端消息
            h.routeServerMessage(msg)

        case req := <-h.join:
            // 处理订阅请求，创建或加载 Topic
            h.processJoin(req)

        case req := <-h.unreg:
            // 注销 Topic
            h.processUnregister(req)

        case done := <-h.shutdown:
            // 关闭所有 Topic
            h.gracefulShutdown()
            done <- true
            return
        }
    }
}
```

### 2. Topic 结构体

Topic 是聊天的核心单元：

```go
// server/topic.go
type Topic struct {
    // Topic 名称 (如 "usrABC", "grpXYZ")
    name string

    // Topic 类型
    cat types.TopicCat  // me, fnd, p2p, grp, sys, slf

    // 订阅者数据
    perUser map[types.Uid]perUserData

    // 附加的会话
    sessions map[*Session]perSessionData

    // 消息通道
    clientMsg chan *ClientComMessage  // 客户端消息
    serverMsg chan *ServerComMessage  // 服务端消息
    reg       chan *ClientComMessage  // 订阅请求
    unreg     chan *ClientComMessage  // 取消订阅
    meta      chan *ClientComMessage  // 元数据操作
    exit      chan *shutDown          // 关闭信号

    // 状态
    status int32  // 0=none, 1=new, 2=ready, 3=paused
}
```

**Topic 类型分类**：

| 类型 | 前缀 | 说明 |
|------|------|------|
| `me` | `me` | 用户个人话题，管理联系人列表 |
| `fnd` | `fnd` | 搜索话题，用于发现用户 |
| `p2p` | `p2p` | 点对点聊天 |
| `grp` | `grp` | 群组聊天 |
| `chn` | `chn` | 频道（只读群组） |
| `sys` | `sys` | 系统话题 |
| `slf` | `slf` | 自我话题（草稿/收藏） |

### 3. Session 结构体

Session 代表一个客户端连接：

```go
// server/session.go
type Session struct {
    // 协议类型
    proto SessionProto  // WEBSOCK, LPOLL, GRPC, PROXY

    // 会话 ID
    sid string

    // 连接句柄
    ws       *websocket.Conn           // WebSocket
    grpcnode pbx.Node_MessageLoopServer // gRPC

    // 用户信息
    uid     types.Uid   // 用户 ID
    authLvl auth.Level  // 认证级别

    // 消息队列
    send chan any       // 发送队列
    stop chan any       // 停止信号
    detach chan string  // 分离话题

    // 订阅的话题
    subs map[string]*Subscription

    // 状态标记
    terminating int32   // 是否正在终止
    background bool     // 是否后台会话
}
```

### 4. 消息结构体

客户端到服务端的消息定义：

```go
// server/datamodel.go

// 握手消息
type MsgClientHi struct {
    Id        string `json:"id,omitempty"`
    UserAgent string `json:"ua,omitempty"`
    Version   string `json:"ver,omitempty"`
    DeviceID  string `json:"dev,omitempty"`
    Platform  string `json:"platf,omitempty"`
}

// 登录消息
type MsgClientLogin struct {
    Id     string `json:"id,omitempty"`
    Scheme string `json:"scheme,omitempty"`
    Secret []byte `json:"secret,omitempty"`
}

// 订阅消息
type MsgClientSub struct {
    Id      string       `json:"id,omitempty"`
    Topic   string       `json:"topic,omitempty"`
    Get     *MsgGetQuery `json:"get,omitempty"`
    Set     *MsgSetQuery `json:"set,omitempty"`
}

// 发布消息
type MsgClientPub struct {
    Id      string `json:"id,omitempty"`
    Topic   string `json:"topic,omitempty"`
    Content any    `json:"content,omitempty"`
}
```

---

## 消息处理流程

### 1. 完整消息流转图

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  客户端发送: {"hi", "ver":"0.25", "ua":"Chrome/120"}                        │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│  WebSocket Handler (hdl_websock.go)                                          │
│  serveWebSocket() -> sess.readLoop() -> sess.dispatchRaw(raw)               │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│  Session 消息分发 (session.go)                                               │
│  dispatchRaw() -> 解析 JSON -> 识别消息类型 -> 调用对应 handler              │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
            ┌───────────────────────┼───────────────────────┐
            ▼                       ▼                       ▼
     ┌──────────┐            ┌──────────┐            ┌──────────┐
     │  hi      │            │  login   │            │  sub     │
     │  handler │            │  handler │            │  handler │
     └──────────┘            └──────────┘            └──────────┘
            │                       │                       │
            └───────────────────────┼───────────────────────┘
                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│  Hub 路由 (hub.go)                                                           │
│  routeCli <- ClientComMessage -> h.routeClientMessage()                      │
│  根据 RcptTo 找到对应 Topic -> 发送到 Topic.clientMsg 通道                   │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│  Topic 处理 (topic.go)                                                       │
│  t.clientMsg <- msg -> t.processClientMessage()                              │
│  根据 msg.Original 类型调用对应处理函数                                       │
│  - hi -> t.handleHi()                                                        │
│  - login -> t.handleLogin()                                                  │
│  - sub -> t.handleSubscribe()                                                │
│  - pub -> t.handlePublish()                                                  │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│  数据库操作 (store/store.go)                                                 │
│  store.MessageSave() / store.TopicGet() / store.UserCreate()                │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│  响应消息生成                                                                 │
│  ServerComMessage{Ctrl/Data/Pres/Meta/Info}                                  │
│  -> 序列化为 JSON -> 发送到 Session.send 通道                                 │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│  Session 发送 (session.go)                                                   │
│  sess.writeLoop() -> sess.send <- msg -> WebSocket.WriteMessage()           │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 2. 消息类型与处理函数映射

| 消息类型 | 结构体 | 处理函数 | 说明 |
|----------|--------|----------|------|
| `hi` | MsgClientHi | handleHi() | 握手，建立连接 |
| `login` | MsgClientLogin | handleLogin() | 用户登录认证 |
| `acc` | MsgClientAcc | handleAccount() | 创建/更新账户 |
| `sub` | MsgClientSub | handleSubscribe() | 订阅话题 |
| `leave` | MsgClientLeave | handleLeave() | 取消订阅 |
| `pub` | MsgClientPub | handlePublish() | 发布消息 |
| `get` | MsgClientGet | handleGet() | 获取元数据/消息 |
| `set` | MsgClientSet | handleSet() | 更新元数据 |
| `del` | MsgClientDel | handleDelete() | 删除消息/话题 |
| `note` | MsgClientNote | handleNote() | 发送通知 |

### 3. WebSocket 消息处理示例

```go
// server/hdl_websock.go

// 处理 WebSocket 连接
func serveWebSocket(wrt http.ResponseWriter, req *http.Request) {
    // 1. 验证 API Key
    if isValid, _ := checkAPIKey(getAPIKey(req)); !isValid {
        wrt.WriteHeader(http.StatusForbidden)
        return
    }

    // 2. 升级 HTTP 连接为 WebSocket
    ws, err := upgrader.Upgrade(wrt, req, nil)

    // 3. 创建 Session
    sess, count := globals.sessionStore.NewSession(ws, "")

    // 4. 启动读写协程
    go sess.writeLoop()  // 发送消息
    go sess.readLoop()   // 接收消息
}

// 读循环
func (sess *Session) readLoop() {
    for {
        _, raw, err := sess.ws.ReadMessage()
        if err != nil {
            return
        }
        sess.dispatchRaw(raw)  // 分发消息
    }
}

// 写循环
func (sess *Session) writeLoop() {
    for {
        select {
        case msg := <-sess.send:
            sess.sendMessage(msg)
        case <-sess.stop:
            return
        }
    }
}
```

---

## 关键模块详解

### 1. 认证模块 (server/auth/)

认证模块采用插件式架构：

```
server/auth/
├── auth.go          # 认证接口定义
├── basic/           # 用户名密码认证
├── token/           # Token 认证
├── anon/            # 匿名认证
├── code/            # 验证码认证
└── rest/            # REST 外部认证
```

**认证接口定义**：

```go
// server/auth/auth.go
type Authenticator interface {
    // 初始化
    Init(jsonconf json.RawMessage, opts ...any) error

    // 认证检查
    Authenticate(secret []byte) (types.Uid, Level, time.Time, error)

    // 生成秘密值
    GenSecret(uid types.Uid, authLvl Level, expires time.Time) ([]byte, error)
}
```

**添加新认证方式步骤**：

1. 在 `server/auth/` 下创建新目录，如 `oauth/`
2. 实现 `Authenticator` 接口
3. 在 `main.go` 中导入: `_ "github.com/tinode/chat/server/auth/oauth"`
4. 在配置文件中添加相应配置

### 2. 数据库适配器 (server/db/)

数据库层采用适配器模式：

```go
// server/db/adapter.go
type Adapter interface {
    // 连接管理
    Open(config json.RawMessage) error
    Close() error
    IsOpen() bool

    // 用户管理
    UserCreate(user *t.User) error
    UserGet(uid t.Uid) (*t.User, error)
    UserUpdate(uid t.Uid, update map[string]any) error
    UserDelete(uid t.Uid, hard bool) error

    // 话题管理
    TopicCreate(topic *t.Topic) error
    TopicGet(name string) (*t.Topic, error)

    // 消息管理
    MessageSave(msg *t.Message) error
    MessageGet(topic string, opts *MsgGetOpts) ([]t.Message, error)

    // 订阅管理
    SubscriptionGet(topic string, uid t.Uid) (*t.Subscription, error)
}
```

**添加新数据库支持**：

1. 在 `server/db/` 下创建新目录，如 `cockroach/`
2. 实现 `Adapter` 接口
3. 在 `main.go` 中导入: `_ "github.com/tinode/chat/server/db/cockroach"`
4. 构建时指定 tag: `go build -tags cockroach`

### 3. 推送通知 (server/push/)

```go
// server/push/push.go
type PushHandler interface {
    // 初始化
    Init(jsonconf json.RawMessage) error

    // 发送推送
    Push(userId types.Uid, data *PushData) error
}
```

**FCM 推送配置**：

```json
// tinode.conf
"push_config": {
    "fcm": {
        "credentials_file": "/path/to/credentials.json"
    }
}
```

### 4. 文件存储 (server/media/)

支持本地文件系统和 S3：

```go
// server/media/media.go
type Handler interface {
    // 上传
    Upload(uri string, reader io.Reader) (string, error)

    // 下载
    Download(uri string) (io.ReadCloser, error)

    // 删除
    Delete(uri string) error
}
```

### 5. 插件系统 (server/plugins.go)

插件可以拦截和处理所有消息：

```go
// 插件事件类型
const (
    plgHi    = 1 << iota  // 握手
    plgAcc                  // 账户操作
    plgLogin                // 登录
    plgSub                  // 订阅
    plgPub                  // 发布
    plgGet                  // 获取
    plgSet                  // 设置
    plgDel                  // 删除
)

// 插件接口
type Plugin interface {
    // 处理客户端消息
    HandleClientMessage(msg *ClientComMessage) (*ClientComMessage, error)

    // 处理服务端消息
    HandleServerMessage(msg *ServerComMessage) (*ServerComMessage, error)
}
```

---

## 开发实战指南

### 场景 1: 添加新的消息类型

**步骤 1**: 定义消息结构体

```go
// server/datamodel.go
type MsgClientTyping struct {
    Id      string `json:"id,omitempty"`
    Topic   string `json:"topic,omitempty"`
    IsTyping bool  `json:"typing"`
}
```

**步骤 2**: 添加解析逻辑

```go
// server/session.go - dispatchRaw()
func (sess *Session) dispatchRaw(raw []byte) {
    var msg struct {
        Hi    *MsgClientHi    `json:"hi"`
        // ... 其他类型
        Typing *MsgClientTyping `json:"typing"`  // 新增
    }
    json.Unmarshal(raw, &msg)

    if msg.Typing != nil {
        sess.handleTyping(msg.Typing)
    }
}
```

**步骤 3**: 实现处理函数

```go
// server/topic.go
func (t *Topic) handleTyping(msg *MsgClientTyping) {
    // 广报给其他订阅者
    for sess := range t.sessions {
        if sess.uid != msg.sess.uid {
            sess.queueOut(&ServerComMessage{
                Info: &MsgServerInfo{
                    Topic:   t.name,
                    From:    msg.sess.uid.String(),
                    Content: map[string]any{"typing": msg.IsTyping},
                },
            })
        }
    }
}
```

### 场景 2: 添加新的 API 端点

**步骤 1**: 定义 HTTP Handler

```go
// server/http.go
func serveCustomAPI(wrt http.ResponseWriter, req *http.Request) {
    // 验证请求
    if req.Method != http.MethodPost {
        wrt.WriteHeader(http.StatusMethodNotAllowed)
        return
    }

    // 处理请求
    var data struct {
        UserID string `json:"user_id"`
    }
    json.NewDecoder(req.Body).Decode(&data)

    // 返回响应
    wrt.Header().Set("Content-Type", "application/json")
    json.NewEncoder(wrt).Encode(map[string]any{
        "status": "ok",
    })
}
```

**步骤 2**: 注册路由

```go
// server/http.go - initHTTP()
func initHTTP() *http.ServeMux {
    mux := http.NewServeMux()
    // ... 其他路由
    mux.HandleFunc("/v1/custom", serveCustomAPI)  // 新增
    return mux
}
```

### 场景 3: 添加新的数据库字段

**步骤 1**: 修改数据模型

```go
// server/store/types/types.go
type User struct {
    Id        types.Uid
    CreatedAt time.Time
    // 新增字段
    Nickname  string `json:"nickname"`
}
```

**步骤 2**: 修改数据库适配器

```go
// server/db/mysql/adapter.go
func (a *adapter) UserCreate(user *t.User) error {
    _, err := a.db.Exec(`
        INSERT INTO users (id, createdat, nickname)
        VALUES (?, ?, ?)
    `, user.Id.String(), user.CreatedAt, user.Nickname)
    return err
}
```

**步骤 3**: 更新数据库 Schema

```sql
-- server/db/mysql/schema.sql
ALTER TABLE users ADD COLUMN nickname VARCHAR(64);
```

### 场景 4: 开发插件

创建一个消息过滤插件：

```go
// myplugin/filter.go
package main

import (
    "github.com/tinode/chat/pbx"
)

type FilterPlugin struct {
    bannedWords []string
}

func (p *FilterPlugin) HandleClientMessage(msg *pb.ClientMsg) (*pb.ClientMsg, error) {
    // 检查消息内容
    if msg.Pub != nil {
        content := string(msg.Pub.Content)
        for _, word := range p.bannedWords {
            if strings.Contains(content, word) {
                // 过滤敏感词
                content = strings.ReplaceAll(content, word, "***")
                msg.Pub.Content = []byte(content)
            }
        }
    }
    return msg, nil
}

func (p *FilterPlugin) HandleServerMessage(msg *pb.ServerMsg) (*pb.ServerMsg, error) {
    return msg, nil
}
```

**注册插件**：

```go
// main.go
func main() {
    plugin := &FilterPlugin{
        bannedWords: []string{"badword1", "badword2"},
    }
    globals.plugins = append(globals.plugins, plugin)
}
```

---

## 测试与调试

### 1. 单元测试

```bash
# 运行所有测试
go test ./server/...

# 运行特定测试
go test -run TestTopicPublish ./server/

# 查看覆盖率
go test -cover ./server/...
```

### 2. 集成测试

使用 tn-cli 进行集成测试：

```bash
# 登录
python tn-cli.py --login-basic=alice:alice123

# 执行测试脚本
python tn-cli.py < test-script.txt
```

### 3. 调试技巧

**开启详细日志**：

```go
// 在 tinode.conf 中
{
    "logging": {
        "level": "debug"
    }
}
```

**使用 pprof 性能分析**：

```bash
# 启动服务器时开启 pprof
./server -expvar=/debug/vars

# 访问性能分析
go tool pprof http://localhost:6060/debug/pprof/profile
```

**调试 WebSocket**：

```javascript
// 浏览器控制台
const ws = new WebSocket('ws://localhost:6060/v0/channels');

ws.onopen = () => {
    ws.send(JSON.stringify({hi: {ver: "0.25", ua: "test"}}));
};

ws.onmessage = (e) => {
    console.log('收到:', JSON.parse(e.data));
};
```

### 4. 数据库调试

```bash
# MySQL
mysql -u root -p tinode -e "SELECT * FROM users LIMIT 10;"

# MongoDB
mongosh tinode --eval "db.users.find().limit(10)"

# PostgreSQL
psql -U postgres -d tinode -c "SELECT * FROM users LIMIT 10;"
```

---

## 部署指南

### 1. 本地开发环境

```bash
# 1. 克隆代码
git clone https://github.com/tinode/chat.git
cd chat

# 2. 安装依赖
go mod download

# 3. 启动数据库（以 MySQL 为例）
docker run -d --name mysql \
    -e MYSQL_ROOT_PASSWORD=root \
    -e MYSQL_DATABASE=tinode \
    -p 3306:3306 \
    mysql:8.0

# 4. 初始化数据库
go run ./tinode-db -tags mysql -data=./tinode-db/data.json

# 5. 启动服务器
go run ./server -tags mysql
```

### 2. 生产环境部署

#### 方式 1: 直接部署

```bash
# 构建
go build -tags mysql -ldflags "-X main.buildstamp=`git describe --tags`" ./server

# 创建配置目录
mkdir -p /etc/tinode
cp server/tinode.conf /etc/tinode/

# 创建 systemd 服务
cat > /etc/systemd/system/tinode.service << 'EOF'
[Unit]
Description=Tinode Chat Server
After=network.target mysql.service

[Service]
Type=simple
User=tinode
ExecStart=/usr/local/bin/server -config=/etc/tinode/tinode.conf
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 启动服务
systemctl daemon-reload
systemctl enable tinode
systemctl start tinode
```

#### 方式 2: Docker 部署

```bash
# 使用官方镜像
docker run -d --name tinode \
    -p 6060:18080 \
    -e MYSQL_DSN="root@tcp(mysql:3306)/tinode?parseTime=true" \
    tinode/tinode-mysql:latest

# Docker Compose
cat > docker-compose.yml << 'EOF'
version: '3'
services:
  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: root
      MYSQL_DATABASE: tinode
    volumes:
      - mysql_data:/var/lib/mysql

  tinode:
    image: tinode/tinode-mysql:latest
    ports:
      - "6060:18080"
    depends_on:
      - mysql
    environment:
      STORE_CONFIG_ADAPTERS_MYSQL_DSN: "root@tcp(mysql:3306)/tinode?parseTime=true"

volumes:
  mysql_data:
EOF

docker-compose up -d
```

### 3. 集群部署

```yaml
# tinode.conf 集群配置
{
    "cluster": {
        "enabled": true,
        "self": "node1",
        "nodes": [
            {"id": "node1", "addr": "node1.example.com:12000"},
            {"id": "node2", "addr": "node2.example.com:12000"},
            {"id": "node3", "addr": "node3.example.com:12000"}
        ]
    }
}
```

### 4. 监控配置

```yaml
# Prometheus 监控
"expvar": "/debug/vars"

# Grafana 仪表盘
# 导入: https://grafana.com/grafana/dashboards/...
```

### 5. 生产环境配置建议

```json
{
    // 安全配置
    "tls": {
        "enabled": true,
        "strict_max_age": 604800
    },

    // 性能配置
    "max_message_size": 262144,
    "max_subscriber_count": 256,

    // 日志配置
    "logging": {
        "level": "info"
    },

    // 数据库连接池
    "store_config": {
        "adapters": {
            "mysql": {
                "dsn": "user:pass@tcp(db-host:3306)/tinode?parseTime=true&max_open_conns=100&max_idle_conns=10"
            }
        }
    }
}
```

---

## 常见问题排查

### 1. 连接问题

```bash
# 检查端口是否监听
netstat -tlnp | grep 6060

# 检查防火墙
iptables -L -n | grep 6060

# 测试连接
curl http://localhost:6060/v0/hi
```

### 2. 数据库问题

```bash
# 检查数据库连接
mysql -u root -p -e "SHOW PROCESSLIST;"

# 检查表结构
mysql -u root -p tinode -e "SHOW TABLES;"

# 查看错误日志
tail -f /var/log/mysql/error.log
```

### 3. 性能问题

```bash
# CPU 分析
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 内存分析
go tool pprof http://localhost:6060/debug/pprof/heap

# 查看统计
curl http://localhost:6060/debug/vars | jq .
```

---

## 扩展阅读

- [API 文档](docs/API.md) - 完整的 API 参考
- [Drafty 格式](docs/drafty.md) - 富文本消息格式
- [通话建立流程](docs/call-establishment.md) - 音视频通话信令
- [Docker 部署](docker/README.md) - 容器化部署详解

---

## 开发流程总结

```
┌─────────────────────────────────────────────────────────────────┐
│                        开发新功能流程                            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  1. 理解需求                                                      │
│     - 阅读需求文档                                                 │
│     - 确认技术方案                                                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. 设计数据结构                                                  │
│     - 修改 datamodel.go 添加消息类型                              │
│     - 修改 store/types/types.go 添加数据模型                      │
│     - 更新数据库 schema                                            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. 实现业务逻辑                                                  │
│     - 在 topic.go 添加处理函数                                     │
│     - 在 store.go 添加数据库操作                                   │
│     - 实现必要的适配器方法                                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. 编写测试                                                      │
│     - 单元测试 *_test.go                                           │
│     - 集成测试（使用 tn-cli）                                      │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  5. 测试验证                                                      │
│     - go test ./...                                               │
│     - 本地运行验证                                                 │
│     - 性能测试                                                    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  6. 代码审查 & 提交                                               │
│     - 代码审查                                                    │
│     - 更新文档                                                    │
│     - Git commit                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

祝开发顺利！