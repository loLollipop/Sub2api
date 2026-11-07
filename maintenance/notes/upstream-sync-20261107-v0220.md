# 原版 v0.2.20 选择性同步（2026-11-07）

## 来源与边界

- 本地起点：个人 v2.1.57，`3ce39e8730f4c3ff383f099f648be106f5411a4a`。
- 原版上次正式检查点：v0.2.19，`dece26153621959ccd7635814f4b2d08304e681a`；另已审 main `a3298b46ebe239fb0720b068a1eb3383955bdac2`。
- 本次正式版：[v0.2.20](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.20)，2026-11-07T06:31:24Z 发布，解引用提交 `dab829a4e9b7d1cc307189aea8b0319eb29e29d0`，annotated tag 对象 `ec41309811cd30af310a0e4b9119b976ce65c9be`。
- 观察 main `6b4b37db43443b5e88e424fe3c69602cc3810dfb` 只比 tag 多 VERSION 更新，无新增业务；不导入未发布 main 代码。
- 相对上次正式版为 112 commits / 459 paths；相对另已审 main 为 106 commits / 457 paths。检查点表示已审查和选择性适配，不能解释为完整合并。

保留本地 native provider、凭据目的地约束、权限、真实终态用量有界排空、禁止重复计费重试、0.15 倍率、冻结价格与显式零价。保留上一轮排行榜配色改动和三个已有未跟踪 Go 测试。

## 选择项

以下 17 组修复已按本地调用链适配，并完成相关回归验证。

| 来源提交 | 适配范围 |
| --- | --- |
| `a0b058a9e` | Bedrock EventStream 分配和切片前校验总长、header/payload 上限。 |
| `7e3b6c72f` | 按已封禁前缀的实际长度查询 cyber transcript，修复长会话误拦截，同时保留命名空间、scope/TTL 和追加防逃逸。 |
| `d446c4a6b` | Gemini native/Claude/Chat/collector 的有界 SSE 读取，及 Antigravity 非流式响应体上限；只提取必要依赖。 |
| `0b535b5c0` | Antigravity 的 responseJsonSchema/response_json_schema 规范化为 responseSchema，并清理 schema；显式 responseSchema 优先。 |
| `aae31c743` | Gemini 3+ 工具媒体绑定到对应 functionResponse.parts，Gemini 2 保留旧表达，按最终上游模型判断。 |
| `210608151` | 精确识别 Cloudflare REST Messages 并省略 beta 查询参数，统一实际转发和账号测试。 |
| `f10a45c03` | 非 UTF-8 密钥查库前拒绝，认证查找时的客户端断连按 499 处理，内部错误保持安全出口。 |
| `22693fb4f` | Grok Voice 请求体读取错误分类，隐藏 socket 细节，保留空请求和大小限制。 |
| `2ce1849bd` | 图片模型 INTERNAL 500 仅冷却该模型，保留原有文本账号惩罚策略。全账号档位配置不随此修复增加。 |
| `c35418032` | 明确区分 direct Gemini JSON 与 V1InternalResponse envelope，避免直传响应被成功解码为空内容。 |
| `2d6b64a18` | completed 消息中的 output_text 复用既有白名单序列化器，补齐 annotations/logprobs 必需数组与空 text，继续保留 refusal；与流式消息结构一致。 |
| `522f1fb99` | Grok Responses 的 agent_message 复用现有正文提取函数，规范化为 user 输入。 |
| `e16e69a7c` | WebSocket 每轮 TTFT 在帧归属确定后记录，覆盖后续 turn 无 response.id 的 token。 |
| `40d9c0964` | WS HTTP bridge 复用已有 Codex fingerprint 收敛，保持 API key/会话隔离。 |
| `c476edbf4` | 强制 Responses/Chat 路由时跳过无效 Responses 探测。 |
| `a8d90a72e` | setup-token 即使没有 5h header，也采样并展示真实 7d 用量。 |
| `fa6f06510` | 内置 body-limit 413 提前返回前先应用现有错误透传规则，保留本地错误隐私与 failover。 |

## 本地适配的额外边界

- 新写入的 cyber exact key、前缀长度索引与 scope 由同一 Lua 操作发布，重写使用较长的有效期。旧 scope 保留 legacy 标记；按真实前缀分批查询旧 block，不能以历史长度直接封禁，也不能让新索引掩盖旧记录。
- Antigravity 图片 INTERNAL 500 的隔离依据实际已构造请求的 model，包括 web_search 转为文字模型的路径，避免冷却客户端别名或错误模型。
- Gemini 新增 SSE 超限错误保留此前已观测 token 和已交付图片的部分结果，不触发重放；首行超限在未提交响应时写 Google 502，并显式设置 JSON Content-Type。嵌套读取器的 ErrTooLong 进入相同错误出口，并保留原错误链。普通非超限读取错误维持既有语义。字节上限复用现有 MaxLineSize，每事件最多 4096 行。
- Responses 仅补齐既有类型支持的必需线上字段和空数组，不扩大任意字段透传。

