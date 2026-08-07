# Tinode 消息存储机制详解

> 本文档详细介绍 Tinode 消息的存储机制，包括数据结构、存储流程、数据库 Schema 和查询方式。

---

## 目录

1. [存储架构概览](#存储架构概览)
2. [数据库表结构](#数据库表结构)
3. [消息数据结构](#消息数据结构)
4. [消息存储流程](#消息存储流程)
5. [消息内容格式](#消息内容格式)
6. [消息序号机制](#消息序号机制)
7. [消息删除机制](#消息删除机制)
8. [消息查询](#消息查询)
9. [各数据库实现差异](#各数据库实现差异)

---

## 存储架构概览

### 整体架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              应用层                                          │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  Topic.handlePublish() -> 消息处理入口                               │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              存储抽象层                                      │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  store.Messages.Save() -> 消息保存接口                               │   │
│   │  store.Messages.GetAll() -> 消息查询接口                             │   │
│   │  store.Messages.DeleteList() -> 消息删除接口                         │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              数据库适配层                                    │
│   ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌─────────┐                   │
│   │  MySQL  │   │PostgreSQL│   │ MongoDB │   │RethinkDB│                   │
│   │ adapter │   │ adapter │   │ adapter │   │ adapter │                   │
│   └─────────┘   └─────────┘   └─────────┘   └─────────┘                   │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              数据库层                                        │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  messages 表/集合 - 存储消息内容                                     │   │
│   │  dellog 表/集合 - 记录删除操作                                       │   │
│   │  topics 表/集合 - 存储话题元数据                                     │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 相关文件

| 文件 | 作用 |
|------|------|
| `server/store/types/types.go` | 消息数据结构定义 |
| `server/store/store.go` | 存储抽象层实现 |
| `server/db/adapter.go` | 数据库适配器接口 |
| `server/db/mysql/adapter.go` | MySQL 实现 |
| `server/db/postgres/adapter.go` | PostgreSQL 实现 |
| `server/db/mongodb/adapter.go` | MongoDB 实现 |
| `server/db/mysql/schema.sql` | MySQL 表结构 |
| `server/topic.go` | 消息处理逻辑 |

---

## 数据库表结构

### MySQL Schema

```sql
-- 消息表
CREATE TABLE messages(
    id          INT NOT NULL AUTO_INCREMENT,   -- 自增主键
    createdat   DATETIME(3) NOT NULL,          -- 创建时间（毫秒精度）
    updatedat   DATETIME(3) NOT NULL,          -- 更新时间
    deletedat   DATETIME(3),                    -- 删除时间（软删除）
    delid       INT DEFAULT 0,                  -- 删除操作ID
    seqid       INT NOT NULL,                  -- 消息序号（Topic内递增）
    topic       CHAR(25) NOT NULL,             -- Topic名称（如 grpXXX）
    `from`      BIGINT NOT NULL,               -- 发送者用户ID（数值）
    head        JSON,                          -- 消息头（元数据）
    content     JSON,                          -- 消息内容（JSON格式）

    PRIMARY KEY(id),
    UNIQUE INDEX messages_topic_seqid (topic, seqid)
);

-- 删除日志表（记录软删除）
CREATE TABLE dellog(
    id          INT NOT NULL AUTO_INCREMENT,
    topic       CHAR(25) NOT NULL,             -- Topic名称
    deletedfor  BIGINT NOT NULL DEFAULT 0,     -- 删除目标用户ID
    delid       INT NOT NULL,                  -- 删除操作ID
    low         INT NOT NULL,                  -- 删除范围起始seqid
    hi          INT NOT NULL,                  -- 删除范围结束seqid（不含）

    PRIMARY KEY(id),
    FOREIGN KEY(topic) REFERENCES topics(name),
    INDEX dellog_topic_delid_deletedfor(topic, delid, deletedfor),
    INDEX dellog_topic_deletedfor_low_hi(topic, deletedfor, low, hi)
);
```

### PostgreSQL Schema

```sql
CREATE TABLE messages(
    id          SERIAL PRIMARY KEY,
    createdat   TIMESTAMP(3) NOT NULL,
    updatedat   TIMESTAMP(3) NOT NULL,
    deletedat   TIMESTAMP(3),
    delid       INT DEFAULT 0,
    seqid       INT NOT NULL,
    topic       CHAR(25) NOT NULL,
    "from"      BIGINT NOT NULL,
    head        JSONB,
    content     JSONB,

    UNIQUE INDEX messages_topic_seqid (topic, seqid)
);
```

### MongoDB Schema

```javascript
// messages 集合
{
    "_id": ObjectId("..."),           // MongoDB ObjectId
    "createdat": ISODate("..."),
    "updatedat": ISODate("..."),
    "deletedat": ISODate("..."),
    "delid": NumberInt(0),
    "seqid": NumberInt(1),
    "topic": "grpABC123",
    "from": "usrXYZ456",
    "head": {
        "mime": "text/x-drafty",
        "attachments": ["file123"]
    },
    "content": {
        "txt": "Hello World",
        "fmt": [...]
    }
}

// 索引
db.messages.createIndex({ "topic": 1, "seqid": 1 }, { unique: true })
```

---

## 消息数据结构

### Go 结构体定义

```go
// server/store/types/types.go

// Message 消息结构体
type Message struct {
    ObjHeader `bson:",inline"`              // 基础头部（ID、时间戳）

    DeletedAt *time.Time `json:"DeletedAt,omitempty" bson:",omitempty"`
    DelId     int        `json:"DelId,omitempty" bson:",omitempty"`
    DeletedFor []SoftDelete `json:"DeletedFor,omitempty" bson:",omitempty"`

    SeqId     int       // 消息序号（Topic内唯一递增）
    Topic     string    // Topic名称
    From      string    // 发送者UID（不带 usr 前缀）
    Head      KVMap     // 消息头（键值对元数据）
    Content   any       // 消息内容（任意JSON结构）
}

// ObjHeader 基础头部
type ObjHeader struct {
    Id        string     `json:"id,omitempty" bson:"_id,omitempty"`
    CreatedAt time.Time  `json:"createdat,omitempty" bson:",omitempty"`
    UpdatedAt time.Time  `json:"updatedat,omitempty" bson:",omitempty"`
}

// SoftDelete 软删除记录
type SoftDelete struct {
    User    string     `json:"user"`
    SeqId   int        `json:"seqid,omitempty"`
}
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 全局唯一ID（Snowflake算法） |
| `createdat` | time.Time | 消息创建时间 |
| `updatedat` | time.Time | 消息更新时间（编辑时） |
| `deletedat` | *time.Time | 删除时间（硬删除时设置） |
| `delid` | int | 删除操作ID（用于追踪删除） |
| `seqid` | int | **消息序号**（Topic内递增，核心字段） |
| `topic` | string | Topic名称（如 `grpABC123`） |
| `from` | string | 发送者UID（如 `usrXYZ456`） |
| `head` | KVMap | 消息头（元数据，如 MIME 类型、附件等） |
| `content` | any | **消息内容**（任意 JSON 结构） |

---

## 消息存储流程

### 完整流程图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  1. 客户端发送消息                                                            │
│     WebSocket: {"pub", "topic":"grpXXX", "content":{"text":"Hello"}}        │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  2. WebSocket Handler 处理 (hdl_websock.go)                                  │
│     serveWebSocket() -> sess.readLoop() -> sess.dispatchRaw(raw)            │
│     解析 JSON，识别消息类型为 "pub"                                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  3. Session 消息分发 (session.go)                                            │
│     dispatchRaw() -> 构造 ClientComMessage -> 发送到 Hub                     │
│     msg := &ClientComMessage{                                                │
│         Pub: &MsgClientPub{Topic: "grpXXX", Content: ...}                   │
│     }                                                                        │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  4. Hub 路由分发 (hub.go)                                                    │
│     hub.routeCli <- msg                                                      │
│     hub.routeClientMessage(msg) -> 根据 msg.RcptTo 找到 Topic               │
│     topic.clientMsg <- msg  // 发送到 Topic 的消息通道                       │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  5. Topic 处理消息 (topic.go)                                                │
│     select {                                                                 │
│         case msg := <-t.clientMsg:                                           │
│             t.processClientMessage(msg)                                      │
│     }                                                                        │
│     -> t.handlePublish(msg)                                                  │
│     -> t.saveAndBroadcastMessage(msg, ...)                                   │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  6. 保存前检查 (topic.go:saveAndBroadcastMessage)                            │
│     a. 检查发送者是否有写权限 (IsWriter)                                      │
│     b. 处理消息头 (sender、attachments)                                       │
│     c. 构造 types.Message 对象                                               │
│        SeqId = t.lastID + 1  // 序号递增                                     │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  7. 存储层保存 (store/store.go:Messages.Save)                                │
│     a. msg.InitTimes()  // 初始化时间戳                                       │
│     b. msg.SetUid(Store.GetUid())  // 生成全局唯一ID                          │
│     c. adp.TopicUpdateOnMessage(msg.Topic, msg)  // 更新Topic的seqid         │
│     d. adp.MessageSave(msg)  // 插入消息到数据库                              │
│     e. adp.SubsUpdate()  // 更新发送者的 ReadSeqId/RecvSeqId                 │
│     f. adp.FileLinkAttachments()  // 关联附件文件                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  8. 数据库写入 (db/mysql/adapter.go:MessageSave)                              │
│     INSERT INTO messages(createdAt, updatedAt, seqid, topic, `from`,        │
│                          head, content)                                       │
│     VALUES(?, ?, ?, ?, ?, ?, ?)                                               │
│                                                                              │
│     参数:                                                                     │
│     - createdAt: msg.CreatedAt                                               │
│     - updatedAt: msg.UpdatedAt                                               │
│     - seqid: msg.SeqId (t.lastID + 1)                                        │
│     - topic: msg.Topic                                                       │
│     - from: store.DecodeUid(ParseUid(msg.From))                              │
│     - head: msg.Head (JSON)                                                  │
│     - content: ToJSON(msg.Content) (JSON)                                    │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  9. 更新 Topic 缓存 (topic.go)                                               │
│     t.lastID++  // 递增最后消息序号                                           │
│     t.touched = msg.Timestamp  // 更新最后消息时间                            │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  10. 广播给订阅者 (topic.go)                                                 │
│      for sess := range t.sessions {                                          │
│          sess.queueOut(&ServerComMessage{                                    │
│              Data: &MsgServerData{                                           │
│                  Topic:   t.name,                                            │
│                  SeqId:   t.lastID,                                          │
│                  From:    asUid.UserId(),                                     │
│                  Content: content,                                           │
│              },                                                               │
│          })                                                                   │
│      }                                                                        │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  11. 推送通知 (push/push.go)                                                 │
│      如果订阅者离线，发送 FCM 推送通知                                         │
│      push.Push(userId, &PushData{...})                                       │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 关键代码解析

#### 消息保存入口 (topic.go:999)

```go
func (t *Topic) saveAndBroadcastMessage(msg *ClientComMessage, asUid types.Uid,
    noEcho bool, attachments []string, head map[string]any, content any) error {

    pud, userFound := t.perUser[asUid]

    // 1. 权限检查
    if t.cat != types.TopicCatSys {
        if !(pud.modeWant & pud.modeGiven).IsWriter() {
            msg.sess.queueOut(ErrPermissionDenied(msg.Id, t.original(asUid), msg.Timestamp))
            return types.ErrPermissionDenied
        }
    }

    // 2. 处理 sender 头（代理发送）
    if msg.sess != nil && msg.sess.uid != asUid {
        if head == nil {
            head = map[string]any{}
        }
        head["sender"] = msg.sess.uid.UserId()
    }

    // 3. 构造消息对象并保存
    if err, _ := store.Messages.Save(
        &types.Message{
            ObjHeader: types.ObjHeader{CreatedAt: msg.Timestamp},
            SeqId:     t.lastID + 1,  // 关键：序号递增
            Topic:     t.name,
            From:      asUid.String(),
            Head:      head,
            Content:   content,
        }, attachments, (pud.modeGiven & pud.modeWant).IsReader()); err != nil {
        return err
    }

    // 4. 更新 Topic 状态
    t.lastID++

    // 5. 广播给订阅者
    // ...
}
```

#### 存储层实现 (store/store.go:682)

```go
func (messagesMapper) Save(msg *types.Message, attachmentURLs []string,
    readBySender bool) (error, bool) {

    // 1. 初始化时间戳
    msg.InitTimes()

    // 2. 生成全局唯一ID（Snowflake算法）
    msg.SetUid(Store.GetUid())

    // 3. 更新 Topic 的 SeqId（数据库层）
    err := adp.TopicUpdateOnMessage(msg.Topic, msg)
    if err != nil {
        return err, false
    }

    // 4. 插入消息到数据库
    err = adp.MessageSave(msg)
    if err != nil {
        return err, false
    }

    // 5. 标记发送者已读
    markedReadBySender := false
    if readBySender {
        fromUid := types.ParseUid(msg.From)
        if !fromUid.IsZero() {
            if err := adp.SubsUpdate(msg.Topic, fromUid,
                map[string]any{
                    "RecvSeqId": msg.SeqId,
                    "ReadSeqId": msg.SeqId,
                }); err == nil {
                markedReadBySender = true
            }
        }
    }

    // 6. 关联附件文件
    if len(attachmentURLs) > 0 {
        var attachments []string
        for _, url := range attachmentURLs {
            if fid := mediaHandler.GetIdFromUrl(url); !fid.IsZero() {
                attachments = append(attachments, fid.String())
            }
        }
        if len(attachments) > 0 {
            return adp.FileLinkAttachments("", types.ZeroUid, msg.Uid(), attachments), markedReadBySender
        }
    }

    return nil, markedReadBySender
}
```

#### MySQL 适配器实现 (db/mysql/adapter.go:2661)

```go
func (a *adapter) MessageSave(msg *t.Message) error {
    ctx, cancel := a.getContext()
    if cancel != nil {
        defer cancel()
    }

    // 执行插入
    res, err := a.db.ExecContext(ctx,
        `INSERT INTO messages(createdAt, updatedAt, seqid, topic, `+"`from`"+`, head, content)
         VALUES(?, ?, ?, ?, ?, ?, ?)`,
        msg.CreatedAt,       // createdat
        msg.UpdatedAt,       // updatedat
        msg.SeqId,           // seqid
        msg.Topic,           // topic
        store.DecodeUid(t.ParseUid(msg.From)),  // from (数值UID)
        msg.Head,            // head (JSON)
        common.ToJSON(msg.Content))             // content (JSON)

    return err
}
```

---

## 消息内容格式

### 1. 纯文本消息

```json
{
  "content": "Hello World"
}
```

### 2. Drafty 富文本消息

Drafty 是 Tinode 的富文本格式：

```json
{
  "content": {
    "txt": "Hello **World**",
    "fmt": [
      {"tp": "SB", "at": 6, "len": 7}
    ]
  }
}
```

**格式类型**:
| tp | 说明 |
|----|------|
| `ST` | 斜体 |
| `SB` | 粗体 |
| `CO` | 代码 |
| `BR` | 换行 |
| `LN` | 链接 |
| `MN` | 提及 (@用户) |
| `HT` | 隐藏文本 |

### 3. 带附件消息

```json
{
  "content": {
    "txt": "Check this image",
    "fmt": [
      {
        "tp": "IM",
        "at": 6,
        "len": 1,
        "url": "https://example.com/image.jpg",
        "mime": "image/jpeg",
        "size": 12345,
        "width": 800,
        "height": 600
      }
    ]
  }
}
```

### 4. 回复/转发消息

```json
{
  "content": {
    "txt": "Replying to...",
    "replyTo": {
      "seqid": 42,
      "from": "usrABC",
      "content": {"txt": "Original message"}
    }
  }
}
```

### 5. 消息头

消息头用于存储元数据：

```json
{
  "head": {
    "mime": "text/x-drafty",      // 内容类型
    "attachments": ["file123"],    // 附件ID列表
    "replyTo": 42,                 // 回复的消息序号
    "sender": "usrXYZ",            // 代理发送者（如果是代理发送）
    "replace": 41                  // 替换的消息序号（编辑消息）
  }
}
```

---

## 消息序号机制

### SeqId 的作用

`seqid` 是消息在 Topic 内的递增序号，是消息同步的核心：

```
Topic: grpABC123
┌────────┬──────────────┬─────────────────────────────────┐
│ seqid │    from      │ content                          │
├────────┼──────────────┼─────────────────────────────────┤
│   1    │ usrAlice     │ {"text": "Hello everyone"}       │
│   2    │ usrBob       │ {"text": "Hi Alice"}             │
│   3    │ usrCarol     │ {"text": "Hey all"}              │
│   4    │ usrAlice     │ {"text": "How are you?"}         │
│  ...   │ ...          │ ...                              │
└────────┴──────────────┴─────────────────────────────────┘
```

### SeqId 的用途

#### 1. 消息同步

客户端通过 `since` 和 `before` 参数拉取消息：

```json
// 客户端请求
{
  "get": {
    "topic": "grpABC123",
    "data": {
      "since": 2,
      "before": 5,
      "limit": 10
    }
  }
}

// 服务端返回 seqid >= 2 且 < 5 的消息
```

#### 2. 已读回执

订阅者表中记录已读位置：

```sql
-- subscriptions 表
recvseqid = 3  -- 已收到消息序号
readseqid = 2  -- 已读消息序号
```

```json
// 客户端发送已读通知
{
  "note": {
    "topic": "grpABC123",
    "what": "read",
    "seqid": 4
  }
}
```

#### 3. 未读消息计数

```go
unreadCount = topic.lastID - subscription.readSeqId
```

#### 4. 消息删除范围

```json
// 删除 seqid 3-5 的消息
{
  "del": {
    "topic": "grpABC123",
    "what": "data",
    "ranges": [{"low": 3, "hi": 6}]
  }
}
```

### SeqId 分配

```go
// topic.go
func (t *Topic) saveAndBroadcastMessage(...) {
    message.SeqId = t.lastID + 1  // Topic 内递增
    t.lastID++                     // 更新缓存
}

// 数据库层保证唯一性
UNIQUE INDEX messages_topic_seqid (topic, seqid)
```

---

## 消息删除机制

Tinode 支持两种删除方式：**软删除**和**硬删除**。

### 1. 软删除

只对当前用户隐藏消息，其他用户仍可见。

#### 数据库记录

```sql
-- dellog 表记录删除操作
INSERT INTO dellog(topic, deletedfor, delid, low, hi)
VALUES ('grpABC123', 123, 1, 3, 6);  -- 删除 seqid [3, 6) 的消息
```

#### 查询时过滤

```sql
-- 查询消息时排除已删除的
SELECT m.* FROM messages m
LEFT JOIN dellog d ON m.topic = d.topic
    AND m.seqid >= d.low
    AND m.seqid < d.hi
    AND d.deletedfor = ?
WHERE m.topic = ?
    AND (d.id IS NULL)  -- 未被软删除
ORDER BY m.seqid DESC
LIMIT ?
```

#### 软删除流程

```
用户 A 删除消息
        │
        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  1. 检查权限                                                                 │
│     - 用户是否有删除权限（D权限）                                             │
│     - 消息是否过期（msg_delete_age 配置）                                    │
└─────────────────────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  2. 生成删除记录                                                             │
│     dellog{                                                                  │
│         topic: "grpABC123",                                                  │
│         deletedfor: usrA,                                                    │
│         delid: topic.delID++,                                                │
│         low: 3, hi: 6                                                        │
│     }                                                                        │
└─────────────────────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  3. 写入数据库                                                               │
│     INSERT INTO dellog(topic, deletedfor, delid, low, hi)                   │
│     更新 subscription.delId                                                  │
└─────────────────────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  4. 广播删除通知                                                             │
│     {pres: "del", seqid: 3, delid: 1}                                       │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2. 硬删除

物理删除消息，所有用户都不可见。

#### 数据库更新

```sql
-- 标记消息为已删除
UPDATE messages
SET deletedat = NOW(), delid = ?
WHERE topic = ? AND seqid BETWEEN ? AND ?;
```

#### 硬删除条件

只有以下情况可以硬删除：
- Topic 所有者
- 消息发送者（在一定时间内）
- `forUser` 为零（管理员操作）

### 3. 删除范围

删除操作支持范围：

```json
// 删除单条消息
{
  "del": {
    "topic": "grpABC123",
    "what": "data",
    "ranges": [{"low": 5, "hi": 0}]  // hi=0 表示单条
  }
}

// 删除范围消息
{
  "del": {
    "topic": "grpABC123",
    "what": "data",
    "ranges": [
      {"low": 1, "hi": 5},   // 删除 seqid [1, 5)
      {"low": 10, "hi": 15}  // 删除 seqid [10, 15)
    ]
  }
}

// 删除所有消息
{
  "del": {
    "topic": "grpABC123",
    "what": "data",
    "ranges": [{"low": 1, "hi": 2147483647}]  // 删除全部
  }
}
```

---

## 消息查询

### 查询接口

```go
// store/store.go
func (messagesMapper) GetAll(topic string, forUser types.Uid,
    opt *types.QueryOpt) ([]types.Message, error)
```

### 查询参数

```go
type QueryOpt struct {
    Since      int         // 起始 seqid（包含）
    Before     int         // 结束 seqid（不包含）
    Limit      int         // 返回数量限制
    IdRanges   []Range     // 指定范围列表
    IfModifiedSince *time.Time  // 增量查询时间戳
}
```

### 查询示例

#### 1. 拉取最新消息

```json
{
  "get": {
    "topic": "grpABC123",
    "data": {
      "limit": 20
    }
  }
}
```

```sql
SELECT * FROM messages
WHERE topic = 'grpABC123'
ORDER BY seqid DESC
LIMIT 20;
```

#### 2. 拉取历史消息

```json
{
  "get": {
    "topic": "grpABC123",
    "data": {
      "before": 50,
      "limit": 20
    }
  }
}
```

```sql
SELECT * FROM messages
WHERE topic = 'grpABC123' AND seqid < 50
ORDER BY seqid DESC
LIMIT 20;
```

#### 3. 增量同步

```json
{
  "get": {
    "topic": "grpABC123",
    "data": {
      "since": 100,
      "limit": 50
    }
  }
}
```

```sql
SELECT * FROM messages
WHERE topic = 'grpABC123' AND seqid >= 100
ORDER BY seqid ASC
LIMIT 50;
```

#### 4. 按范围查询

```json
{
  "get": {
    "topic": "grpABC123",
    "data": {
      "ranges": [
        {"low": 10, "hi": 15},
        {"low": 20, "hi": 25}
      ]
    }
  }
}
```

```sql
SELECT * FROM messages
WHERE topic = 'grpABC123'
  AND ((seqid >= 10 AND seqid < 15)
    OR (seqid >= 20 AND seqid < 25))
ORDER BY seqid;
```

---

## 各数据库实现差异

### 存储方式对比

| 特性 | MySQL | PostgreSQL | MongoDB | RethinkDB |
|------|-------|------------|---------|-----------|
| 内容类型 | JSON | JSONB | BSON | 原生JSON |
| 主键 | AUTO_INCREMENT | SERIAL | ObjectId | UUID |
| 索引 | B+Tree | B+Tree | B+Tree | B+Tree |
| 事务 | 支持 | 支持 | 支持 | 支持 |
| 分布式 | 否 | 否 | 分片 | 分片 |

### MySQL 适配器

```go
// db/mysql/adapter.go
func (a *adapter) MessageSave(msg *t.Message) error {
    _, err := a.db.ExecContext(ctx,
        `INSERT INTO messages(createdAt, updatedAt, seqid, topic, `+"`from`"+`, head, content)
         VALUES(?, ?, ?, ?, ?, ?, ?)`,
        msg.CreatedAt, msg.UpdatedAt, msg.SeqId, msg.Topic,
        store.DecodeUid(t.ParseUid(msg.From)),
        msg.Head,
        common.ToJSON(msg.Content))
    return err
}
```

### PostgreSQL 适配器

```go
// db/postgres/adapter.go
func (a *adapter) MessageSave(msg *t.Message) error {
    var id int
    err := a.db.QueryRow(ctx,
        `INSERT INTO messages(createdAt, updatedAt, seqid, topic, "from", head, content)
         VALUES($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
        msg.CreatedAt, msg.UpdatedAt, msg.SeqId, msg.Topic,
        store.DecodeUid(t.ParseUid(msg.From)),
        msg.Head,
        common.ToJSON(msg.Content)).Scan(&id)
    return err
}
```

### MongoDB 适配器

```go
// db/mongodb/adapter.go
func (a *adapter) MessageSave(msg *t.Message) error {
    // 直接插入整个结构体
    _, err := a.db.Collection("messages").InsertOne(a.ctx, msg)
    return err
}
```

### 性能优化

#### 索引设计

```sql
-- MySQL/PostgreSQL
CREATE UNIQUE INDEX messages_topic_seqid ON messages(topic, seqid);
CREATE INDEX messages_topic_created ON messages(topic, createdat);
```

```javascript
// MongoDB
db.messages.createIndex({ "topic": 1, "seqid": 1 }, { unique: true })
db.messages.createIndex({ "topic": 1, "createdat": -1 })
```

#### 分区策略

对于大流量场景，可以考虑按 Topic 分区：

```sql
-- MySQL 分区（按 topic hash）
CREATE TABLE messages (
    ...
) PARTITION BY HASH(topic) PARTITIONS 16;
```

---

## 常见问题

### 1. 消息 ID vs SeqId 的区别？

- **id**: 全局唯一，使用 Snowflake 算法生成，用于数据库主键
- **seqid**: Topic 内唯一递增，用于消息同步、已读回执、删除范围

### 2. 如何实现消息编辑？

编辑消息实际上是创建新消息并关联原消息：

```json
{
  "pub": {
    "topic": "grpABC123",
    "content": {"txt": "Edited content"},
    "head": {
      "replace": 42  // 替换 seqid=42 的消息
    }
  }
}
```

### 3. 如何实现消息撤回？

在一定时间内（`msg_delete_age` 配置）可以撤回：

```json
{
  "del": {
    "topic": "grpABC123",
    "what": "data",
    "ranges": [{"low": 42, "hi": 0}]
  }
}
```

### 4. 消息存储限制？

- 最大消息大小：由 `max_message_size` 配置（默认 128KB）
- 大文件通过附件存储，消息中只保留引用

---

## 总结

Tinode 消息存储的核心设计：

1. **JSON 格式存储** - 灵活支持各种消息类型
2. **SeqId 递增** - 每个 Topic 内消息有序，便于同步
3. **Topic 分片** - 消息按 Topic 分组，便于查询
4. **软/硬删除** - 支持两种删除策略
5. **多数据库支持** - 统一接口，适配多种数据库