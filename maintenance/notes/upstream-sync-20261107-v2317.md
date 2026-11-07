# 二改正式 v2.3.16–v2.3.17 选择性同步（2026-11-07）

## 结果与来源

本轮选择性适配注册验证码防枚举、管理端在途列表上限、Grok TTS 文本字段、Images HTTP 200 错误体和 CC Switch v4.0.26。API Key 额度预占与模型元数据作为完整能力延期，原因见下文。正式来源检查点推进到 v2.3.17。同步完成后，用户授权提交 GitHub 并更新部署，按个人 v2.1.55 的 PR、CI、自有 Release/GHCR 和既有备份恢复校验流程发布；最终制品身份与线上验收另存发布证据。

| 正式版本 | 发布时间（UTC） | 标签解引用提交 |
| --- | --- | --- |
| [v2.3.16](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.16) | 2026-11-06T18:04:34Z | `64f195d6f447d2dd3acda18227cac04919cce608` |
| [v2.3.17](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.17) | 2026-11-07T03:20:56Z | `ad1a9ea8b446b9a73613d58e7f568aed9f55816b` |

两者均为正式非草稿、非预发布 Release。旧正式检查点为 v2.3.15（`a17cc8118ffdd1dc5931a3462aabf30685994630`）；本次 main 与 v2.3.17 解引用提交相同。共审查 44 个提交，差异涉及 138 个路径，14,823 行增加、863 行删除；无依赖清单和数据库迁移变更。原版仓库检查点、仅监控正式发布的策略、部署配置保持原决策。Release JSON、原始差异与同步前保护文件摘要存于本地 `.artifacts/upstream-v2317-20261107/`。

## 采用与本地适配

| 源提交 | 采用内容与契约 |
| --- | --- |
| `e03a860de477420bb5534b4523290d99b9a4f155` | 注册验证码发送遇到已注册邮箱或别名时返回同正常发送的 HTTP 200、消息与 60 秒倒计时，不入邮件队列；保留人机验证和其他失败响应。 |
| `04d010f3cf0c1b765f9b60d7bb3c655e97bf2559` | 仅采用管理员使用记录第一页最多补充 50 条在途记录；先筛选、去重、按时间排序，再限制条数。在途专用列表继续完整分页。该提交中的 Solana 行为不适用。 |
| `a12763f02de64d9404fdfdd7e03be671869d17d8` | Grok TTS 按 `text > input > prompt` 取首个非空字符串，内容审核、在途估算、实际结算共用同一提取器；中文及 emoji 按 Unicode 字符计量。保留无可识别字段时原有结算回退。 |
| `c064c3cb6b7df9c84698daba0bff2495dce2594c` | 识别 Images 的 HTTP 200 错误对象、可识别错误文本，返回对应 typed error 和错误状态，不返回成功 ForwardResult，不按默认一张图计费；尺寸/帧数组合错误返回 400，正常图片优先通过；修正嵌套 `response.failed` 的尺寸错误分类，避免误记账号健康失败。保留财务错误先行判定；共享客户端错误出口脱敏，内部诊断保留；错误消息截断保持 UTF-8 完整。 |
| `f85fa7707a63d8a0fbfa924fba4339ab1e66dc33` | 新增 MiniMax Code、Kimi Code、DeepSeek Harness、Antigravity、Oh My Pi，开放 Claude Desktop/Pi provider deeplink。保留本地原生平台、独立 Antigravity endpoint 和模型发现。依据实际 CC Switch v4.0.26 parser 补齐 Pi 系列模型必填校验及界面提示。 |

## API Key 额度预占延期依据

`57ae74f4e66197f101211cd6c6bc0b35748defec` 的目标适用于本地：当前鉴权快照和数据库原子累加并不等价于按 API Key 的并发准入预占。但源实现截至本次正式标签仍存在以下实际问题，因此本轮不导入部分门禁，也不宣称现有 Key 额度已成为严格的并发费用上限。

- Redis 镜像先读 DB 再 `HSETNX` 播种，结算则在 DB 提交后另行累加镜像，没有一致性边界。镜像过期时，可能先播种已包含消费的 DB 值、再重复加一次消费；也可能 DB 已提交但增量因镜像缺失被跳过，随后播种旧值。镜像 TTL 为十分钟。依据：源 `billing_api_key_quota.go:276`、`billing_api_key_quota_cache.go:75–89`、`gateway_usage_billing.go:448`。暖镜像更新实际费用与删除在途预占也不是同一原子操作。
- 持久化 media 结算构造的 APIKey 不带 Quota/APIKeyService，无法进入源镜像更新条件；视频提交 handler 返回时释放新增预占，未交给持久化 job，余额 hold 仍是另一条生命周期。依据：源 `media_billing.go:198–206`、本地 `media_billing_job_repo.go:532`、`media_billing.go:327–330`。
- 两套 Responses WebSocket URL 仅检查准入资格，未接入逐 turn 预占；本地切组重定价、批量图片也需要完整适配，不能只修改公共 HTTP helper。
- 图片估算取所有尺寸最高价，可能把剩余 0.10、实际 1K 费用 0.02 的请求按 4K 费用 0.30 拒绝。已知尺寸应参与估算。估算窗口也不是实际消费上界。
- Lua 浮点直接比较 `used + inflight + estimate > quota`，如 `0.1 + 0.2 > 0.3` 会误拒；应与本地 `NUMERIC(20,8)` 金额口径一致。

后续若采用，应先建立播种/结算一致性方案，覆盖持久化 media 与恢复、WS 每 turn、切组倍率重定价、已知尺寸、精度边界、续期及失败释放；保留现有真实用量结算，不用拒绝已发生费用的数据库扣款代替准入控制。

