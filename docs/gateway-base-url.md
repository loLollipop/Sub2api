# 网关 Base URL

建议 OpenAI 兼容客户端填写 `https://你的域名/v1`，Anthropic SDK / Claude Code 填写 `https://你的域名`。Gemini 原生接口沿用 `/v1beta`，Codex 直连入口沿用 `/backend-api/codex`。

服务端会在路由匹配前兼容常见的漏写、重复版本段，但改写结果必须命中当前已注册的网关路由且 HTTP 方法一致。例如：

| 客户端请求 | 实际处理路径 |
| --- | --- |
| `/messages` | `/v1/messages` |
| `/chat/completions` | `/v1/chat/completions` |
| `/v1/v1/responses` | `/v1/responses` |
| `/antigravity/v1/v1/messages` | `/antigravity/v1/messages` |
| `/backend-api/codex/v1/responses` | `/backend-api/codex/responses` |

改写保留查询参数、请求头、请求体和 HTTP 方法，继续使用规范路径的鉴权、限流、分组选择和计费逻辑。访问日志同时记录规范路径和原始路径。

`/v1beta` 不会变成 `/v1`。带前缀的漏写版本段只有在原路径未命中、补齐后只有一个匹配时才补齐；存在歧义时保留原请求。超过 16 个版本段的路径不处理。管理 API、静态资源、健康检查和浏览器的 `/usage` 页面不参与网关别名。

重复版本段折叠后仍不存在的 `/v1/v1/*` 请求，在 Gin 的既有全局中间件之后返回 `404 gateway_path_not_found`，响应提供原始路径、折叠结果和 Base URL 提示。其他未知路径沿用原来的 404 行为。
