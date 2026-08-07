# Tinode Redis 改造方案

## Context

Tinode 当前是单体应用，所有状态（Topic、Session、perUser map、sessions map）都保存在进程内存中。万人群场景下，`broadcastToSessions` 遍历 `t.sessions` map 向所有 session 写入 channel，O(n) 扇出导致 CPU 和内存瓶颈。

**目标**：将内部缓存和广播层迁移到 Redis，使单机可承载万人群。

---

## 改造范围

### 1. 新增 Redis 基础设施

**新增文件**：
- `server/redis/client.go` — Redis 连接池初始化、配置解析
- `server/redis/pubsub.go` — Redis Pub/Sub 订阅/发布封装
- `server/redis/cache.go` — perUser/sessions 状态缓存封装

**依赖**：在 `go.mod` 中添加 `github.com/redis/go-redis/v9`

**配置**（`tinode.conf` 新增 `redis` 段）：
```json
"redis": {
    "addr": "localhost:6379",
    "password": "",
    "db": 0,
    "pool_size": 100,
    "enabled": true
}
```

### 2. 将 perUser map 迁移到 Redis Hash

**现状**：
- `Topic.perUser map[types.Uid]perUserData` — 存每个订阅者的在线数、readID、recvID、权限等
- 每次 `saveAndBroadcastMessage` 写回 `t.perUser[asUid] = pud`（L1042-1045）
- 大量 `t.perUser[uid]` 读/写散落在 topic.go 各处（50+ 处）

**改造**：
- `perUserData` 序列化为 JSON，存入 Redis Hash：`topic:{topicName}:peruser`，field 为 `uid`
- Topic 结构体中保留 `perUser map` 作为热数据缓存（LRU），冷数据走 Redis
- `getPerUserAcs`、`userIsReader` 等方法优先查本地缓存，未命中再查 Redis
- 关键写操作（如 `saveAndBroadcastMessage` 中的 `pud.readID/pud.recvID` 更新）同步写 Redis

### 3. 将 sessions map 迁移到 Redis Set

**现状**：
- `Topic.sessions map[*Session]perSessionData` — 存当前连接到该 topic 的所有 session
- `broadcastToSessions` 遍历该 map 发消息（L1331-1404）
- `registerSession` / `unregisterSession` 增删 entry

**改造**：
- 不再用 `map[*Session]`（因为 *Session 是进程内指针，无法跨进程共享）
- 改用 Redis Set 存 session 列表：`topic:{topicName}:sessions`，member 为 `sid`
- 每个 session 的信息单独存 Hash：`session:{sid}`，存 uid、isChanSub、muids 等
- Topic 结构体中保留 `sessions map` 作为当前节点的本地连接视图（不含远程 session）

### 4. 消息广播改为 Redis Pub/Sub

**核心改造**：替换 `broadcastToSessions` 函数（L1326-1412）

**现状流程**：
```
saveAndBroadcastMessage()
  → store.Messages.Save() (写 DB)
  → broadcastToSessions()
      → for sess := range t.sessions { sess.queueOut(msgCopy) }
```

**改造后流程**：
```
saveAndBroadcastMessage()
  → store.Messages.Save() (写 DB)
  → publishToRedis(topicName, msg)
      → Redis Publish(topic:{topicName}:broadcast, serializedMsg)

同时每个 Topic 启动时：
  → Redis Subscribe(topic:{topicName}:broadcast)
      → 收到消息 → 只发给当前节点 attached 的 sessions
```

**关键设计**：
- 每个 Topic 在 `runLocal` 中增加一个 goroutine 监听 Redis Pub/Sub channel
- Pub channel 命名：`topic:{topicName}:broadcast`
- 消息序列化：JSON（与现有 WebSocket 协议一致）
- 收到 Pub/Sub 消息后，只遍历**当前节点**的 `t.sessions`（大幅减少遍历量）
- 如果万人群 10,000 人分布在 5 个节点，每个节点只需遍历 2,000 个 session

### 5. 修改 Hub 路由

**现状**：
- `hub.routeCli` 和 `hub.routeSrv` 通过 `h.topicGet()` 找本地 topic 发消息
- 如果 topic 不在本地内存，消息丢弃

