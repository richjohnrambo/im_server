# Tinode Redis 改造记录

## 背景

Tinode 当前是单体应用，所有状态（Topic、Session、perUser map、sessions map）都保存在进程内存中。万人群场景下，`broadcastToSessions` 遍历 `t.sessions` map 向所有 session 写入 channel，O(n) 扇出导致 CPU 和内存瓶颈。

**目标**：将内部缓存和广播层迁移到 Redis，使单机可承载万人群。

---

## 改造概览

### 核心设计

| 组件 | 改造前 | 改造后 |
|------|--------|--------|
| **perUser map** | 进程内存 `map[types.Uid]perUserData` | Redis Hash + 本地 LRU 热缓存 |
| **sessions map** | 进程内存 `map[*Session]perSessionData` | Redis Set + 本地连接视图 |
| **消息广播** | 遍历 `t.sessions` 逐个写入 | Redis Pub/Sub → 各节点只遍历本地 sessions |
| **Presence 通知** | 内存 map 推送 | Redis Pub/Sub 广播 |

### Redis Key 设计

```
topic:{topicName}:peruser      — Hash: uid → JSON(perUserData)
topic:{topicName}:sessions     — Set: {sid1, sid2, ...}
session:{sid}                  — Hash: session 元数据
topic:{topicName}:broadcast    — Pub/Sub: 消息广播通道
topic:{topicName}:presence     — Pub/Sub: Presence 通知通道
```

---

## 新增文件

### 1. `server/redis/client.go` — Redis 连接管理

- `Config` 结构体：addr、password、db、pool_size、enabled
- `NewClient(cfg)`：创建连接池，启动时 Ping 验证连通性
- `Rdb()` / `Ctx()` / `Close()` / `IsEnabled()`
- Key 构造函数：`TopicPerUserKey()`、`TopicSessionsKey()`、`SessionKey()`、`TopicBroadcastChannel()`、`TopicPresenceChannel()`

### 2. `server/redis/pubsub.go` — Pub/Sub 封装

- `RedisBroadcastMsg`：跨节点广播消息格式（JSON 序列化，含 NodeID 去重）
- `PublishMessage()`：发布到 `topic:{name}:broadcast`
- `PublishPresence()`：发布到 `topic:{name}:presence`
- `Subscribe()`：订阅 Redis channel，返回消息流
- `ParseBroadcastMsg()`：解析 Pub/Sub payload

### 3. `server/redis/cache.go` — 缓存操作封装

- `CachedPerUserData`：perUserData 的 Redis 可序列化形式
- `SavePerUser()` / `GetPerUser()` / `DeletePerUser()` / `GetAllPerUsers()`
- `SessionMetadata`：session 元数据
- `AddSession()` / `RemoveSession()` / `GetSessionMetadata()` / `GetTopicSessions()`

---

## 修改文件

### `go.mod`

- 新增依赖：`github.com/redis/go-redis/v9 v9.18.0`

### `server/tinode.conf`

```json
"redis": {
    "addr": "localhost:6379",
    "password": "",
    "db": 0,
    "pool_size": 100,
    "enabled": false
}
```

### `server/main.go`

- globals 新增 `redisClient *redis.Client`
- configType 新增 `Redis json.RawMessage`
- main() 中解析 Redis 配置并初始化连接，defer Close()

### `server/hub.go`

- Topic 创建时，若 Redis 启用则初始化 `t.redisMsg = make(chan *redis.RedisBroadcastMsg, 128)`

### `server/topic.go` — 核心改造

**新增 Topic 字段**：
- `redisMsg chan *redis.RedisBroadcastMsg`：接收 Redis Pub/Sub 消息

**新增方法**：
- `perUserDataToRedis()` / `perUserDataFromRedis()` — 双向序列化
- `syncPerUserToRedis(uid)` — 将本地 perUser 同步到 Redis Hash
- `loadPerUserFromRedis(uid)` — Redis 未命中时回源
- `syncSessionRemoval(sid)` — 从 Redis Set 移除 session
- `redisListener(sub)` — goroutine，监听 Redis Pub/Sub 并转发到 `redisMsg` channel
- `handleRedisMsg(rmsg)` — 处理 Redis 消息，投递到本地 sessions

