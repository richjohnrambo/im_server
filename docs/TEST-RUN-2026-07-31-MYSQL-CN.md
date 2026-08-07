# 后端测试执行报告（MySQL）

> 执行时间：2026-07-31 10:50:28 CST  
> 项目路径：`/Users/superysh/im/chat-0.25.3`  
> Go 版本：`go1.26.0 darwin/arm64`  
> MySQL：`8.0.42`  
> 数据库连接：`127.0.0.1:3306`，用户 `root/root`

## 结论

本次后端测试通过。按 MySQL build tag 编译并执行后，核心后端包、公共组件、MySQL 数据库适配器集成测试均通过；`server` 与 `tinode-db` 的 MySQL 目标构建也通过。

需要注意：不能直接把 `go test ./...` 作为本项目的标准全量测试命令，因为数据库适配器使用 Go build tags。未指定 tag 时，MySQL/PostgreSQL/MongoDB/RethinkDB 测试包会引用不到真实 adapter 的 `GetTestAdapter`，导致编译失败。

## 测试环境检查

已验证 MySQL 服务可连接，并确认存在业务库 `tinode` 与测试库 `tinode_test`。

```bash
mysql -uroot -proot -h127.0.0.1 -P3306 -e "SELECT VERSION() AS mysql_version; SHOW DATABASES LIKE 'tinode'; SHOW DATABASES LIKE 'tinode_test';"
```

结果摘要：

| 项 | 结果 |
|---|---|
| MySQL 版本 | `8.0.42` |
| `tinode` | 存在 |
| `tinode_test` | 存在 |

说明：仓库自带 MySQL 测试配置 [server/db/mysql/tests/test.conf](/Users/superysh/im/chat-0.25.3/server/db/mysql/tests/test.conf:1) 使用 `tinode_test`，并设置 `reset_db_data=true`。这组测试会重建测试库数据。为避免影响你已经建好的业务库 `tinode`，本次 MySQL adapter 测试使用 `tinode_test`。

## 执行命令与结果

### 1. 后端非数据库实例依赖测试

```bash
go test -count=1 $(go list ./server/... | grep -v '/server/db/.*/tests$')
```

结果：通过。

覆盖包摘要：

| 包 | 结果 |
|---|---|
| `github.com/tinode/chat/server` | `ok` |
| `github.com/tinode/chat/server/db/common` | `ok` |
| `github.com/tinode/chat/server/drafty` | `ok` |
| `github.com/tinode/chat/server/media` | `ok` |
| `github.com/tinode/chat/server/ringhash` | `ok` |
| `github.com/tinode/chat/server/store/types` | `ok` |

### 2. MySQL adapter 集成测试

```bash
go test -count=1 -tags mysql ./server/db/mysql/tests
```

结果：通过。

```text
ok github.com/tinode/chat/server/db/mysql/tests 0.687s
```

覆盖的主要持久化能力：

| 能力 | 典型测试 |
|---|---|
| 建库与 schema 初始化 | `TestCreateDb` |
| 用户创建、查询、更新、删除 | `TestUserCreate`、`TestUserGet`、`TestUserUpdate`、`TestUserDelete` |
| 登录认证记录 | `TestAuthAddRecord`、`TestAuthGetRecord`、`TestAuthUpdRecord` |
| 凭证验证 | `TestCredUpsert`、`TestCredGetActive`、`TestCredConfirm`、`TestCredDel` |
| Topic 创建与更新 | `TestTopicCreate`、`TestTopicCreateP2P`、`TestTopicUpdate`、`TestTopicDelete` |
| 订阅关系 | `TestTopicShare`、`TestSubscriptionGet`、`TestSubsUpdate`、`TestSubsDelete` |
| 消息存储、查询、删除 | `TestMessageSave`、`TestMessageGetAll`、`TestMessageDeleteList`、`TestMessageGetDeleted` |
| 文件与附件 | `TestFileStartUpload`、`TestFileFinishUpload`、`TestMessageAttachments` |
| 设备与推送记录 | `TestDeviceUpsert`、`TestDeviceGetAll`、`TestDeviceDelete` |
| 未读数 | `TestUserUnreadCount` |
| 持久缓存 | `TestPCacheUpsert`、`TestPCacheGet`、`TestPCacheDelete`、`TestPCacheExpire` |

### 3. MySQL tag 下的 Go 包测试

```bash
go test -count=1 -tags mysql $(go list ./... | grep -v '/server/db/mongodb/tests$' | grep -v '/server/db/postgres/tests$' | grep -v '/server/db/rethinkdb/tests$')
```

结果：通过。

结果摘要：

| 范围 | 结果 |
|---|---|
| 根模块 Go 包 | 通过 |
| `server` 核心测试 | 通过 |
| `server/db/common` | 通过 |
| `server/db/mysql/tests` | 通过 |
| `drafty`、`media`、`ringhash`、`store/types` | 通过 |
| `tinode-db` | 无测试文件，编译检查通过 |

本次实际执行的测试函数约 `185` 个，不含 PostgreSQL/MongoDB/RethinkDB adapter 测试。

### 4. MySQL 目标构建

```bash
go build -tags mysql ./server ./tinode-db
```

结果：通过。

## 直接全量测试的已知失败

执行以下命令：

```bash
go test -count=1 ./...
```

结果：失败，原因是未指定数据库 build tag，导致各数据库测试包无法引用真实 adapter。

失败摘要：

```text
server/db/mysql/tests/mysql_test.go:1319:16: undefined: backend.GetTestAdapter
server/db/postgres/tests/postgres_test.go:1385:16: undefined: backend.GetTestAdapter
server/db/rethinkdb/tests/rethink_test.go:1552:16: undefined: backend.GetTestAdapter
server/db/mongodb/tests/mongo_test.go:1256:16: undefined: backend.GetTestAdapter
```

这是项目构建方式导致的预期问题，不代表 MySQL 后端测试失败。标准命令应按目标数据库指定 build tag。

## 建议后续标准测试命令

当前以 MySQL 为后端开发目标时，建议使用：

```bash
go test -count=1 -tags mysql $(go list ./... | grep -v '/server/db/mongodb/tests$' | grep -v '/server/db/postgres/tests$' | grep -v '/server/db/rethinkdb/tests$')
```

如果只改核心消息、会话、Topic 逻辑，可先跑：

```bash
go test -count=1 ./server ./server/db/common ./server/drafty ./server/media ./server/ringhash ./server/store/types
```

如果改 MySQL 持久化层，必须补跑：

```bash
go test -count=1 -tags mysql ./server/db/mysql/tests
```

如果改启动、配置或数据库初始化工具，补跑：

```bash
go build -tags mysql ./server ./tinode-db
```

## 开发后回归记录

> 执行时间：2026-07-31 11:46:05 CST

本轮新增 `x-im-*` 协议扩展实现与测试后，已执行以下回归：

