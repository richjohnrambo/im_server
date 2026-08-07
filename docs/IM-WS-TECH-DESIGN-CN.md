# IM WebSocket 后端技术方案

> 本文档基于 [IM WebSocket 前后端接口文档](/Users/superysh/im/chat-0.25.3/docs/IM-WS-API-CN.md:1)、Tinode 原生 [API 文档](/Users/superysh/im/chat-0.25.3/docs/API.md:1)、[代码通读指南](/Users/superysh/im/chat-0.25.3/docs/CODE-WALKTHROUGH.md:1) 与当前 Go 代码整理，作为后续后端开发依据。目标是保持 Tinode 现有协议和代码风格，默认全部前后端业务通信走 WebSocket，尽量复用现有 Topic、Session、Store、ACL、Drafty、文件上传和 JSON 扩展字段，减少侵入式改动。

## 一、目标与原则

### 1.1 建设目标

| 目标 | 说明 |
|---|---|
| 保持 Tinode 协议兼容 | 继续使用 `{hi}`、`{login}`、`{sub}`、`{pub}`、`{get}`、`{set}`、`{del}`、`{note}` |
| 默认 WebSocket 通信 | 客户端统一连接 `/v0/channels`，服务端通过现有 `Session -> Hub -> Topic` 消息链路处理 |
| 小步扩展业务能力 | 好友、群聊、文件、收藏、通话优先映射到 Tinode 原生能力 |
| 小改动 | 第一阶段不新增数据库表，不改核心消息协议字段语义 |
| 可测试 | 所有新增逻辑必须补充 Go 单元测试，MySQL 持久化改动必须补 adapter 集成测试 |

### 1.2 设计原则

1. 原生字段优先：用户资料使用 `users.public`，用户私有设置使用 `subscriptions.private` 或 `users.private`，群业务配置使用 `topics.aux`。
2. 扩展字段隔离：所有产品扩展字段统一使用 `x-im-*`，避免污染 Tinode 原生字段。
3. 权限使用 ACL：管理员、禁言、踢人、群主转让优先通过 Tinode `modewant/modegiven` 权限实现。
4. 持久消息走 `{pub}`：文本、图片、文件、语音、名片、转发、收藏、系统通知都以持久化消息建模。
5. 临时状态走 `{note}`：输入中、录音中、已收、已读、通话信令使用临时通知，不落库。
6. 数据库变更后置：只有搜索、积分、签到这类确实需要索引或事务的能力才新增业务表。

## 二、系统设计

### 2.1 总体架构

```text
Client
  |
  | WebSocket JSON frames
  v
server/hdl_websock.go
  |
  v
Session.dispatchRaw / Session.dispatch
  |
  +-- hi/login/acc: Session 内处理
  |
  +-- sub/get/set/del/pub/note
        |
        v
      Hub
        |
        v
      Topic.runLocal
        |
        +-- handleSubscription
        +-- handleClientMsg
        +-- handleMeta
        +-- handleServerMsg
        |
        v
      store.* interfaces
        |
        v
      server/db/mysql adapter
```

本方案不新增独立 HTTP 业务 API。除 Tinode 已有大文件上传 `/v0/file/u/` 与下载 `/v0/file/s/` 外，业务请求全部复用 WebSocket 消息包。

### 2.2 功能映射

| 原型能力 | 后端实现 |
|---|---|
| 登录、注册、忘记密码 | 复用 `{hi}`、`{login}`、`{acc}`、`basic/token/code/rest` auth |
| 私聊 | `{sub topic="usrXXX"}` 创建或进入 P2P topic |
| 群聊 | `{sub topic="new"}` 创建群，`grpXXX` 进入群 |
| 消息列表 | 订阅 `me`，`{get what="sub"}` 获取会话列表 |
| 文本、表情、@ | `{pub}` + Drafty，`head.mentions` |
| 图片、文件、语音 | 上传文件后 `{pub}` + Drafty `IM/EX/AU` entity + `extra.attachments` |
| 已收、已读、输入中 | `{note what="recv/read/kp/kpa/kpv"}` |
| 撤回、删除 | `{del what="msg"}` |
| 好友申请 | P2P `{sub}` + ACL 状态，申请备注写 `private.x-im-applyText` |
| 备注、置顶、免打扰、拉黑 | `{set desc.private}` |
| 管理员、禁言、踢人 | `{set sub}`、`{del what="sub"}` + `aux.x-im-group` |
| 群公告、进群验证 | `{set aux}` + 可选系统样式 `{pub}` |
| 收藏 | 使用 `slf` topic 存储 `head.x-im-type="favorite"` 消息 |
| 通话 | `{note what="call"}` 信令，结束后 `{pub}` 持久化通话记录；产品通话邀请和控制事件校验 `callId` |
| 话题 | 使用 `nch/chn` 频道建模，话题元数据写 `public.x-im-topic` |
| 举报、反馈 | 向 `sys` topic 发送 `x-im-type="report/feedback"` 消息 |
| 系统消息 | 系统账号或 `sys` topic 投递 `x-im-type="system-notice"` |

