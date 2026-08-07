# 监控 Tinode 服务器

Tinode 服务器可以选择在可配置的 HTTP(S) 端点将运行时统计信息公开为 JSON 文档。该功能通过在配置文件中添加字符串参数 `expvar` 启用。`expvar` 的值是变量发布位置的 URL 路径。除了配置文件，该功能也可以从命令行添加 `--expvar` 参数启用。如果 `expvar` 的值为空字符串 `""` 或短横线 `"-"`，则禁用该功能。命令行参数的非空值会覆盖配置文件值。

默认配置文件中启用该功能，在 `/debug/vars` 发布统计信息。

截至撰写时，发布以下统计信息：

* `memstats`: Go 的内存统计，如 https://golang.org/pkg/runtime/#MemStats 所述
* `cmdline`: 服务器的命令行参数，以字符串数组形式。
* `TotalSessions`: 服务器生命周期内创建的所有会话计数。
* `LiveSessions`: 当前存活的会话数，无论认证状态。
* `TotalTopics`: 服务器生命周期内激活的所有话题计数。
* `LiveTopics`: 当前活跃的话题数。