# Tinode 承载量与可扩展性分析

## 1. 承载量 / 并发量

**官方基准测试**（AWS t3.xlarge, 4 vCPU, 16GB RAM）：

| 指标 | 数值 |
|------|------|
| 总并发 WebSocket 连接 | **50,000** |
| 单话题并发会话 | **1,500** |

**单机理论上限**：
- Go goroutine 模型，每连接约 2-4 KB 栈
- 16GB 内存下纯连接上限可达 **10-20 万**（受限于 fd 和网络带宽）
- 实际瓶颈：**数据库连接池**、**消息广播 CPU 密集度**
- `max_subscriber_count` 默认 256，可调高

**生产建议**：单节点推荐 **2-5 万在线**，超出后水平扩展集群。

---

## 2. 万人群可行性

**结论：原生架构下无法直接支撑，需要改造。**

| 问题 | 原因 |
|------|------|
| 消息广播扇出 | `topic.go` 中遍历所有 session 写入 channel，O(n) 开销 |
| 单话题上限 | 基准测试仅 1,500 并发，远低于 1 万 |
| DB 写放大 | 每条消息需更新 seqid、写入 messages、更新订阅者 recvseqid |
| 内存膨胀 | 10,000 订阅者导致 `perUser` map 和 Topic 对象膨胀 |

**改造方向**：
- 分层广播：仅推送活跃用户 + 按需拉取
- 消息合并：高频群消息批量推送
- 独立群模块：万人群走独立 Topic + Redis Pub/Sub 替代内存广播
- 异步刷盘：消息先写缓存，批量持久化

---

## 3. 黑盒识别

| 黑盒 | 位置 | 风险 |
|------|------|------|
| Email 验证 Dummy 模式 | `tinode.conf` `"debug_response": "123456"` | 硬编码验证码，生产必须关闭 |
| ACME TLS SNI | `server/main.go`，Let's Encrypt | 不支持 SNI 的客户端会断开 |
| Token AES 加密 | `auth/token/auth_token.go`，密钥来自 `api_key_salt` | 密钥泄露 = 令牌可伪造 |
| Snowflake ID | `store/store.go` | 时钟回拨可能冲突 |
| FCM 推送凭证 | `server/push/fcm/push_fcm.go` | 路径硬编码，换项目需重配 |
| 媒体存储 GC | `server/hdl_files.go` | 默认 1h 扫描，高并发下可能堆积 |

核心协议（WebSocket JSON）完全透明，`docs/PROTOCOLS.md` 和 `docs/API-CN.md` 详尽到帧级别。

---

## 4. SDK 生成方式

**后端不动态生成 SDK。** 项目采用多语言独立 SDK 架构：

| 客户端 | 语言 | 协议 |
|--------|------|------|
| Android | Java | WebSocket JSON / gRPC |
| iOS | Swift | WebSocket JSON / gRPC |
| Web | React + JS | WebSocket JSON |
| CLI | Python | gRPC |

- **gRPC**：通过 `protoc` 从 `model.proto` 生成
- **WebSocket**：完全手写，不依赖代码生成

---

## 5. 可扩展性

### 已支持

| 维度 | 实现 |
|------|------|
| 水平扩展 | `cluster.go` + 一致性哈希 + 故障转移 |
| 多数据库 | `db/adapter.go` 接口，MySQL/PostgreSQL/MongoDB/RethinkDB |
| 多认证 | `auth/Authenticator` 接口，anon/basic/token/code/rest |
| 推送通知 | `push/PushHandler` 接口，FCM/TNPG |
| 文件存储 | `media/Handler` 接口，FS / S3 |
| 插件系统 | gRPC Plugin 服务，可拦截消息事件 |

### 局限

| 局限 | 影响 |
|------|------|
| 集群是 Topic 分片 | 跨节点需代理转发，延迟增加 |
| 不支持 Federation | 不能与外部实例互通 |
| 无端到端加密 | 服务端明文存储 |
| 无全文搜索 | 依赖 MySQL FULLTEXT，大规模需接 Elasticsearch |

### 综合评分：7.5/10

| 维度 | 评分 |
|------|------|
| 架构设计 | 8/10 |
| 数据库 | 9/10 |
| 认证 | 8/10 |
| 水平扩展 | 7/10 |
| 协议透明 | 9/10 |
| 插件生态 | 6/10 |