| 命令 | 结果 |
|---|---|
| `go test -count=1 ./server` | 通过 |
| `go test -count=1 ./server/drafty ./server/store/types ./server/db/common` | 通过 |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过 |
| `go build -tags mysql ./server ./tinode-db` | 通过 |
| 校验 [docs/IM-WS-API-CN.md](/Users/superysh/im/chat-0.25.3/docs/IM-WS-API-CN.md:1) 内全部 `json` 代码块 | `94/94` 通过 |

新增覆盖点：

| 覆盖点 | 说明 |
|---|---|
| `x-im-type` 白名单 | 拒绝未知或非字符串业务消息类型 |
| 通话记录类型 | 支持 `head.x-im-type="call"` 并保持原样广播 |
| 聊天记录筛选 | 覆盖按 `x-im-filter.types` 与 `x-im-search.keyword` 过滤 |
| 群全员禁言 | 覆盖普通成员拒绝、管理员允许 |

## 开发后回归记录（二）

> 执行时间：2026-07-31 11:57:58 CST

本轮新增好友/入群申请资料校验、群业务配置校验、创建群初始 `aux` 保存后，已执行以下回归：

| 命令 | 结果 |
|---|---|
| `go test -count=1 ./server` | 通过 |
| `go test -count=1 ./server/drafty ./server/store/types ./server/db/common` | 通过 |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过 |
| `go build -tags mysql ./server ./tinode-db` | 通过 |
| 校验 [docs/IM-WS-API-CN.md](/Users/superysh/im/chat-0.25.3/docs/IM-WS-API-CN.md:1) 内全部 `json` 代码块 | `94/94` 通过 |

新增覆盖点：

| 覆盖点 | 说明 |
|---|---|
| 好友申请备注 | `private.x-im-applyText` 支持保存，非法类型或超长返回 400 |
| 私有业务开关 | `x-im-blocked`、`x-im-muted`、`x-im-pinned` 必须为布尔值 |
| 群业务配置 | `aux.x-im-group` 的核心布尔字段、禁言成员数组、公告文本类型受校验 |
| 创建群初始配置 | `{sub topic="new" set.aux.x-im-group}` 会写入 `topics.aux` |
| 群配置更新 | 管理员 `{set aux}` 可更新 `x-im-group`，非法配置返回 400 |

## 开发后回归记录（三）

> 执行时间：2026-07-31 12:02:05 CST

本轮新增群级业务策略后，已执行以下回归：

| 命令 | 结果 |
|---|---|
| `go test -count=1 ./server ./server/drafty ./server/store/types ./server/db/common` | 通过 |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过 |
| `go build -tags mysql ./server ./tinode-db` | 通过 |
| 校验 [docs/IM-WS-API-CN.md](/Users/superysh/im/chat-0.25.3/docs/IM-WS-API-CN.md:1) 内全部 `json` 代码块 | `95/95` 通过 |

新增覆盖点：

| 覆盖点 | 说明 |
|---|---|
| 单成员禁言 | `aux.x-im-group.mutedUsers` 中的普通成员发消息返回 403 |
| 成员文件上传限制 | `allowMemberFileUpload=false` 时普通成员发送附件类消息返回 403 |
| 仅管理员邀请 | `onlyAdminInvite=true` 时非管理员邀请成员返回 403 |
| 管理员豁免 | 群全员禁言下管理员仍可发消息，沿用前序测试覆盖 |

## 开发后回归记录（四）

> 执行时间：2026-07-31 12:10:16 CST

本轮启动 Phase 2，新增 MySQL 消息业务索引 `im_message_index`，并把 MySQL adapter 版本升级到 `117`。测试库检查结果：

| 项 | 结果 |
|---|---|
| `tinode_test.kvmeta.version` | `117` |
| `tinode_test.im_message_index` | 存在 |

已执行以下回归：

| 命令 | 结果 |
|---|---|
| `go test -count=1 ./server ./server/drafty ./server/store/types ./server/db/common` | 通过 |
| `go test -count=1 -tags mysql ./server/db/mysql/tests` | 通过 |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过 |
| `go build -tags mysql ./server ./tinode-db` | 通过 |

新增覆盖点：

| 覆盖点 | 说明 |
|---|---|
| 数据库迁移 | MySQL adapter `116 -> 117`，`CreateDb` 与 `UpgradeDb` 均支持 `im_message_index` |
| 历史消息回填 | 旧库升级时扫描 `messages` 与 `filemsglinks` 回填索引 |
| 索引写入 | `MessageIndexSave` 保存消息类型、检索文本、附件数量 |
| 索引查询 | `MessageIndexSearch` 支持 topic、seq 范围、类型、关键词、limit |
| 软删除过滤 | 索引查询继续按 Tinode `dellog` 排除当前用户已删除消息 |
| 服务层接入 | `{get data}` 携带 `x-im-search` 或 `x-im-filter` 时优先使用索引，未实现索引的 adapter 回退内存过滤 |

注意：业务库 `tinode` 如仍是旧版本，需要使用 `tinode-db --upgrade` 升级到 `117` 后，当前 MySQL server 构建才能通过数据库版本检查。

## 业务库升级记录

> 执行时间：2026-07-31 12:28:43 CST

已按 Phase 2 迁移要求升级业务库 `127.0.0.1:3306/tinode`。

升级前检查：

| 项 | 结果 |
|---|---|
| `tinode.kvmeta.version` | `116` |
| `tinode.im_message_index` | 不存在 |

升级前已备份：

```bash
mysqldump -uroot -proot -h127.0.0.1 -P3306 --single-transaction --routines --triggers tinode > /Users/superysh/im/chat-0.25.3/db_backups/tinode-before-117-20260731-122808.sql
```

执行升级：

```bash
go run -tags mysql ./tinode-db --upgrade --config=server/tinode.conf
```

升级日志摘要：

```text
Database adapter: 'mysql'; version: 117
Wrong DB version: expected 117, got 116. Upgrading the database.
Database successfully upgraded.
All done.
```

升级后验证：

| 项 | 结果 |
|---|---|
| `tinode.kvmeta.version` | `117` |
| `tinode.im_message_index` | 存在 |
| `tinode.messages` | `100` 条 |
| `tinode.im_message_index` | `100` 条 |
| `go run -tags mysql ./tinode-db --config=server/tinode.conf` | `Database exists, version is correct.` |
| `go build -tags mysql ./server ./tinode-db` | 通过 |
| `go test -count=1 -tags mysql ./server/db/mysql/tests` | 通过 |

## 开发后回归记录（五）

> 执行时间：2026-07-31 12:37:59 CST

本轮继续 Phase 2，新增 MySQL 积分与每日事件持久化能力，并把 MySQL adapter 版本升级到 `118`。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| 数据库迁移 | MySQL adapter `117 -> 118`，`CreateDb` 与 `UpgradeDb` 均支持 `im_user_points`、`im_user_daily_events` |
| 积分查询 | `IMUserPointsGet` 支持读取用户积分余额 |
| 积分增加 | `IMUserPointsAdd` 支持首次创建与余额累加 |
| 积分扣减 | `IMUserPointsConsume` 使用条件更新保证余额不足时不扣减 |
| 每日事件幂等 | `IMDailyEventCreate` 依赖唯一索引保证同一用户、日期、事件类型只记录一次 |
| 每日事件读取 | `IMDailyEventGet` 支持读取事件 payload，供签到、每日任务等业务复用 |