### 2.3 分阶段实现

| 阶段 | 范围 | 数据库 |
|---|---|---|
| Phase 1 | 登录、私聊、群聊、消息、附件、回执、基础好友、群管理、收藏、通话信令、举报反馈 | 不新增表 |
| Phase 2 | 聊天记录关键词搜索、按文件类型搜索、话题积分、签到幂等、运营后台检索 | 新增少量 `im_*` 业务表 |
| Phase 3 | 反诈风控、内容审核、推荐排序、全文搜索优化 MVP | 复用 `messages/topics/topictags` 与 `im_message_index`，MySQL schema `119` 新增全文索引 |

当前 Phase 1、Phase 2 与 Phase 3 MVP 已按小改动路线接入。更复杂的风控模型、人工审核工作台、离线推荐服务和独立搜索引擎仍作为后续演进，不影响现有 WebSocket 对接。

## 三、代码结构设计

### 3.1 现有代码复用点

| 文件/模块 | 复用方式 |
|---|---|
| [server/hdl_websock.go](/Users/superysh/im/chat-0.25.3/server/hdl_websock.go:117) | 保持 WebSocket 入口不变 |
| [server/session.go](/Users/superysh/im/chat-0.25.3/server/session.go:431) | 复用消息分发、鉴权、topic 展开逻辑 |
| [server/hub.go](/Users/superysh/im/chat-0.25.3/server/hub.go:93) | 复用 topic 创建、加载、路由 |
| [server/topic.go](/Users/superysh/im/chat-0.25.3/server/topic.go:568) | 复用订阅、发布、meta、presence、call 处理 |
| [server/datamodel.go](/Users/superysh/im/chat-0.25.3/server/datamodel.go:336) | 复用 C2S/S2C 消息结构 |
| [server/store/store.go](/Users/superysh/im/chat-0.25.3/server/store/store.go:505) | 复用 Topic/Sub/Message/File/PCache 抽象 |
| [server/db/mysql/adapter.go](/Users/superysh/im/chat-0.25.3/server/db/mysql/adapter.go:1) | 复用 MySQL adapter、迁移机制、业务索引与推荐查询 |
| [server/drafty](/Users/superysh/im/chat-0.25.3/server/drafty/drafty.go:1) | 复用富文本纯文本提取与预览能力 |
| [server/store/mock_store](/Users/superysh/im/chat-0.25.3/server/store/mock_store/mock_store.go:1) | 新增测试时继续使用 gomock |

### 3.2 建议新增文件

第一阶段新增文件控制在少量 helper 与测试文件：

| 文件 | 作用 |
|---|---|
| `server/imext.go` | 定义 `x-im-*` 常量、消息类型枚举、Drafty/JSON 解析辅助方法 |
| `server/imext_test.go` | 测试扩展字段解析、消息类型判断、搜索文本提取 |
| `server/im_policy.go` | 封装业务策略：群禁言、进群验证、拉黑、反诈标记判断 |
| `server/im_policy_test.go` | 业务策略单元测试 |

第二阶段如需数据库索引，再新增：

| 文件 | 作用 |
|---|---|
| `server/store/imindex.go` | 定义业务索引查询接口，避免把搜索逻辑塞进 Topic |
| `server/db/mysql/imindex.go` | MySQL 业务索引表读写实现 |
| `server/db/mysql/imindex_test.go` | MySQL 业务索引集成测试 |

### 3.3 最小代码变更点

#### 3.3.1 扩展字段解析

新增 `server/imext.go`，只做轻量解析，不改变原有消息结构语义。

```go
const (
	imHeadType      = "x-im-type"
	imHeadClientMID = "x-im-client-mid"
	imHeadAntiFraud = "x-im-anti-fraud"
)

type imMessageType string

const (
	imMsgText              imMessageType = "text"
	imMsgImage             imMessageType = "image"
	imMsgAudio             imMessageType = "audio"
	imMsgVideo             imMessageType = "video"
	imMsgFile              imMessageType = "file"
	imMsgCard              imMessageType = "card"
	imMsgFavorite          imMessageType = "favorite"
	imMsgForwardBundle     imMessageType = "forward-bundle"
	imMsgGroupJoinApply    imMessageType = "group-join-apply"
	imMsgGroupAnnouncement imMessageType = "group-announcement"
	imMsgCall              imMessageType = "call"
	imMsgTopicShare        imMessageType = "topic-share"
	imMsgSystemNotice      imMessageType = "system-notice"
	imMsgReport            imMessageType = "report"
	imMsgFeedback          imMessageType = "feedback"
	imMsgCheckin           imMessageType = "checkin"
)
```