**改造**：
- `topicGet` 未找到时，不立即丢弃，而是通过 Redis Pub/Sub 转发
- 新增 `hub.routeRedis` channel，用于接收 Redis Pub/Sub 转回本地的消息

### 6. 修改 Session 注册/注销

**现状**：
- `registerSession` → `t.sessions[s] = pssd`
- `unregisterSession` → `delete(t.sessions, s)`

**改造**：
- 本地操作不变（保持当前节点连接跟踪）
- 额外同步到 Redis Set：`SADD topic:{topicName}:sessions "{sid}"`
- 注销时：`SREM topic:{topicName}:sessions "{sid}"`
- Redis 中存 session 元数据用于其他节点查询

### 7. 修改 Presence 通知

**现状**：
- `presSubsOnline`、`presSubsOffline` 等函数通过内存 map 推送在线/离线通知

**改造**：
- 在线/离线状态存入 Redis Hash：`topic:{topicName}:peruser` 中的 `online` 字段
- Presence 通知通过 Redis Pub/Sub 广播

---

## 关键文件清单

| 文件 | 改动类型 | 说明 |
|------|----------|------|
| `server/redis/client.go` | **新增** | Redis 连接管理 |
| `server/redis/pubsub.go` | **新增** | Pub/Sub 封装 |
| `server/redis/cache.go` | **新增** | perUser/session 缓存封装 |
| `server/topic.go` | **修改** | perUser/sessions 访问走 Redis、broadcastToSessions 改造 |
| `session/session.go` | **修改** | session 注册时同步 Redis |
| `server/hub.go` | **修改** | topicGet 未命中时走 Redis 转发 |
| `server/main.go` | **修改** | globals 加 redisClient、启动 Redis 连接 |
| `server/tinode.conf` | **修改** | 新增 redis 配置段 |
| `server/globals.go` 或 `server/main.go` | **修改** | globals 结构体加 redis 字段 |
| `go.mod` | **修改** | 添加 go-redis 依赖 |

---

## 实施步骤

### Phase 1: 基础设施（不改变行为）
1. 添加 `go-redis/v9` 依赖
2. 创建 `server/redis/` 包：连接池、配置解析
3. `main.go` 初始化 Redis 连接，注册到 globals
4. 配置段添加到 `tinode.conf`
5. 编译通过、连接测试

### Phase 2: perUser 缓存 Redis 化
6. 创建 `server/redis/cache.go`，封装 perUser Hash 操作
7. 在 Topic 中加一层：本地 map 保留（热缓存），所有读写同步到 Redis
8. `getPerUserAcs`、`userIsReader` 等方法先查本地，未命中查 Redis
9. `saveAndBroadcastMessage` 中的 perUser 更新同步写 Redis
10. 测试：单节点功能正常

### Phase 3: sessions 注册 Redis 同步
11. `registerSession` 时 `SADD` 到 Redis Set
12. `unregisterSession` 时 `SREM` 从 Redis Set
13. session 元数据 Hash 存取
14. 测试：多连接、断连、重连

### Phase 4: Redis Pub/Sub 广播
15. 重写 `broadcastToSessions`：改为 `Redis Publish`
16. Topic `runLocal` 中新增 goroutine 监听 Redis Pub/Sub channel
17. 收到 Pub/Sub 消息后过滤并分发给本地 sessions
18. 去掉 `for sess := range t.sessions` 的全量遍历
19. 测试：万人群消息投递

### Phase 5: Hub 路由和 Presence
20. Hub routeCli/routeSrv 未命中时通过 Redis 转发
21. Presence 通知走 Redis Pub/Sub
22. 全量测试

---

## 验证方案

1. **单元测试**：对 `server/redis/` 包编写测试
2. **集成测试**：启动 Redis + Tinode，用 WebSocket 客户端连接验证
3. **压测**：用 `loadtest/` 工具模拟万人群，对比改造前后 CPU/内存/延迟
4. **功能回归**：运行现有 `go test ./server/...` 确保全部通过
5. **多节点验证**：启动 2+ 节点共享同一个 Redis，验证跨节点消息投递