已执行以下回归：

| 命令 | 结果 |
|---|---|
| `go test -count=1 ./server ./server/drafty ./server/store/types ./server/db/common` | 通过 |
| `go test -count=1 -tags mysql ./server/db/mysql/tests` | 通过 |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过 |
| `go build -tags mysql ./server ./tinode-db` | 通过 |

新增测试用例：

| 用例 | 目的 |
|---|---|
| `TestIMUserPointsAddAndConsume` | 验证积分首次创建、增加、扣减后的余额 |
| `TestIMUserPointsInsufficient` | 验证余额不足时返回 `types.ErrPolicy` |
| `TestIMDailyEventIdempotent` | 验证每日事件重复写入只成功一次，并可读取 payload |

## 业务库升级记录（二）

> 执行时间：2026-07-31 12:37:59 CST

已按 Phase 2 二期迁移要求升级业务库 `127.0.0.1:3306/tinode`。

升级前检查：

| 项 | 结果 |
|---|---|
| `tinode.kvmeta.version` | `117` |
| `tinode.im_message_index` | 存在 |
| `tinode.im_user_points` | 不存在 |
| `tinode.im_user_daily_events` | 不存在 |

升级前已备份：

```bash
mysqldump -h127.0.0.1 -P3306 -uroot -proot tinode > /Users/superysh/im/chat-0.25.3/db_backups/tinode-before-118-20260731-123747.sql
```

执行升级：

```bash
go run -tags mysql ./tinode-db --upgrade --config=server/tinode.conf
```

升级日志摘要：

```text
Database adapter: 'mysql'; version: 118
Wrong DB version: expected 118, got 117. Upgrading the database.
Database successfully upgraded.
All done.
```

升级后验证：

| 项 | 结果 |
|---|---|
| `tinode.kvmeta.version` | `118` |
| `tinode.im_message_index` | 存在 |
| `tinode.im_user_points` | 存在 |
| `tinode.im_user_daily_events` | 存在 |
| `tinode.messages` | `100` 条 |
| `tinode.im_message_index` | `100` 条 |

## 开发后回归记录（六）

> 执行时间：2026-07-31 12:50:00 CST

本轮将 Phase 2 签到能力正式接入 WebSocket `{pub}` 流程。客户端向 `slf` 发送 `head.x-im-type="checkin"` 时，服务端会先执行每日事件幂等与积分发放，再决定是否保存签到消息。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| WS 签到接入 | `Topic.handlePubBroadcast` 在消息落库前处理 `checkin` |
| 幂等控制 | 同一用户、日期、事件类型重复签到返回 `304 no action` |
| 消息去重 | 重复签到不保存第二条 `slf` 消息 |
| 固定奖励 | 服务端固定每日签到奖励为 `5`，拒绝客户端伪造其它积分值 |
| 原子发放 | `IMDailyEventCreateWithPoints` 在同一事务内创建每日事件并增加积分 |

已执行以下回归：

| 命令 | 结果 |
|---|---|
| `go test -count=1 ./server ./server/drafty ./server/store/types ./server/db/common` | 通过 |
| `go test -count=1 -tags mysql ./server/db/mysql/tests` | 通过 |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过 |
| `go build -tags mysql ./server ./tinode-db` | 通过 |

新增测试用例：

| 用例 | 目的 |
|---|---|
| `TestIMParseCheckinPayload` | 验证签到日期与默认积分解析 |
| `TestIMParseCheckinPayloadRejectsClientControlledReward` | 验证客户端不能伪造非默认签到奖励 |
| `TestIMDailyEventCreateWithPoints` | 验证每日事件与积分发放的事务一致性和重复不加分 |

## 开发后回归记录（七）

> 执行时间：2026-07-31 13:05:00 CST

本轮将 Phase 2 话题发布积分扣减接入 `{sub topic="nch"}` 频道创建流程。客户端在 `public.x-im-topic.pointsCost` 中声明扣费金额时，MySQL 后端会在同一事务中创建话题、创建作者订阅并扣减积分。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| 话题扣费解析 | 从 `public.x-im-topic.pointsCost` 读取非负整数扣费金额 |
| 普通群聊隔离 | 仅 `nch` 频道创建触发扣费，`new` 群聊创建不受影响 |
| 原子创建 | `IMTopicCreateWithPoints` 在一个事务内完成 topic 创建、owner subscription 创建和积分扣减 |
| 余额不足保护 | 余额不足返回 `types.ErrPolicy`，事务回滚，话题不会创建 |
| 可选 adapter | 不修改 Tinode 全局 adapter 接口，非 MySQL 后端没有实现时返回 unsupported |

已执行以下回归：

| 命令 | 结果 |
|---|---|
| `go test -count=1 ./server ./server/drafty ./server/store/types ./server/db/common` | 通过 |
| `go test -count=1 -tags mysql ./server/db/mysql/tests` | 通过 |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过 |
| `go build -tags mysql ./server ./tinode-db` | 通过 |

新增测试用例：

| 用例 | 目的 |
|---|---|
| `TestIMTopicPointsCost` | 验证话题发布扣费字段解析 |
| `TestIMTopicPointsCostRejectsMalformedCost` | 验证非法扣费金额返回 malformed |
| `TestIMTopicCreateWithPoints` | 验证话题创建成功后积分被扣减，并创建作者订阅 |
| `TestIMTopicCreateWithPointsInsufficient` | 验证余额不足时返回 policy，且不会创建 topic |

## 开发后回归记录（八）

> 执行时间：2026-07-31 13:58:00 CST

本轮接入产品通话 payload 校验，同时保持 Tinode 原生 WebRTC 信令兼容。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| 通话邀请校验 | `head.x-im-type="call"` 且 `head.webrtc="started"` 时，`content.callId` 必填 |
| 控制事件校验 | `ringing`、`accept`、`hang-up` 的 `payload.callId` 必填，非法 payload 静默丢弃 |
| 原生信令兼容 | `offer`、`answer`、`ice-candidate` 不强制产品字段，继续透传 SDP/ICE payload |
| 通话记录兼容 | 普通 `x-im-type="call"` 持久消息不强制 `callId`，避免影响历史通话记录展示 |

已执行以下回归：

| 命令 | 结果 |
|---|---|
| `go test -count=1 ./server` | 通过 |
| `go test -count=1 ./server/drafty ./server/store/types ./server/db/common` | 通过 |
| `go test -count=1 -tags mysql ./server/db/mysql/tests` | 通过 |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过 |
| `go build -tags mysql ./server ./tinode-db` | 通过 |

新增测试用例：

| 用例 | 目的 |
|---|---|
| `TestIMValidateCallContent` | 验证产品通话内容中 `callId` 合法 |
| `TestIMValidateCallContentMalformed` | 验证产品通话内容缺少 `callId` 被拒绝 |
| `TestHandleBroadcastProductCallRejectsMissingCallID` | 验证产品通话邀请缺少 `callId` 返回 `400 malformed`，且不进入通话状态 |

