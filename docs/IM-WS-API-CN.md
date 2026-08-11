# IM WebSocket 前后端接口文档

> 本文档根据 `/Users/superysh/im/prototype` 下的产品原型图整理，用于移动端/前端与 Tinode 后端对接。协议风格遵循 Tinode `docs/API.md`：默认全部业务通信通过 WebSocket JSON 文本帧完成；大文件仍沿用 Tinode 现有 HTTP 上传/下载端点，上传完成后通过 WebSocket 发送附件引用。

## 目录

1. [适用范围](#适用范围)
2. [协议总则](#协议总则)
3. [公共数据约定](#公共数据约定)
4. [连接与认证](#连接与认证)
5. [消息与会话](#消息与会话)
6. [好友与通讯录](#好友与通讯录)
7. [群聊与群管理](#群聊与群管理)
8. [文件、收藏与聊天记录](#文件收藏与聊天记录)
9. [语音、视频与通话](#语音视频与通话)
10. [话题功能](#话题功能)
11. [系统消息、举报与安全提示](#系统消息举报与安全提示)
12. [个人中心与设置](#个人中心与设置)
13. [错误码与状态码](#错误码与状态码)

## 适用范围

### 原型覆盖页面

原型图覆盖以下业务域：

| 业务域 | 页面示例 |
|---|---|
| 登录注册 | 登录、注册、忘记密码、验证码、实名认证、身份验证、独立密码 |
| 消息 | 消息列表、私聊、群聊、文件助手、系统消息、长按菜单、多选、转发、撤回、删除 |
| 联系人 | 朋友、新朋友、添加好友、好友主页、备注、拉黑、黑名单、删除好友 |
| 群聊 | 创建群聊、加入群聊、群二维码、群资料、群公告、群文件、群管理员、禁言、群主转让、进群验证 |
| 文件与收藏 | 文件发送、下载、打开、群文件、我的收藏、收藏分享 |
| 通话 | 语音通话、视频通话、等待接听、通话中、关闭摄像头/麦克风/扬声器、悬浮状态 |
| 话题 | 话题列表、发布话题、话题详情、话题人数、话题类型、积分不足 |
| 设置 | 个人信息、个性签名、隐私设置、青少年模式、版本更新、清空缓存、意见反馈 |
| 安全 | 反诈弹窗、用户协议、推送权限、相机/麦克风权限、安全提醒、举报 |

### 与 Tinode 原生能力的关系

| 能力 | 接入方式 |
|---|---|
| 登录、注册、Token、验证码 | 使用 `{hi}`、`{acc}`、`{login}`，必要时接入 `code`/`rest` auth |
| 私聊、群聊、频道 | 使用 `{sub}`、`{leave}`、`{pub}`、`{get}`、`{set}`、`{del}` |
| 消息回执、输入中、语音录制中 | 使用 `{note}`，服务端转发为 `{info}` |
| 用户资料、群资料、备注、置顶、免打扰 | 使用 `public`、`private`、`aux` |
| 文件、图片、语音、视频 | 大文件先上传 `/v0/file/u/`，再通过 `{pub}` 发送 Drafty 附件 |
| 群权限、管理员、禁言、踢人 | 使用 Tinode ACL 与 `{set sub}`、`{del what="sub"}`，业务状态写入 `aux` |
| 收藏、话题、签到、青少年模式、反诈提示 | 使用本产品扩展命名空间 `x-im-*` |

## 协议总则

### WebSocket 端点

客户端统一连接：

```text
ws://{host}/v0/channels?apikey={api_key}
wss://{host}/v0/channels?apikey={api_key}
```

每个 WebSocket 文本帧只发送一个 JSON 消息。二进制帧不用于业务数据。

### 首包与登录顺序

客户端连接成功后必须先发送 `{hi}`，再发送 `{login}` 或 `{acc}`。登录后建议立即订阅 `me` 和 `fnd`：

```json
{"hi":{"id":"100","ver":"0.25","ua":"IM/1.0 (iOS)","platf":"ios","lang":"zh-CN","tenant":"acme"}}
```

```json
{"login":{"id":"101","scheme":"basic","secret":"MTg4MjY4Mjc3NTQ6MTIzNDU2"}}
```

```json
{"sub":{"id":"102","topic":"me","get":{"what":"desc sub tags cred"}}}
```

```json
{"sub":{"id":"103","topic":"fnd"}}
```

### 消息类型

客户端到服务端：

| 包 | 用途 |
|---|---|
| `{hi}` | 握手、声明企业码、客户端版本和设备信息 |
| `{acc}` | 注册、修改认证参数、提交验证码 |
| `{login}` | 登录、Token 登录、重置密码流程 |
| `{sub}` | 订阅/进入会话、创建群、创建私聊、拉取初始数据 |
| `{leave}` | 离开会话或退订会话 |
| `{pub}` | 发送持久化消息 |
| `{get}` | 查询资料、订阅、消息历史、删除记录、辅助数据 |
| `{set}` | 更新资料、备注、权限、群设置、辅助数据 |
| `{del}` | 删除消息、订阅、topic、凭证 |
| `{note}` | 发送临时通知，如已读、已收、输入中、通话信令 |

服务端到客户端：

| 包 | 用途 |
|---|---|
| `{ctrl}` | 请求结果，应答成功或失败 |
| `{data}` | 持久化消息投递 |
| `{meta}` | 资料、订阅、标签、凭证、删除记录、辅助数据返回 |
| `{pres}` | 在线、离线、权限、资料更新、消息到达等通知 |
| `{info}` | 由 `{note}` 转发而来的临时通知 |

### 请求 ID 规范

客户端必须为需要应答的请求设置 `id`。建议格式：

```text
{module}-{timestamp_ms}-{seq}
```

示例：

```json
{"get":{"id":"msg-1722400000000-1","topic":"grpAbC123","what":"data","data":{"limit":20}}}
```

### 扩展命名空间

Tinode 原生字段保持不变。本产品扩展字段统一使用 `x-im-*`：

| 位置 | 示例 | 说明 |
|---|---|---|
| `head.x-im-type` | `"text"`、`"card"`、`"topic-share"` | 消息业务类型 |
| `head.x-im-client-mid` | `"local-uuid"` | 客户端本地消息 ID，用于发送中/失败重试 |
| `head.x-im-anti-fraud` | `true` | 需要客户端展示反诈提醒 |
| `content.x-im-*` | `{...}` | 消息内容里的业务负载 |
| `public.x-im-*` | `{...}` | 对所有可见的业务资料 |
| `private.x-im-*` | `{...}` | 仅当前用户可见的配置 |
| `aux.x-im-*` | `{...}` | 群/频道管理员可写、成员可读的配置 |

第一阶段服务端已启用轻量校验：

| 字段 | 规则 |
|---|---|
| `head.x-im-type` | 必须是文档附录里的已知消息类型 |
| `private.x-im-applyText` | 字符串，最长 256 个字符 |
| `private.x-im-applyStatus`、`private.x-im-joinStatus` | 枚举：`pending`、`accepted`、`rejected`、`expired` |
| `private.x-im-joinApplyText` | 字符串，最长 256 个字符 |
| `private.x-im-blocked`、`private.x-im-muted`、`private.x-im-pinned` | 布尔值 |
| `aux.x-im-group.joinApproval`、`onlyAdminInvite`、`allowMemberFileUpload`、`muteAll` | 布尔值 |
| `aux.x-im-group.mutedUsers` | Tinode 用户 ID 数组 |
| `aux.x-im-group.announcement.text` | 字符串，最长 2000 个字符 |

## 公共数据约定

### 时间与 ID

| 字段 | 说明 |
|---|---|
| `ts`、`created`、`updated` | RFC3339 UTC 字符串，如 `"2026-07-31T08:00:00.000Z"` |
| `user` | Tinode 用户 ID，如 `usrAbCdEf123` |
| `topic` | Tinode topic，如 `usrAbCdEf123`、`grpAbCdEf123`、`chnAbCdEf123` |
| `seq` | topic 内递增消息序号 |
| `clear` | 删除事务序号 |

### 用户公开资料 `public`

用户资料页面、好友主页、群成员头像昵称均使用用户 `public`。

```json
{
  "fn": "张三",
  "photo": {
    "ref": "/v0/file/s/avatar.jpg",
    "type": "image/jpeg"
  },
  "note": "成人最懂什么都没留下",
  "x-im-gender": "female",
  "x-im-birthday": "2008-02-07",
  "x-im-area": "广东省广州市",
  "x-im-id-no": "84444646554"
}
```

### 用户私有资料 `private`

仅当前用户可见。

```json
{
  "comment": "好友备注",
  "tpins": ["usrPeer001", "grpTeam001"],
  "x-im-settings": {
    "teenMode": false,
    "independentPassword": true,
    "allowSearchByPhone": true,
    "allowSearchByQr": true,
    "allowSearchByGroup": true,
    "pushEnabled": true
  }
}
```

### 群公开资料 `public`

群资料、群二维码、群设置页读取群 topic 的 `public`。

```json
{
  "fn": "新人交流群",
  "photo": {
    "ref": "/v0/file/s/group-avatar.jpg",
    "type": "image/jpeg"
  },
  "note": "群主很懒，还没有群简介",
  "x-im-ownerName": "王子民",
  "x-im-qrcode": "im://group/grpAbCdEf123"
}
```

### 群业务配置 `aux`

群设置、群公告、群文件权限、进群验证、禁言列表使用 `aux`。

```json
{
  "x-im-group": {
    "joinApproval": true,
    "onlyAdminInvite": false,
    "onlyAdminEditInfo": true,
    "allowMemberFileUpload": true,
    "muteAll": false,
    "mutedUsers": ["usrMuted001"],
    "announcement": {
      "text": "为帮助新用户熟悉群规则，请勿发布违规内容。",
      "updatedBy": "usrOwner001",
      "updatedAt": "2026-07-31T08:00:00.000Z"
    }
  }
}
```

### 当前用户对会话的私有设置 `desc.private`

聊天设置页里的备注、免打扰、置顶、拉黑、本地草稿等使用当前用户对 topic 的 `private`。

```json
{
  "comment": "王子民",
  "arch": false,
  "x-im-muted": false,
  "x-im-pinned": true,
  "x-im-blocked": false,
  "x-im-draft": "稍后回复"
}
```

## 连接与认证

### 登录

原型页面：登录页面、注册页面。

客户端先发送 `{hi}`，再使用手机号/用户名 + 密码登录。`secret` 为 `base64(username + ":" + password)`。

```json
{"login":{"id":"login-1","scheme":"basic","secret":"MTg4MjY4Mjc3NTQ6MTIzNDU2"}}
```

成功响应：

```json
{
  "ctrl": {
    "id": "login-1",
    "code": 200,
    "text": "ok",
    "params": {
      "user": "usrUser001",
      "token": "token-string",
      "expires": "2026-08-30T08:00:00.000Z"
    },
    "ts": "2026-07-31T08:00:00.000Z"
  }
}
```

### Token 自动登录

```json
{"login":{"id":"login-token-1","scheme":"token","secret":"token-string"}}
```

### 注册

```json
{
  "acc": {
    "id": "acc-1",
    "user": "new",
    "scheme": "basic",
    "secret": "MTg4MjY4Mjc3NTQ6MTIzNDU2",
    "login": true,
    "tags": ["tel:+8618826827754", "basic:18826827754"],
    "cred": [
      {"meth": "tel", "val": "+8618826827754", "resp": "123456"}
    ],
    "desc": {
      "defacs": {"auth": "JRWS", "anon": "N"},
      "public": {
        "fn": "用户1882",
        "x-im-area": "广东省广州市"
      },
      "private": {
        "x-im-settings": {
          "pushEnabled": true,
          "teenMode": false
        }
      }
    }
  }
}
```

### 忘记密码

发送重置请求：

```json
{"login":{"id":"reset-1","scheme":"reset","secret":"YmFzaWM6dGVsOis4NjE4ODI2ODI3NzU0"}}
```

使用收到的临时 token 修改密码：

```json
{
  "acc": {
    "id": "reset-2",
    "token": "restricted-reset-token",
    "scheme": "basic",
    "secret": "OjEyMzQ1Njc4"
  }
}
```

### 实名认证

实名认证是业务扩展，建议写入用户 `private.x-im-realname`，服务端审核通过后可由 root 写入 `trusted.verified=true`。

```json
{
  "set": {
    "id": "realname-1",
    "topic": "me",
    "desc": {
      "private": {
        "x-im-realname": {
          "name": "张三",
          "idCard": "440100199001011234",
          "status": "pending"
        }
      }
    }
  }
}
```

服务端返回：

```json
{"ctrl":{"id":"realname-1","topic":"me","code":200,"text":"ok","ts":"2026-07-31T08:00:00.000Z"}}
```

## 消息与会话

### 获取消息列表

原型页面：消息、文件助手、系统消息、聊天会话列表。

订阅 `me` 后获取当前用户订阅的所有会话：

```json
{"get":{"id":"contacts-1","topic":"me","what":"sub","sub":{"limit":50}}}
```

响应：

```json
{
  "meta": {
    "id": "contacts-1",
    "topic": "me",
    "sub": [
      {
        "topic": "usrPeer001",
        "updated": "2026-07-31T08:00:00.000Z",
        "touched": "2026-07-31T08:10:00.000Z",
        "seq": 18,
        "read": 15,
        "recv": 18,
        "online": true,
        "public": {"fn": "王子民", "photo": {"ref": "/v0/file/s/a.jpg"}},
        "private": {"comment": "王子民", "x-im-muted": false, "x-im-pinned": true}
      }
    ]
  }
}
```

### 进入私聊

私聊 topic 使用对方用户 ID。首次 `{sub}` 会创建 P2P topic。

```json
{
  "sub": {
    "id": "p2p-sub-1",
    "topic": "usrPeer001",
    "get": {
      "what": "desc sub data",
      "data": {"limit": 20}
    }
  }
}
```

### 进入群聊

```json
{
  "sub": {
    "id": "grp-sub-1",
    "topic": "grpTeam001",
    "get": {
      "what": "desc sub data aux",
      "data": {"limit": 20}
    }
  }
}
```

### 发送文本消息

Tinode 推荐使用 Drafty。发送 Drafty 时必须设置 `head.mime="text/x-drafty"`。

```json
{
  "pub": {
    "id": "pub-text-1",
    "topic": "usrPeer001",
    "head": {
      "mime": "text/x-drafty",
      "x-im-type": "text",
      "x-im-client-mid": "local-001"
    },
    "content": {
      "txt": "你好，我是王理娟，很高兴认识你，你怎么称呼"
    }
  }
}
```

接收方收到：

```json
{
  "data": {
    "topic": "usrPeer001",
    "from": "usrUser001",
    "seq": 19,
    "ts": "2026-07-31T08:10:00.000Z",
    "head": {
      "mime": "text/x-drafty",
      "x-im-type": "text",
      "x-im-client-mid": "local-001"
    },
    "content": {
      "txt": "你好，我是王理娟，很高兴认识你，你怎么称呼"
    }
  }
}
```

### 已收、已读、未读数

原型页面：个人对话已读状态、消息未读红点。

```json
{"note":{"topic":"usrPeer001","what":"recv","seq":19,"unread":3}}
```

```json
{"note":{"topic":"usrPeer001","what":"read","seq":19,"unread":0}}
```

对端收到：

```json
{"info":{"topic":"usrPeer001","from":"usrPeer002","what":"read","seq":19}}
```

### 输入中与录音中

```json
{"note":{"topic":"usrPeer001","what":"kp"}}
```

```json
{"note":{"topic":"usrPeer001","what":"kpa"}}
```

```json
{"note":{"topic":"usrPeer001","what":"kpv"}}
```

### @ 提醒

原型页面：`@人员-选择提醒人`、`@人员—选择提醒人-多选`。

```json
{
  "pub": {
    "id": "pub-mention-1",
    "topic": "grpTeam001",
    "head": {
      "mime": "text/x-drafty",
      "mentions": ["usrA001", "usrB001"],
      "x-im-type": "text"
    },
    "content": {
      "txt": "@万先生 @钱哥你们看一下",
      "fmt": [
        {"at": 0, "len": 4, "key": 0},
        {"at": 5, "len": 3, "key": 1}
      ],
      "ent": [
        {"tp": "MN", "data": {"val": "usrA001"}},
        {"tp": "MN", "data": {"val": "usrB001"}}
      ]
    }
  }
}
```

### 回复、引用、编辑

长按菜单中的引用、回复、编辑沿用 Tinode `head.reply`、`head.replace`。

```json
{
  "pub": {
    "id": "pub-reply-1",
    "topic": "grpTeam001",
    "head": {
      "mime": "text/x-drafty",
      "reply": "grpTeam001:19",
      "x-im-type": "text"
    },
    "content": {"txt": "我同意这个方案"}
  }
}
```

编辑消息：

```json
{
  "pub": {
    "id": "pub-edit-1",
    "topic": "grpTeam001",
    "head": {
      "mime": "text/x-drafty",
      "replace": ":19",
      "x-im-type": "text"
    },
    "content": {"txt": "修改后的消息内容"}
  }
}
```

### 撤回与删除

单聊/群聊撤回使用硬删除，前提是用户有 `D` 权限或符合后端业务规则。

```json
{
  "del": {
    "id": "del-msg-1",
    "topic": "grpTeam001",
    "what": "msg",
    "hard": true,
    "delseq": [{"low": 19}]
  }
}
```

仅自己删除：

```json
{
  "del": {
    "id": "del-msg-self-1",
    "topic": "grpTeam001",
    "what": "msg",
    "hard": false,
    "delseq": [{"low": 19}]
  }
}
```

### 转发

逐条转发：

```json
{
  "pub": {
    "id": "forward-1",
    "topic": "usrPeer002",
    "head": {
      "mime": "text/x-drafty",
      "forwarded": "grpTeam001:19",
      "x-im-type": "text"
    },
    "content": {"txt": "被转发的内容"}
  }
}
```

合并转发：

```json
{
  "pub": {
    "id": "forward-merge-1",
    "topic": "usrPeer002",
    "head": {
      "mime": "application/json",
      "x-im-type": "forward-bundle"
    },
    "content": {
      "title": "群聊的聊天记录",
      "items": [
        {"topic": "grpTeam001", "seq": 19, "from": "usrA001", "text": "第一条"},
        {"topic": "grpTeam001", "seq": 20, "from": "usrB001", "text": "第二条"}
      ]
    }
  }
}
```

## 好友与通讯录

### 查找用户

原型页面：添加好友、输入精准用户 ID。

使用 `fnd` topic 查询标签。手机号、用户名、用户 ID 由后端查询重写或精确匹配。

```json
{
  "set": {
    "id": "find-1",
    "topic": "fnd",
    "desc": {
      "public": "18826827754"
    }
  }
}
```

```json
{"get":{"id":"find-2","topic":"fnd","what":"sub","sub":{"limit":20}}}
```

### 添加好友申请

P2P 建立关系使用 `{sub topic=userId}`。申请备注放入 `set.desc.private.x-im-applyText`，审批状态放入 `private.x-im-applyStatus`。服务端会校验状态枚举，真实可聊权限仍以 Tinode ACL 为准。

```json
{
  "sub": {
    "id": "friend-apply-1",
    "topic": "usrPeer001",
    "set": {
      "desc": {
        "private": {
          "x-im-applyText": "我是王子民",
          "x-im-applyStatus": "pending"
        }
      }
    },
    "get": {"what": "desc"}
  }
}
```

对方在线时会收到 `me` topic 的 `pres what="acs"`；客户端可据此刷新新朋友列表。

### 新朋友列表

客户端从 `me` 的订阅列表中过滤待处理 P2P 权限：

```json
{"get":{"id":"new-friends-1","topic":"me","what":"sub","sub":{"limit":100}}}
```

列表状态建议：

| 条件 | 含义 |
|---|---|
| `private.x-im-applyStatus="pending"` | 申请待处理 |
| `private.x-im-applyStatus="accepted"` | 申请已同意 |
| `private.x-im-applyStatus="rejected"` | 申请已拒绝 |
| 对方订阅变更产生 `pres what="acs"` | 我收到的新申请 |

### 同意好友

```json
{
  "set": {
    "id": "friend-approve-1",
    "topic": "usrPeer001",
    "sub": {
      "user": "usrPeer001",
      "mode": "JRWPA"
    }
  }
}
```

同意后建议双方把各自订阅 `private.x-im-applyStatus` 更新为 `accepted`；拒绝时更新为 `rejected`。非法状态会返回 `400 malformed`。

### 设置备注

```json
{
  "set": {
    "id": "remark-1",
    "topic": "usrPeer001",
    "desc": {
      "private": {
        "comment": "王子民",
        "x-im-remarkUpdatedAt": "2026-07-31T08:00:00.000Z"
      }
    }
  }
}
```

### 拉黑与取消拉黑

客户端私有状态用于 UI 展示，服务端应同时调整 P2P ACL，阻止对方继续发起会话。

```json
{
  "set": {
    "id": "block-1",
    "topic": "usrPeer001",
    "desc": {
      "private": {
        "x-im-blocked": true
      }
    },
    "sub": {
      "user": "usrPeer001",
      "mode": "JRPA"
    }
  }
}
```

取消拉黑：

```json
{
  "set": {
    "id": "unblock-1",
    "topic": "usrPeer001",
    "desc": {
      "private": {
        "x-im-blocked": false
      }
    },
    "sub": {
      "user": "usrPeer001",
      "mode": "JRWPA"
    }
  }
}
```

### 删除好友

```json
{
  "leave": {
    "id": "friend-delete-1",
    "topic": "usrPeer001",
    "unsub": true
  }
}
```

## 群聊与群管理

### 创建群聊

原型页面：创建群聊。

```json
{
  "sub": {
    "id": "group-create-1",
    "topic": "new",
    "set": {
      "desc": {
        "defacs": {"auth": "JRWP", "anon": "N"},
        "public": {
          "fn": "新人交流群",
          "note": "群主很懒，还没有群简介"
        },
        "private": {}
      },
      "aux": {
        "x-im-group": {
          "joinApproval": false,
          "onlyAdminInvite": false,
          "allowMemberFileUpload": true,
          "muteAll": false,
          "mutedUsers": []
        }
      }
    },
    "get": {"what": "desc sub aux"}
  }
}
```

成功响应的 `ctrl.topic` 为新群 ID：

说明：创建群时传入的 `set.aux.x-im-group` 会写入 `topics.aux`，后续可通过 `{get what="aux"}` 或 `{sub get.what="desc sub aux"}` 读取。

```json
{"ctrl":{"id":"group-create-1","topic":"grpNew001","code":200,"text":"ok","ts":"2026-07-31T08:00:00.000Z"}}
```

### 邀请成员入群

原型页面：邀请进群申请、开启群验证拉人状态。

无需审批时直接共享订阅：

如果 `aux.x-im-group.onlyAdminInvite=true`，服务端只允许群管理员或群主执行邀请。

```json
{
  "set": {
    "id": "group-invite-1",
    "topic": "grpNew001",
    "sub": {
      "user": "usrInvitee001",
      "mode": "JRWP"
    }
  }
}
```

需要审批时，申请人发送 `{sub}`，请求 `JRWPS`，申请备注写入自己的订阅 `private.x-im-joinApplyText`。若群 `aux.x-im-group.joinApproval=true` 且默认 `defacs.auth` 不包含 `J`，服务端会保存一条 `want=JRWPS/given=N` 的待审批订阅并返回 `200 ok`，但不会把申请人 attach 到群。

```json
{
  "sub": {
    "id": "group-join-apply-1",
    "topic": "grpNew001",
    "set": {
      "sub": {"mode": "JRWPS"},
      "desc": {
        "private": {
          "x-im-joinApplyText": "申请加入群聊",
          "x-im-joinStatus": "pending"
        }
      }
    }
  }
}
```

管理员获取待审批列表：

```json
{"get":{"id":"group-apply-list-1","topic":"grpNew001","what":"sub","sub":{"limit":100}}}
```

返回中待审批用户表现为 `acs.want` 包含 `J`、`acs.given="N"`、`private.x-im-joinStatus="pending"`：

```json
{
  "meta": {
    "id": "group-apply-list-1",
    "topic": "grpNew001",
    "sub": [
      {
        "user": "usrInvitee001",
        "acs": {"want": "JRWPS", "given": "N", "mode": "N"},
        "private": {
          "x-im-joinApplyText": "申请加入群聊",
          "x-im-joinStatus": "pending"
        }
      }
    ]
  }
}
```

管理员审批通过：

```json
{
  "set": {
    "id": "group-approve-1",
    "topic": "grpNew001",
    "sub": {
      "user": "usrInvitee001",
      "mode": "JRWP"
    }
  }
}
```

服务端会把申请人的 `private.x-im-joinStatus` 自动更新为 `accepted`。如果拒绝，管理员设置 `mode="N"`，服务端会把状态更新为 `rejected`：

```json
{
  "set": {
    "id": "group-reject-1",
    "topic": "grpNew001",
    "sub": {
      "user": "usrInvitee001",
      "mode": "N"
    }
  }
}
```

如需在群内留痕，管理员可额外发布审批结果消息：

```json
{
  "pub": {
    "id": "group-join-decision-1",
    "topic": "grpNew001",
    "head": {
      "mime": "application/json",
      "x-im-type": "group-join-approve"
    },
    "content": {
      "applicant": "usrInvitee001",
      "status": "accepted",
      "reviewedAt": "2026-07-31T08:00:00.000Z"
    }
  }
}
```

### 群资料与群公告

更新群名称、头像、简介：

```json
{
  "set": {
    "id": "group-profile-1",
    "topic": "grpNew001",
    "desc": {
      "public": {
        "fn": "新人交流群",
        "photo": {"ref": "/v0/file/s/group-avatar.jpg", "type": "image/jpeg"},
        "note": "欢迎交流，禁止广告。"
      }
    }
  }
}
```

更新群公告：

```json
{
  "set": {
    "id": "group-ann-1",
    "topic": "grpNew001",
    "aux": {
      "x-im-group": {
        "announcement": {
          "text": "为帮助新用户熟悉群规则，请勿发布违规内容。",
          "updatedBy": "usrOwner001",
          "updatedAt": "2026-07-31T08:00:00.000Z"
        }
      }
    }
  }
}
```

公告也可以发送一条系统样式消息，方便聊天页展示：

```json
{
  "pub": {
    "id": "group-ann-msg-1",
    "topic": "grpNew001",
    "head": {
      "mime": "application/json",
      "x-im-type": "group-announcement"
    },
    "content": {
      "text": "为帮助新用户熟悉群规则，请勿发布违规内容。"
    }
  }
}
```

### 设置管理员

原型页面：设置群管理员、添加管理员、删除管理员。

当前后端 `{set.sub.mode}` 使用绝对权限字符串，不支持 `+A`、`-A` 这类增量写法。前端需要按目标最终权限发送完整 mode；用户有效权限为 `want & given`，因此授予管理员时通常需要管理员设置目标用户的 `given`，目标用户再通过自己的 `{set.sub}` 提升 `want`。

授予管理员权限：

```json
{
  "set": {
    "id": "admin-add-1",
    "topic": "grpNew001",
    "sub": {
      "user": "usrAdmin001",
      "mode": "JRWPASD"
    }
  }
}
```

移除管理员权限：

```json
{
  "set": {
    "id": "admin-del-1",
    "topic": "grpNew001",
    "sub": {
      "user": "usrAdmin001",
      "mode": "JRWPS"
    }
  }
}
```

### 群禁言

禁言单个成员建议同时更新 ACL 与 `aux.x-im-group.mutedUsers`。第一阶段服务端会在发送消息时检查 `mutedUsers`，普通成员在列表中会被拒绝；管理员和群主不受该业务禁言影响。

```json
{
  "set": {
    "id": "mute-user-1",
    "topic": "grpNew001",
    "sub": {
      "user": "usrMuted001",
      "mode": "JRP"
    },
    "aux": {
      "x-im-group": {
        "mutedUsers": ["usrMuted001"]
      }
    }
  }
}
```

解除禁言：

```json
{
  "set": {
    "id": "unmute-user-1",
    "topic": "grpNew001",
    "sub": {
      "user": "usrMuted001",
      "mode": "JRWP"
    },
    "aux": {
      "x-im-group": {
        "mutedUsers": []
      }
    }
  }
}
```

全员禁言：

第一阶段服务端会拒绝普通成员发送消息，管理员和群主仍可发送。

```json
{
  "set": {
    "id": "mute-all-1",
    "topic": "grpNew001",
    "aux": {
      "x-im-group": {
        "muteAll": true
      }
    }
  }
}
```

### 群文件上传权限

当 `aux.x-im-group.allowMemberFileUpload=false` 时，服务端会拒绝普通成员发送 `x-im-type` 为 `image`、`audio`、`video`、`file` 的消息，或带 `extra.attachments` 的消息。管理员和群主不受限制。

```json
{
  "set": {
    "id": "group-file-policy-1",
    "topic": "grpNew001",
    "aux": {
      "x-im-group": {
        "allowMemberFileUpload": false
      }
    }
  }
}
```

### 群主转让

原型页面：选择新群主、选择新群主提示。

第一步，当前群主发起转让，把新群主的 `given` 权限设置为包含 `O` 的完整权限：

```json
{
  "set": {
    "id": "owner-transfer-1",
    "topic": "grpNew001",
    "sub": {
      "user": "usrNewOwner001",
      "mode": "JRWPASDO"
    }
  }
}
```

第二步，新群主接受转让，把自己的 `want` 权限设置为包含 `O` 的完整权限：

```json
{
  "set": {
    "id": "owner-accept-1",
    "topic": "grpNew001",
    "sub": {
      "mode": "JRWPASDO"
    }
  }
}
```

后端必须保证同一 topic 只有一个 `O` 权限用户。当前 Tinode 实际行为是：新群主接受后，服务端自动移除旧群主的 `O`，并向双方推送 `pres what="acs"`。

### 踢出群成员

```json
{
  "del": {
    "id": "kick-1",
    "topic": "grpNew001",
    "what": "sub",
    "user": "usrTarget001",
    "hard": false
  }
}
```

### 退出群聊

```json
{
  "leave": {
    "id": "leave-group-1",
    "topic": "grpNew001",
    "unsub": true
  }
}
```

## 文件、收藏与聊天记录

### 上传并发送图片

原型页面：单张图片状态、图片和语音发送中。

1. 客户端通过 `/v0/file/u/` 上传图片，获得 `ctrl.params.url`。
2. 通过 WebSocket `{pub}` 发送 Drafty 图片消息。

```json
{
  "pub": {
    "id": "pub-img-1",
    "topic": "usrPeer001",
    "head": {
      "mime": "text/x-drafty",
      "attachments": ["/v0/file/s/img001.jpg"],
      "x-im-type": "image",
      "x-im-client-mid": "local-img-1"
    },
    "content": {
      "txt": " ",
      "fmt": [{"at": -1, "len": 1, "key": 0}],
      "ent": [
        {
          "tp": "IM",
          "data": {
            "mime": "image/jpeg",
            "ref": "/v0/file/s/img001.jpg",
            "width": 1080,
            "height": 1920,
            "size": 245760
          }
        }
      ]
    }
  },
  "extra": {
    "attachments": ["/v0/file/s/img001.jpg"]
  }
}
```

### 发送普通文件

原型页面：文件发送状态、文件下载、文件打开。

```json
{
  "pub": {
    "id": "pub-file-1",
    "topic": "usrPeer001",
    "head": {
      "mime": "text/x-drafty",
      "attachments": ["/v0/file/s/doc001.pdf"],
      "x-im-type": "file"
    },
    "content": {
      "txt": "365天天高清晰记忆曲线复习_pdf",
      "fmt": [{"at": -1, "len": 0, "key": 0}],
      "ent": [
        {
          "tp": "EX",
          "data": {
            "mime": "application/pdf",
            "ref": "/v0/file/s/doc001.pdf",
            "name": "365天天高清晰记忆曲线复习_pdf",
            "size": 115000
          }
        }
      ]
    }
  },
  "extra": {
    "attachments": ["/v0/file/s/doc001.pdf"]
  }
}
```

### 发送语音消息

```json
{
  "pub": {
    "id": "pub-audio-1",
    "topic": "usrPeer001",
    "head": {
      "mime": "text/x-drafty",
      "attachments": ["/v0/file/s/audio001.m4a"],
      "x-im-type": "audio"
    },
    "content": {
      "txt": " ",
      "fmt": [{"at": -1, "len": 1, "key": 0}],
      "ent": [
        {
          "tp": "AU",
          "data": {
            "mime": "audio/mp4",
            "ref": "/v0/file/s/audio001.m4a",
            "duration": 6000,
            "name": "voice.m4a",
            "size": 48000
          }
        }
      ]
    }
  },
  "extra": {
    "attachments": ["/v0/file/s/audio001.m4a"]
  }
}
```

### 群文件列表

群文件是群消息附件的业务视图。二期 MySQL 后端已建立 `im_message_index`，带 `x-im-filter` 或 `x-im-search` 的 `{get data}` 会优先走索引查询；其它未实现索引的数据库后端仍回退到一阶段内存过滤。

扩展查询：

```json
{
  "get": {
    "id": "group-files-1",
    "topic": "grpNew001",
    "what": "data",
    "data": {
      "limit": 50
    },
    "x-im-filter": {
      "types": ["file"],
      "keyword": "复习"
    }
  }
}
```

### 收藏消息

原型页面：我的收藏、收藏文本、收藏文件、收藏语音、收藏链接。

建议收藏使用 `slf` topic 存储，`head.x-im-type="favorite"`，原始消息定位放在内容里。

```json
{
  "pub": {
    "id": "fav-1",
    "topic": "slf",
    "head": {
      "mime": "application/json",
      "x-im-type": "favorite"
    },
    "content": {
      "sourceTopic": "grpNew001",
      "sourceSeq": 19,
      "kind": "file",
      "title": "365天天高清晰记忆曲线复习_pdf",
      "summary": "PDF 115KB",
      "collectedAt": "2026-07-31T08:00:00.000Z"
    }
  }
}
```

获取收藏：

```json
{
  "sub": {
    "id": "fav-list-1",
    "topic": "slf",
    "get": {
      "what": "data",
      "data": {"limit": 50}
    }
  }
}
```

### 查找聊天记录

原型页面：查看聊天记录-全部、图片、视频、文件、链接、搜索关键词。

Tinode 原生 `{get what="data"}` 支持按 `seq` 分页，不直接支持关键词搜索。关键词、类型搜索需要后端扩展索引。
二期以后 MySQL 后端会优先查询 `im_message_index`；当前 schema `119` 对 `search_text` 增加 `FULLTEXT` 索引，同时保留 `LIKE` fallback，避免中文短词或未命中全文分词时漏查。

```json
{
  "get": {
    "id": "search-msg-1",
    "topic": "usrPeer001",
    "what": "data",
    "data": {"limit": 50},
    "x-im-search": {
      "keyword": "装修",
      "types": ["text", "image", "file", "link"],
      "before": 0
    }
  }
}
```

## 语音、视频与通话

### 发起语音通话

原型页面：语音通话-对方待接受状态、双方语音沟通中。

Tinode 原生通话流程为：先用 `{pub}` 发送 `head.webrtc="started"` 的通话邀请并持久化一条通话消息；后续信令用 `{note what="call"}`。本产品在通话消息内容与 `note.payload` 中约定 `callId`、`callType`。

后端校验规则：显式发送 `head.x-im-type="call"` 且 `head.webrtc="started"` 的产品通话邀请，`content.callId` 必填，否则返回 `400 malformed`。`ringing`、`accept`、`hang-up` 控制事件的 `payload.callId` 必填；`offer`、`answer`、`ice-candidate` 仍按 Tinode 原生 WebRTC 信令透传，避免破坏 SDP/ICE 结构。

```json
{
  "pub": {
    "id": "call-audio-1",
    "topic": "usrPeer001",
    "head": {
      "mime": "application/json",
      "webrtc": "started",
      "x-im-type": "call"
    },
    "content": {
      "callId": "call-001",
      "callType": "audio",
      "audio": true,
      "video": false
    }
  }
}
```

### 发起视频通话

```json
{
  "pub": {
    "id": "call-video-1",
    "topic": "usrPeer001",
    "head": {
      "mime": "application/json",
      "webrtc": "started",
      "x-im-type": "call"
    },
    "content": {
      "callId": "call-002",
      "callType": "video",
      "audio": true,
      "video": true,
      "cameraOn": true
    }
  }
}
```

### 接受、拒绝、挂断

接受：

```json
{
  "note": {
    "topic": "usrPeer001",
    "what": "call",
    "event": "accept",
    "seq": 102,
    "payload": {
      "callId": "call-002"
    }
  }
}
```

拒绝或挂断使用 Tinode 原生 `hang-up` 事件；通话尚未接通时服务端会把最终消息标记为 declined/missed。

```json
{
  "note": {
    "topic": "usrPeer001",
    "what": "call",
    "event": "hang-up",
    "seq": 102,
    "payload": {
      "callId": "call-002",
      "reason": "user_declined"
    }
  }
}
```

挂断：

```json
{
  "note": {
    "topic": "usrPeer001",
    "what": "call",
    "event": "hang-up",
    "seq": 102,
    "payload": {
      "callId": "call-002",
      "duration": 65000
    }
  }
}
```

### 通话状态消息

通话结束后发送一条持久消息用于聊天记录展示。

```json
{
  "pub": {
    "id": "call-record-1",
    "topic": "usrPeer001",
    "head": {
      "mime": "application/json",
      "webrtc": "finished",
      "webrtc-duration": 65000,
      "x-im-type": "call"
    },
    "content": {
      "callId": "call-002",
      "callType": "video",
      "status": "finished",
      "duration": 65000
    }
  }
}
```

### SDP 与 ICE 信令

通话被接受后，WebRTC 元数据使用 Tinode 原生 `offer`、`answer`、`ice-candidate` 事件转发。麦克风、扬声器、摄像头开关属于本地 UI 状态；如后续需要跨端同步，应在 `server/calls.go` 明确扩展事件白名单。

```json
{
  "note": {
    "topic": "usrPeer001",
    "what": "call",
    "event": "ice-candidate",
    "seq": 102,
    "payload": {
      "callId": "call-002",
      "candidate": "candidate:..."
    }
  }
}
```

## 话题功能

话题不是 Tinode 原生 topic 的“群聊 topic”，而是产品信息流/活动报名类业务。建议实现为频道 `chn`，每个话题详情页对应一个频道 topic；话题列表由 `fnd` 搜索或专用系统频道承载。

### 发布话题

原型页面：发布话题、话题类型选择、话题人数、积分不足。

二期 MySQL 后端已接入话题发布积分扣减：创建 `nch` 频道时，如果 `public.x-im-topic.pointsCost` 大于 `0`，服务端会在同一事务中创建话题、创建作者订阅并扣减 `im_user_points.balance`。余额不足返回 `422 policy violation`，话题不会创建；普通群聊 `{sub topic="new"}` 不受该规则影响。

```json
{
  "sub": {
    "id": "topic-create-1",
    "topic": "nch",
    "set": {
      "desc": {
        "defacs": {"auth": "R", "anon": "N"},
        "public": {
          "fn": "国防办举行高质量完成十四五规划系列主题新闻发布会",
          "photo": {"ref": "/v0/file/s/topic-cover.jpg", "type": "image/jpeg"},
          "note": "话题摘要内容",
          "x-im-topic": {
            "type": "news",
            "labels": ["美食", "天气"],
            "durationHours": 3,
            "participantLimit": 20,
            "pointsCost": 234,
            "joinEnabled": true
          }
        }
      },
      "tags": ["topic", "topic:news", "美食", "天气"],
      "aux": {
        "x-im-topic": {
          "participants": [],
          "status": "open"
        }
      }
    },
    "get": {"what": "desc aux"}
  }
}
```

### 获取话题列表

```json
{
  "set": {
    "id": "topic-search-1",
    "topic": "fnd",
    "desc": {
      "public": "topic news"
    }
  }
}
```

```json
{"get":{"id":"topic-search-2","topic":"fnd","what":"sub","sub":{"limit":20}}}
```

### 推荐话题列表

推荐排序仍复用 Tinode `fnd` topic，通过 `{get what="sub"}` 附带 `x-im-recommend` 传入推荐条件。MySQL 后端当前按关键词、话题类型、标签与订阅数做轻量排序；频道型话题会按 Tinode 规范以 `chnXXX` 返回。

```json
{
  "get": {
    "id": "topic-recommend-1",
    "topic": "fnd",
    "what": "sub",
    "sub": {"limit": 20},
    "x-im-recommend": {
      "keyword": "发布会",
      "types": ["news"],
      "labels": ["美食"],
      "limit": 10
    }
  }
}
```

响应示例：

```json
{
  "meta": {
    "id": "topic-recommend-1",
    "topic": "fnd",
    "sub": [
      {
        "topic": "chnTopic001",
        "public": {
          "fn": "本周美食发布会",
          "x-im-topic": {
            "type": "news",
            "labels": ["美食", "活动"]
          }
        },
        "private": ["topic", "topic:news", "美食"],
        "subcnt": 128
      }
    ]
  }
}
```

### 参与话题

话题详情页点击“参与话题”时订阅频道。

```json
{
  "sub": {
    "id": "topic-join-1",
    "topic": "chnTopic001",
    "set": {
      "sub": {"mode": "R"}
    },
    "get": {"what": "desc data aux"}
  }
}
```

若要发送评论，需要后端根据业务授予 `W` 权限。

### 转发帖子

```json
{
  "pub": {
    "id": "topic-forward-1",
    "topic": "usrPeer001",
    "head": {
      "mime": "application/json",
      "x-im-type": "topic-share"
    },
    "content": {
      "topic": "chnTopic001",
      "title": "国防办举行高质量完成十四五规划系列主题新闻发布会",
      "cover": "/v0/file/s/topic-cover.jpg",
      "summary": "话题摘要内容"
    }
  }
}
```

## 系统消息、举报与安全提示

### 系统消息

系统消息建议使用 `sys` topic 或专用系统账号向用户 P2P 发送。客户端消息列表可将系统账号展示为“系统消息”。

```json
{
  "data": {
    "topic": "usrSystem001",
    "from": "usrSystem001",
    "seq": 1,
    "head": {
      "mime": "application/json",
      "x-im-type": "system-notice"
    },
    "content": {
      "title": "平台整治净化治理公告",
      "summary": "国防办举行高质量完成十四五规划系列...",
      "url": "/notice/20260731"
    },
    "ts": "2026-07-31T08:00:00.000Z"
  }
}
```

### 举报

原型页面：举报、举报原因列表。

普通用户可向 `sys` 发布举报内容。

```json
{
  "pub": {
    "id": "report-1",
    "topic": "sys",
    "head": {
      "mime": "application/json",
      "x-im-type": "report"
    },
    "content": {
      "targetType": "message",
      "targetUser": "usrBad001",
      "targetTopic": "grpNew001",
      "targetSeq": 19,
      "reason": "发布违法违规内容",
      "description": "群成员发布诈骗信息",
      "evidence": ["/v0/file/s/evidence001.jpg"]
    }
  },
  "extra": {
    "attachments": ["/v0/file/s/evidence001.jpg"]
  }
}
```

### 反诈与安全提醒

反诈弹窗可由服务端在消息 `head` 上标记，前端收到后根据策略展示一次或二次确认。当前后端已实现轻量规则 MVP：疑似转账、验证码、外链、违禁词等命中后不阻断消息，只追加风险和审核标记，便于前端提示与后台后续处理。

```json
{
  "data": {
    "topic": "usrPeer001",
    "from": "usrPeer001",
    "seq": 20,
    "head": {
      "mime": "text/x-drafty",
      "x-im-type": "text",
      "x-im-anti-fraud": true,
      "x-im-risk-level": "high",
      "x-im-moderation": {
        "status": "flagged",
        "reasons": ["payment-scam", "credential-risk", "external-link"],
        "reviewedAt": "2026-07-31T08:00:00.000Z"
      }
    },
    "content": {"txt": "请点击链接完成转账"},
    "ts": "2026-07-31T08:00:00.000Z"
  }
}
```

客户端确认提醒后可发送临时回执：

```json
{
  "note": {
    "topic": "usrPeer001",
    "what": "data",
    "payload": {
      "x-im-event": "anti-fraud-confirmed",
      "seq": 20
    }
  }
}
```

## 个人中心与设置

### 获取个人资料

```json
{"get":{"id":"profile-1","topic":"me","what":"desc tags cred"}}
```

### 修改昵称、头像、生日、地区、个性签名

```json
{
  "set": {
    "id": "profile-update-1",
    "topic": "me",
    "desc": {
      "public": {
        "fn": "林南",
        "photo": {"ref": "/v0/file/s/avatar.jpg", "type": "image/jpeg"},
        "note": "成人最懂什么都没留下",
        "x-im-gender": "female",
        "x-im-birthday": "2008-02-07",
        "x-im-area": "广东省广州市"
      }
    }
  },
  "extra": {
    "attachments": ["/v0/file/s/avatar.jpg"]
  }
}
```

### 置顶、免打扰、清空聊天记录

置顶写入 `me.private.tpins` 或当前 topic `private.x-im-pinned`。

```json
{
  "set": {
    "id": "pin-1",
    "topic": "usrPeer001",
    "desc": {
      "private": {
        "x-im-pinned": true
      }
    }
  }
}
```

免打扰：

```json
{
  "set": {
    "id": "mute-topic-1",
    "topic": "usrPeer001",
    "desc": {
      "private": {
        "x-im-muted": true
      }
    }
  }
}
```

清空聊天记录：

```json
{
  "del": {
    "id": "clear-local-1",
    "topic": "usrPeer001",
    "what": "msg",
    "hard": false,
    "delseq": [{"low": 1, "hi": 2147483647}]
  }
}
```

### 隐私设置

原型页面：隐私设置-添加我的方式选项。

```json
{
  "set": {
    "id": "privacy-1",
    "topic": "me",
    "desc": {
      "private": {
        "x-im-settings": {
          "allowSearchByCard": true,
          "allowSearchByQr": true,
          "allowSearchByGroup": true,
          "allowSearchByPhone": false,
          "joinGroupNeedsApproval": true
        }
      }
    }
  }
}
```

### 青少年模式

```json
{
  "set": {
    "id": "teen-1",
    "topic": "me",
    "desc": {
      "private": {
        "x-im-settings": {
          "teenMode": true,
          "teenModeEnabledAt": "2026-07-31T08:00:00.000Z"
        }
      }
    }
  }
}
```

### 签到任务

签到属于业务扩展，使用 `slf` topic 记录用户行为，同时后端更新积分系统。

二期 MySQL 后端已接入 `head.x-im-type="checkin"` 的 `{pub}` 处理：同一用户、同一自然日、同一 `event_type` 只记录一次，首次签到成功后固定增加 `5` 积分并保存一条 `slf` 消息；重复签到返回 `304 no action`，不重复加分，也不保存重复消息。前后端通信仍保持 WebSocket `{pub}` 形态。

```json
{
  "pub": {
    "id": "checkin-1",
    "topic": "slf",
    "head": {
      "mime": "application/json",
      "x-im-type": "checkin"
    },
    "content": {
      "date": "2026-07-31",
      "points": 5
    }
  }
}
```

重复签到响应示例：

```json
{
  "ctrl": {
    "id": "checkin-1",
    "topic": "slf",
    "code": 304,
    "text": "no action",
    "params": {
      "x-im-checkin": {
        "already": true,
        "date": "2026-07-31",
        "points": 5,
        "balance": 35
      }
    }
  }
}
```

### 意见反馈

```json
{
  "pub": {
    "id": "feedback-1",
    "topic": "sys",
    "head": {
      "mime": "application/json",
      "x-im-type": "feedback"
    },
    "content": {
      "type": "功能建议",
      "text": "希望优化消息搜索",
      "images": ["/v0/file/s/feedback001.jpg"]
    }
  },
  "extra": {
    "attachments": ["/v0/file/s/feedback001.jpg"]
  }
}
```

## 错误码与状态码

Tinode `{ctrl.code}` 遵循 HTTP 状态码模型。

| code | 含义 | 前端处理 |
|---|---|---|
| `200` | 成功 | 更新本地状态 |
| `201` | 已创建 | 使用返回的 `topic` 或 `params` |
| `300` | 需要额外验证 | 展示验证码/身份验证页面 |
| `400` | 请求格式错误 | 检查 JSON 和字段 |
| `401` | 未登录或 Token 失效 | 重新登录 |
| `403` | 无权限 | 展示“无权限”或禁言/拉黑提示 |
| `404` | 资源不存在 | 提示用户不存在、群不存在 |
| `409` | 状态冲突 | 已申请、已加入、重复操作 |
| `413` | 消息或文件过大 | 提示压缩或重新选择 |
| `423` | Topic 锁定/处理中 | 稍后重试 |
| `500` | 服务端错误 | 展示通用错误并上报 |
| `503` | 队列满或服务不可用 | 自动重试或稍后重试 |

### 业务状态枚举

| 字段 | 值 | 含义 |
|---|---|---|
| `x-im-type` | `text` | 文本 |
| `x-im-type` | `image` | 图片 |
| `x-im-type` | `audio` | 语音 |
| `x-im-type` | `video` | 视频 |
| `x-im-type` | `file` | 文件 |
| `x-im-type` | `card` | 名片 |
| `x-im-type` | `favorite` | 收藏 |
| `x-im-type` | `forward-bundle` | 合并转发 |
| `x-im-type` | `group-join-apply` | 加群申请 |
| `x-im-type` | `group-join-approve` | 加群审批通过 |
| `x-im-type` | `group-join-reject` | 加群审批拒绝 |
| `x-im-type` | `group-announcement` | 群公告 |
| `x-im-type` | `call` | 通话记录 |
| `x-im-type` | `topic-share` | 话题分享 |
| `x-im-type` | `system-notice` | 系统通知 |
| `x-im-type` | `report` | 举报 |
| `x-im-type` | `feedback` | 意见反馈 |
| `x-im-type` | `checkin` | 签到 |

### 前端发送状态建议

| UI 状态 | 判定方式 |
|---|---|
| 发送中 | 本地消息已入队，尚未收到 `{ctrl}` 或 `{data}` echo |
| 发送成功 | 收到同 `id` 的 `{ctrl.code=200}` 或同 `x-im-client-mid` 的 `{data}` |
| 发送失败 | 收到 `{ctrl.code>=400}` 或超时 |
| 已送达 | 收到对端 `{info.what="recv"}` |
| 已读 | 收到对端 `{info.what="read"}` |
| 已撤回 | 收到 `{pres.what="del"}` 或 `{meta.del}` 包含对应 `seq` |

## 后端实现提示

1. 原生 Tinode 已支持的字段优先复用：`public`、`private`、`aux`、`head.mentions`、`head.reply`、`head.replace`、`head.forwarded`、`extra.attachments`。
2. 业务扩展统一从 `x-im-*` 读取和写入，避免污染 Tinode 原生字段。
3. 需要服务端索引的功能包括：聊天记录关键词搜索、按附件类型搜索、收藏分类、话题列表、积分扣减、反诈风险标记。
4. 所有 WebSocket 消息必须是严格 JSON，示例中的字段名和字符串在真实请求中都需要双引号。
5. 文件上传仍使用 Tinode 大文件接口；上传后的文件引用必须写入 `extra.attachments`，否则服务端 GC 可能清理未引用文件。
6. Phase 3 当前为后端内置 MVP：反诈和内容审核只做规则标记，推荐只做 MySQL 轻量排序，复杂模型、人工审核工作台、异步搜索服务仍可后续独立演进。