## 模型元数据延期依据

`7437f686dd5ebf3162cec2b08df778eac66adcf9` 新增 `/v1/models` 描述、能力、models.dev 元数据和价格。暂缓整组功能，避免引入不能被现有授权与计费契约可靠消费的部分基础设施。

- 源价格直接取 group/channel 原始价卡，未使用本地基础价补齐、区间价、分时价、用户/分组倍率和最终计费模型；智能路由按多个 group 合并模型，却只用主 group 查价。同一模型的响应价格可能与实际结算不同。本地 `/v1/models` 明确不承担计费报价，模型广场已有同源计费规则及倍率展示。依据：源 `model_metadata_handler.go:45–74`，本地 `model_pricing_resolver.go:68–143`、`gateway_usage_billing.go:822–857`、`gateway_handler.go:1281–1287`。
- models.dev 解析只读标量 cost，会丢失 `tiers/context_over_200k` 等阶梯。它的外部供应商参考价也不能作为本地有效价。
- 源 lookup 接收 platform 但不使用，把真实 ID 与 canonical alias 放入同层索引；同供应商的 `kimi-k2.7-code` 与 `kimi-k2.7-code-highspeed` 可能因 map 遍历顺序被随机选中。同名跨供应商模型也可能借用不同的限制或价格。依据：源 `model_metadata_catalog.go:504–573,715–831`。
- 通用 vendor stripping、normalized/family/prefix 回退可能把不同 SKU 或自定义别名误当同一模型；能力值用非指针 bool 会把 unknown 写成 false。
- 直接搬 gateway hunk 会将本地 authoritative empty upstream catalog 回退为静态模型；pinned OpenAI 与 Codex 清单尚未增强，未来增强必须对最终内容重新计算 ETag，retrieve 仍需忽略 collection conditional。

后续可独立采用描述字段，但须挂在现有授权、发现和 allowlist 完成后的条目上，只填字段、不增加模型，使用 provider-scoped exact 匹配，真实 ID 优先于 alias，有歧义则省略，能力保持可选。价格和 pinned/Codex ETag 应分别完成适配后再开放。

## 继续排除与保留

- `344c1d697` 至 `7ebf8e716` 等断连策略整组仍排除：不采用立即终止上游、无终态估算用量和 95% 缓存收费。保留 bounded drain 和真实终态用量。
- 原生 crypto/payment 已退役，不恢复 `04d010f3c` 的支付部分、`08eeeeaf3`、`96d6c2515`、`9b758b0c7` 等订单/归集/RPC 变化。
- `97e38826d` 修复的是本地不存在的 `ChannelsView.vue` 隐藏 required 字段；不为此引入来源专属表单模块。来源 docs 路由与 VERSION bookkeeping 不覆盖本地入口和个人版本。
- 不采用 `c064c3cb6` 对所有非官方 base URL 一律假定方形拼图及 3840 单边上限的预拒。已知中转站的实现不代表所有自定义上游；保留本地帧数、像素、内存预算和透明合成边界，真实上游尺寸错误通过本轮错误识别返回。
- 保留本地主题、紧凑时间范围、模型后独立思考强度列、原生平台、权限、模型发现和计费倍率；三份同步前未跟踪 Go 测试保持原内容。

## 验证

- 后端针对 service、handler、admin handler 的图片、Grok、验证码、在途列表、客户端脱敏、财务错误、账号健康与来源检查点回归，最终 998 个顶层测试、2,140 个测试及子测试通过，无跳过。命令：`go test -p 2 -tags=unit -json ./internal/service ./internal/handler ./internal/handler/admin -run 'Test.*(Images|Grok|SendVerifyCode|AdminUsageInflight|ClientErrorPrivacy|UpstreamFinancial|Update(Source|Tracking|Service)|OpenAIAPIKeyHealth)'`。
- 首次相邻回归发现已有隐私测试仍要求 HTTP 200 错误图片响应成功返回；按新行为更新为 typed error、502、零图片/用量，保留原始数据及脱敏断言。单独复核 `TestClientErrorPrivacyImageAndMediaJSONWrites` 的全部用例通过，其余未改变输入的通过结果复用。原始事件分别为 `backend-regression.jsonl`、`backend-privacy-recheck.jsonl`；汇总为 `verification-summary.json`，均在本轮 `.artifacts` 目录。
- `go vet -p 2 -tags=unit ./internal/service ./internal/handler ./internal/handler/admin` 通过；`CGO_ENABLED=0 go build -p 2 -tags embed ./cmd/server` 通过，验证二进制输出到本轮 `.artifacts`，未启动服务。
- 前端 pnpm 9.15.5 定向 Vitest 72/72（utility 35、KeysView 34、locale 3）、六文件增量 ESLint、typecheck 全部通过。完整生产构建（含 locale 完整性、类型检查和控制台静态资源检查）通过；保留 Vite 关于既有静态/动态混合导入的提示。
- Playwright + Edge 156 实际操作七类新增支持应用，核对模型、endpoint 和生成的 deeplink；清空 Pi 系列模型后禁止导入，重新选择后恢复。使用本地合成数据，拦截 `window.open`，无外部应用启动、页面错误为零。已查看正常弹窗与模型必填提示截图。
- 修改的 Go 文件格式检查、`git diff --check` 通过；来源锁文件和正式标签一致，历史延期项保持原内容。同步前三份未跟踪 Go 测试逐项 SHA-256 比对一致，未纳入本轮改动。

以上为本轮及相邻关键路径的本地定向验证，未在本地重跑全仓测试，未调用真实模型或发送注册邮件。本次完整发布的最终 CI、制品身份、数据库备份与隔离恢复、受控切换及线上验收另存发布证据。