## 开发后回归记录（九）

> 执行时间：2026-07-31 14:17:42 CST

本轮新增 WebSocket mock 联调程序 `tools/wsmock`，用于按前后端对接文档直接模拟前端通过 `/v0/channels` 与后端通信。程序不新增第三方依赖，复用项目已有 `github.com/gorilla/websocket`。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| WS 握手 | 通过 `X-Tinode-APIKey` 连接 `/v0/channels` 并发送 `{hi}` |
| 注册登录 | 支持 basic 注册；本地配置要求 email credential 时，自动走 `acc -> 300 validate credentials -> login cred.resp` |
| 自存消息 | 订阅 `slf` 并发布 `head.x-im-type="text"` 消息，验证 `202 accepted` |
| 签到幂等 | 首次签到验证 `202 accepted`，重复同日签到验证 `304 no action` |
| 频道积分 | 多日签到累计积分后创建 `nch` 频道并扣减积分，再用超高 `pointsCost` 验证 `422 policy violation` |
| 通话校验 | 产品通话邀请缺少 `content.callId` 时验证返回 `400 malformed` |

已执行以下联调：

| 命令 | 结果 |
|---|---|
| `go build ./tools/wsmock` | 通过 |
| `go run -tags mysql ./server -config=tinode.conf -static_data=-` | 服务启动成功，监听 `:6060` |
| `go run ./keygen -salt 'T713/rYYgW7g4m3vG6zGRh7+FM1t0T8j13koXScOAj4='` | 生成本地联调用 API key |
| `go run ./tools/wsmock -apikey '<generated>' -scenario all -timeout 12s` | 通过 |

联调观察：

| 协议点 | 实际返回 |
|---|---|
| `{hi}` 成功 | `201 created` |
| `{pub}` 成功 | `202 accepted`，随后下发 `{data}` |
| 重复签到 | `304 no action`，`params.x-im-checkin.already=true` |
| 频道余额不足 | `422 policy violation` |
| 产品通话缺少 `callId` | `400 malformed` |
| 本地 SMTP | `tinode.conf` 启用 dummy email response，但 SMTP 服务未启动；联调可通过，服务端会异步记录 SMTP connection refused |

## 开发后回归记录（十）

> 执行时间：2026-07-31 14:27:57 CST

本轮增强 `tools/wsmock`，在原有 Phase 2 smoke/checkin/topic/call 场景基础上，新增双端 P2P、聊天记录搜索/分类筛选、举报到 `sys` 的端到端联调。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| 双端客户端 | 自动创建 Alice 和 Bob 两个 WebSocket 连接，各自完成 `{hi}`、注册、email credential 验证、登录和订阅 `me` |
| P2P 建链 | Alice 订阅 Bob 的 `usr...`，Bob 订阅 Alice 的 `usr...`，验证 P2P topic 可创建和进入 |
| P2P 消息 | Alice 向 Bob 发送 Drafty 文本消息，Bob 收到 `{data}` |
| 已收已读 | Bob 发送 `recv/read` `{note}`，Alice 收到 `read` `{info}` |
| 文件分类 | Alice 发送 `head.x-im-type="file"` 文件卡片，Bob 收到 `{data}` |
| 索引搜索 | Alice 使用 `{get what="data" x-im-search}` 搜索关键词，返回 `208 delivered` 且 `params.count=1` |
| 类型筛选 | Alice 使用 `{get what="data" x-im-filter.types=["file"]}` 查询文件消息，返回 `208 delivered` 且 `params.count=1` |
| 举报 | Alice 向 `sys` 发布 `head.x-im-type="report"`，验证 `202 accepted` |

已执行以下联调：

| 命令 | 结果 |
|---|---|
| `go build ./tools/wsmock` | 通过 |
| `go run -tags mysql ./server -config=tinode.conf -static_data=-` | 服务启动成功，监听 `:6060` |
| `go run ./tools/wsmock -apikey '<generated>' -scenario all -timeout 12s` | 通过 |

联调观察：

| 协议点 | 实际返回 |
|---|---|
| P2P 建链 | `{sub topic="usrPeer"}` 返回 `200 ok`，双方订阅列表包含两个用户 |
| P2P 文本发布 | 发布方 `{ctrl code=202}`，双方收到对应 `{data}` |
| 已读回执 | 接收方 `{note what="read"}` 后，发送方收到 `{info what="read"}` |
| 搜索命中 | `{get x-im-search}` 返回 `208 delivered`，`params.count=1` |
| 文件筛选命中 | `{get x-im-filter}` 返回 `208 delivered`，`params.count=1` |
| 举报到 `sys` | 普通用户不订阅 `sys` 也可 `{pub}`，返回 `202 accepted` |

## 开发后回归记录（十一）

> 执行时间：2026-07-31 14:31:44 CST

本轮继续增强 `tools/wsmock`，新增群聊多角色与业务权限策略联调，并纳入 `all` 全量场景。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| 建群 | Alice 使用 `{sub topic="new"}` 创建群，初始化 `aux.x-im-group` |
| 邀请成员 | Alice 使用 `{set sub.user mode="JRWP"}` 邀请 Bob |
| 成员入群 | Bob 订阅 `grp...` 并读取 `desc sub data aux` |
| 成员发言 | Bob 发布 Drafty 文本，Alice 收到 `{data}` |
| 全员禁言 | Alice 设置 `aux.x-im-group.muteAll=true` 后，Bob 发言返回 `403 permission denied` |
| 群主豁免 | 全员禁言下 Alice 作为群主发言仍返回 `202 accepted` |
| 文件权限 | Alice 设置 `allowMemberFileUpload=false` 后，Bob 发送 `x-im-type="file"` 返回 `403 permission denied` |
| 管理员/群主文件豁免 | 文件上传受限时 Alice 发送文件仍返回 `202 accepted` |

已执行以下联调：

| 命令 | 结果 |
|---|---|
| `go build ./tools/wsmock` | 通过 |
| `go run -tags mysql ./server -config=tinode.conf -static_data=-` | 服务启动成功，监听 `:6060` |
| `go run ./tools/wsmock -apikey '<generated>' -scenario group -timeout 12s` | 通过 |
| `go run ./tools/wsmock -apikey '<generated>' -scenario all -timeout 12s` | 通过 |

联调观察：

| 协议点 | 实际返回 |
|---|---|
| 群创建 | `{sub topic="new"}` 返回 `200 ok`，`ctrl.topic=grp...` |
| 邀请成员 | `{set sub.user}` 返回 `200 ok`，`params.user=usr...` |
| 成员正常发言 | `{pub}` 返回 `202 accepted`，群主收到 `{data}` |
| 全员禁言拦截 | 普通成员 `{pub text}` 返回 `403 permission denied` |
| 群主禁言豁免 | 群主 `{pub text}` 返回 `202 accepted` |
| 文件上传限制 | 普通成员 `{pub file}` 返回 `403 permission denied`，群主 `{pub file}` 返回 `202 accepted` |

