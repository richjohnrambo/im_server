# 服务端 API 文档

> 本文档详细介绍 Tinode 服务端 API，包括消息格式、协议细节和使用方法。

---

## 目录

- [工作原理](#工作原理)
- [通用说明](#通用说明)
- [连接服务器](#连接服务器)
  - [gRPC](#grpc)
  - [WebSocket](#websocket)
  - [长轮询](#长轮询)
  - [带外大文件](#带外大文件)
- [用户](#用户)
  - [认证](#认证)
  - [凭证验证](#凭证验证)
  - [访问控制](#访问控制)
- [话题](#话题)
  - [me 话题](#me-话题)
  - [fnd 话题与标签](#fnd-话题与标签)
  - [点对点话题](#点对点话题)
  - [群组话题](#群组话题)
- [消息](#消息)
  - [客户端到服务端消息](#客户端到服务端消息)
  - [服务端到客户端消息](#服务端到客户端消息)
- [内容格式](#内容格式)
- [推送通知](#推送通知)
- [视频通话](#视频通话)

---

## 工作原理

Tinode 是一个 IM 路由器和存储系统。概念上它松散地遵循 [发布-订阅](https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern) 模式。

服务器连接会话、用户和话题：

- **会话（Session）**：客户端应用与服务器之间的网络连接
- **用户（User）**：通过会话连接到服务器的人
- **话题（Topic）**：在会话之间路由内容的命名通信通道

用户和话题被分配唯一 ID：

- **用户 ID**：字符串，格式为 `usr` 前缀 + base64-URL 编码的伪随机 64 位数字，如 `usr2il9suCbuko`
- **话题 ID**：见下文话题类型说明

客户端（如移动应用或 Web 应用）通过 WebSocket 或长轮询建立会话。大多数操作需要客户端认证。客户端通过发送 `{login}` 数据包认证会话。认证后，客户端收到一个令牌用于后续认证。同一用户可建立多个并发会话。不支持登出设计。

会话建立后，用户可通过话题与其他用户交互。可用的话题类型：

| 类型 | 名称格式 | 说明 |
|------|----------|------|
| `me` | `me` | 管理用户资料和接收通知；每个用户都有一个 |
| `fnd` | `fnd` | 查找其他用户和话题；每个用户都有一个 |
| **点对点** | `usrXXX` | 两个用户之间的通信通道 |
| **群组** | `grpXXX` | 多用户通信通道；必须显式创建 |
| **频道** | `chnXXX` | 只读群组；无限订阅者 |

会话通过发送 `{sub}` 数据包加入话题。`{sub}` 有三个功能：
1. 创建新话题
2. 订阅话题
3. 将会话附加到话题

会话加入话题后，用户可通过 `{pub}` 数据包生成内容。内容以 `{data}` 数据包传递给其他附加会话。

用户可通过 `{get}` 和 `{set}` 数据包查询或更新话题元数据。

话题元数据变更（如描述变更、用户加入/离开）通过 `{pres}`（在线状态）数据包报告给活跃会话。

---

## 通用说明

### 时间戳格式

时间戳始终表示为 [RFC 3339](http://tools.ietf.org/html/rfc3339) 格式字符串，精度到毫秒，时区始终为 UTC：

```
"2015-10-06T18:07:29.841Z"
```

### Base64 编码

文档中提到的 base64 编码指 **base64 URL 编码**，去掉填充字符，见 [RFC 4648](http://tools.ietf.org/html/rfc4648)。

### 消息 ID

- `{data}` 数据包有服务端生成的顺序 ID：从 1 开始的十进制数字，每条消息递增 1
- 保证在每个话题内唯一

### 客户端消息 ID

客户端可为所有数据包分配消息 ID 来连接请求和响应：
- ID 是客户端定义的字符串
- 应至少在会话内唯一
- 服务端不解释，原样返回

---

## 连接服务器

有三种网络访问方式：WebSocket、长轮询和 [gRPC](https://grpc.io/)。

### 端点

当客户端通过 HTTP(S) 建立连接时，服务器提供以下端点：

| 端点 | 用途 |
|------|------|
| `/v0/channels` | WebSocket 连接 |
| `/v0/channels/lp` | 长轮询 |
| `/v0/file/u` | 文件上传 |
| `/v0/file/s` | 文件下载 |

`v0` 表示 API 版本（当前为零）。

### API Key

每个 HTTP(S) 请求必须包含 API Key，服务器按以下顺序检查：

1. HTTP 头 `X-Tinode-APIKey`
2. URL 查询参数 `apikey`（如 `/v0/file/s/abcdefg.jpeg?apikey=...`）
3. 表单值 `apikey`
4. Cookie `apikey`

演示应用包含默认 API Key。生产环境请使用 [`keygen`](../keygen) 工具生成自己的 Key。

### 握手

连接建立后，客户端必须向服务器发送 `{hi}` 消息。服务器以 `{ctrl}` 消息响应，表示成功或错误。响应的 `params` 字段包含服务器协议版本 `"params":{"ver":"0.15"}` 等信息。

---

### gRPC

gRPC API 定义见 [proto 文件](../pbx/model.proto)。gRPC API 比本文档描述的 API 功能更多：
- 允许 `root` 用户代表其他用户发送消息
- 允许删除用户

**注意**：protobuf 消息中的 `bytes` 字段期望 JSON 编码的 UTF-8 内容。例如，字符串在转换为 bytes 前应加引号：
- Go: `[]byte("\"some string\"")`
- Python 3: `'"another string"'.encode('utf-8')`

---

### WebSocket

消息在文本帧中发送，每帧一条消息。二进制帧保留供将来使用。

默认服务器允许任意 `Origin` 头值的连接。

---

### 长轮询

长轮询通过 `HTTP POST`（推荐）或 `GET` 工作。

首次请求时，服务器在响应中发送包含 `sid`（会话 ID）的 `{ctrl}` 消息。长轮询客户端必须在后续每个请求中包含 `sid`（URL 或请求体）。

服务器允许所有来源的连接：`Access-Control-Allow-Origin: *`

---

### 带外大文件

大文件使用 `HTTP POST` 以 `Content-Type: multipart/form-data` 带外发送。详见[带外大文件处理](#带外大文件处理)部分。

---

### 反向代理

Tinode 服务器可在反向代理（如 NGINX）后运行：

- 通过 Unix socket 接收连接：设置 `listen` 和/或 `grpc_listen` 为 socket 文件路径，如 `unix:/run/tinode.sock`
- 从 `X-Forwarded-For` HTTP 头读取对端 IP：设置 `use_x_forwarded_for` 为 `true`

---

## 用户

用户表示一个人，即消息的生产者和消费者。

### 认证级别

用户通常分配以下两种认证级别之一：

| 级别 | 说明 |
|------|------|
| `auth` | 已认证用户 |
| `anon` | 匿名用户 |
| `root` | 仅 gRPC 可访问，允许代表其他用户发送消息 |

### 用户 ID 格式

用户 ID 格式为 `usr` + base64 编码的 64 位数值，如 `usr2il9suCbuko`。

### 用户属性

| 属性 | 类型 | 说明 |
|------|------|------|
| `created` | timestamp | 用户记录创建时间 |
| `updated` | timestamp | `public` 或 `trusted` 最后更新时间 |
| `status` | string | 账户状态（见下表） |
| `username` | string | `basic` 认证使用的唯一字符串；其他用户不可见 |
| `defacs` | object | 用户 P2P 会话的默认访问模式 |
| `trusted` | object | 应用定义对象，由系统管理，所有人可读 |
| `public` | object | 应用定义对象，描述用户，所有人可查询 |
| `private` | object | 应用定义对象，仅当前用户可访问 |
| `tags` | array | 发现和凭证标签 |

### 账户状态

| 状态 | 说明 |
|------|------|
| `ok` | 正常，默认状态 |
| `susp` | 暂停，禁止访问，搜索不可见，可恢复 |
| `del` | 软删除，标记为已删除但保留数据，目前不支持恢复 |
| `undef` | 未定义，内部使用 |

### 多会话

用户可维护与服务器的多个并发会话。每个会话用客户端提供的 `User Agent` 字符串标记以区分客户端软件。

**不支持登出**。如需切换用户，应打开新连接并用新用户凭证认证。

---

## 认证

认证概念上类似于 [SASL](https://en.wikipedia.org/wiki/Simple_Authentication_and_Security_Layer)：通过适配器集合实现不同认证方法。

### 内置认证方法

| 方法 | 说明 |
|------|------|
| `token` | 通过加密令牌认证（主要方式） |
| `basic` | 通过登录名-密码对认证 |
| `anonymous` | 用于临时用户，如客服聊天 |
| `rest` | [元方法](../server/auth/rest/)，通过 JSON RPC 使用外部认证系统 |

### 认证流程

1. **令牌是主要认证方式**
   - 轻量级，通常不访问数据库
   - 所有处理在内存中完成

2. **其他方法用于获取令牌**
   - 一旦获得令牌，后续登录应使用令牌

3. **basic 认证**
   - `secret` 为 base64 编码的 `用户名:密码` 字符串
   - 用户名不能包含冒号 `:`

4. **anonymous 认证**
   - 仅用于创建账户，不能用于登录
   - 用户使用 `anonymous` 创建账户并获得令牌
   - 如果令牌丢失或过期，用户将无法再访问账户

### 创建账户

创建新账户时，用户必须：
1. 告知服务器后续将使用的认证方法
2. 提供共享密钥（如适用）

仅 `basic` 和 `anonymous` 可用于账户创建。

**立即登录**：设置 `{acc login=true}` 使用新账户认证当前会话。

### 登录

通过 `{login}` 请求执行登录。仅支持 `basic` 和 `token`。

响应为 `{ctrl}` 消息：
- **成功（200）**：包含可用于后续 `token` 登录的令牌
- **需要信息（300）**：请求验证凭证或多步认证挑战
- **错误（4xx）**：认证失败

令牌有服务器配置的过期时间，需要定期刷新。

### 修改认证参数

通过 `{acc}` 请求修改认证参数（如修改登录名和密码）：

```json
{
  "acc": {
    "id": "1a2b3",
    "user": "usr2il9suCbuko",
    "scheme": "basic",
    "secret": "base64encode('new_username:new_password')"
  }
}
```

**修改密码**：`secret` 中用户名留空，即 `base64encode(':new_password')`

### 重置密码（"忘记密码"）

发送 `{login}` 消息，`scheme` 设为 `reset`：

```json
{
  "login": {
    "id": "1a2b3",
    "scheme": "reset",
    "secret": "base64encode('basic:email:jdoe@example.com')"
  }
}
```

服务器将发送包含重置说明的消息。邮件包含受限安全令牌，用户可在 `{acc}` 请求中使用该令牌设置新密码。

---

## 凭证验证

服务器可配置要求验证与用户账户关联的某些凭证，如：
- 唯一邮箱
- 电话号码
- 验证码

### 验证流程

如果需要某些凭证：
1. 用户必须始终保持其验证状态
2. 如需更改凭证，必须先添加并验证新凭证，再删除旧凭证

### 凭证操作

| 操作 | 消息 |
|------|------|
| 注册时分配 | `{acc}` |
| 添加 | `{set topic="me"}` |
| 删除 | `{del topic="me"}` |
| 查询 | `{get topic="me"}` |
| 验证 | `{login}` 或 `{acc}` |

---

## 访问控制

访问控制通过访问控制列表（ACL）管理用户对话题的访问。

### 权限定义

权限以位图表示，通过 ASCII 字符集合表示：

| 字符 | 权限 | 说明 |
|------|------|------|
| `N` | 无访问 | 表示权限已清除/未设置，不应使用默认权限 |
| `J` | 加入 | 订阅话题的权限 |
| `R` | 读取 | 接收 `{data}` 数据包的权限 |
| `W` | 写入 | 向话题 `{pub}` 的权限 |
| `P` | 在线状态 | 接收在线状态更新 `{pres}` 的权限 |
| `A` | 审批 | 批准加入请求、删除和禁止成员的权限（管理员） |
| `S` | 分享 | 邀请他人加入话题的权限 |
| `D` | 删除 | 硬删除消息的权限；仅所有者可完全删除话题 |
| `O` | 所有者 | 话题所有者；可分配任何权限给成员、更改话题描述、删除话题；单所有者；某些话题无所有者 |

### 实际访问权限

实际访问权限 = `want` AND `given`（按位与）

### 默认访问权限

默认访问为两类用户定义：
- **已认证用户**：`auth`
- **匿名用户**：`anon`

默认访问值作为 "given" 权限应用于所有新订阅。

---

## 话题

话题是一个或多个人的命名通信通道。话题有持久属性，可通过 `{get what="desc"}` 查询。

### 话题属性

**与用户无关的属性**：

| 属性 | 说明 |
|------|------|
| `created` | 话题创建时间 |
| `updated` | `trusted`、`public`、`private` 最后更新时间 |
| `touched` | 最后一条消息发送时间 |
| `defacs` | 话题默认访问模式 |
| `seq` | 最新 `{data}` 消息的服务端顺序 ID |
| `trusted` | 应用定义对象，系统管理，所有人可读 |
| `public` | 应用定义对象，描述话题，所有订阅者可读 |

**用户相关属性**：

| 属性 | 说明 |
|------|------|
| `acs` | 当前用户的访问权限 |
| `private` | 应用定义对象，仅当前用户可访问 |

---

### me 话题

`me` 话题在账户创建时自动为每个用户创建。用途：
- 管理账户信息
- 接收关注的人和话题的在线状态通知

**特点**：
- 无所有者
- 不能删除或取消订阅
- 可以 `leave`（停止通信，表示离线）
- 只读（`{pub}` 消息被拒绝）

**`{get what="desc"}`**：返回用户参数，`public` 更新会影响所有 P2P 话题。

**`{get what="sub"}`**：返回用户订阅的话题列表，而非用户对 `me` 的订阅。

返回字段：
- `seq`：话题最后消息 ID
- `recv`：用户报告已收到的 seq 值
- `read`：用户报告已读的 seq 值
- `seen`：P2P 订阅中，对方最后在线时间和 User Agent

---

### slf 话题

`slf`（self）话题用于存储信息，如书签或保存的消息。发送到 `slf` 的消息仅发送者可访问。

用户首次订阅时自动创建。

---

### fnd 话题与标签

`fnd` 话题在账户创建时自动为每个用户创建，用于发现其他用户和群组话题。

#### 标签

标签是用于发现的任意大小写不敏感的 Unicode 字符串：
- 最大 96 字符
- 可包含字母、数字和 `_`, `.`, `+`, `-`, `@`, `#`, `!`, `?`
- 可有前缀（命名空间）：如 `tel:+14155551212`、`email:alice@example.com`
- 某些带前缀标签可强制唯一
- 某些标签可强制不可变

#### 查询语言

查询字符串包含原子术语，用空格或逗号分隔：
- **空格**：`AND` 操作
- **逗号**：`OR` 操作
- `OR` 优先于 `AND`

**示例**：
- `flowers`：匹配包含 `flowers` 标签
- `flowers travel`：同时包含 `flowers` 和 `travel`
- `flowers, travel`：包含 `flowers` 或 `travel`
- `flowers travel, puppies`：包含 `flowers` 且包含 `travel` 或 `puppies`

#### 查询方式

设置 `fnd` 话题的 `public` 或 `private` 为查询字符串，然后发送 `{get topic="fnd" what="sub"}`。

- `private`：跨会话和设备持久化，适合大查询（如通讯录匹配）
- `public`：不保存到数据库，适合简短特定查询

---

### 点对点话题

点对点（P2P）话题表示严格两个用户之间的通信通道。

**特点**：
- 话题名对两个参与者不同：每人看到的是对方的用户 ID
- 例：用户 `usrOj0B3-gSBSs` 和 `usrIU_LOVwRNsc` 的 P2P 话题：
  - 第一个用户看到：`usrIU_LOVwRNsc`
  - 第二个用户看到：`usrOj0B3-gSBSs`
- 无所有者
- 内部存储为 `p2p` + 两个 64 位用户 ID 的拼接（数值小的在前）

**创建**：发送 `{sub topic="usrXXX"}`（对方用户 ID）。

**`public`**：用户相关，自动同步到所有 P2P 话题。

---

### 群组话题

群组话题表示多个用户之间的通信通道。

**名称格式**：`grp` 或 `chn` + base64 URL 字符串

**特点**：
- 支持有限数量订阅者（`max_subscriber_count` 配置）
- 每个订阅者的访问权限单独管理
- 可启用频道功能，支持无限只读订阅者（读者）

**创建**：
- 普通群组：`{sub topic="new"}`
- 频道：`{sub topic="nch"}`
- 服务器返回新话题名，如 `grpYiqEXb4QY6s`
- 创建者成为所有者

**频道与普通群组的区别**：

| 特性 | 普通群组 | 频道 |
|------|----------|------|
| 创建 | `{sub topic="new"}` | `{sub topic="nch"}` |
| 订阅 | `{sub topic="grpXXX"}` | `{sub topic="chnXXX"}` |
| 消息 From 字段 | 包含发送者 | 无 |
| 默认权限 | JRWPS | 无 |
| 加入通知 | 有 | 无 |

---

### sys 话题

`sys` 话题是与系统管理员通信的始终可用通道。

**普通用户**：
- 不能订阅 `sys`
- 可以不订阅就向 `sys` 发布（如举报滥用）

**root 用户**：
- 可以订阅 `sys`
- 订阅后接收其他用户发送到 `sys` 的消息

---

## 消息

消息是逻辑关联的数据集合。消息以 JSON 格式的 UTF-8 文本传递。

### 消息格式说明

- 所有客户端到服务端消息可有可选 `id` 字段
- 服务器要求严格有效的 JSON（包括字段名的双引号）
- 清除字段：发送单个 Unicode DEL 字符 `\u2421`（而非 `null`）
- 未知字段被静默忽略

---

## 客户端到服务端消息

每条消息可包含顶层 `extra` 字段：

```json
{
  "abc": { ... },  // 主载荷
  "extra": {
    "attachments": ["/v0/file/s/sJOD_tZDPz0.jpg"],
    "obo": "usr2il9suCbuko",
    "authlevel": "auth"
  }
}
```

| 字段 | 说明 |
|------|------|
| `attachments` | 带外附件 URL 数组，递增文件使用计数 |
| `obo` | root 用户设置的代理用户 ID |
| `authlevel` | 代理时的认证级别 |

---

### `{hi}` - 握手

客户端向服务器告知其版本和用户代理。必须是第一条消息。

```json
{
  "hi": {
    "id": "1a2b3",
    "ver": "0.15.8-rc2",
    "ua": "JS/1.0 (Windows 10)",
    "dev": "L1iC2...dNtk2",
    "platf": "android",
    "lang": "en-US"
  }
}
```

| 字段 | 必需 | 说明 |
|------|------|------|
| `id` | 否 | 客户端消息 ID |
| `ver` | 是 | 支持的协议版本 |
| `ua` | 否 | 用户代理字符串 |
| `dev` | 否 | 设备唯一标识（用于推送） |
| `platf` | 否 | 平台：android/ios/web |
| `lang` | 否 | 客户端语言 |

服务器响应 `{ctrl}`，包含 `build`、`ver`、`sid` 等参数。

---

### `{acc}` - 账户

创建用户或更新认证凭证。

```json
{
  "acc": {
    "id": "1a2b3",
    "user": "newABC123",
    "token": "XMgS...8+BO0=",
    "scheme": "basic",
    "secret": "base64encode('username:password')",
    "login": true,
    "tags": ["alice johnson"],
    "cred": [{"meth": "email", "val": "alice@example.com"}],
    "desc": {
      "defacs": {"auth": "JRWS", "anon": "N"},
      "public": {...},
      "private": {...}
    }
  }
}
```

| 字段 | 说明 |
|------|------|
| `user` | `"new"` 创建新用户；默认当前用户 |
| `token` | 未认证时使用的令牌 |
| `scheme` | 认证方案：basic/anonymous |
| `secret` | base64 编码的密钥 |
| `login` | 是否立即使用新账户登录 |
| `tags` | 发现标签数组 |
| `cred` | 凭证数组 |
| `desc` | 用户初始化数据 |

---

### `{login}` - 登录

认证当前会话。

```json
{
  "login": {
    "id": "1a2b3",
    "scheme": "basic",
    "secret": "base64encode('username:password')",
    "cred": [{"meth": "email", "resp": "178307"}]
  }
}
```

| 字段 | 说明 |
|------|------|
| `scheme` | 认证方案：basic/token/reset |
| `secret` | base64 编码的密钥 |
| `cred` | 凭证验证响应 |

服务器响应 `{ctrl}`，`params` 包含：
- `user`：登录用户 ID
- `token`：加密令牌
- `expires`：过期时间

---

### `{sub}` - 订阅

功能：
1. 创建新话题
2. 订阅现有话题
3. 将会话附加到话题
4. 获取话题数据

```json
{
  "sub": {
    "id": "1a2b3",
    "topic": "me",
    "set": {
      "desc": {"defacs": {"auth": "JRWS", "anon": "N"}},
      "sub": {"mode": "JRWS"},
      "tags": ["email:alice@example.com"]
    },
    "get": {
      "what": "desc sub data",
      "desc": {"ims": "2015-10-06T18:07:30.038Z"},
      "sub": {"limit": 20},
      "data": {"since": 123, "before": 321, "limit": 20}
    }
  }
}
```

| 字段 | 说明 |
|------|------|
| `topic` | 话题名；`new` 创建群组，`nch` 创建频道 |
| `set` | 设置参数（镜像 `{set}`） |
| `get` | 查询参数（镜像 `{get}`） |

---

### `{leave}` - 离开

功能：
1. 离开话题但不取消订阅（`unsub=false`）
2. 取消订阅（`unsub=true`）

```json
{
  "leave": {
    "id": "1a2b3",
    "topic": "grp1XUtEhjv6HND",
    "unsub": true
  }
}
```

---

### `{pub}` - 发布

向话题订阅者分发内容。

```json
{
  "pub": {
    "id": "1a2b3",
    "topic": "grp1XUtEhjv6HND",
    "noecho": false,
    "head": {"mime": "text/x-drafty"},
    "content": {...}
  }
}
```

| 字段 | 说明 |
|------|------|
| `noecho` | 禁止回显（不接收自己发布的消息） |
| `head` | 消息头键值对 |
| `content` | 应用定义内容 |

**`head` 字段值**：

| 字段 | 说明 |
|------|------|
| `attachments` | 附件路径数组 |
| `auto` | 消息由机器人自动发送 |
| `forwarded` | 转发消息的原始消息 ID |
| `mentions` | 提及的用户 ID 数组 |
| `mime` | 内容 MIME 类型 |
| `replace` | 替换的消息 ID |
| `reply` | 回复的消息 ID |
| `thread` | 线程首消息 ID |

---

### `{get}` - 查询

查询话题元数据或消息历史。

```json
{
  "get": {
    "id": "1a2b3",
    "topic": "grp1XUtEhjv6HND",
    "what": "sub desc data del cred",
    "desc": {"ims": "2015-10-06T18:07:30.038Z"},
    "sub": {"user": "usr2il9suCbuko", "limit": 20},
    "data": {"since": 123, "before": 321, "limit": 20},
    "del": {"since": 5, "before": 12, "limit": 25}
  }
}
```

**`what` 值**：

| 值 | 说明 |
|----|------|
| `desc` | 话题描述 |
| `sub` | 订阅者列表 |
| `tags` | 索引标签 |
| `data` | 消息历史 |
| `del` | 删除历史 |
| `cred` | 凭证 |
| `aux` | 辅助数据 |

---

### `{set}` - 更新

更新话题元数据。

```json
{
  "set": {
    "id": "1a2b3",
    "topic": "grp1XUtEhjv6HND",
    "desc": {
      "defacs": {"auth": "JRWP", "anon": "JRW"},
      "public": {...}
    },
    "sub": {
      "user": "usr2il9suCbuko",
      "mode": "JRWP"
    },
    "tags": ["email:alice@example.com"]
  }
}
```

---

### `{del}` - 删除

删除消息、订阅、话题、用户。

```json
{
  "del": {
    "id": "1a2b3",
    "topic": "grp1XUtEhjv6HND",
    "what": "msg",
    "hard": false,
    "delseq": [{"low": 123, "hi": 125}, {"low": 156}],
    "user": "usr2il9suCbuko"
  }
}
```

**`what` 值**：

| 值 | 说明 |
|----|------|
| `msg` | 删除消息 |
| `sub` | 删除订阅 |
| `topic` | 删除话题 |
| `user` | 删除用户 |
| `cred` | 删除凭证 |

**消息删除**：
- `hard=false`：软删除（仅对当前用户隐藏），需 `R` 权限
- `hard=true`：硬删除（物理删除），需 `D` 权限

---

### `{note}` - 通知

客户端生成的临时通知，如输入通知或送达回执。

```json
{
  "note": {
    "topic": "grp1XUtEhjv6HND",
    "what": "kp",
    "seq": 123,
    "unread": 10
  }
}
```

**`what` 值**：

| 值 | 说明 |
|----|------|
| `kp` | 按键（正在输入） |
| `kpa` | 正在录制语音消息 |
| `kpv` | 正在录制视频消息 |
| `recv` | 消息已接收 |
| `read` | 消息已读取 |
| `call` | 视频通话状态更新 |
| `data` | 结构化数据包 |

---

## 服务端到客户端消息

对特定请求的响应消息包含等于原始消息 ID 的 `id` 字段。大多数消息有 `ts` 时间戳字段。

---

### `{data}` - 数据内容

话题中发布的内容。唯一持久化到数据库的消息。

```json
{
  "data": {
    "topic": "grp1XUtEhjv6HND",
    "from": "usr2il9suCbuko",
    "head": {...},
    "ts": "2015-10-06T18:07:30.038Z",
    "seq": 123,
    "content": {...}
  }
}
```

| 字段 | 说明 |
|------|------|
| `topic` | 发布此消息的话题 |
| `from` | 发布者用户 ID |
| `head` | 消息头（从 `{pub}` 原样传递） |
| `seq` | 服务端顺序 ID（话题内唯一） |
| `content` | 应用定义内容 |

---

### `{ctrl}` - 控制响应

表示错误或成功条件的通用响应。

```json
{
  "ctrl": {
    "id": "1a2b3",
    "topic": "grp1XUtEhjv6HND",
    "code": 200,
    "text": "OK",
    "params": {...},
    "ts": "2015-10-06T18:07:30.038Z"
  }
}
```

| 字段 | 说明 |
|------|------|
| `code` | 状态码（遵循 HTTP 状态码模型） |
| `text` | 结果详情文本 |
| `params` | 上下文相关参数 |

---

### `{meta}` - 元数据

话题元数据或订阅者信息，响应 `{get}`、`{set}` 或 `{sub}`。

```json
{
  "meta": {
    "id": "1a2b3",
    "topic": "grp1XUtEhjv6HND",
    "ts": "2015-10-06T18:07:30.038Z",
    "desc": {
      "created": "2015-10-24T10:26:09.716Z",
      "defacs": {"auth": "JRWP", "anon": "N"},
      "acs": {"want": "JRWP", "given": "JRWP", "mode": "JRWP"},
      "seq": 123,
      "read": 112,
      "recv": 115,
      "public": {...}
    },
    "sub": [
      {
        "user": "usr2il9suCbuko",
        "updated": "2015-10-24T10:26:09.716Z",
        "acs": {"want": "JRWP", "given": "JRWP", "mode": "JRWP"},
        "read": 112,
        "online": true
      }
    ],
    "tags": ["email:alice@example.com"],
    "del": {"clear": 3, "delseq": [{"low": 15}]}
  }
}
```

---

### `{pres}` - 在线状态

通知客户端重要事件。

```json
{
  "pres": {
    "topic": "me",
    "src": "grp1XUtEhjv6HND",
    "what": "on",
    "seq": 123,
    "ua": "Tinode/1.0 (Android 2.2)"
  }
}
```

**`what` 值**：

| 值 | 说明 |
|----|------|
| `on` | 上线 |
| `off` | 下线 |
| `ua` | 用户代理变更 |
| `upd` | 话题描述变更 |
| `tags` | 标签变更 |
| `acs` | 访问权限变更 |
| `gone` | 话题不可用 |
| `term` | 订阅终止 |
| `msg` | 新消息可用 |
| `read` | 消息已读 |
| `recv` | 消息已接收 |
| `del` | 消息已删除 |

---

### `{info}` - 信息

转发的客户端通知 `{note}`。

```json
{
  "info": {
    "topic": "grp1XUtEhjv6HND",
    "from": "usr2il9suCbuko",
    "what": "read",
    "seq": 123
  }
}
```

---

## 内容格式

`{pub}` 和 `{data}` 的 `content` 字段格式由应用定义，服务器不强制特定结构。

### 支持的内容类型

| 类型 | 说明 |
|------|------|
| 纯文本 | 普通字符串 |
| Drafty | 富文本格式，见 [Drafty 文档](./drafty.md) |

**使用 Drafty**：消息头必须设置 `"head": {"mime": "text/x-drafty"}`。

---

## 可信、公开、私有、辅助字段

话题有 `trusted`、`public`、`aux` 字段，订阅有 `private` 字段。

### 访问控制区别

| 字段 | 写权限 | 读权限 |
|------|--------|--------|
| `trusted` | ROOT 用户 | 所有人 |
| `public` | 所有者或用户 | 所有人 |
| `aux` | 话题管理员 | 订阅者 |
| `private` | 用户自己 | 用户自己 |

### trusted 格式

```json
{
  "trusted": {
    "verified": true,
    "staff": true,
    "danger": false
  }
}
```

| 字段 | 说明 |
|------|------|
| `verified` | 已验证/可信用户或话题标识 |
| `staff` | 属于服务器管理团队 |
| `danger` | 不可信标识 |

### public 格式

群组、P2P、系统话题的 `public` 字段应为 [theCard](./thecard.md) 格式。

`fnd` 话题的 `public` 应为搜索查询字符串。

### private 格式

```json
{
  "private": {
    "comment": "some comment",
    "arch": true,
    "accepted": "JRWS",
    "tpins": ["grpmiKBkQVXnm3P"]
  }
}
```

| 字段 | 说明 |
|------|------|
| `comment` | 用户对话题或对方的备注 |
| `arch` | 话题已归档 |
| `accepted` | 用户接受的 'given' 模式 |
| `tpins` | 置顶的话题 ID（仅 `me` 话题） |

### aux 格式

```json
{
  "aux": {
    "pins": [1001, 23456]
  }
}
```

| 字段 | 说明 |
|------|------|
| `pins` | 置顶的消息 ID 数组 |

---

## 带外大文件处理

大文件通过 `HTTP POST` 以 `multipart/form-data` 带外处理。

### 端点

| 端点 | 用途 |
|------|------|
| `/v0/file/u` | 文件上传 |
| `/v0/file/s` | 文件下载 |

### 认证

需要 API Key 和登录凭证，检查顺序：
1. HTTP 头 `Authorization`
2. URL 参数 `auth` 和 `secret`
3. 表单值 `auth` 和 `secret`
4. Cookie `auth` 和 `secret`

### 上传流程

1. 创建 RFC 2388 multipart 请求
2. POST 到服务器
3. 响应 `307 Temporary Redirect` 或 `200 OK` + `{ctrl}`
4. `ctrl.params.url` 包含文件路径

```json
{
  "ctrl": {
    "params": {
      "url": "/v0/file/s/mfHLxDWFhfU.pdf"
    },
    "code": 200,
    "text": "ok"
  }
}
```

### 在消息中使用

将 URL 用于 `{pub}` 消息的 Drafty 内容：

```json
{
  "pub": {
    "head": {"mime": "text/x-drafty"},
    "content": {
      "ent": [{"data": {"mime": "image/jpeg", "ref": "/v0/file/s/sJOD_tZDPz0.jpg"}}]
    }
  },
  "extra": {
    "attachments": ["/v0/file/s/sJOD_tZDPz0.jpg"]
  }
}
```

**重要**：必须在 `extra.attachments` 中列出使用的 URL，服务器使用此字段维护文件使用计数。

---

## 推送通知

Tinode 使用编译时适配器处理推送通知。

### 内置适配器

| 适配器 | 支持平台 |
|--------|----------|
| Tinode Push Gateway (TNPG) | Android (Play Services)、iOS、Web 浏览器（Safari 除外） |
| Google FCM | Android (Play Services)、iOS、Web 浏览器（Safari 除外） |
| stdout | 调试、测试、日志 |

### 通知载荷

```json
{
  "topic": "grpnG99YhENiQU",
  "xfrom": "usr2il9suCbuko",
  "ts": "2019-01-06T18:07:30.038Z",
  "seq": "1234",
  "mime": "text/x-drafty",
  "content": "Lorem ipsum..."
}
```

---

## 视频通话

详见 [视频通话文档](call-establishment.md)。

---

## 链接预览

Tinode 提供可选服务帮助客户端生成链接预览。

**端点**：`/v0/urlpreview?url=...`

**响应**：

```json
{
  "title": "Page title",
  "description": "Page description",
  "image_url": "https://..."
}
```

需要认证，方式同[带外大文件](#带外大文件处理)。

---

## 附录：状态码

服务器响应使用 HTTP 状态码模型：

| 状态码范围 | 说明 |
|------------|------|
| 2xx | 成功 |
| 3xx | 需要更多信息 |
| 4xx | 客户端错误 |
| 5xx | 服务器错误 |

---

## 附录：错误消息

错误以 JSON 返回：`{"err": "error-message"}`

| 错误 | 说明 |
|------|------|
| `internal` | 数据库失败或其他内部错误 |
| `malformed` | 请求无法解析 |
| `failed` | 认证失败 |
| `duplicate value` | 重复凭证 |
| `unsupported` | 不支持的操作 |
| `expired` | 密钥已过期 |
| `policy` | 策略违规 |
| `credentials` | 需要验证凭证 |
| `not found` | 对象未找到 |
| `denied` | 操作不允许 |