解析 helper 建议只返回类型与布尔值，不在业务层直接类型断言散落代码：

```go
func imGetMessageType(head map[string]any) imMessageType
func imIsAttachmentType(tp imMessageType) bool
func imExtractSearchText(head map[string]any, content any) string
func imValidateCallPayload(payload json.RawMessage) error
```

#### 3.3.2 查询扩展字段

当前 `MsgGetQuery` 没有 `x-im-search` 或 `x-im-filter` 字段，Go 反序列化会忽略未知字段。若第一阶段需要服务端处理聊天记录搜索，需要在 [server/datamodel.go](/Users/superysh/im/chat-0.25.3/server/datamodel.go:40) 显式增加字段：

```go
type MsgGetQuery struct {
	What string `json:"what"`

	Desc *MsgGetOpts `json:"desc,omitempty"`
	Sub  *MsgGetOpts `json:"sub,omitempty"`
	Data *MsgGetOpts `json:"data,omitempty"`
	Del  *MsgGetOpts `json:"del,omitempty"`

	XIMSearch *MsgSearchOpts `json:"x-im-search,omitempty"`
	XIMFilter *MsgFilterOpts `json:"x-im-filter,omitempty"`
}

type MsgSearchOpts struct {
	Keyword string   `json:"keyword,omitempty"`
	Types   []string `json:"types,omitempty"`
}

type MsgFilterOpts struct {
	Types   []string `json:"types,omitempty"`
	Keyword string   `json:"keyword,omitempty"`
}
```

第一阶段可以先不实现数据库索引，`Topic.replyGetData` 仍按 `data.limit` 拉取消息，再在内存里用 `imGetMessageType` 和 `imExtractSearchText` 做轻量过滤。第二阶段再替换为索引表查询。

#### 3.3.3 群禁言与拉黑策略

Tinode ACL 已支持写权限控制。业务层只需约定：

| 场景 | 实现 |
|---|---|
| 群禁言成员 | 将目标用户 `mode` 设置为不含 `W` 的完整权限，如 `JRP`；同时维护 `aux.x-im-group.mutedUsers` |
| 解除群禁言 | 将目标用户 `mode` 设置为含 `W` 的完整权限，如 `JRWP` 或 `JRWPS`；同时清理 `aux.x-im-group.mutedUsers` |
| 拉黑好友 | 当前用户 `desc.private.x-im-blocked=true`，同时可调整对方 P2P 权限 |
| 群全员禁言 | `aux.x-im-group.muteAll=true`，发送消息时后端策略拒绝普通成员 |
| 单成员业务禁言 | `aux.x-im-group.mutedUsers`，发送消息时后端策略拒绝列表中的普通成员 |
| 群文件上传限制 | `aux.x-im-group.allowMemberFileUpload=false`，普通成员不能发送附件类消息 |
| 仅管理员邀请 | `aux.x-im-group.onlyAdminInvite=true`，非管理员不能邀请或重新邀请成员 |

群级业务策略是 Tinode 原生 ACL 不能一次表达的约束，在 `Topic.handlePubBroadcast` 与 `Topic.anotherUserSub` 进入持久化前做小的策略检查：

```go
if err := imCanPublish(t, asUid, msg); err != nil {
	msg.sess.queueOut(ErrPermissionDeniedReply(msg, msg.Timestamp))
	return
}
```

`imCanPublish` 与 `imCanInvite` 只读取 `t.aux` 与 `t.perUser`，不访问数据库，避免扩大影响面。

#### 3.3.4 收藏

收藏不新增接口，不新增表。客户端向 `slf` 发送 `x-im-type="favorite"` 消息即可。

后端只需保证：

1. `slf` topic 可正常创建和订阅。
2. 收藏消息里的附件 URL 被写入 `extra.attachments`，复用文件 GC 引用计数。
3. 收藏列表通过 `{sub topic="slf" get.what="data"}` 返回。

#### 3.3.5 话题

话题用频道建模：

1. 发布话题：`{sub topic="nch"}` 创建 channel topic。
2. 话题详情：读取 `desc.public.x-im-topic`。
3. 参与话题：订阅 `chnXXX`。
4. 评论：根据业务授予 `W` 权限后使用 `{pub}`。

第一阶段不做积分扣减时，不需要新增表。第二阶段接入积分时增加 `im_user_points`。

### 3.4 不建议改动的地方

| 模块 | 原因 |
|---|---|
| WebSocket handler | 入口稳定，不应新增业务分支 |
| Hub 路由模型 | 现有 channel/goroutine 模型清晰，业务应落在 Topic/Store |
| 原生消息字段命名 | 客户端 SDK 与文档依赖现有字段 |
| Adapter 核心接口 | 第一阶段避免给所有数据库后端增加方法 |
| gRPC converter | 本项目默认 WS，不做 gRPC 扩展 |