## 开发后回归记录（十二）

> 执行时间：2026-07-31 14:48:06 CST

本轮继续增强 `tools/wsmock`，新增完整 WebRTC 通话信令专项联调 `call-flow`，并修正 mock 客户端等待逻辑：当服务端异步下发的 `{meta}`、`{pres}`、`{data}`、`{info}` 与当前等待条件不匹配时，先缓存后续步骤可消费，避免真实 WS 多帧交错导致误判超时。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| 通话邀请 | Alice 向 Bob 的 P2P topic 发布 `head.webrtc="started"`、`head.x-im-type="call"`、`content.callId` |
| 来电展示 | Bob 收到 `webrtc=started` 的 `{data}` |
| 振铃 | Bob 发送 `{note what="call" event="ringing"}`，Alice 收到 `{info event="ringing"}` |
| 接听 | Bob 发送 `{note event="accept"}`，Alice 收到 `{info event="accept"}`，双方收到 `webrtc=accepted` 替换消息 |
| SDP 交换 | Alice 发送 `offer`，Bob 收到；Bob 发送 `answer`，Alice 收到 |
| ICE 交换 | Alice/Bob 双向发送 `ice-candidate`，对端均收到对应 `{info}` |
| 挂断 | Alice 发送 `hang-up`，双方收到 `{info event="hang-up"}` 和 `webrtc=finished` 替换消息 |

已执行以下联调：

| 命令 | 结果 |
|---|---|
| `go build ./tools/wsmock` | 通过 |
| `go run -tags mysql ./server -config=/tmp/tinode-webrtc.conf -static_data=-` | 服务启动成功，监听 `:6060`，日志显示 `Video calls enabled with 2 ICE servers` |
| `go run ./tools/wsmock -apikey '<generated>' -scenario call-flow -timeout 12s` | 通过 |
| `go run ./tools/wsmock -apikey '<generated>' -scenario all -timeout 12s` | 通过 |

联调观察：

| 协议点 | 实际返回 |
|---|---|
| `{hi}` WebRTC 参数 | `params.callTimeout=30`，并返回配置的 `iceServers` |
| 通话邀请 | `{pub}` 返回 `202 accepted`，接收方收到 `webrtc=started` |
| 接听行为 | 发起方收到 `{info event="accept"}`；接听方不额外收到 `accept info`，而是直接收到 `webrtc=accepted` 替换消息 |
| 通话媒体协商 | `offer`、`answer`、双向 `ice-candidate` 均通过 `{note}` 发送，并由服务端以 `{info}` 转发给对端 |
| 挂断结束 | 双方均收到 `event="hang-up"` 与 `webrtc=finished` |

## 开发后回归记录（十三）

> 执行时间：2026-07-31 15:02:40 CST

本轮新增更完整的群管理 WebSocket 专项联调 `group-admin`，覆盖 Alice 群主、Bob 管理员候选、Carol 普通成员、Dave 被邀请/被踢成员四个角色，并继续复用真实 `/v0/channels` 协议。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| 群配置读取 | Alice 设置 `aux.x-im-group.joinApproval/onlyAdminInvite/announcement` 后，通过 `{get what="aux"}` 读取并断言字段 |
| 仅管理员邀请 | Bob 先具备 `S` 分享权限但不具备 `A` 管理权限，在 `onlyAdminInvite=true` 下邀请 Dave 返回 `403 permission denied` |
| 管理员授权 | Alice 使用绝对权限 `JRWPASD` 授予 Bob 管理权限，Bob 再用自己的 `{set.sub}` 将 `want` 调整为 `JRWPASD`，最终有效权限变为管理员 |
| 管理员邀请 | Bob 成为管理员后邀请 Dave 返回 `200 ok`，Dave 可订阅入群 |
| 单成员禁言 | Alice 将 Carol 写入 `aux.x-im-group.mutedUsers` 后，Carol 发言返回 `403`；清空后 Carol 发言返回 `202` |
| 全员禁言管理员豁免 | Bob 设置 `muteAll=true` 后，Dave 发言返回 `403`，Bob 作为管理员发言返回 `202` |
| 踢人 | Bob 使用 `{del what="sub" user=dave}` 踢出 Dave 返回 `200`；Dave 当前会话收到 `205 evicted`，后续发言返回 `409 must attach first` |

已执行以下联调：

| 命令 | 结果 |
|---|---|
| `gofmt -w tools/wsmock/main.go && go build ./tools/wsmock` | 通过 |
| `go test -count=1 ./tools/wsmock` | 通过，无测试文件 |
| `go run -tags mysql ./server -config=tinode.conf -static_data=-` | 服务启动成功，监听 `:6060`，日志显示 `Video calls disabled` |
| `go run ./tools/wsmock -apikey '<generated>' -scenario group-admin -timeout 12s` | 通过 |
| `go run ./tools/wsmock -apikey '<generated>' -scenario all -timeout 12s` | 通过 |

联调观察：

| 协议点 | 实际返回 |
|---|---|
| `{set.sub.mode}` 权限写法 | 当前后端使用绝对权限字符串，不接受 `+A`、`-W` 这类增量字符串；错误写法返回 `400 malformed` |
| 有效权限计算 | 用户有效权限为 `want & given`，管理员授权需要服务端授予 `given`，目标用户也要把自己的 `want` 提升到包含 `A` |
| 踢人后状态 | 被踢用户会收到无 id 的 `{ctrl code=205 text="evicted" params.unsub=true}`，同一 WS 会话再发群消息返回 `409 must attach first` |

## 开发后回归记录（十四）

> 执行时间：2026-07-31 15:12:24 CST

本轮补齐真实文件上传链路，不再只用“文件卡片消息”模拟附件。新增 `tools/wsmock -scenario upload`，通过 HTTP `/v0/file/u/` 上传小文件，再通过 WebSocket 发布 Drafty 文件消息，并显式写入 `extra.attachments`，验证服务端保存消息时把上传文件绑定到消息，避免后续 GC 清理。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| 上传入口 | 从 `ws://127.0.0.1:6060/v0/channels` 自动推导 `http://127.0.0.1:6060/v0/file/u/` |
| 上传鉴权 | HTTP 请求带 `X-Tinode-APIKey` 与 `Authorization: token <login-token>` |
| Multipart 上传 | 使用字段 `file` 上传真实二进制内容，服务端返回 `{ctrl code=200 params.url}` |
| 附件消息 | WebSocket `{pub}` 使用 `mime=text/x-drafty`、`x-im-type=file`、Drafty `EX` entity，并写 `extra.attachments=[url]` |
| 接收验证 | Bob 收到 `{data}`，其 Drafty content 中的 `ref` 与上传返回 URL 一致 |
| 查询验证 | Alice 使用 `x-im-filter.types=["file"]` 查询到该文件消息 |
| 防 GC 引用 | MySQL `fileuploads` 记录成功完成，`filemsglinks` 已绑定到对应 `messages.id` |