## 已有覆盖及其他小修复

Kimi 并发 403 短冷却、OAuth 非流式 Responses 的 SSE/终态识别、buffered reasoning_text、GPT Image 2.6 核心链路、RPM override 认证快照以及本次漏洞依赖版本已由本地覆盖，不重复导入。额外核对的十个小修复中，八项纳入上表，另两项按下述依赖/策略边界延期。

## 继续延期或排除

- 管理 MCP、OAuth consent、源迁移 249、MCP go-sdk、全权管理员密钥兼容及整套权限扩展。
- Nuxt SSR、Hosted、已移除支付、Ops/capture/timers、用户撤回的 journal 与财务恢复框架。
- GPT-6.1 Terra 能力/报价/长上下文收费、catalog/供应商/别名放宽、Kimi 推断参数覆盖、Codex 导出默认模型。
- 断连后截断或估算收费、95% cache 收费、替换本地真实终态排空语义。
- `457602c8f` 的通用请求体内存预算、读写超时、session cache、通知/配额后台 worker 等大型资源架构，不作为有界 SSE 修复的附带导入。
- 注册邮箱黑名单、图片尺寸档位调度、Antigravity 配额组与连接测试 UI、可配置全账号 INTERNAL 500 惩罚档位等产品能力。
- 上游整套界面、Sponsors、VERSION、发行/部署策略和源历史。
- `6b565e45a` 的图片尺寸溢出粘性依赖未引入的尺寸档位调度；本地一般容量溢出已覆盖，不摘孤立尺寸分支。
- `a699c2bae` 的空 instructions 和响应回显组会改变本地 Codex 非空校验、默认注入及多协议回显策略；保持既有行为，留待整组专项适配，不只摘回显 helper。

## 验证与状态

本地选择性同步完成；原版正式检查点已推进至 v0.2.20。用户随后授权审查通过后提交 GitHub 并更新部署，准备个人 v2.1.58；最终提交、CI、制品身份、备份恢复与线上验收另存发布证据。

- 复用输入未变的有效结果并去重，8 个包共 **217 个顶层测试、869 个测试及子测试结果通过**。覆盖原生厂商路由、真实终态/断连排空、Grok 拒绝计费、错误隐私、cyber 隔离，以及本轮协议与读取边界。
- `final-contract-tests.jsonl`：4 包，194 个顶层／777 个含子测试通过。最终嵌套读取器修复后，`final-gemini-tests.jsonl`：2 包，30 个顶层／107 个含子测试通过。其他输入未变的小包结果直接复用，不把中间失败算成通过。
- 内层读取器上限低于外层时的首行超限、已有 usage、已交付图片三种情况均取得修复前失败、修复后通过的证据。独立审查的 4 项发现已修复，9 组高风险路径复核无剩余具体阻断。发布前补审发现首行 JSON 错误残留 SSE Content-Type，已以 1 行出口修复及两处响应头断言闭环；5 个顶层／11 个含子测试红→绿验证通过。
- Go 1.27.3 的 Windows 后端 embed 构建通过，复用既有前端 dist。命令（仓库根目录）：`go -C backend build -p 1 -tags embed -o ../.artifacts/original-v0220-sync-20261107/sub2api.exe ./cmd/server`。
- 68 个本轮后端 Go 文件的 gofmt 检查及 `git diff --check` 通过；17 个来源补丁摘要与正式版归属检查通过。1,678 个受保护文件及三个原有未跟踪测试哈希不变，迁移、Ent、依赖、第二上游锁文件和既有配色未被覆盖。

本地 Redis 行为使用 miniredis 验证，未连接生产数据库/Redis或开展真实供应商及付费请求联调；未跑 race（本机无 C 编译器）。三份已有 `billing_audit_deferred` 测试保持原样且未启用，该延期计费策略不属于本轮验收范围。

本地来源补丁、计划、中间失败与最终通过日志保存于 `.artifacts/original-v0220-sync-20261107/`，最终汇总为 `backend-verification-summary.json`、`final-backend-source.json`、`protection-check.json` 及 `security-audit.md`。原有配色的有效浏览器、类型和构建证据保存在 `.artifacts/leaderboard-palette-20261107/`；后端适配不改变其输入，直接复用。