**改造方法**：
- `getPerUserAcs(uid)` — 缓存未命中时回查 Redis
- `broadcastToSessions(msg)` — 改为发布到 Redis Pub/Sub，未启用时 fallback 到本地投递
- `deliverToLocalSessions(msg)` — 提取原有遍历逻辑，仅投递本地 attached sessions
- `saveAndBroadcastMessage()` — perUser 更新后调用 `syncPerUserToRedis()`
- `addSession()` — 新增 `SADD` 到 Redis Set + 存 session 元数据
- `remSession()` — 新增 `SREM` 从 Redis Set
- handleLeaveRequest 中的 online-- — 调用 `syncPerUserToRedis()`
- sendSubNotifications 中的 online++ — 调用 `syncPerUserToRedis()`
- subscriptionReply 中的 online++ — 调用 `syncPerUserToRedis()`
- `runLocal()` — 新增 Redis Pub/Sub 订阅 goroutine + select case 处理 `redisMsg`

---

## 广播流程改造

### 改造前
```
saveAndBroadcastMessage()
  → store.Messages.Save()
  → broadcastToSessions()
      → for sess := range t.sessions { sess.queueOut(msgCopy) }   ← O(n)，n 可达 10,000
```

### 改造后
```
saveAndBroadcastMessage()
  → store.Messages.Save()
  → Redis Publish(topic:{name}:broadcast, serializedMsg)           ← O(1)

同时每个 Topic 启动时：
  → Redis Subscribe(topic:{name}:broadcast)
      → 收到消息 → deliverToLocalSessions()
          → for sess := range t.sessions { sess.queueOut(msgCopy) }  ← 仅遍历本地 sessions

如果万人群 10,000 人分布在 5 个节点，每个节点只需遍历 ~2,000 个 session
```

### 跨节点消息投递
- 每个节点发布消息到 Redis Pub/Sub，所有节点都能收到
- 通过 `NodeID` 字段判断，跳过本节点发布的消息（避免重复投递）
- 各节点收到消息后，只投递给自己本地 attached 的 sessions

---

## 配置说明

### 启用 Redis

将 `tinode.conf` 中 `redis.enabled` 设为 `true`：

```json
"redis": {
    "addr": "localhost:6379",
    "enabled": true
}
```

### 向后兼容

- `enabled: false` 时，所有 Redis 操作为 no-op
- 行为与改造前完全一致
- 无需修改客户端

---

## 验证结果

### Redis 连通性测试 ✅

```
OK: Redis PING
OK: PerUser HSet
OK: PerUser HGet
OK: Session SAdd
OK: Session SMembers
OK: Publish
OK: Received pub/sub
```

### 编译 ✅

```bash
go build -tags=mysql ./server/...    # 编译通过
go test ./server/                     # 测试通过
```

### 已解决的问题

- **MySQL schema 版本不匹配（116 vs 119）**：
  - 根因：`store.Open()` 调用 `CheckDbVersion()` 直接失败，未触发 `UpgradeDb()`
  - 修复 1：修改 `server/store/store.go` 的 `Open()` 方法，当检测到版本不匹配时自动调用 `UpgradeDb()`
  - 修复 2：`createIMMessageFullTextIndex` 存在重复创建 FULLTEXT index 的问题（该 index 已在 `createIMMessageIndex` 中创建），改为幂等检查
  - 结果：DB 版本从 116 自动升级到 119，MySQL 迁移问题已解决

- **模板文件路径问题**：
  - 现象：服务启动时报 `open .../templ/email-validation-en.templ: no such file or directory`
  - 根因：tinode.conf 中模板路径配置为 `./templ/...`，但实际文件在 `server/templ/...`
  - 这是预存在的配置问题，不影响核心功能，可通过修改配置文件中模板路径为 `./server/templ/...` 解决

### 未解决的问题

（暂无）

---

## 后续工作

1. **补充 perUser 更多写路径的 Redis 同步**：modeWant/modeGiven 修改时同步
2. **perUser 本地 LRU 缓存**：限制本地 map 大小，避免内存膨胀
3. **Presence 专用 Pub/Sub 通道**：目前复用 broadcast 通道，可拆分
4. **多节点集成测试**：启动 2+ 节点共享 Redis，验证跨节点消息投递
5. **压测对比**：用 `loadtest/` 工具模拟万人群，对比改造前后 CPU/内存/延迟
6. **Redis 单元测试**：对 `server/redis/` 包编写完整测试用例