## 四、数据库设计与变更

### 4.1 当前可复用字段

现有 MySQL schema 已经覆盖多数业务扩展需求：

| 表 | 字段 | 用途 |
|---|---|---|
| `users` | `public` | 昵称、头像、性别、生日、地区、个性签名 |
| `users` | `tags` + `usertags` | 用户查找、手机号/用户名发现 |
| `topics` | `public` | 群资料、话题资料、频道封面 |
| `topics` | `aux` | 群公告、进群验证、群设置、全员禁言 |
| `subscriptions` | `private` | 备注、置顶、免打扰、拉黑、草稿 |
| `messages` | `head` | `x-im-type`、附件类型、引用、转发、@ |
| `messages` | `content` | Drafty 或业务 JSON 内容 |
| `fileuploads` | 全表 | 上传文件记录 |
| `filemsglinks` | 全表 | 文件与消息/topic/user 绑定，防止 GC |
| `kvmeta` | 全表 | 小型持久缓存，已有 `PCache` 封装 |

### 4.2 Phase 1 数据库变更

Phase 1 不新增表、不新增字段。

原因：

1. 用户资料、群资料、群设置、收藏、举报、反馈都可用 JSON 字段或消息承载。
2. 图片、文件、语音已经有 `fileuploads` 与 `filemsglinks`。
3. 消息查询先复用 `messages(topic, seqid)` 分页。
4. 小改动优先，先保证 WebSocket 主流程稳定。

### 4.3 Phase 2 数据库变更

聊天记录搜索、话题积分、签到等功能进入后端强一致实现时新增业务表，表名统一 `im_` 前缀，避免与 Tinode 原生表混淆。当前 MySQL adapter 已实现 `im_message_index`、`im_user_points`、`im_user_daily_events`，数据库版本为 `118`。

#### 4.3.1 消息业务索引表

用途：支持按消息类型、关键词、附件维度查询，避免频繁扫描 `messages.content` JSON。

```sql
CREATE TABLE im_message_index(
	id INT NOT NULL,
	createdat DATETIME(3) NOT NULL,
	updatedat DATETIME(3) NOT NULL,
	topic CHAR(25) NOT NULL,
	seqid INT NOT NULL,
	fromuid BIGINT NOT NULL,
	msgtype VARCHAR(32) NOT NULL,
	search_text TEXT,
	attachment_count INT NOT NULL DEFAULT 0,
	deletedat DATETIME(3),

	PRIMARY KEY(id),
	FOREIGN KEY(id) REFERENCES messages(id) ON DELETE CASCADE,
	UNIQUE INDEX im_message_index_topic_seqid(topic, seqid),
	INDEX im_message_index_topic_type_seq(topic, msgtype, seqid),
	INDEX im_message_index_topic_createdat(topic, createdat),
	FOREIGN KEY(topic) REFERENCES topics(name) ON DELETE CASCADE
);
```

说明：`id` 与 Tinode 现有 `messages.id` 保持一致使用 `INT`。中文全文检索初期使用 `LIKE`，MySQL `FULLTEXT` 是否启用 `ngram` parser 作为后续部署决策，不作为第一版强依赖。

#### 4.3.2 用户积分表

用途：话题发布积分扣减、签到积分发放。

```sql
CREATE TABLE im_user_points(
	userid BIGINT NOT NULL,
	createdat DATETIME(3) NOT NULL,
	updatedat DATETIME(3) NOT NULL,
	balance BIGINT NOT NULL DEFAULT 0,
	frozen BIGINT NOT NULL DEFAULT 0,

	PRIMARY KEY(userid),
	FOREIGN KEY(userid) REFERENCES users(id) ON DELETE CASCADE
);
```

#### 4.3.3 用户每日事件表

用途：签到幂等、每日任务、青少年模式时长限制等按日事件。

```sql
CREATE TABLE im_user_daily_events(
	id BIGINT NOT NULL AUTO_INCREMENT,
	createdat DATETIME(3) NOT NULL,
	userid BIGINT NOT NULL,
	event_date DATE NOT NULL,
	event_type VARCHAR(32) NOT NULL,
	payload JSON,

	PRIMARY KEY(id),
	UNIQUE INDEX im_user_daily_events_user_date_type(userid, event_date, event_type),
	FOREIGN KEY(userid) REFERENCES users(id) ON DELETE CASCADE
);
```

### 4.4 MySQL 迁移方式

Phase 2/3 按现有 MySQL adapter 风格升级。当前 MySQL adapter 已从 `116` 升级到 `119`，其中 `117` 新增 `im_message_index`，`118` 新增 `im_user_points` 与 `im_user_daily_events`，`119` 为 `im_message_index.search_text` 增加全文索引：