已执行以下联调：

| 命令 | 结果 |
|---|---|
| `gofmt -w tools/wsmock/main.go && go build ./tools/wsmock` | 通过 |
| `go test -count=1 ./tools/wsmock` | 通过，无测试文件 |
| `go run -tags mysql ./server -config=tinode.conf -static_data=-` | 服务启动成功，监听 `:6060`，日志显示 `Large media handling enabled fs` |
| `go run ./tools/wsmock -apikey '<generated>' -scenario upload -timeout 12s` | 通过 |
| `mysql -uroot -proot -h127.0.0.1 -P3306 tinode -e '<fileuploads/filemsglinks join query>'` | 查到上传文件与消息引用 |
| `curl -H 'X-Tinode-APIKey: ...' 'http://127.0.0.1:6060/v0/file/s/KX2Un74V9eI.conf?...'` | `200 OK`，内容与上传文本一致 |

联调观察：

| 协议点 | 实际返回 |
|---|---|
| `/v0/file/u/` | `200 ok`，`ctrl.params.url="/v0/file/s/KX2Un74V9eI.conf"`，`ctrl.params.expires` 返回未绑定前临时保留时间 |
| 文件落库 | `fileuploads.id=1557212408072638464`，`status=1`，`mimetype=text/plain; charset=utf-8`，`size=50`，`location=uploads/ff6zjh56cx26e` |
| 消息绑定 | `filemsglinks.fileid=1557212408072638464` 关联 `messages.id=170`，消息 topic 为 P2P topic，`seqid=1` |
| 文件下载 | `/v0/file/s/KX2Un74V9eI.conf` 返回 `200 OK`，`Content-Length=50`，内容与上传文本一致 |
| 场景归属 | `upload` 会写真实文件和业务库记录，暂不纳入默认 `all`，避免普通回归持续堆积附件 |

## 开发后回归记录（十五）

> 执行时间：2026-07-31 15:19:10 CST

本轮补齐好友/联系人业务状态专项联调。新增 `tools/wsmock -scenario contacts`，通过真实 WebSocket 验证 P2P 建链时的好友申请备注、联系人私有状态读写、拉黑 ACL 阻断、取消拉黑恢复发言，以及非法 private 字段校验。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| 申请备注 | Alice 使用 `{sub topic=bobUserId set.desc.private.x-im-applyText}` 建立 P2P 关系 |
| 详情读回 | Alice 使用 `{get topic=bobUserId what="desc"}` 读取 `desc.private` 并断言申请备注 |
| 通讯录读回 | Alice 使用 `{get topic="me" what="sub"}` 读取联系人列表，断言目标联系人 `sub.private` |
| 私有状态 | 写入并读回 `comment`、`x-im-remarkUpdatedAt`、`x-im-muted`、`x-im-pinned`、`x-im-blocked` |
| 拉黑联动 | Alice 设置 `x-im-blocked=true` 并把 Bob 的 P2P `given` 调整为 `JRPA`，Bob 再发言返回 `403 permission denied` |
| 取消拉黑 | Alice 设置 `x-im-blocked=false` 并把 Bob 的 P2P `given` 调整为 `JRWPA`，Bob 发言返回 `202 accepted` 且 Alice 收到 `{data}` |
| 参数校验 | `x-im-blocked` 非布尔值、`x-im-applyText` 超长均返回 `400 malformed` |

已执行以下联调：

| 命令 | 结果 |
|---|---|
| `gofmt -w tools/wsmock/main.go` | 通过 |
| `go test -count=1 ./tools/wsmock` | 通过，无测试文件 |
| `go run -tags mysql ./server -config=tinode.conf -static_data=-` | 服务启动成功，监听 `:6060`，MySQL adapter schema `118` |
| `go run ./tools/wsmock -apikey '<generated>' -scenario contacts -timeout 12s` | 通过 |

联调观察：

| 协议点 | 实际返回 |
|---|---|
| 好友申请备注 | `private.x-im-applyText="好友申请备注联调 151910"` 在 topic desc 和 `me.sub` 中均可读回 |
| 私有状态 | `comment`、`x-im-muted`、`x-im-pinned`、`x-im-blocked`、`x-im-remarkUpdatedAt` 在 `desc.private` 与联系人列表中保持一致 |
| 拉黑 ACL | `{set.desc.private + set.sub.mode="JRPA"}` 返回 `200`；服务端向 Bob 推送 `pres what="acs" dacs.given="+P-W"` |
| 被拉黑发言 | Bob 发布 P2P 文本返回 `403 permission denied` |
| 取消拉黑 ACL | `{set.desc.private + set.sub.mode="JRWPA"}` 返回 `200`；服务端向 Bob 推送 `pres what="acs" dacs.given="+W"` |
| 恢复发言 | Bob 发布 P2P 文本返回 `202 accepted`，Alice 收到对应 `{data}` |
| 非法 private | `x-im-blocked="true"` 与 257 字符 `x-im-applyText` 均返回 `400 malformed` |
| 场景归属 | `contacts` 会写真实联系人私有状态并调整 P2P ACL，暂不纳入默认 `all`，便于专项定位 |

## 开发后回归记录（十六）

> 执行时间：2026-07-31 15:37:26 CST

本轮补齐“页面型能力”中可复用 Tinode 原生协议低改动落地的专项联调。新增 `tools/wsmock -scenario pages` 与 `tools/wsmock -scenario group-owner`，覆盖收藏/文件助手/个人设置/黑名单视图/删除好友/群二维码/群主转让。

代码变更摘要：

| 覆盖点 | 说明 |
|---|---|
| 个人设置 | Alice 写入 `me.desc.private.x-im-settings`，再通过 `{get topic="me" what="desc"}` 读回 |
| 收藏列表 | Alice 向 `slf` 发布 `head.x-im-type="favorite"`，再用 `x-im-search.types=["favorite"]` 搜索收藏 |
| 文件助手 | Alice 向 `slf` 发布文件卡片消息，再用 `x-im-filter.types=["file"]` 读取文件助手列表 |
| 黑名单视图 | Alice 拉黑 Bob 后，通过 `{get topic="me" what="sub"}` 读取联系人列表并过滤 `sub.private.x-im-blocked=true` |
| 删除好友 | Alice 对 P2P topic 执行 `{leave unsub=true}`，再次读取 `me.sub` 时目标联系人已消失 |
| 群二维码 | 群主写入群 `public.x-im-qrcode`，再通过 `{get what="desc"}` 读回 |
| 群主转让 | Alice 先给 Bob 的 `given` 授予 `JRWPASDO`，Bob 再设置自己的 `want=JRWPASDO` 接受转让 |
| 群主唯一性 | 转让后 Bob 的 `acs.mode` 包含 `O`，Alice 的 `acs.mode` 不再包含 `O` |

已执行以下联调：

