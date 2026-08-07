# theCard: 人物/话题描述格式

Tinode 使用 `theCard` 存储和传输人物和话题的描述。该格式概念上类似于 [vCard](https://www.rfc-editor.org/rfc/rfc6350.txt) 3.0。

当使用 `JSON` 表示 `theCard` 数据时，其方式与 [jCard](https://tools.ietf.org/html/rfc7095) 不同。`theCard` 和 `jCard` 不兼容。主要区别是 `theCard` 使用对象表示逻辑相关数据，而 `jCard` 使用有序数组。

`theCard` 结构为对象：

```js
{
  fn: "John Doe", // 字符串，人物或话题的格式化名称。
  photo: { // 对象，头像照片；必须有 'data' 或 'ref'，其他字段可选。
    type: "jpeg", // 字符串，MIME 类型但去掉 'image/'。
    data: "Rt53jUU...iVBORw0KGgoA==", // 字符串，base64 编码的二进制图片数据
    ref: "https://api.tinode.co/file/s/abcdef12345.jpg", // 字符串，图片 URL。
    width: 512, // 整数，图片宽度（像素）。
    height: 512, // 整数，图片高度（像素）。
    size: 123456 // 整数，图片大小（字节）。
  },
  note: "Some notes", // 字符串，人物或话题的描述。
  //
  // 以下字段目前没有任何已知客户端实现：
  //
  n: { // 对象，人物的结构化名称。
    surname: "Miner", // 姓氏或姓或家族名。
    given: "Coal", // 名字或名。
    additional: "Diamond", // 额外名称，如中间名或父名。
    prefix: "Dr.", // 前缀，如荣誉头衔或性别标识。
    suffix: "Jr.", // 后缀，如 'Jr' 或 'II'。
  },
  org: { // 对象，人物或话题所属的组织。
    fn: "Most Evil Corp", // 字符串，组织的格式化名称。
    title: "CEO", // 字符串，人物在组织中的职位。
  },
  comm: [ // 对象数组，定义与人物或话题的通信方式。
    {
      des: ["home", "voice"], // 联系标识，可选。
      proto: "tel", // 通信协议，必需
      value: "+17025551234" // 电话号码。
    },
    {
      des: ["work"],
      proto: "email",
      value: "alice@example.com", // 电子邮件地址
    },
    {
      des: ["other"],
      proto: "tinode",
      value: "tinode:topic/usrRkDVe0PYDOo", // tinode ID URI，可包含服务器地址。
    },
    {
      proto: "http", // 应用于 http 或 https 网站地址。
      value: "https://tinode.co", // 网站实际地址。
    }, ...
  ],
  bday: { // 对象，人物生日。
    y: 1970, // 整数，年
    m: 1, // 整数，月 1..12
    d: 15 // 整数，日 1..31
  },
}
```

所有字段都是可选的。Tinode 客户端目前只使用 `fn`、`photo`、`org`、`note`、`comm` 字段。如果将来需要其他字段，将从相应的 [vCard](https://www.rfc-editor.org/rfc/rfc6350.txt) 字段采用。