1. [server/db/mysql/adapter.go](/Users/superysh/im/chat-0.25.3/server/db/mysql/adapter.go:44) 中 `adpVersion` 已更新为 `119`。
2. `CreateDb` 的建表流程已加入三张 `im_*` 表，并在 `im_message_index` 上创建 `FULLTEXT INDEX im_message_index_search_text(search_text)`。
3. `UpgradeDb` 已增加 `116 -> 117`、`117 -> 118` 与 `118 -> 119` 迁移块。
4. 升级旧库到 `117` 时会扫描历史 `messages` 与 `filemsglinks` 回填索引，避免历史消息搜索缺失。
5. 升级旧库到 `118` 时创建积分表和每日事件表。
6. 升级旧库到 `119` 时通过 `ALTER TABLE im_message_index ADD FULLTEXT INDEX im_message_index_search_text(search_text)` 优化全文搜索。
7. 迁移成功后通过 `bumpVersion(a, 119)` 更新 `kvmeta.version`。
8. schema 文档 [server/db/mysql/schema.sql](/Users/superysh/im/chat-0.25.3/server/db/mysql/schema.sql:1) 已同步。
9. MySQL adapter 集成测试已覆盖索引保存、搜索、全文索引存在性、推荐排序、软删除过滤、积分增扣和每日事件幂等。

不建议在第一阶段修改 `adapter.Adapter` 总接口，因为这会要求 PostgreSQL、MongoDB、RethinkDB 同步实现，改动面会变大。业务索引可以在第二阶段单独设计 `imstore` 接口，再按实际支持的数据库接入。

## 五、核心业务设计

### 5.1 登录注册与资料

使用现有认证模块：

| 功能 | 实现 |
|---|---|
| 手机号密码登录 | `basic` 或 `rest` 认证 |
| Token 登录 | `token` 认证 |
| 验证码 | `code` 临时认证或 credential validator |
| 忘记密码 | `{login scheme="reset"}` |
| 实名认证 | `me.desc.private.x-im-realname` 保存申请，审核通过后 root 写 `trusted.verified=true` |

代码改动：无核心改动。若对接外部账号系统，优先配置 `auth/rest`，避免改 `auth/basic`。

### 5.2 好友关系

P2P topic 即好友/联系人关系。添加好友通过 `{sub topic="usrXXX"}`，是否需要审批由 ACL 决定。

建议规则：

| 状态 | ACL/字段 |
|---|---|
| 申请中 | `private.x-im-applyStatus="pending"`，ACL 仍记录当前 `want/given` |
| 已同意 | `private.x-im-applyStatus="accepted"`，双方 P2P 权限满足可聊条件 |
| 已拒绝/过期 | `private.x-im-applyStatus="rejected/expired"`，前端从 `me.sub.private` 过滤 |
| 已好友 | 双方 P2P 权限满足 `JRWPA` |
| 已拉黑 | 当前用户 `private.x-im-blocked=true`，必要时移除对方 `W` |
| 已删除 | `{leave unsub=true}` |

### 5.3 群聊与群权限

复用 Tinode group topic：

| 群角色 | Tinode 权限 |
|---|---|
| 群主 | `O` |
| 管理员 | `A/S/D` |
| 普通成员 | `J/R/W/P` |
| 禁言成员 | 移除 `W` |
| 被踢成员 | 删除订阅 |

群设置统一写 `topics.aux.x-im-group`。前端展示用 `get what="desc sub aux"` 获取。

进群审批使用已有 subscriptions 表表达状态，不新增表：开启 `aux.x-im-group.joinApproval=true` 且群默认 `defacs.auth` 不含 `J` 时，申请者 `{sub set.sub.mode="JRWPS"}` 会创建 `want=JRWPS/given=N` 的待审批订阅，并保存 `private.x-im-joinApplyText` 与 `private.x-im-joinStatus="pending"`。管理员 `{get what="sub"}` 可看到待审批用户及其 private；通过时 `{set sub user=... mode="JRWPS"}` 自动把状态更新为 `accepted`，拒绝时 `{set sub user=... mode="N"}` 自动更新为 `rejected`。

### 5.4 消息与附件

所有消息仍由 `Topic.handlePubBroadcast` 保存为 `messages`：

1. 文本、表情、@ 使用 Drafty。
2. 图片、文件、语音、视频先上传文件，再发送 Drafty entity。
3. `extra.attachments` 必填，复用 `store.Messages.Save` 中的附件绑定逻辑。
4. `head.x-im-client-mid` 用于前端本地发送状态关联。
5. `head.x-im-type` 用于客户端渲染和后续索引。

### 5.5 收藏