| 命令 | 结果 |
|---|---|
| `gofmt -w tools/wsmock/main.go` | 通过 |
| `go test -count=1 ./tools/wsmock` | 通过，无测试文件 |
| `go build ./tools/wsmock` | 通过 |
| `go run -tags mysql ./server -config=tinode.conf -static_data=-` | 服务启动成功，监听 `:6060`，MySQL adapter schema `118` |
| `go run ./tools/wsmock -apikey '<generated>' -scenario pages -timeout 12s` | 通过 |
| `go run ./tools/wsmock -apikey '<generated>' -scenario group-owner -timeout 12s` | 通过 |

联调观察：

| 协议点 | 实际返回 |
|---|---|
| `x-im-settings` | `allowSearchByCard/allowSearchByQr/allowSearchByGroup/allowSearchByPhone/joinGroupNeedsApproval/teenMode` 均可通过 `me.private` 保存并读回 |
| 收藏消息 | `slf` 收藏消息返回 `202 accepted`，搜索返回 `208 delivered` 且 `params.count=1` |
| 文件助手 | `slf` 文件卡片返回 `202 accepted`，文件类型筛选返回 `208 delivered` 且 `params.count=1` |
| 黑名单列表 | 拉黑后 `me.sub` 中对应联系人 `private.x-im-blocked=true` |
| 删除好友 | `{leave topic=usrPeer unsub=true}` 返回 `200 ok`，后续 `me.sub` 不再包含该联系人 |
| 群二维码 | `public.x-im-qrcode="im://group/<groupTopic>"` 可从群 `desc.public` 读回 |
| 群主转让 | Bob 接受后服务端向 Bob 推送 `given="+O"`，向 Alice 推送 `want="-O", given="-O"` |
| 新群主操作 | Bob 成为群主后可继续更新 `aux.x-im-group.announcement`，返回 `200 ok` |
| 场景归属 | `pages` 会写真实用户 private、`slf` 消息和 P2P 订阅；`group-owner` 会改变群 owner，均暂不纳入默认 `all` |

后续仍需前端或运营侧承接的页面：

| 页面/能力 | 当前状态 |
|---|---|
| 新朋友审批队列 | 后续记录（十九）已补齐 `private.x-im-applyStatus` 状态字段和 WS 联调 |
| 进群审批队列 | 后续记录（十九）已补齐 `want/given` 待审批订阅、`x-im-joinStatus` 自动流转和 WS 联调 |
| 系统消息列表 | 当前已覆盖普通用户向 `sys` 举报；平台主动系统通知需要系统账号、运营后台或服务端投递入口 |
| 版本更新/权限弹窗 | 多数为客户端本地或应用商店/配置中心能力，后端只需在后续配置接口中承载版本策略 |

## 开发后回归记录（十七）

> 执行时间：2026-07-31 15:40 CST

本轮按本地 MySQL 环境重新整理并验证测试命令。普通 `go test ./server/...` 会同时编译 PostgreSQL、MongoDB、RethinkDB 的 adapter 测试包；这些测试包没有对应 build tag 时会使用 blank adapter，因此 `backend.GetTestAdapter` 未定义。MySQL CI 应显式使用 `-tags mysql`，并排除其它数据库 adapter 的测试包。

测试环境：

| 项 | 值 |
|---|---|
| MySQL 地址 | `127.0.0.1:3306` |
| 业务库 | `tinode`，未用于 adapter reset 测试 |
| 测试库 | `tinode_test` |
| 测试配置 | `server/db/mysql/tests/test.conf`，`reset_db_data=true` |
| 数据库存在性 | `tinode` 与 `tinode_test` 均存在 |

已执行以下测试：

| 命令 | 结果 |
|---|---|
| `mysql -uroot -proot -h127.0.0.1 -P3306 -e 'SHOW DATABASES LIKE "tinode%";'` | 查到 `tinode`、`tinode_test` |
| `go test -count=1 -tags mysql ./server/db/mysql/tests` | 通过，`ok github.com/tinode/chat/server/db/mysql/tests 0.617s` |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过，包含核心 `server`、MySQL adapter tests、`tools/wsmock` 等包 |

后续 CI 建议：

```bash
go test -count=1 -tags mysql ./server/db/mysql/tests
go test -count=1 -tags mysql $(go list ./... | grep -v '/server/db/mongodb/tests$' | grep -v '/server/db/postgres/tests$' | grep -v '/server/db/rethinkdb/tests$')
```

## 风险与注意事项

1. `server/db/mysql/tests/test.conf` 当前连接的是 `tinode_test`，并且会重置测试数据。不要把该配置直接改成生产/业务库 `tinode` 后执行测试。
2. PostgreSQL、MongoDB、RethinkDB 测试本次未执行；如后续需要支持这些后端，应分别准备对应数据库实例并使用对应 build tag 单独测试。
3. WebSocket 端到端联调已覆盖核心 Phase 2 场景、基础 P2P/搜索/举报、好友/联系人私有状态、拉黑 ACL、收藏、文件助手、删除好友、黑名单视图、群二维码、群主转让、真实文件上传与附件 GC 绑定、群业务权限、完整群管理流程和完整 WebRTC 信令；真实浏览器音视频采集、设备权限、NAT 穿透质量仍需在前端或真机环境补测。
4. 当前目录不是 git 仓库，无法通过 `git status` 生成变更基线。

## 开发后回归记录（十八）

> 执行时间：2026-07-31 16:00 CST

本轮实现并验证 Phase 3 MVP：反诈风控、内容审核标记、话题推荐排序、MySQL 全文搜索优化。设计原则仍保持小改动：不新增 HTTP 业务接口，不修改 Tinode 原生协议含义，前后端继续通过 WebSocket `x-im-*` 扩展字段对接。

代码与数据库变更：

| 项 | 说明 |
|---|---|
| 反诈风控 | `{pub}` 入库和广播前调用 `imReviewPubMessage`，命中转账、验证码、外链、违禁词等规则时写入 `head.x-im-anti-fraud=true`、`head.x-im-risk-level`、`head.x-im-moderation` |
| 内容审核 | 当前为内置规则标记 MVP，只标记 `flagged` 与 `reasons`，不阻断消息，避免误伤主聊天链路 |
| 推荐排序 | `fnd` 的 `{get what="sub"}` 支持 `x-im-recommend`，MySQL adapter 通过 `FindRecommendedTopics` 按关键词、类型标签、业务标签、订阅数和活跃时间排序 |
| 全文搜索优化 | `im_message_index.search_text` 增加 `FULLTEXT INDEX im_message_index_search_text(search_text)`；查询使用 `MATCH ... AGAINST` + `LIKE` fallback |
| 数据库版本 | MySQL adapter/schema 从 `118` 升级到 `119` |
| 顺手修复 | MySQL `UserDelete` 硬删除拥有 topic 的用户时，`tx.Exec` 参数切片已改为 `args...` 展开 |

数据库执行与确认：

| 命令 | 结果 |
|---|---|
| `go run -tags mysql ./tinode-db -config=server/tinode.conf -upgrade` | 业务库从 `118` 升级到 `119`，输出 `Database successfully upgraded.` |
| `SELECT value FROM kvmeta WHERE key='version'` | 返回 `119` |
| `SHOW INDEX FROM im_message_index WHERE Key_name='im_message_index_search_text'` | 返回 `Index_type=FULLTEXT` |

