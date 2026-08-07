# Tinode 本地化

**重要！** 请使用 `devel` 分支进行翻译。

## 服务器

服务器在新账户创建时和用户请求重置密码时向用户发送电子邮件或短信：

* [/server/templ/email-validation-en.templ](../server/templ/email-validation-en.templ)
* [/server/templ/email-password-reset-en.templ](../server/templ/email-password-reset-en.templ)
* [/server/templ/sms-validation-en.templ](../server/templ/sms-validation-en.templ)

创建文件副本，命名为 `email-password-reset-XX.templ`、`email-validation-XX.templ`、`sms-validation-XX.templ`，其中 `XX` 是新语言的 [ISO-631-1](https://en.wikipedia.org/wiki/List_of_ISO_639-1_codes) 代码。翻译内容并通过新文件发送拉取请求。如果您不知道如何创建拉取请求，可以任何方式发送翻译后的文件。


## Webapp

翻译位于两个位置：[/src/i18n/](https://github.com/tinode/webapp/tree/devel/src/i18n/) 和 [/service-worker.js](https://github.com/tinode/webapp/blob/devel/service-worker.js#L11)。

要添加翻译，将 `/src/i18n/en.json` 复制到名为 `/src/i18n/XX.json` 的文件，其中 `XX` 是新语言的 [BCP-47](https://tools.ietf.org/rfc/bcp/bcp47.txt) 代码。如果不确定如何选择 BCP-47 语言代码，请使用两位字母的 [ISO-631-1](https://en.wikipedia.org/wiki/List_of_ISO_639-1_codes)。只需翻译 `"translation":` 行，不应翻译 `"defaultMessage"`、`"description"` 等，它们仅作为帮助：

```js
"action_block_contact": {
  "translation": "Bloquear contacto", // <<<---- 只需翻译此字符串
  "defaultMessage": "Block Contact",  // 这是英文默认消息
  "description": "Flat button [Block Contact]", // 这是字符串使用位置/方式的说明
  "missing": false,
  "obsolete": false
},
```

翻译 `service-worker.js` 时，直接将字符串添加到文件中。只需翻译两个字符串 "New message" 和 "New chat"：

```js
const i18n = {
  ...
  'XX': {
    'new_message': "New message",
    'new_chat': "New chat",
  },
  ...
```

请通过新文件发送拉取请求。如果您不知道如何创建拉取请求，可以任何方式发送文件。


## Android

需要翻译单个文件：[/tinode/tindroid/app/src/main/res/values/strings.xml](https://github.com/tinode/tindroid/blob/devel/app/src/main/res/values/strings.xml)

在 [app/src/main/res](https://github.com/tinode/tindroid/tree/devel/app/src/main/res) 创建新目录 `values-XX`，其中 `XX` 是两位字母的 [ISO-631-1](https://en.wikipedia.org/wiki/List_of_ISO_639-1_codes) 代码，可选后跟两位字母的 [ISO 3166-1-alpha-2](https://en.wikipedia.org/wiki/ISO_3166-1_alpha-2) 区域代码（以小写 r 开头）。例如 `values-pt.xml` 包含葡萄牙语翻译，而 `values-pt-rBR.xml` 用于_巴西_葡萄牙语翻译。

复制英文字符串文件的副本，放到新目录。翻译所有未标记 `translatable="false"` 的字符串（标记 `translatable="false"` 的字符串根本不需要包含），然后通过新文件发送拉取请求。如果您不知道如何创建拉取请求，可以任何方式发送文件。


## iOS

不幸的是，iOS 本地化过程非常复杂，通常需要只在 Mac 上运行的 `Xcode`。除非您熟悉 iOS 开发，请为所需语言创建[功能请求](https://github.com/tinode/ios/issues/new?assignees=&labels=&template=feature_request.md&title=)，我们会发送文件供您翻译。

如果您足够勇敢，可以翻译[以下 .xliff 文件](https://github.com/tinode/ios/blob/devel/Localizations/en.xcloc/Localized%20Contents/en.xliff)然后以任何方式发送给我们。翻译 `<target>` 标签之间的所有字符串：

```xml
<trans-unit id="Action failed: %@" xml:space="preserve"> <!-- 不要更改此行 -->
  <source>Action failed: %@</source> <!-- 这是英文默认消息 -->
  <target>Se ha producido un error al realizar la acción: %@</target> <!-- 只有此字符串 "target" 需要翻译 -->
  <note>Toast notification</note> <!-- 这是字符串使用位置/方式的说明 -->
</trans-unit>
```

如果您熟悉 Xcode 和 iOS 本地化，导出的本地化位于 [/Localizations](https://github.com/tinode/ios/tree/devel/Localizations)。