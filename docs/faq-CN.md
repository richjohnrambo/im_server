# 常见问题解答

### Q: 在 Docker 中运行时，服务器日志在哪里？<br/>
**A**: 日志在容器内的 `/var/log/tinode.log`。使用以下命令附加到运行中的容器：
```
docker exec -it name-of-the-running-container /bin/bash
```
然后，例如用 `tail -50 /var/log/tinode.log` 查看日志。

如果容器已停止，可以从容器复制日志（保存到 `./tinode.log`）：
```
docker cp name-of-the-container:/var/log/tinode.log ./tinode.log
```

或者，可以通过将主机目录映射到容器内的 `/var/log/`，让 Docker 容器将日志保存到主机目录。在 `docker run` 命令中添加 `-v /where/to/save/logs:/var/log`。


### Q: 启用推送通知有哪些选项？<br/>
**A**: 可以使用 [Tinode Push Gateway (TNPG)](https://github.com/tinode/chat/tree/master/server/push/tnpg) 或 [Google FCM](https://firebase.google.com/docs/cloud-messaging)：
 * _Tinode Push Gateway_ 使用 Tinode 服务器代您发送推送。需要最少的设置：您的服务器向 TNPG 发送请求，TNPG 转发给 Google FCM 或 Apple APNS。
 * _Google FCM_ 不依赖 Tinode 基础设施推送，但需要您构建和发布自己的移动应用（iOS 和 Android）。


### Q: 如何使用 Tinode Push Gateway 设置推送通知？<br/>
**A**: 启用 TNPG 推送通知需要两步：
 * 在 [console.tinode.co](https://console.tinode.co) 注册并获取 TNPG 令牌。
 * 使用令牌配置服务器。
详见[此处](../server/push/tnpg/)。


### Q: 如何使用 Google FCM 设置推送通知？<br/>
**A**: 此选项需要您构建和发布自己的移动应用。如果您不想这样做，请使用上面的 TNPG 选项。

启用 FCM 推送通知需要以下步骤：
 * 启用服务器端推送发送。
 * 启用客户端推送接收。

#### 服务器和 TinodeWeb

1. 如尚未创建，请在 https://firebase.google.com/ 创建项目。
2. 按照 https://cloud.google.com/iam/docs/creating-managing-service-account-keys 的说明下载凭据文件。
3. 更新服务器配置 [`tinode.conf`](../server/tinode.conf#L255)，在 `"push"` -> `"name": "fcm"` 部分。执行以下**任一**操作：
  * 将下载的凭据文件路径填入 `"credentials_file"`。
  * 或将文件内容复制到 `"credentials"`。<br/><br/>
    删除另一个条目。例如，如果更新了 `"credentials_file"`，则删除 `"credentials"`，反之亦然。
4. 更新 [TinodeWeb](/tinode/webapp/) 配置 [`firebase-init.js`](https://github.com/tinode/webapp/blob/master/firebase-init.js)：更新 `apiKey`、`messagingSenderId`、`projectId`、`appId`、`messagingVapidKey`。详见 https://github.com/tinode/webapp/#push_notifications

#### iOS 和 Android
1. 如果使用 Android 客户端，按照 https://developers.google.com/android/guides/google-services-plugin 的说明将 `google-services.json` 添加到 [Tindroid](/tinode/tindroid/) 并重新编译客户端。您也可以选择提交到 Google Play Store。
详见 https://github.com/tinode/tindroid/#push_notifications
2. 如果使用 iOS 客户端，按照 https://firebase.google.com/docs/cloud-messaging/ios/client 的说明将 `GoogleService-Info.plist` 添加到 [Tinodios](/tinode/ios/) 并重新编译客户端。您也可以选择提交到 Apple AppStore。
详见 https://github.com/tinode/ios/#push_notifications


### Q: 如何添加新用户？<br/>
**A**: 创建账户有三种方式：
* 用户可以使用应用程序（Web、Android、iOS）创建新账户。
* 可以使用 [tn-cli](../tn-cli/) 创建新账户（`acc` 命令或 `useradd` 宏）。该过程可脚本化。
* 如果用户已存在于外部数据库，可以在首次登录时使用 [rest authenticator](../server/auth/rest/) 自动创建 Tinode 账户。


### Q: 如何让我的安装私有化？<br/>
**A**: 如果想限制只有您批准的人才能注册，最简单的方法是将 Tinode 注册限制到您控制的邮件域名：注册自定义域名，在域名注册商处设置 catch-all 邮件转发服务（通常免费）。然后在 Tinode 配置中使用您的域名（`"acc_validation" -> "email" -> "domains"`，例如 `"domains": ["my-domain.com"]`）。您会在 catch-all 邮箱收到注册邮件，然后可以手动转发验证码给您的用户。或者，如果用户很多，可以使用 [rest authenticator](../server/auth/rest/)。


### Q: 如何创建 `root` 用户？<br/>
**A**: 从 Tinode 0.18 版本开始，可以通过运行以下命令授予用户 `root` 权限：
```sh
./tinode-db -auth=ROOT -uid=usrAbcDef123 -scheme=basic
```
从 0.21 版本开始，可以使用更简单的命令：
```sh
./tinode-db -make_root=usrAbcDef123
```
其中 `usrAbcDef123` 是要更新的用户 ID。

在 0.17 及更早版本中，只能通过执行数据库查询授予用户 `root` 权限。
首先创建或选择要提升为 `root` 的用户，然后执行查询：
* RethinkDB:
```js
r.db('tinode').table('auth').get('basic:login-of-the-user-to-make-root').update({authLvl: 30})
```
* MySQL, PostgreSQL:
```sql
USE 'tinode';
UPDATE auth SET authlvl=30 WHERE uname='basic:login-of-the-user-to-make-root';
```
* MongoDB:
```js
db.getCollection('auth').updateOne({_id: 'basic:login-of-the-user-to-make-root'}, {$set: {authlvl: 30}})
```
测试数据库有一个预设的 `xena` 用户具有 root 权限。


### Q: 当网络连接数达到每节点约 1000 时，各种问题开始出现。这是 bug 吗？<br/>
**A**: 这很可能不是 bug。为确保服务器良好性能，Linux 在内核级别限制每个进程的打开文件描述符（活动网络连接、打开文件）总数。默认限制通常是 1024。文件描述符数量还有其他可能的限制。您遇到的问题很可能是因为超过了某个 Linux 限制。请寻求系统管理员的帮助。


### Q: 群组话题和频道有什么区别？<br/>
**A**: 频道是群组话题的特例。普通群组话题允许有限数量的订阅者（默认 128）。每个订阅者可以单独管理：邀请、移除、封禁、提升为管理员或所有者，其他访问权限也可以单独调整。启用频道功能的群组话题额外允许无限数量的 `读者`。读者对话题有只读访问权限，不能单独管理，不能被邀请或移除，不能发布消息。读者加入或退出话题时不生成在线状态通知，也不接收普通群组成员的在线状态通知。读者接收的频道消息的 `From` 字段为 `null`，即他们不知道谁发布了频道中的任何特定消息。读者不能删除频道消息。


### Q: 格式化 gRPC {pub content} 的正确方式是什么？<br/>
**A**: gRPC 将 `{pub}` 消息的 `content` 字段作为字节数组发送，而客户端应用期望它是有效的 JSON。因此，在传递给 gRPC 之前，必须将字段格式化为有效的 JSON。例如，要发送纯文本 `Hello world` 消息，必须发送带引号的字符串 `"Hello world"`。大多数情况下，传递给 gRPC 调用的字符串看起来像 `"\"Hello world\""` 或 `'"Hello world"'`。


### Q: 如何修复 PostgreSQL 初始化因 'missing database' 错误而失败的问题？<br/>
**A**: PostgreSQL 有一个（不当）特性：数据库连接必须始终选择一个数据库。如果连接尝试使用不存在的数据库（即使是为了创建它），连接会失败。当 Tinode 首次启动时，它尝试创建数据库，通常是 `tinode`（见 `tinode.conf`，`"store_config": {"adapters": {"postgres": {"DBName": "tinode"}}}`。数据库 `tinode` 显然不存在，所以 Tinode 连接回退到 'default' 数据库，该数据库与连接的 PostgreSQL 用户同名。默认配置指定用户为 `postgres`（`"User": "postgres"`），数据库 `postgres` 总是存在，所以连接成功，一切正常。但如果您将用户更改为 `postgres` 以外的任何值，比如 `tinodeadmin`，麻烦就开始了：名为 `tinodeadmin` 的数据库不存在，连接失败。如果要将用户名更改为 `postgres` 以外的任何值，必须创建数据库 `tinode`（或您为 Tinode 数据库命名的任何名称）或创建一个与您的用户 `tinodeadmin` 同名的空数据库。例如：
```
$ psql
	postgres=# create database tinode;
	exit
```