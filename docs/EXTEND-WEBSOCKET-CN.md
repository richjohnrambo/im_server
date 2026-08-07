# 扩展 WebSocket 协议开发指南

> 本文档详细介绍如何在 Tinode 中扩展 WebSocket 协议，包括添加新消息类型、新端点、修改现有行为等。

---

## 目录

- [概述](#概述)
- [场景一：添加新消息类型](#场景一添加新消息类型)
- [场景二：添加新 WebSocket 端点](#场景二添加新-websocket-端点)
- [场景三：修改现有消息处理](#场景三修改现有消息处理)
- [场景四：添加新的认证方式](#场景四添加新的认证方式)
- [测试指南](#测试指南)
- [调试技巧](#调试技巧)

---

## 概述

### WebSocket 协议扩展的几种方式

| 方式 | 用途 | 复杂度 |
|------|------|--------|
| 添加新消息类型 | 新增业务功能（如撤回消息、输入状态） | 中 |
| 添加新端点 | 独立的服务入口（如管理后台） | 高 |
| 修改现有消息 | 增强现有功能（如消息增加字段） | 低 |
| 添加认证方式 | 支持新的登录方式（如 OAuth） | 中 |

### 核心文件

| 文件 | 说明 |
|------|------|
| `server/datamodel.go` | 消息结构定义 |
| `server/session.go` | 消息路由和分发 |
| `server/hdl_websock.go` | WebSocket 连接处理 |
| `server/hub.go` | 会话和话题管理 |
| `pbx/model.proto` | gRPC 协议定义（如需同步支持） |

---

## 场景一：添加新消息类型

### 示例：添加消息撤回功能

假设我们要添加一个 `{recall}` 消息类型，用于撤回已发送的消息。

### 步骤 1：定义消息结构

**文件**: `server/datamodel.go`

```go
// 在文件末尾添加

// MsgClientRecall 是撤回消息的请求 {recall}
type MsgClientRecall struct {
    // 消息 ID（用于响应匹配）
    Id string `json:"id,omitempty"`
    // 话题名称
    Topic string `json:"topic"`
    // 要撤回的消息 ID 列表
    SeqList []int `json:"seq"`
    // 是否硬删除（对所有人生效）
    Hard bool `json:"hard,omitempty"`
}

// MsgServerRecall 是撤回消息的响应
type MsgServerRecall struct {
    // 原请求 ID
    Id string `json:"id,omitempty"`
    // 话题名称
    Topic string `json:"topic,omitempty"`
    // 已撤回的消息 ID 列表
    SeqList []int `json:"seq,omitempty"`
    // 撤回时间
    Deleted *time.Time `json:"deleted,omitempty"`
}
```

### 步骤 2：添加到 ClientComMessage

**文件**: `server/datamodel.go`

```go
// ClientComMessage 是客户端消息的包装器
type ClientComMessage struct {
    Hi    *MsgClientHi    `json:"hi"`
    Acc   *MsgClientAcc   `json:"acc"`
    Login *MsgClientLogin `json:"login"`
    Sub   *MsgClientSub   `json:"sub"`
    Leave *MsgClientLeave `json:"leave"`
    Pub   *MsgClientPub   `json:"pub"`
    Get   *MsgClientGet   `json:"get"`
    Set   *MsgClientSet   `json:"set"`
    Del   *MsgClientDel   `json:"del"`
    Note  *MsgClientNote  `json:"note"`
    Recall *MsgClientRecall `json:"recall"`  // ← 添加这一行

    // ... 其他字段
}
```

### 步骤 3：添加到 ServerComMessage

**文件**: `server/datamodel.go`

```go
// ServerComMessage 是服务器消息的包装器
type ServerComMessage struct {
    Ctrl *MsgServerCtrl `json:"ctrl,omitempty"`
    Data *MsgServerData `json:"data,omitempty"`
    Meta *MsgServerMeta `json:"meta,omitempty"`
    Pres *MsgServerPres `json:"pres,omitempty"`
    Info *MsgServerInfo `json:"info,omitempty"`
    Recall *MsgServerRecall `json:"recall,omitempty"`  // ← 添加这一行

    // ... 其他字段
}
```

### 步骤 4：实现消息处理器

**文件**: `server/session.go`

```go
// 在文件末尾添加

// recall 处理消息撤回请求
func (s *Session) recall(msg *ClientComMessage) {
    // 1. 检查是否已认证
    if msg.AsUser == "" {
        s.queueOut(ErrAuthRequiredReply(msg, msg.Timestamp))
        return
    }

    // 2. 获取话题
    topic := msg.Original
    t := globals.hub.topicGet(topic)
    if t == nil {
        s.queueOut(ErrTopicNotFoundReply(msg, msg.Timestamp))
        return
    }

    // 3. 检查权限
    modeWant, modeGiven := t.getPerUserAcs(types.ParseUserId(msg.AsUser))
    if !(modeGiven & modeWant).IsWriter() {
        s.queueOut(ErrPermissionDeniedReply(msg, msg.Timestamp))
        return
    }

    // 4. 调用话题处理撤回
    t.clientMsg <- msg
}
```

### 步骤 5：注册消息路由

**文件**: `server/session.go`

找到 `dispatch` 函数，在 switch 语句中添加新的 case：

```go
func (s *Session) dispatch(msg *ClientComMessage) {
    // ... 前面的代码保持不变

    switch {
    case msg.Pub != nil:
        handler = checkVers(checkUser(s.publish))
        msg.Id = msg.Pub.Id
        msg.Original = msg.Pub.Topic

    // ... 其他 case

    case msg.Recall != nil:  // ← 添加这个 case
        handler = checkVers(checkUser(s.recall))
        msg.Id = msg.Recall.Id
        msg.Original = msg.Recall.Topic

    default:
        // 未知消息
        s.queueOut(ErrMalformed("", "", msg.Timestamp))
        logs.Warn.Println("s.dispatch: unknown message", s.sid)
        return
    }

    // ... 后续代码
}
```

### 步骤 6：在 Topic 中实现业务逻辑

**文件**: `server/topic.go`

```go
// 在 Topic 的消息处理循环中添加
func (t *Topic) runLocal(hub *Hub) {
    for {
        select {
        case msg := <-t.clientMsg:
            switch {
            case msg.Recall != nil:
                t.handleRecall(msg)
            // ... 其他消息类型
            }

        // ... 其他 case
        }
    }
}

// handleRecall 处理消息撤回
func (t *Topic) handleRecall(msg *ClientComMessage) {
    // 1. 验证消息所有权
    uid := types.ParseUserId(msg.AsUser)

    // 2. 从数据库删除消息
    var deletedSeqs []int
    for _, seq := range msg.Recall.SeqList {
        // 检查是否是自己发送的消息
        if t.isMessageOwner(seq, uid) || msg.Recall.Hard {
            err := store.Messages.Delete(t.name, seq, msg.Recall.Hard)
            if err == nil {
                deletedSeqs = append(deletedSeqs, seq)
            }
        }
    }

    // 3. 更新 delID
    if len(deletedSeqs) > 0 {
        t.delID++
        store.Topics.Update(t.name, map[string]any{"delId": t.delID})
    }

    // 4. 广播给所有订阅者
    now := types.TimeNow()
    resp := &ServerComMessage{
        Recall: &MsgServerRecall{
            Id:      msg.Id,
            Topic:   t.name,
            SeqList: deletedSeqs,
            Deleted: &now,
        },
    }

    for sess := range t.sessions {
        sess.queueOut(resp)
    }
}
```

### 步骤 7：同步 gRPC 支持（可选）

如果需要 gRPC 客户端也支持，需要修改：

**文件**: `pbx/model.proto`

```protobuf
// 添加新的消息定义
message ClientRecall {
    string id = 1;
    string topic = 2;
    repeated int32 seq = 3;
    bool hard = 4;
}

message ServerRecall {
    string id = 1;
    string topic = 2;
    repeated int32 seq = 3;
    int64 deleted = 4;
}

// 添加到 ClientMsg
message ClientMsg {
    oneof Message {
        // ... 现有字段
        ClientRecall recall = 14;  // 使用新的字段号
    }
    ClientExtra extra = 13;
}

// 添加到 ServerMsg
message ServerMsg {
    oneof Message {
        // ... 现有字段
        ServerRecall recall = 6;
    }
}
```

然后生成 Go 代码：

```bash
# 在 pbx 目录执行
protoc --go_out=plugins=grpc:. model.proto
```

**文件**: `server/pbconverter.go`

添加序列化和反序列化函数：

```go
func pbCliSerialize(msg *ClientComMessage) *pbx.ClientMsg {
    // ... 现有代码

    case msg.Recall != nil:
        pkt.Message = &pbx.ClientMsg_Recall{
            Recall: &pbx.ClientRecall{
                Id:    msg.Recall.Id,
                Topic: msg.Recall.Topic,
                Seq:   msg.Recall.SeqList,
                Hard:  msg.Recall.Hard,
            },
        }
}

func pbCliDeserialize(pkt *pbx.ClientMsg) *ClientComMessage {
    // ... 现有代码

    if recall := pkt.GetRecall(); recall != nil {
        msg.Recall = &MsgClientRecall{
            Id:      recall.GetId(),
            Topic:   recall.GetTopic(),
            SeqList: recall.GetSeq(),
            Hard:    recall.GetHard(),
        }
    }
}
```

### 步骤 8：编写单元测试

**文件**: `server/session_test.go`

```go
func TestRecallMessage(t *testing.T) {
    // 1. 创建测试会话
    sess := createTestSession()
    defer sess.cleanUp(false)

    // 2. 发送登录
    sess.dispatchRaw([]byte(`{
        "login": {
            "id": "login1",
            "scheme": "basic",
            "secret": "dXNlcjpwYXNz"
        }
    }`))

    // 3. 订阅话题
    sess.dispatchRaw([]byte(`{
        "sub": {
            "id": "sub1",
            "topic": "test-topic"
        }
    }`))

    // 4. 发送消息
    sess.dispatchRaw([]byte(`{
        "pub": {
            "id": "pub1",
            "topic": "test-topic",
            "content": "test message"
        }
    }`))

    // 5. 撤回消息
    sess.dispatchRaw([]byte(`{
        "recall": {
            "id": "recall1",
            "topic": "test-topic",
            "seq": [1]
        }
    }`))

    // 6. 验证响应
    // ... 检查撤回是否成功
}
```

---

## 场景二：添加新 WebSocket 端点

### 示例：添加管理后台专用端点

假设我们要添加一个 `/admin/channels` 端点，用于管理后台的 WebSocket 连接。

### 步骤 1：定义新的处理器

**文件**: `server/hdl_websock.go`

```go
// Admin WebSocket 升级器
var adminUpgrader = websocket.Upgrader{
    ReadBufferSize:    1024,
    WriteBufferSize:   1024,
    EnableCompression: true,
    CheckOrigin: func(r *http.Request) bool {
        // 只允许特定来源
        origin := r.Header.Get("Origin")
        return origin == "https://admin.example.com"
    },
}

// serveAdminWebSocket 处理管理后台 WebSocket 连接
func serveAdminWebSocket(wrt http.ResponseWriter, req *http.Request) {
    now := types.TimeNow()

    // 1. 验证 API Key（管理后台专用）
    apiKey := getAPIKey(req)
    if !isAdminAPIKey(apiKey) {
        wrt.WriteHeader(http.StatusForbidden)
        json.NewEncoder(wrt).Encode(ErrAPIKeyRequired(now))
        return
    }

    // 2. 验证管理员令牌
    token := req.Header.Get("X-Admin-Token")
    if !validateAdminToken(token) {
        wrt.WriteHeader(http.StatusUnauthorized)
        json.NewEncoder(wrt).Encode(ErrPermissionDenied("", "", now))
        return
    }

    // 3. 升级 WebSocket
    ws, err := adminUpgrader.Upgrade(wrt, req, nil)
    if err != nil {
        logs.Err.Println("admin ws: failed to Upgrade", err)
        return
    }

    // 4. 创建管理员会话
    sess, count := globals.sessionStore.NewSession(ws, "admin")
    sess.isAdmin = true  // 标记为管理员会话
    sess.remoteAddr = getRemoteAddr(req)

    logs.Info.Println("admin ws: session started", sess.sid, count)

    // 5. 启动读写循环
    go sess.writeLoop()
    go sess.readLoop()
}

// isAdminAPIKey 检查是否为管理 API Key
func isAdminAPIKey(apiKey string) bool {
    // 检查配置中的管理 API Key
    return apiKey == globals.adminAPIKey
}

// validateAdminToken 验证管理员令牌
func validateAdminToken(token string) bool {
    // JWT 验证或其他验证逻辑
    return token != ""
}
```

### 步骤 2：注册路由

**文件**: `server/http.go`

```go
func setupHTTPHandlers() {
    // ... 现有路由

    // 添加管理后台 WebSocket 路由
    http.HandleFunc("/admin/channels", serveAdminWebSocket)
}
```

### 步骤 3：扩展 Session 结构

**文件**: `server/session.go`

```go
type Session struct {
    // ... 现有字段

    // 是否为管理员会话
    isAdmin bool
}
```

### 步骤 4：添加管理员专用消息

可以在 dispatch 函数中添加管理员专用的消息处理：

```go
func (s *Session) dispatch(msg *ClientComMessage) {
    // 管理员专用消息检查
    if msg.AdminCmd != nil {
        if !s.isAdmin {
            s.queueOut(ErrPermissionDenied("", "", now))
            return
        }
        handler = s.handleAdminCommand
        msg.Id = msg.AdminCmd.Id
    }

    // ... 其他消息处理
}
```

---

## 场景三：修改现有消息处理

### 示例：为 {pub} 消息添加过期时间

假设我们要为消息添加自动过期功能。

### 步骤 1：扩展消息结构

**文件**: `server/datamodel.go`

```go
// MsgClientPub 是客户端发布消息的请求
type MsgClientPub struct {
    Id      string         `json:"id,omitempty"`
    Topic   string         `json:"topic"`
    NoEcho  bool           `json:"noecho,omitempty"`
    Head    map[string]any `json:"head,omitempty"`
    Content any            `json:"content"`

    // 新增：消息过期时间（秒）
    ExpiresAfter int `json:"expires_after,omitempty"`
}
```

### 步骤 2：修改 Topic 处理逻辑

**文件**: `server/topic.go`

```go
func (t *Topic) handlePublish(msg *ClientComMessage) {
    // ... 现有逻辑

    // 处理过期时间
    var expiresAt *time.Time
    if msg.Pub.ExpiresAfter > 0 {
        exp := time.Now().Add(time.Duration(msg.Pub.ExpiresAfter) * time.Second)
        expiresAt = &exp
    }

    // 存储消息时包含过期时间
    err := store.Messages.Create(&types.Message{
        // ... 其他字段
        ExpiresAt: expiresAt,
    })
}
```

### 步骤 3：添加过期清理任务

**文件**: `server/main.go`

```go
// 启动消息过期清理
func startMessageExpirationCleaner() {
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        for range ticker.C {
            store.Messages.DeleteExpired()
        }
    }()
}
```

### 步骤 4：数据库支持

**文件**: `server/db/mysql/messages.go`

```go
// Create 创建消息
func (m *MessageAdapter) Create(msg *types.Message) error {
    _, err := m.db.Exec(`
        INSERT INTO messages (id, topic, ` + "`from`" + `, head, content, expires_at)
        VALUES (?, ?, ?, ?, ?, ?)
    `, msg.Id, msg.Topic, msg.From, msg.Head, msg.Content, msg.ExpiresAt)
    return err
}

// DeleteExpired 删除过期消息
func (m *MessageAdapter) DeleteExpired() error {
    _, err := m.db.Exec(`
        DELETE FROM messages
        WHERE expires_at IS NOT NULL AND expires_at < NOW()
    `)
    return err
}
```

---

## 场景四：添加新的认证方式

### 示例：添加 OAuth 认证

### 步骤 1：实现认证处理器

**文件**: `server/auth/oauth.go`

```go
package auth

import (
    "context"
    "encoding/json"
    "net/http"
    "net/url"

    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
)

// OAuth 认证处理器
type OAuthAuthenticator struct {
    oauthConfig *oauth2.Config
}

func NewOAuthAuthenticator(clientID, clientSecret, redirectURL string) *OAuthAuthenticator {
    return &OAuthAuthenticator{
        oauthConfig: &oauth2.Config{
            ClientID:     clientID,
            ClientSecret: clientSecret,
            RedirectURL:  redirectURL,
            Scopes:       []string{"email", "profile"},
            Endpoint:     google.Endpoint,
        },
    }
}

// Authenticate 验证 OAuth 令牌
func (o *OAuthAuthenticator) Authenticate(secret []byte) (Uid, []byte, time.Duration, error) {
    // secret 是 OAuth authorization code
    code := string(secret)

    // 交换令牌
    token, err := o.oauthConfig.Exchange(context.Background(), code)
    if err != nil {
        return ZeroUid, nil, 0, err
    }

    // 获取用户信息
    client := o.oauthConfig.Client(context.Background(), token)
    resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
    if err != nil {
        return ZeroUid, nil, 0, err
    }
    defer resp.Body.Close()

    var userInfo struct {
        Email string `json:"email"`
        Name  string `json:"name"`
    }
    json.NewDecoder(resp.Body).Decode(&userInfo)

    // 查找或创建用户
    uid := findOrCreateUserByEmail(userInfo.Email, userInfo.Name)

    return uid, nil, 0, nil
}
```

### 步骤 2：注册认证器

**文件**: `server/auth/auth.go`

```go
func init() {
    // 注册 OAuth 认证器
    Register("oauth", func(params map[string]any) (Authenticator, error) {
        clientID, _ := params["client_id"].(string)
        clientSecret, _ := params["client_secret"].(string)
        redirectURL, _ := params["redirect_url"].(string)
        return NewOAuthAuthenticator(clientID, clientSecret, redirectURL), nil
    })
}
```

### 步骤 3：配置文件支持

**文件**: `server/tinode.conf`

```json
{
    "auth": {
        "oauth": {
            "enabled": true,
            "client_id": "your-client-id",
            "client_secret": "your-client-secret",
            "redirect_url": "https://your-app.com/oauth/callback"
        }
    }
}
```

### 步骤 4：前端使用

```javascript
// 前端发送登录请求
ws.send(JSON.stringify({
    login: {
        id: "login1",
        scheme: "oauth",
        secret: "authorization_code_from_provider"
    }
}));
```

---

## 测试指南

### 单元测试

```go
// server/session_test.go
func TestNewMessageType(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected *ClientComMessage
    }{
        {
            name:  "recall message",
            input: `{"recall": {"id": "1", "topic": "test", "seq": [1, 2]}}`,
            expected: &ClientComMessage{
                Recall: &MsgClientRecall{
                    Id:      "1",
                    Topic:   "test",
                    SeqList: []int{1, 2},
                },
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var msg ClientComMessage
            err := json.Unmarshal([]byte(tt.input), &msg)
            assert.NoError(t, err)
            assert.Equal(t, tt.expected.Recall, msg.Recall)
        })
    }
}
```

### 集成测试

```go
// server/integration_test.go
func TestWebSocketProtocol(t *testing.T) {
    // 1. 启动测试服务器
    srv := startTestServer()
    defer srv.Close()

    // 2. 连接 WebSocket
    ws := connectWebSocket(t, srv.URL)
    defer ws.Close()

    // 3. 测试消息流程
    testFullFlow(t, ws)
}

func testFullFlow(t *testing.T, ws *websocket.Conn) {
    // 发送 hi
    sendAndExpect(t, ws,
        `{"hi": {"id": "hi1", "ver": "0.22", "ua": "test"}}`,
        func(resp map[string]any) {
            assert.Equal(t, 200, int(resp["ctrl"].(map[string]any)["code"].(float64)))
        })

    // 发送 login
    sendAndExpect(t, ws,
        `{"login": {"id": "login1", "scheme": "basic", "secret": "dXNlcjpwYXNz"}}`,
        func(resp map[string]any) {
            assert.Equal(t, 200, int(resp["ctrl"].(map[string]any)["code"].(float64)))
        })
}
```

### 压力测试

```go
// server/benchmark_test.go
func BenchmarkMessageDispatch(b *testing.B) {
    sess := createTestSession()
    defer sess.cleanUp(false)

    raw := []byte(`{"pub": {"id": "1", "topic": "test", "content": "hello"}}`)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        sess.dispatchRaw(raw)
    }
}
```

---

## 调试技巧

### 1. 启用详细日志

```go
// 在 tinode.conf 中
{
    "log_level": "debug",
    "log_file": "/var/log/tinode/debug.log"
}
```

### 2. WebSocket 抓包

使用 Wireshark 或浏览器开发者工具：

```javascript
// 浏览器控制台
const ws = new WebSocket('ws://localhost:6060/v0/channels');

ws.onopen = () => {
    console.log('Connected');
    ws.send(JSON.stringify({hi: {ver: "0.22", ua: "debug"}}));
};

ws.onmessage = (e) => {
    console.log('Received:', JSON.parse(e.data));
};

// 发送测试消息
ws.send(JSON.stringify({
    recall: {id: "1", topic: "test", seq: [1]}
}));
```

### 3. 使用 wscat 工具

```bash
# 安装
npm install -g wscat

# 连接
wscat -c ws://localhost:6060/v0/channels

# 发送消息
> {"hi": {"ver": "0.22", "ua": "wscat"}}
< {"ctrl":{"code":200,"text":"ok",...}}

> {"recall": {"id": "1", "topic": "test", "seq": [1]}}
< {"ctrl":{"code":200,"text":"ok",...}}
```

### 4. 断点调试

在 VSCode 中设置断点：

```go
// 在 dispatchRaw 函数设置断点
func (s *Session) dispatchRaw(raw []byte) {
    logs.Debug.Printf("Received: %s", string(raw))  // 添加日志
    // ...
}
```

---

## 最佳实践

### 1. 向后兼容

添加新字段时使用 `omitempty`：

```go
type MsgClientPub struct {
    // ... 现有字段
    ExpiresAfter int `json:"expires_after,omitempty"`  // 可选字段
}
```

### 2. 版本检查

对于需要特定版本的功能：

```go
func (s *Session) recall(msg *ClientComMessage) {
    if s.ver < minVersionForRecall {
        s.queueOut(ErrUnsupportedVersion(msg.Id, msg.Original, msg.Timestamp))
        return
    }
    // ...
}
```

### 3. 权限控制

始终检查用户权限：

```go
func (s *Session) recall(msg *ClientComMessage) {
    // 检查用户是否已认证
    if msg.AsUser == "" {
        s.queueOut(ErrAuthRequiredReply(msg, msg.Timestamp))
        return
    }

    // 检查话题权限
    modeWant, modeGiven := t.getPerUserAcs(uid)
    if !(modeGiven & modeWant).IsWriter() {
        s.queueOut(ErrPermissionDeniedReply(msg, msg.Timestamp))
        return
    }
}
```

### 4. 错误处理

提供有意义的错误信息：

```go
func (s *Session) recall(msg *ClientComMessage) {
    if len(msg.Recall.SeqList) == 0 {
        s.queueOut(&ServerComMessage{
            Ctrl: &MsgServerCtrl{
                Id:    msg.Id,
                Code:  400,
                Text:  "No message IDs provided",
                Topic: msg.Original,
            },
        })
        return
    }
}
```

### 5. 单元测试覆盖

确保新功能有测试覆盖：

```go
func TestRecallValidation(t *testing.T) {
    tests := []struct {
        name    string
        msg     *MsgClientRecall
        wantErr bool
    }{
        {"empty seq list", &MsgClientRecall{SeqList: []int{}}, true},
        {"valid seq list", &MsgClientRecall{SeqList: []int{1, 2}}, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateRecallMessage(tt.msg)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

---

## 常见问题

### Q1: 如何处理大消息？

使用消息分片或文件上传：

```go
// 大消息应该使用文件上传
if len(content) > maxInlineMessageSize {
    // 上传到文件服务
    url, err := uploadFile(content)
    // 发送文件引用
    sendFileReference(url)
}
```

### Q2: 如何支持消息压缩？

启用 WebSocket 压缩：

```go
var upgrader = websocket.Upgrader{
    EnableCompression: true,  // 启用压缩
}
```

### Q3: 如何处理客户端断线重连？

使用会话恢复：

```javascript
// 客户端重连时提供之前的 sid
ws.send(JSON.stringify({
    hi: {
        ver: "0.22",
        ua: "client",
        device_id: "unique-device-id"  // 用于恢复会话
    }
}));
```

### Q4: 如何支持集群部署？

使用 Hub 的集群功能：

```go
// 配置集群
{
    "cluster": {
        "enabled": true,
        "nodes": ["node1:6060", "node2:6060"]
    }
}
```

---

## 总结

扩展 WebSocket 协议的核心步骤：

1. **定义数据结构** - 在 `datamodel.go` 中添加
2. **实现处理逻辑** - 在 `session.go` 中添加处理器
3. **注册消息路由** - 在 `dispatch` 函数中注册
4. **实现业务逻辑** - 在 `topic.go` 中实现
5. **同步 gRPC** - 如果需要，更新 `model.proto`
6. **编写测试** - 确保功能正确
7. **文档更新** - 更新 API 文档

遵循这些步骤，可以安全地扩展 Tinode 的 WebSocket 协议，同时保持向后兼容和系统稳定性。