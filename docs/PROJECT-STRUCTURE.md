# Tinode Chat Server 项目结构详解

> 本文档详细介绍 Tinode Chat Server 项目各个目录的功能、内容和技术细节。

---

## 目录

1. [server/ - 核心服务器](#server---核心服务器)
2. [tinode-db/ - 数据库工具](#tinode-db---数据库工具)
3. [pbx/ - Protocol Buffer 定义](#pbx---protocol-buffer-定义)
4. [py_grpc/ - Python gRPC 绑定](#py_grpc---python-grpc-绑定)
5. [chatbot/ - 聊天机器人](#chatbot---聊天机器人)
6. [tn-cli/ - 命令行客户端](#tn-cli---命令行客户端)
7. [docker/ - Docker 配置](#docker---docker-配置)
8. [loadtest/ - 负载测试](#loadtest---负载测试)
9. [docs/ - 项目文档](#docs---项目文档)
10. [keygen/ - API 密钥生成器](#keygen---api-密钥生成器)
11. [monitoring/ - 监控组件](#monitoring---监控组件)
12. [rest-auth/ - REST 认证示例](#rest-auth---rest-认证示例)

---

## server/ - 核心服务器

### 概述

Tinode 的核心 Go 服务器，实现即时通讯的所有核心功能。

### 主要文件

| 文件 | 大小 | 功能描述 |
|------|------|----------|
| `main.go` | 28KB | 应用入口点，初始化和启动服务 |
| `hub.go` | 27KB | 连接管理中心，处理所有客户端连接 |
| `topic.go` | 124KB | Topic（聊天室/会话）核心逻辑 |
| `session.go` | 41KB | 会话管理，处理用户连接状态 |
| `datamodel.go` | 54KB | 核心数据结构定义 |
| `user.go` | 37KB | 用户管理逻辑 |
| `cluster.go` | 34KB | 集群支持（多服务器部署） |
| `pres.go` | 20KB | 在线状态（Presence）管理 |
| `plugins.go` | 19KB | 插件系统框架 |

### 协议处理

| 文件 | 功能 |
|------|------|
| `hdl_websock.go` | WebSocket 协议处理 |
| `hdl_grpc.go` | gRPC 协议处理 |
| `hdl_longpoll.go` | 长轮询协议处理 |
| `hdl_files.go` | 文件上传下载处理 |
| `http.go` | HTTP 接口 |

### 子目录结构

#### server/auth/ - 认证模块

提供多种认证方式的实现：

```
auth/
├── auth.go          # 认证接口定义
├── anon/            # 匿名认证
│   └── auth_anon.go
├── basic/           # 用户名密码认证
│   └── auth_basic.go
├── token/           # Token 认证
│   └── auth_token.go
├── code/            # 验证码认证
│   └── auth_code.go
├── rest/            # REST 外部认证
│   ├── auth_rest.go
│   └── README.md
└── mock_auth/       # 测试用 Mock 认证
    └── mock_auth.go
```

#### server/db/ - 数据库适配器

支持四种数据库的完整实现：

```
db/
├── adapter.go       # 数据库适配器接口
├── common/          # 公共测试和数据
│   ├── common.go
│   ├── common_test.go
│   └── test_data/
├── mysql/           # MySQL 适配器
│   ├── adapter.go (98KB)
│   ├── schema.sql
│   └── tests/
├── postgres/        # PostgreSQL 适配器
│   ├── adapter.go (100KB)
│   ├── schema.sql
│   └── tests/
├── mongodb/         # MongoDB 适配器
│   ├── adapter.go (89KB)
│   ├── schema.md
│   └── tests/
└── rethinkdb/       # RethinkDB 适配器
    ├── adapter.go (90KB)
    ├── schema.md
    └── tests/
```

#### server/push/ - 推送通知

```
push/
├── push.go          # 推送接口定义
├── common/          # 公共类型定义
│   └── typedef.go
├── fcm/             # Firebase Cloud Messaging
│   ├── push_fcm.go
│   ├── payload.go
│   └── README.md
├── tnpg/            # Tinode Push Gateway
│   └── push_tnpg.go
└── stdout/          # 测试用标准输出推送
    └── push_stdout.go
```

#### server/media/ - 文件存储

```
media/
├── media.go         # 存储接口
├── media_test.go
├── fs/              # 本地文件系统存储
│   └── filesys.go
└── s3/              # AWS S3 存储
    └── s3.go
```

#### server/store/ - 数据存储层

内部数据存储抽象层。

#### server/templ/ - 模板引擎

29个模板文件，用于邮件通知、消息格式化等。

#### server/drafty/ - 富文本处理

Drafty 富文本格式的处理逻辑。

#### server/ringhash/ - 一致性哈希

用于集群环境的一致性哈希实现。

#### server/concurrency/ - 并发工具

并发控制相关的辅助工具。

#### server/logs/ - 日志组件

日志记录和格式化。

#### server/validate/ - 数据验证

输入数据验证和校验。

### 配置文件

- `tinode.conf` - 主配置文件（28KB），包含所有可配置选项

### 测试文件

- `session_test.go` - 会话测试
- `topic_test.go` - Topic 逻辑测试（99KB）
- `utils_test.go` - 工具函数测试

---

## tinode-db/ - 数据库工具

### 概述

数据库初始化和升级命令行工具，用于创建、迁移和填充测试数据。

### 文件结构

```
tinode-db/
├── main.go           # 入口文件
├── gendb.go          # 数据库生成逻辑
├── tinode.conf       # 示例配置
├── data.json         # 测试数据集
├── credentials.sh    # 凭证处理脚本
├── generate_dataset.py # 数据集生成 Python 脚本
├── README.md
└── *-128.jpg         # 测试用户头像图片
```

### 构建方式

```bash
# RethinkDB
go build -tags rethinkdb

# MySQL
go build -tags mysql

# MongoDB
go build -tags mongodb

# PostgreSQL
go build -tags postgres
```

### 主要功能

| 参数 | 功能 |
|------|------|
| `--reset` | 删除并重建数据库 |
| `--upgrade` | 从旧版本升级数据库 |
| `--no_init` | 仅检查数据库存在性 |
| `--data=FILENAME` | 加载测试数据 |
| `--make_root=USER_ID` | 提升用户为管理员 |
| `--add_root=USERNAME:PASSWORD` | 创建管理员账户 |

### 测试数据

默认 `data.json` 创建：

- **6个用户**: alice, bob, carol, dave, frank, tino（机器人）
- **密码格式**: 用户名 + "123"（如 alice123）
- **群组话题**: 3个
- **随机消息**: 自动填充

---

## pbx/ - Protocol Buffer 定义

### 概述

Tinode gRPC API 的 Protocol Buffer 定义文件，是跨语言通信的核心。

### 文件结构

```
pbx/
├── model.proto       # 原始定义文件（14KB）
├── model.pb.go       # Go 生成的绑定（185KB）
├── model_grpc.pb.go  # Go gRPC 服务定义（18KB）
├── go-generate.sh    # Go 代码生成脚本
├── py-generate.sh    # Python 代码生成脚本
├── py_fix.py         # Python 导入修复脚本
└── README.md
```

### 核心服务定义

#### Node 服务（客户端实现）

```protobuf
service Node {
    rpc MessageLoop(stream ClientMsg) returns (stream ServerMsg);
    rpc LargeFileReceive(stream FileUpReq) returns (FileUpResp);
    rpc LargeFileServe(FileDownReq) returns (stream FileDownResp);
}
```

#### Plugin 服务（插件实现）

```protobuf
service Plugin {
    // 插件事件流
}
```

### 生成代码

**Go 绑定**:
```bash
protoc --proto_path=. --go_out=plugins=grpc:. model.proto
```

**Python 绑定**:
```bash
python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. model.proto
```

---

## py_grpc/ - Python gRPC 绑定

### 概述

Python gRPC 客户端绑定包，可直接 `pip install` 安装使用。

### 文件结构

```
py_grpc/
├── pyproject.toml    # PEP 517 项目配置
├── README.md
├── version.py
├── .gitignore
├── LICENSE
└── tinode_grpc/
    ├── __init__.py
    ├── model_pb2.py      # 消息定义
    └── model_pb2_grpc.py # gRPC 服务存根
```

### 安装使用

```bash
pip install tinode_grpc
```

```python
from tinode_grpc import pb, pbx

# 使用客户端存根
channel = grpc.insecure_channel('localhost:16060')
stub = pbx.NodeStub(channel)
```

### 项目配置

```toml
[project]
name = "tinode_grpc"
dependencies = [
    "protobuf>=3.6.1",
    "grpcio>=1.19.0",
]
```

---

## chatbot/ - 聊天机器人

### 概述

基于 gRPC Plugin API 的示例聊天机器人，展示如何开发 Tinode 插件。

### 文件结构

```
chatbot/
├── python/
│   ├── chatbot.py        # 主程序（14KB）
│   ├── quotes.txt        # 回复语录库
│   ├── requirements.txt
│   ├── setup.py
│   ├── basic-cookie.sample
│   ├── token-cookie.sample
│   └── README.md
└── csharp/              # C# 版本（占位）
```

### 工作流程

1. 作为普通用户登录 Tinode
2. 订阅事件流（Plugin API）
3. 监听新账户创建事件
4. 主动与新用户建立 p2p 会话
5. 收到消息时回复随机语录

### 运行方式

```bash
# 安装依赖
pip install -r requirements.txt

# 启动机器人
python chatbot.py --login-basic=tino:tino123

# 后台运行
nohup python chatbot.py --login-basic=tino:tino123 &
```

### Docker 部署

```bash
docker run -d --name tino-chatbot \
    --network tinode-net \
    --volume botdata:/botdata \
    tinode/chatbot:latest
```

---

## tn-cli/ - 命令行客户端

### 概述

可脚本化的命令行聊天客户端，用于管理和测试 Tinode 服务。

### 文件结构

```
tn-cli/
├── tn-cli.py        # 主入口
├── client.py        # gRPC 客户端封装
├── commands.py      # 命令实现（33KB）
├── macros.py        # 高级宏命令（12KB）
├── input_handler.py # 输入处理
├── utils.py         # 工具函数
├── tn_globals.py    # 全局状态
├── requirements.txt
├── sample-script.txt      # 示例脚本
├── sample-macro-script.txt # 宏脚本示例
├── test-128.jpg     # 测试图片
├── CODE-STRUCTURE.md
└── README.md
```

### 命令列表

#### 本地命令

| 命令 | 功能 |
|------|------|
| `.exit` / `.quit` | 退出 CLI |
| `.await` | 执行 gRPC 调用并等待 |
| `.must` | 执行 gRPC 调用，失败则抛异常 |
| `.sleep` | 休眠指定毫秒 |
| `.log` | 输出变量值 |
| `.use` | 设置默认用户/话题 |
| `.verbose` | 切换日志详细模式 |

#### gRPC 命令

| 命令 | 功能 |
|------|------|
| `acc` | 创建/修改账户 |
| `login` | 登录认证 |
| `sub` | 订阅话题 |
| `leave` | 取消订阅 |
| `pub` | 发布消息 |
| `get` | 查询元数据/消息 |
| `set` | 更新元数据 |
| `del` | 删除消息/话题/用户 |
| `note` | 发送通知 |
| `file` | 文件上传下载 |

#### 宏命令

| 宏 | 功能 |
|----|------|
| `useradd` | 创建用户 |
| `userdel` | 删除用户（需 root） |
| `usermod` | 修改用户 |
| `passwd` | 修改密码（需 root） |
| `resolve` | 解析登录名 |

### 使用示例

```bash
# 交互模式
python tn-cli.py --login-basic=alice:alice123

# 脚本模式
python tn-cli.py < sample-script.txt

# 连接安全服务器
python tn-cli.py --host=localhost:16060 --ssl --ssl-host=my-server.com
```

---

## docker/ - Docker 配置

### 概述

Docker 容器构建和部署配置，支持多种数据库后端。

### 文件结构

```
docker/
├── README.md            # 详细使用说明
├── tinode/              # Tinode 服务器镜像
│   └── ...
├── exporter/            # 指标导出器
│   └── ...
├── chatbot/             # 聊天机器人镜像
│   └── ...
└── docker-compose/      # Docker Compose 配置
    └── ...
```

### 支持的镜像

- `tinode/tinode-mysql:latest`
- `tinode/tinode-rethink:latest`
- `tinode/tinode-mongodb:latest`
- `tinode/chatbot:latest`

### 快速启动

```bash
# MySQL 版本
docker run -p 6060:18080 -d --name tinode-srv \
    --network tinode-net \
    tinode/tinode-mysql:latest

# 使用 Docker Compose
cd docker/docker-compose
docker-compose up -d
```

---

## loadtest/ - 负载测试

### 概述

服务器性能测试工具集，支持 Tsung 和 Gatling 两种框架。

### 文件结构

```
loadtest/
├── tsung.xml        # Tsung 配置
├── tinode.erl       # Erlang 辅助模块
├── tinode.beam      # 编译后的 Erlang 模块
├── loadtest.scala   # Gatling 测试脚本
├── tinode.scala     # Gatling 辅助类
├── users.csv        # 测试用户列表
├── LICENSE
└── README.md
```

### 测试场景

| 场景 | 描述 |
|------|------|
| `tinode.Loadtest` | 连接后订阅话题，发送消息 |
| `tinode.MeLoadtest` | 压测 `me` 话题连接 |
| `tinode.SingleTopicLoadtest` | 单话题压力测试 |

### 参数配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `num_sessions` | 10000 | 总连接数 |
| `ramp` | 300 | 爬升时间（秒） |
| `publish_count` | 10 | 每用户消息数 |
| `publish_interval` | 100 | 消息间隔（秒） |

### 运行测试

**Tsung**:
```bash
tsung -f ./tsung.xml start
```

**Gatling**:
```bash
JAVA_OPTS="-Dnum_sessions=1000 -Dramp=60" \
    gatling.sh -sf . -rsf . -rd "na" -s tinode.Loadtest
```

### 性能基准

在 AWS t3.xlarge (4 vCPU, 16GB) 上测试结果：
- 支持 50,000 并发连接
- 单话题支持 1,500 并发会话

---

## docs/ - 项目文档

### 概述

项目文档和营销素材。

### 文件结构

```
docs/
├── API.md           # API 文档（92KB）
├── drafty.md        # Drafty 富文本格式规范
├── faq.md           # 常见问题
├── call-establishment.md  # 音视频通话建立流程
├── translations.md  # 翻译指南
├── monitoring.md    # 监控配置
├── thecard.md       # TheCard 功能说明
├── CLA.md           # 贡献者许可协议
├── logo.svg         # Logo
├── app-store.svg    # App Store 徽章
├── play-store.svg   # Play Store 徽章
├── web-app.svg      # Web 应用徽章
└── *.png/*.jpg      # 产品截图和宣传图片
```

### 主要文档

| 文档 | 内容 |
|------|------|
| API.md | 完整的客户端 API 参考 |
| drafty.md | Drafty 富文本格式规范 |
| call-establishment.md | 音视频通话信令流程 |
| faq.md | 常见问题解答 |

---

## keygen/ - API 密钥生成器

### 概述

生成和验证 API 密钥的命令行工具，用于防止 API 被滥用。

### 文件结构

```
keygen/
├── keygen.go
├── README.md
└── LICENSE
```

### 功能参数

| 参数 | 说明 |
|------|------|
| `sequence` | 密钥序号，用于撤销旧密钥 |
| `isroot` | 是否为管理员密钥（暂未使用） |
| `validate` | 验证已有密钥 |
| `salt` | HMAC 盐值 |

### 使用方式

```bash
./keygen
```

输出示例：
```
API key v1 seq1 [ordinary]: AQAAAAABAACGOIyP2vh5avSff5oVvMpk
HMAC salt: TC0Jzr8f28kAspXrb4UYccJUJ63b7CSA16n1qMxxGpw=
```

### 密钥部署

- **服务器**: 将 `HMAC salt` 添加到 `tinode.conf` 的 `api_key_salt`
- **客户端**:
  - TinodeWeb: `config.js` 的 `API_KEY`
  - Tindroid: `Cache.java` 的 `API_KEY`
  - Tinodious: `SharedUtils.swift` 的 `kApiKey`

---

## monitoring/ - 监控组件

### 概述

Prometheus 指标导出器和监控配置。

### 文件结构

```
monitoring/
├── exporter/        # Prometheus Exporter
│   └── ...
├── README.md
└── LICENSE
```

### 功能

导出 Tinode 服务器运行指标供 Prometheus 采集，支持 Grafana 可视化。

---

## rest-auth/ - REST 认证示例

### 概述

REST 认证服务器的 Python 示例实现。

### 文件结构

```
rest-auth/
├── auth.py          # Flask 认证服务
├── dummy_data.json  # 测试数据
├── requirements.txt
└── README.md
```

### 功能

作为 `server/auth/rest/` 模块的外部认证服务示例：
- 接收认证请求
- 返回认证结果
- 支持用户信息查询

### 运行

```bash
pip install flask
python auth.py
```

---

## 构建脚本

项目根目录下的构建脚本：

| 脚本 | 功能 |
|------|------|
| `build-all.sh` | 构建所有组件 |
| `build-py-grpc.sh` | 构建 Python gRPC 包 |
| `docker-build.sh` | 构建 Docker 镜像 |
| `docker-release.sh` | 发布 Docker 镜像 |

---

## 依赖管理

- `go.mod` / `go.sum` - Go 模块依赖

---

## 技术栈总结

| 层次 | 技术 |
|------|------|
| 语言 | Go 1.26, Python 2.7+/3.4+ |
| 协议 | WebSocket, gRPC, Long Polling, HTTP |
| 数据库 | MySQL, PostgreSQL, MongoDB, RethinkDB |
| 推送 | Firebase Cloud Messaging |
| 存储 | AWS S3, 本地文件系统 |
| 容器化 | Docker, Docker Compose |
| 监控 | Prometheus, Grafana |
| 测试 | Tsung, Gatling |