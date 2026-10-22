# UI Bridge v1

## 加载方式

宿主为每次打开配置页创建短时 UI 会话：

```text
/api/v1/plugin-ui/<asset-token>/index.html#bridge_token=<bridge-token>
```

资源 Token 用于读取包内 `ui/` 文件，Bridge Token 只存在于 URL fragment，不会发送到服务器。iframe 使用 `sandbox="allow-scripts"`，不授予 `allow-same-origin`。

UI 只能加载包内、已在清单声明的资源。CSP 禁止外部网络连接、表单提交和外部 frame。

### 文档隔离传输

宿主在校验包内 HTML 后，在其脚本之前注入 v1 传输适配器。现有 UI 仍使用
`parent.postMessage` 发送下方信封、通过 `window` 的 `message` 事件接收结果，无须改写插件包。
适配器为当前文档创建 `MessageChannel`，通过带会话 Token 的
`sub2api.plugin.connect` 消息把一个端口交给宿主。

宿主收到普通窗口请求后，只通过该文档端口中继原请求；适配器通过同一端口确认请求，
宿主再执行 API 操作。异步结果仅发回该端口，适配器在当前文档内派发兼容的消息事件，
保留 `event.source === parent`、宿主 `event.origin` 和原信封字段。离开页面时通知宿主作废待处理请求并关闭端口，尚未完成的二次验证不会继续提交旧页面操作。
宿主不会通过可导航的 `WindowProxy.postMessage` 返回配置、错误详情或 Bridge Token。

同一会话只接收第一个文档端口；收到页面离开的通知时立即重新创建会话并更换 Token，
即使最初页面的 load 尚未完成也会恢复。再次 load 也会使旧会话失效。
未建立文档端口的请求不会执行配置 API。前后端须一起更新，避免使用未注入适配器的旧后端。
HTML 与 XHTML 入口均注入适配器；XHTML 保留 XML 声明、命名空间及 CDATA 脚本语法。

## 消息信封

UI 到宿主：

```json
{
  "source": "sub2api-plugin-ui",
  "bridge_token": "TOKEN",
  "type": "config.load",
  "request_id": "UNIQUE_ID"
}
```

宿主到 UI：

```json
{
  "source": "sub2api-plugin-host",
  "bridge_token": "TOKEN",
  "request_id": "UNIQUE_ID",
  "ok": true
}
```

## 方法

| `type` | UI 参数 | 成功响应 |
|---|---|---|
| `sub2api.plugin.ready` | 无 | 无响应 |
| `config.load` | 无 | `config` |
| `config.save` | `config` 对象 | 规范化后的 `config` |
| `config.test` | 无 | `result` |
| `ui.resize` | `height` | 无响应 |
| `ui.notify` | `level`、`message` | 无响应 |

`config.test` 在 v1 中测试已保存配置。UI 若要测试当前表单，应先调用 `config.save`。

## 必须执行的校验

UI 接收消息时必须验证 `event.source === parent`、消息来源标识、Bridge Token 和等待中的 `request_id`。每个请求必须有超时和卸载清理。

适配器派发的是当前文档内的兼容事件，`event.isTrusted` 为 `false`；不要依赖该属性来验证宿主。

宿主不会向 iframe 提供管理员 Token。插件 UI 不得尝试访问管理 API、Cookie、父页面 DOM 或浏览器存储中的宿主数据。