收藏使用 `slf` topic，不新增接口：

| 操作 | 实现 |
|---|---|
| 收藏消息 | `{pub topic="slf" head.x-im-type="favorite"}` |
| 查看收藏 | `{sub topic="slf" get.what="data"}` |
| 删除收藏 | `{del topic="slf" what="msg"}` |
| 分享收藏 | 普通转发 `{pub}` |

### 5.6 通话

通话信令复用 `{note what="call"}`：

| 事件 | `note.event` |
|---|---|
| 已振铃 | `ringing` |
| 接受通话 | `accept` |
| SDP offer | `offer` |
| SDP answer | `answer` |
| ICE 候选 | `ice-candidate` |
| 拒绝/挂断 | `hang-up` |

通话结束后发送持久化 `{pub}`，`head.webrtc` 与 `head.webrtc-duration` 使用 Tinode 原生字段。

### 5.7 话题

话题使用 channel topic：

| 操作 | 实现 |
|---|---|
| 发布话题 | `{sub topic="nch"}` |
| 话题列表 | `fnd` 搜索 `topic` tags |
| 话题详情 | `{sub topic="chnXXX" get.what="desc data aux"}` |
| 参与话题 | 订阅 `chnXXX` |
| 转发帖子 | `{pub head.x-im-type="topic-share"}` |

积分能力进入 Phase 2 后，通过 `im_user_points` 做事务扣减；当前签到已通过 `{pub topic="slf" head.x-im-type="checkin"}` 接入 `im_user_daily_events` 与 `im_user_points`，首次签到固定发放 `5` 积分，重复签到返回 `304 no action`。话题发布已通过 `{sub topic="nch"}` 接入 `public.x-im-topic.pointsCost`，MySQL 后端在同一事务中创建频道 topic、作者订阅并扣减积分，余额不足返回 `422 policy violation`。

### 5.8 举报、反馈与安全

举报和反馈向 `sys` topic 发送持久化消息。root/admin 后台订阅 `sys` 处理。

反诈与内容审核 Phase 3 MVP 已在发布链路内置轻量规则：`Topic.handlePubBroadcast` 保存消息前调用 `imReviewPubMessage`，命中疑似转账、验证码、外链、违禁词等关键词时追加 `head.x-im-anti-fraud=true`、`head.x-im-risk-level` 与 `head.x-im-moderation`。当前策略只标记不拦截，避免误伤聊天主流程；复杂风控评分、人工审核队列、模型服务可在后续通过相同 `head` 字段向前端兼容扩展。

### 5.9 推荐排序与搜索优化

话题推荐仍复用 Tinode `fnd` topic，避免新增 HTTP 接口或修改通用 adapter 接口。前端发送 `{get topic="fnd" what="sub" x-im-recommend={...}}`，`topic.go` 在 plugin find 未返回结果时检测 MySQL adapter 是否实现 `FindRecommendedTopics`，再按关键词、类型标签、业务标签、订阅数和活跃时间排序返回。

聊天记录全文搜索继续走 `im_message_index`。MySQL schema `119` 在 `search_text` 上增加 `FULLTEXT` 索引，查询条件使用 `MATCH ... AGAINST` 与 `LIKE ESCAPE` 组合：优先利用全文索引，仍保留中文短词和简单子串查询的兼容性。

## 六、单元测试与集成测试方案

### 6.1 已有测试继续保留

后续开发前后建议固定跑：

```bash
go test -count=1 -tags mysql $(go list ./... | grep -v '/server/db/mongodb/tests$' | grep -v '/server/db/postgres/tests$' | grep -v '/server/db/rethinkdb/tests$')
```

MySQL adapter 变更补跑：

```bash
go test -count=1 -tags mysql ./server/db/mysql/tests
```

构建检查：

```bash
go build -tags mysql ./server ./tinode-db
```

### 6.2 新增单元测试用例

#### `server/imext_test.go`

| 用例 | 目的 |
|---|---|
| `TestIMGetMessageType` | 从 `head.x-im-type` 正确读取消息类型 |
| `TestIMGetMessageTypeMissing` | 缺失类型时回退为普通文本或空类型 |
| `TestIMIsAttachmentType` | 判断 image/audio/video/file 是否为附件类型 |
| `TestIMExtractSearchTextPlain` | 从普通字符串内容提取搜索文本 |
| `TestIMExtractSearchTextDrafty` | 从 Drafty `txt` 提取搜索文本 |
| `TestIMExtractSearchTextJSON` | 从业务 JSON 的 `title/summary/text` 提取搜索文本 |
| `TestIMValidateCallPayload` | 校验 call payload 必填字段 |
| `TestIMValidateCallPayloadMalformed` | 非法 JSON 或缺失 callId 返回错误 |
| `TestIMReviewPubMessageFlagsFraudRisk` | 风控规则命中后追加反诈、风险等级和审核标记 |