已执行以下测试：

| 命令 | 结果 |
|---|---|
| `gofmt -w server/datamodel.go server/imext.go server/topic.go server/store/types/types.go server/db/mysql/adapter.go server/db/mysql/tests/mysql_test.go server/imext_test.go tools/wsmock/main.go` | 通过 |
| `go test -count=1 ./server` | 通过，`ok github.com/tinode/chat/server 0.032s` |
| `go test -count=1 ./tools/wsmock` | 通过，无测试文件 |
| `go test -count=1 -tags mysql ./server/db/mysql/tests` | 通过，`ok github.com/tinode/chat/server/db/mysql/tests 0.688s` |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过，包含核心 server、MySQL adapter tests、`tools/wsmock` 等包 |
| `go run -tags mysql . -config=tinode.conf -static_data=-` | 服务启动成功，监听 `:6060`，MySQL adapter schema `119` |
| `go run ./tools/wsmock -apikey '<generated>' -scenario phase3 -timeout 12s` | 通过，输出 `wsmock scenario "phase3" passed` |

WebSocket 联调观察：

| 协议点 | 实际返回 |
|---|---|
| 风险消息 | Alice 发送 `请点击链接并输入验证码后转账` 后，Bob 收到 `{data}`，`head.x-im-anti-fraud=true` |
| 风险等级 | 返回 `head.x-im-risk-level="high"` |
| 审核标记 | 返回 `head.x-im-moderation.status="flagged"`，`reasons=["payment-scam","credential-risk","external-link"]` |
| 推荐查询 | Alice 通过 `fnd` 发送 `{get what="sub" x-im-recommend={keyword:"发布会",types:["news"],labels:["美食"]}}` |
| 推荐返回 | 返回 `meta.sub`，频道话题按 Tinode 规范显示为 `chn...`，并带 `public.x-im-topic` 与标签列表 |

后续可演进项：

| 能力 | 当前状态 | 后续建议 |
|---|---|---|
| 风控 | 内置关键词规则标记 | 接入独立风控服务、用户画像、设备/IP 规则 |
| 内容审核 | 规则命中后 `flagged` | 建后台审核队列、人工复核和处置闭环 |
| 推荐 | MySQL 轻量排序 | 引入异步行为统计、热度衰减、个性化召回 |
| 全文搜索 | MySQL FULLTEXT + LIKE fallback | 如果搜索量上升，再接 OpenSearch/Elastic/Manticore 等独立搜索服务 |

## 开发后回归记录（十九）

> 执行时间：2026-07-31 16:31 CST

本轮补齐好友审批状态与进群审批队列，继续保持 WebSocket + Tinode ACL 的实现方式，不新增数据库表、不新增 HTTP 业务接口。

代码变更：

| 项 | 说明 |
|---|---|
| 好友审批状态 | P2P 订阅 `private` 增加 `x-im-applyStatus`，支持 `pending/accepted/rejected/expired` 枚举校验 |
| 进群申请备注 | 群订阅 `private` 增加 `x-im-joinApplyText` 与 `x-im-joinStatus`，备注最长 256 字符 |
| 待审批订阅 | 群 `joinApproval=true` 且默认 `defacs.auth=N` 时，申请人 `{sub set.sub.mode="JRWPS"}` 会保存 `want=JRWPS/given=N`，但不会 attach 到群 |
| 管理员队列 | 管理员 `{get topic=grp what=sub}` 可看到待审批用户的 `acs` 与 `private.x-im-join*` |
| 审批流转 | 管理员 `{set sub user=... mode="JRWPS"}` 自动把申请人 `x-im-joinStatus` 更新为 `accepted`；`mode="N"` 自动更新为 `rejected` |
| 审批留痕 | 新增 `x-im-type="group-join-approve/group-join-reject"` 消息类型，管理员可按需发布审批结果消息 |
| Mock 联调 | 新增 `tools/wsmock -scenario approvals`，覆盖好友状态、非法状态校验、进群 pending/accepted/rejected |

已执行以下测试：

| 命令 | 结果 |
|---|---|
| `gofmt -w server/imext.go server/topic.go server/imext_test.go server/topic_test.go tools/wsmock/main.go` | 通过 |
| `go test -count=1 ./server` | 通过，`ok github.com/tinode/chat/server 0.027s` |
| `go test -count=1 ./tools/wsmock` | 通过，无测试文件 |
| `go test -count=1 -tags mysql ./server/db/mysql/tests` | 通过，`ok github.com/tinode/chat/server/db/mysql/tests 0.936s` |
| `go test -count=1 -tags mysql $(go list ./... \| grep -v '/server/db/mongodb/tests$' \| grep -v '/server/db/postgres/tests$' \| grep -v '/server/db/rethinkdb/tests$')` | 通过，包含核心 server、MySQL adapter tests、`tools/wsmock` 等包 |
| `go run -tags mysql . -config=tinode.conf -static_data=-` | 服务启动成功，监听 `:6060`，MySQL adapter schema `119` |
| `go run ./tools/wsmock -apikey '<generated>' -scenario approvals -timeout 12s` | 通过，输出 `wsmock scenario "approvals" passed` |

WebSocket 联调观察：

| 协议点 | 实际返回 |
|---|---|
| 好友申请 | Alice 创建 P2P 时写入 `private.x-im-applyText` 与 `x-im-applyStatus=pending`，`me.sub.private` 可读回 |
| 好友同意 | Bob 订阅 P2P 并写入 `x-im-applyStatus=accepted`；Alice 更新后从联系人列表读回 `accepted` |
| 好友拒绝 | Alice 写入 `x-im-applyStatus=rejected` 后从 `me.sub.private` 读回 |
| 非法状态 | `x-im-applyStatus="done"` 返回 `400 malformed` |
| 进群申请 | Carol 对审批群 `{sub set.sub.mode="JRWPS"}` 返回 `200 ok`，`acs.want=JRWPS/given=N/mode=N` |
| 待审批发言 | Carol 未 attach 到群，发言返回 `409 must attach first` |
| 管理员队列 | Alice `{get what=sub}` 可看到 Carol 的 `private.x-im-joinApplyText` 与 `x-im-joinStatus=pending` |
| 通过入群 | Alice 授权 Carol `mode=JRWPS` 后，Carol 订阅群并读回 `x-im-joinStatus=accepted`，随后可发言 |
| 拒绝入群 | Dave 申请后 Alice 设置 `mode=N`，管理员列表读回 `x-im-joinStatus=rejected` |

仍需后续承接：

| 能力 | 说明 |
|---|---|
| 平台主动系统通知 | 仍需要系统账号、运营后台或服务端投递入口 |
| 审批后台页面 | 后端状态已可查可改，管理页面需要前端按 `me.sub` 和 `grp.sub` 渲染 |
| 真机推送 | 当前 WS 在线链路已通，离线 push 文案与跳转需要移动端补测 |
