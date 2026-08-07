# Drafty: 富消息格式

Drafty 是 Tinode 用于消息样式的文本格式。Drafty 的设计目标是足够表达力，同时避免过多的安全风险。可以将其视为 JSON 封装的 [markdown](https://en.wikipedia.org/wiki/Markdown)。Drafty 受到 FB 的 [draft.js](https://draftjs.org/) 规范影响。目前已有 [Javascript](https://github.com/tinode/tinode-js/blob/master/src/drafty.js)、[Java](https://github.com/tinode/tindroid/blob/master/tinodesdk/src/main/java/co/tinode/tinodesdk/model/Drafty.java) 和 [Swift](https://github.com/tinode/ios/blob/master/TinodeSDK/model/Drafty.swift) 实现。[Go 实现](https://github.com/tinode/chat/blob/master/server/drafty/drafty.go) 可将 Drafty 转换为纯文本和预览。

## 示例

> this is **bold**, `code` and _italic_, ~~strike~~<br/>
>  combined **bold and _italic_**<br/>
>  an url: https://www.example.com/abc#fragment and another _[https://web.tinode.co](https://web.tinode.co)_<br/>
>  this is a [@mention](#) and a [#hashtag](#) in a string<br/>
> second [#hashtag](#)<br/>

上述文本的 Drafty-JSON 表示：
```js
{
   "txt":  "this is bold, code and italic, strike combined bold and italic an url: https://www.example.com/abc#fragment and another www.tinode.co this is a @mention and a #hashtag in a string second #hashtag",
   "fmt": [
       { "at":8, "len":4,"tp":"ST" },{ "at":14, "len":4, "tp":"CO" },{ "at":23, "len":6, "tp":"EM"},
       { "at":31, "len":6, "tp":"DL" },{ "tp":"BR", "len":1, "at":37 },{ "at":56, "len":6, "tp":"EM" },
       { "at":47, "len":15, "tp":"ST" },{ "tp":"BR", "len":1, "at":62 },{ "at":120, "len":13, "tp":"EM" },
       { "at":71, "len":36, "key":0 },{ "at":120, "len":13, "key":1 },{ "tp":"BR", "len":1, "at":133 },
       { "at":144, "len":8, "key":2 },{ "at":159, "len":8, "key":3 },{ "tp":"BR", "len":1, "at":179 },
       { "at":187, "len":8, "key":3 },{ "tp":"BR", "len":1, "at":195 }
   ],
   "ent": [
       { "tp":"LN", "data":{ "url":"https://www.example.com/abc#fragment" } },
       { "tp":"LN", "data":{ "url":"http://www.tinode.co" } },
       { "tp":"MN", "data":{ "val":"mention" } },
       { "tp":"HT", "data":{ "val":"hashtag" } }
   ]
}
```

## 结构

Drafty 对象有三个字段：纯文本 `txt`、内联标记 `fmt` 和实体 `ent`。

### 纯文本

要发送的消息转换为纯 Unicode 文本，所有标记被剥离并保存在 `txt` 字段。通常，有效的 Drafty 可以只包含 `txt` 字段。

### 内联格式 `fmt`

内联格式是 `fmt` 字段中的样式数组。每个样式由至少包含 `at` 和 `len` 字段的对象表示。`at` 值表示 `txt` 中从 0 开始的偏移量，`len` 是要应用格式的字符数。样式的第三个值是 `tp` 或 `key`。

如果提供 `tp`，表示样式是基本文本装饰：
 * `BR`: 换行。
 * `CO`: 代码或等宽文本，可能有不同背景：`monotype`。
 * `DL`: 删除或删除线文本：~~strikethrough~~。
 * `EM`: 强调文本，通常表示为斜体：_italic_。
 * `FM`: 表单/字段集；也可表示为实体。
 * `HD`: 隐藏内容。
 * `HL`: 高亮文本，如不同颜色或背景的文本；无法指定颜色。
 * `RW`: 格式的逻辑分组，行；也可表示为实体。
 * `ST`: 强调或粗体文本：**bold**。

如果提供 key，它是 `ent` 数组的 0 基索引，包含扩展样式参数（如图片或 URL）：
 * `AU`: 嵌入音频。
 * `BN`: 交互按钮。
 * `EX`: 通用附件。
 * `FM`: 表单/字段集；也可表示为基本装饰。
 * `HT`: 话题标签，如 [#hashtag](#)。
 * `IM`: 内联图片。
 * `LN`: 链接（URL）[https://api.tinode.co](https://api.tinode.co)。
 * `MN`: 提及，如 [@tinode](#)。
 * `RW`: 格式的逻辑分组，行；也可表示为基本装饰。
 * `VC`: 视频（和音频）通话。
 * `VD`: 内联视频。

示例：
 * `{ "at":8, "len":4, "tp":"ST"}`: 从 `txt` 偏移 8 开始对 4 个字符应用 `ST`（粗体）格式。
 * `{ "at":144, "len":8, "key":2 }`: 在位置 144 插入实体 `ent[2]`，实体跨越 8 个字符。
 * `{ "at":-1, "len":0, "key":4 }`: 将 `ent[4]` 显示为文件附件，不应用任何文本样式。

客户端应能处理缺失的 `at`、`key` 和 `len` 值。缺失值假定为 `0`。

索引 `at` 和 `len` 以 [Unicode 码点](https://developer.mozilla.org/en-US/docs/Glossary/Code_point)计算，而非字节或字符。带有 Fitzpatrick 肤调修饰符、变体选择器或用 `ZWJ` 分组等多码点字形的行为目前未定义。

#### `FM`: 表单，有序字段集

表单提供了为逻辑元素组添加段落级格式的方法。它可以表示为文本装饰或实体。
<table>
<tr><th>Do you agree?</th></tr>
<tr><td><a href="">Yes</a></td></tr>
<tr><td><a href="">No</a></td></tr>
</table>

```js
{
 "txt": "Do you agree? Yes No",
 "fmt": [
   {"len": 20, "tp": "FM"}, // 缺失 'at' 为零: "at": 0
   {"len": 13, "tp": "ST"}
   {"at": 13, "len": 1, "tp": "BR"},
   {"at": 14, "len": 3}, // 缺失 'key' 为零: "key": 0
   {"at": 17, "len": 1, "tp": "BR"},
   {"at": 18, "len": 2, "key": 1},
 ],
 "ent": [
   {"tp": "BN", "data": {"name": "yes", "act": "pub", "val": "oh yes!"}},
   {"tp": "BN", "data": {"name": "no", "act": "pub"}}
 ]
}
```
如果上例中按了 `Yes` 按钮，客户端应向服务器发送以下内容的消息：
```js
{
 "txt": "Yes",
 "fmt": [{
   "at":-1
 }],
 "ent": [{
   "tp": "EX",
   "data": {
     "mime": "text/x-drafty-fr", // drafty form-response.
     "val": {
       "seq": 15, // 包含表单的消息 seq id.
       "resp": {"yes": "oh yes!"}
     }
   }
 }]
}
```

表单也可表示为实体：
```js
{
  "tp": "FM",
  "data": {
    "su": true
  }
}
```
`data.su` 描述点击后交互表单元素的行为。`"su": true` 表示表单是`单次使用`：首次交互后表单应更改以显示不再接受输入。

### 实体 `ent`

通常，实体是需要额外（可能很大）数据的文本装饰。实体由包含两个字段的物件表示：`tp` 表示实体类型，`data` 是类型相关的样式信息。未知字段被忽略。

#### `AU`: 嵌入音频记录
`AU` 是音频记录。`data` 包含以下字段：
```js
{
  "tp": "AU",
  "data": {
    "mime": "audio/aac",
    "val": "Rt53jUU...iVBORw0KGgoA==",
    "ref": "/v0/file/s/e769gvt1ILE.m4v",
    "preview": "Aw4JKBkAAAAKMSM...vHxgcJhsgESAY"
    "duration": 180000,
    "name": "ding_dong.m4a",
    "size": 595496
  }
}
```
 * `mime`: 数据类型，如 'audio/ogg'。
 * `val`: 可选带内音频数据：base64 编码的音频位。
 * `ref`: 可选带外音频数据引用。必须有 `val` 或 `ref`。
 * `preview`: base64 编码的字节数组，用于生成可视化预览；每个字节是一个振幅条。
 * `duration`: 录音时长（毫秒）。
 * `name`: 原始文件的可选名称。
 * `size`: 文件大小（字节）的可选值。

创建只包含单个音频记录、无文本的消息，使用以下 Drafty：
```js
{
  txt: " ",
  fmt: [{len: 1}],
  ent: [{tp: "AU", data: {<你的音频数据>}}]
}
```

_重要安全考虑_：`val` 和 `ref` 字段可能包含恶意载荷。客户端应将 `ref` 字段的 URL 方案限制为仅 `http` 和 `https`。客户端仅当正确转换为音频时，才应向用户展示 `val` 字段内容。


#### `BN`: 交互按钮
`BN` 提供向服务器发送数据的选项，可以是源服务器或其他服务器。`data` 包含以下字段：
```js
{
  "tp": "BN",
  "data": {
    "name": "confirmation",
    "act": "url",
    "val": "some-value",
    "ref": "https://www.example.com/path/?foo=bar"
  }
}
```
* `act`: 按钮点击响应的动作类型：
  * `pub`: 向当前话题发送 Drafty 格式的 `{pub}` 消息，表单数据作为附件：
  ```js
  { "tp":"EX", "data":{ "mime":"text/x-drafty-fr", "val": { "seq": 3, "resp": { "confirmation": "some-value" } } } }
  ```
  * `url`: 向 `data.ref` 字段的 URL 发起 `HTTP GET` 请求。以下查询参数附加到 URL：`<name>=<val>`、`uid=<当前用户ID>`、`topic=<话题名>`、`seq=<消息序列ID>`。
  * `note`: 向当前话题发送 `{note}` 消息，`what` 设为 `data`（暂未实现，需要请联系我们）。
  ```js
  { "what": "data", "data": { "mime": "text/x-drafty-fr", "val": { "seq": 3, "resp": { "confirmation": "some-value" } } }
  ```
* `name`: 按钮的可选名称，将回传给服务器。
* `val`: 额外的不透明数据。
* `ref`: `url` 动作的 URL。

如果提供了 `name` 但没有 `val`，`val` 假定为 `1`。如果 `name` 未定义，则 `name` 和 `val` 都不发送。

上例按钮将发送 HTTP GET 到 https://www.example.com/path/?foo=bar&confirmation=some-value&uid=usrFsk73jYRR&topic=grpnG99YhENiQU&seq=3（假设当前用户 ID 为 `usrFsk73jYRR`，话题为 `grpnG99YhENiQU`，含按钮消息的序列 ID 为 `3`）。

_重要安全考虑_：客户端应将 `ref` 字段的 URL 方案限制为仅 `http` 和 `https`。


#### `EX`: 文件附件
`EX` 是客户端不应尝试解释的附件。`data` 包含以下字段：
```js
{
  "tp": "EX",
  "data": {
    "mime", "text/plain",
    "val", "Q3l0aG9uPT0w...PT00LjAuMAo=",
    "ref": "/v0/file/s/abcdef12345.txt",
    "name", "requirements.txt",
    "size": 1234
  }
}
```
* `mime`: 数据类型，如 'application/octet-stream'。
* `val`: 可选带内 base64 编码文件数据。
* `ref`: 可选带外文件数据引用。必须有 `val` 或 `ref`。
* `name`: 原始文件的可选名称。
* `size`: 文件大小（字节）的可选值。

生成将文件附件显示为可下载文件的消息，使用以下格式：
```js
{
  at: -1,
  len: 0,
  key: <EX 实体引用>
}
```

_重要安全考虑_：`ref` 字段可能包含恶意载荷。客户端应将 `ref` 字段的 URL 方案限制为仅 `http` 和 `https`。


#### `IM`: 内联图片或带内联预览的附加图片
`IM` 是图片。`data` 包含以下字段：
```js
{
  "tp": "IM",
  "data": {
    "mime": "image/png",
    "val": "Rt53jUU...iVBORw0KGgoA==",
    "ref": "/v0/file/s/abcdef12345.jpg",
    "width": 512,
    "height": 512,
    "name": "sample_image.png",
    "size": 123456
  }
}
```
 * `mime`: 数据类型，如 'image/jpeg'。
 * `val`: 可选带内图片数据：base64 编码的图片位。
 * `ref`: 可选带外图片数据引用。必须有 `val` 或 `ref`。
 * `width`, `height`: 图片的线性尺寸（像素）。
 * `name`: 原始文件的可选名称。
 * `size`: 文件大小（字节）的可选值。

创建只包含单个图片、无文本的消息，使用以下 Drafty：
```js
{
  txt: " ",
  fmt: [{len: 1}],
  ent: [{tp: "IM", data: {<你的图片数据>}}]
}
```

_重要安全考虑_：`val` 和 `ref` 字段可能包含恶意载荷。客户端应将 `ref` 字段的 URL 方案限制为仅 `http` 和 `https`。客户端仅当正确转换为图片时，才应向用户展示 `val` 字段内容。


#### `LN`: 链接（URL）
`LN` 是 URL。`data` 包含单个 `url` 字段：
`{ "tp": "LN", "data": { "url": "https://www.example.com/abc#fragment" } }`
`url` 可以是客户端知道如何解释的任何有效 URL，例如也可以是邮件或电话 URL：`email:alice@example.com` 或 `tel:+17025550001`。

_重要安全考虑_：`url` 字段可能是恶意构造的。客户端应禁用某些 URL 方案，如 `javascript:` 和 `data:`。


#### `MN`: 提及，如 [@alice](#)
提及 `data` 包含单个 `val` 字段，值为被提及用户的 ID：
```js
{ "tp":"MN", "data":{ "val":"usrFsk73jYRR" } }
```


#### `HT`: 话题标签，如 [#tinode](#)
话题标签 `data` 包含单个 `val` 字段，值为客户端软件需要解释的话题标签值，例如可以是搜索词：
```js
{ "tp":"HT", "data":{ "val":"tinode" } }
```

#### `VC`: 视频通话控制消息
视频通话 `data` 包含通话当前状态和时长：
```js
{
  "tp": "VC",
  "data": {
    "duration": 10000,
    "state": "disconnected",
    "incoming": false,
    "aonly": true
  }
}
```

* `duration`: 通话时长（毫秒）。
* `state`: 当前通话状态；支持的状态：
	* `accepted`: 通话已建立（进行中）。
	* `busy`: 通话无法建立，因为被叫方正忙于另一通电话。
	* `finished`: 之前建立的通话已成功结束。
	* `disconnected`: 通话断开，例如因错误。
	* `missed`: 未接来电，即被叫方未接电话。
	* `declined`: 拒接来电，即被叫方在接听前挂断。
* `incoming`: 如果是来电则为 true，否则是去电。
* `aonly`: 如果是纯音频通话（无视频）则为 true。

`VC` 也可表示为格式 `"fmt": [{"len": 1, "tp": "VC"}]`，无实体。这种情况下，所有通话信息包含在消息的 `head` 字段中。

#### `VD`: 带预览的视频
`VD` 表示视频录像。`data` 包含以下字段：
```js
{
  "tp": "VD",
  "data": {
    "mime": "video/webm",
    "ref": "/v0/file/s/abcdef12345.webm",
    "preview": "AsTrsU...k86n00Ggo=="
    "preref": "/v0/file/s/abcdef54321.jpeg",
    "premime": "image/jpeg",
    "width": 640,
    "height": 360,
    "duration": 32000,
    "name": " bigbuckbunny.webm",
    "size": 1234567
  }
}
```
 * `mime`: 视频数据类型，如 'video/webm'。
 * `val`: 可选带内视频数据：base64 编码的视频位，通常不存在（null）。
 * `ref`: 可选带外视频数据引用。必须有 `val` 或 `ref`。
 * `preview`: 可选 base64 编码的视频截图图像（海报）。
 * `preref`: 可选带外视频截图图像（海报）引用。
 * `premime`: 可选截图图像（海报）的数据类型；缺失时假定为 'image/jpeg'。
 * `width`, `height`: 视频和海报的线性尺寸（像素）。
 * `duration`: 视频时长（毫秒）。
 * `name`: 原始文件的可选名称。
 * `size`: 文件大小（字节）的可选值。

创建只包含单个视频、无文本的消息，使用以下 Drafty：
```js
{
  txt: " ",
  fmt: [{len: 1}],
  ent: [{tp: "VD", data: {<你的视频数据>}}]
}
```

_重要安全考虑_：`val`、`ref`、`preview` 字段可能包含恶意载荷。客户端应将 `ref` 和 `preview` 字段的 URL 方案限制为仅 `http` 和 `https`。客户端仅当正确转换为视频时，才应向用户展示 `val` 字段内容。