#### `server/session_test.go`

| 用例 | 目的 |
|---|---|
| `TestDispatchGetWithXIMSearch` | 确认 `{get}` 中 `x-im-search` 不会被丢弃 |
| `TestDispatchPublishWithXIMClientMID` | 确认 `head.x-im-client-mid` 原样进入 topic |
| `TestDispatchReportToSys` | 普通用户可向 `sys` 发布举报消息 |
| `TestDispatchFavoriteToSlf` | 用户可向 `slf` 发布收藏消息 |

#### `server/topic_test.go`

| 用例 | 目的 |
|---|---|
| `TestHandleBroadcastDataWithXIMType` | 消息保存后 `head.x-im-type` 原样广播 |
| `TestHandleBroadcastDataWithAttachments` | 已有附件测试继续覆盖 `extra.attachments` |
| `TestHandleBroadcastDataGroupMuteAll` | 群全员禁言时普通成员发送被拒绝 |
| `TestHandleBroadcastDataGroupMuteAllAdminAllowed` | 管理员在全员禁言时可发送 |
| `TestHandleBroadcastCardMessage` | 名片消息 JSON 内容原样投递 |
| `TestReplyGetDataWithXIMTypeFilter` | 第一阶段内存过滤按类型返回 |
| `TestReplyGetDataWithXIMKeywordSearch` | 第一阶段内存过滤按关键词返回 |
| `TestHandleCallNoteMediaEvent` | 通话媒体状态 `{note}` 正确转发为 `{info}` |
| `TestGroupJoinApplyMessage` | 加群申请消息可持久化并广播给管理员 |
| `TestGroupAnnouncementAuxUpdate` | 群公告写入 `aux` 后产生 `pres what="aux"` |

#### `server/store/types` 或 `server/db/common`

| 用例 | 目的 |
|---|---|
| `TestMergeMapsWithXIMNestedValues` | 嵌套 `x-im-*` 设置更新不破坏已有值 |
| `TestNullValueClearsXIMSetting` | 使用 `types.NullValue` 清理扩展字段 |

### 6.3 Phase 2 MySQL 集成测试

`im_*` 表需要补充 MySQL 集成测试：

| 用例 | 目的 |
|---|---|
| `TestIMMessageIndexCreateDb` | `CreateDb(reset=true)` 创建索引表 |
| `TestIMMessageIndexUpgradeDb` | 旧版本升级到新版本后表存在 |
| `TestIMMessageIndexUpsert` | 消息索引写入/更新成功 |
| `TestIMMessageIndexSearchByType` | 按消息类型查询 |
| `TestIMMessageIndexSearchByKeyword` | 按关键词查询 |
| `TestIMUserPointsAddAndConsume` | 积分增加与扣减 |
| `TestIMUserPointsInsufficient` | 积分不足时事务回滚 |
| `TestIMDailyEventIdempotent` | 同一用户同一天同类事件只能记录一次 |
| `TestIMDailyEventCreateWithPoints` | 每日事件与积分发放在同一事务中完成，重复事件不重复加分 |
| `TestIMMessageIndexFullTextExists` | MySQL schema `119` 已创建全文索引 |
| `TestFindRecommendedTopics` | 按关键词、类型和标签返回推荐话题 |

### 6.4 手工 WebSocket 联调用例

后端联调时至少覆盖：

1. 登录后订阅 `me/fnd`。
2. 创建 P2P 并发送文本、图片、文件、语音。
3. 发送 `recv/read/kp/kpa`，对端收到 `{info}`。
4. 创建群、邀请成员、设置管理员、禁言、解除禁言。
5. 设置群公告，客户端收到 `pres what="aux"`。
6. 收藏消息到 `slf` 并读取收藏列表。
7. 发起音视频通话，完成 offer/answer/hangup。
8. 发布话题频道并转发到私聊。
9. 举报消息发送到 `sys`。
10. 发送疑似风险文本，接收方收到 `x-im-anti-fraud` 与 `x-im-moderation`。
11. 通过 `fnd + x-im-recommend` 获取推荐话题，频道 topic 以 `chnXXX` 返回。

## 七、开发顺序建议

### 7.1 第一迭代

1. 新增 `server/imext.go` 与测试。
2. 明确 `x-im-*` 常量和消息类型解析。
3. 增加 `MsgGetQuery.XIMSearch/XIMFilter` 字段。
4. 在 `Topic.replyGetData` 中实现第一阶段内存过滤。
5. 增加群全员禁言、单成员禁言、成员文件上传限制策略 `imCanPublish`。
6. 增加 `x-im-type`、`private.x-im-*`、`aux.x-im-group` 轻量校验。
7. 支持创建群时保存 `set.aux.x-im-group` 到 `topics.aux`。
8. 增加仅管理员邀请策略 `imCanInvite`。
9. 补齐 session/topic 单元测试。

### 7.2 第二迭代

1. 完成好友申请状态、备注、拉黑、黑名单联调。
2. 完成群公告、群设置、管理员、禁言、踢人联调。
3. 完成收藏、文件助手、聊天记录分类筛选联调。
4. 完成通话信令和通话记录落库。

### 7.3 第三迭代

1. 已实施 `im_message_index` 与 MySQL 全文索引。
2. 已接入积分和签到表。
3. 已增加后端内置反诈/内容审核标记 MVP。
4. 已增加话题推荐轻量排序 MVP。
5. 后续如需要更高召回率或运营可控性，再引入独立搜索服务、风控模型、人工审核台和离线推荐任务。

## 八、风险与决策点

| 风险/决策 | 建议 |
|---|---|
| 直接改 adapter 接口会牵连多数据库 | 第一阶段避免，第二阶段单独设计业务 store |
| JSON 字段缺少强校验 | 用 `imext.go` 做轻量校验，关键状态由 ACL 保证 |
| 聊天记录关键词搜索性能 | 第一阶段内存过滤，第二阶段索引表或搜索服务 |
| 群全员禁言不是原生 ACL | 通过 `aux` + `imCanPublish` 策略补齐 |
| 积分扣减需要事务 | 不要写 JSON 字段，Phase 2 使用 `im_user_points` |
| 规则风控存在误判 | 当前只标记不拦截，客户端提示可降级，后续再接审核工作台 |
| 推荐排序召回有限 | 当前使用 MySQL 轻量排序，后续可替换为异步索引或推荐服务 |
| `go test ./...` 未指定 tag 会失败 | 使用测试报告中的 MySQL 标准命令 |
| 前端发送未知扩展字段被忽略 | 需要在 datamodel 显式声明查询扩展字段 |

## 九、验收标准

1. 所有新增 Go 代码通过 `gofmt`。
2. 不修改 Tinode 原生协议字段含义。
3. 新增业务字段统一使用 `x-im-*`。
4. 第一阶段不新增数据库表。
5. 核心测试与 MySQL 测试通过。
6. WebSocket 联调用例覆盖登录、私聊、群聊、文件、收藏、通话、举报、风控标记、推荐话题。
7. 文档中的接口示例与实际后端行为一致。

## 十、推荐测试命令

```bash
go test -count=1 ./server ./server/db/common ./server/drafty ./server/media ./server/ringhash ./server/store/types
```

```bash
go test -count=1 -tags mysql ./server/db/mysql/tests
```

```bash
go test -count=1 -tags mysql $(go list ./... | grep -v '/server/db/mongodb/tests$' | grep -v '/server/db/postgres/tests$' | grep -v '/server/db/rethinkdb/tests$')
```

```bash
go build -tags mysql ./server ./tinode-db
```

本地 WebSocket 联调：

```bash
go run ./keygen -salt 'T713/rYYgW7g4m3vG6zGRh7+FM1t0T8j13koXScOAj4='
```

```bash
go run -tags mysql ./server -config=server/tinode.conf -static_data=-
```

```bash
go run ./tools/wsmock -apikey '<generated>' -scenario all -timeout 12s
```

`tools/wsmock` 当前覆盖：登录注册、`slf` 自存消息、双端 P2P 文本/文件消息、好友申请备注 `x-im-applyText`、好友审批状态 `x-im-applyStatus`、联系人 `private.comment/x-im-muted/x-im-pinned/x-im-blocked` 读写、黑名单视图、删除好友、个人 `x-im-settings`、收藏列表、文件助手、拉黑 ACL 阻断与取消拉黑、真实 `/v0/file/u/` 上传、Drafty 附件消息、`extra.attachments` 防 GC 绑定、`recv/read` 回执、聊天记录 `x-im-search`、文件类型 `x-im-filter`、群创建/邀请/全员禁言/文件权限、进群审批 `x-im-joinApplyText/x-im-joinStatus`、群二维码公开资料、群主转让、群管理 `aux` 读取、仅管理员邀请、管理员授权、单成员禁言/解禁、管理员踢人、签到幂等、频道发布积分扣减、通话邀请参数校验、完整通话信令 `started/ringing/accept/offer/answer/ice-candidate/hang-up/finished`、举报到 `sys`。其中完整通话信令专项场景为 `call-flow`，需要服务端配置 `webrtc.enabled=true`；完整审批专项场景为 `approvals`；完整群管理专项场景为 `group-admin`；联系人状态专项场景为 `contacts`；页面型复用能力专项场景为 `pages`；群主转让专项场景为 `group-owner`；真实文件上传专项场景为 `upload`，会写入真实文件和数据库附件引用。
