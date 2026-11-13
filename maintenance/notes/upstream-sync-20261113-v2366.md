# 二改 v2.3.66 选择性同步（2026-11-13）

同步阶段个人 `backend/cmd/server/VERSION` 保持 **2.1.63**，仅在本地适配。用户随后授权审查修复并提交 GitHub、更新部署，发布使用个人 **2.1.64**；最终 CI、制品和部署验收单独留证。

## 来源与范围

| 项目 | 检查点 |
| --- | --- |
| 二改仓库 | `kiss-kedaya/sub2api` |
| 原正式检查点 | v2.3.58 / `28ff1b3d41dddb2959892e66f4b410b064a266d4` |
| 本次正式目标 | [v2.3.66](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.66) / `00e1d64b7e5a12375507f8a31958297d7f6f99b8` |
| 正式发布时间 | `2026-11-13T06:41:10Z` |
| 观察到的 main | `524f1af7cd31a30398fc6ebcef1076ca610e6921`，仅观察，不导入未发布内容 |
| 增量 | 60 个提交（含 merge），46 个非 merge 提交，104 个变化路径 |
| 原版检查点 | v0.2.21 / `10ba8f9b4dc2e6023d4a9879ab77b21841babd10`，保持原值 |
| 本地分支 | `sync/fork-20261113-v2366`，起点 `4ed7e79b020be5683db2608c9e03a53608b7cf45` |

以独立 remote release ref 获取正式源码，没有覆盖个人 tag 或整树合并。来源 API、完整 diff、提交和路径清单保存在忽略目录 `.artifacts/fork-sync-20261113/`。`classify-paths.py` 和 `path-review.json` 对全部 104 路径逐项记录：20 项适配、3 项由本地调用链覆盖、30 项延期、51 项排除。适配表示行为选择性回移；源文件新增位置、函数名和本地写入位置可以不同。

## 已适配及本地覆盖

| 来源 | 本地结果与保留的契约 |
| --- | --- |
| `f337251f8`、`621bfa1a2`：版本化 URL 拼接 | 引入 `urljoin.APIPath`，地址已带 `/v1`、完整接口或前缀时不再重复拼接；保留 query，丢弃不会发送到上游的 fragment，以 escaped path 匹配边界，不把 `%2Fv1` 或 `v10` 当作版本段。适配现有 Anthropic Messages/count_tokens/models、自定义 relay、Antigravity、通道监控及安全审计调用。 |
| 同组源调用方的差异 | 复用本地 Messages helper，严格保留 Cloudflare account REST 的隐式 beta 例外和显式 beta 值；原 Antigravity 上游继续不加 beta。账号测试已使用本地 helper，无需重复替换。OpenAI 原生目标及模型列表保留本地多厂商 v2/v3/v4 规则，Gemini 保留版本剥离契约，不把源版通用 `/v1` 规则覆盖进去。既有 URL allowlist、SSRF 校验、凭据目的地和 header 规则保持。 |
| `5d52ece17`：流式 input 占位零 | 在 passthrough、general Messages、转换 chat/responses 和 `AnthropicUsage.Merge` 四个合并点，裸 `message_delta.usage.input_tokens:0` 保留先前报告的 input，继续接受 output。源版只检查正数 cache，本地改为 canonical/TTL cache 字段明确存在时也接受 input 零，包括 cache 明确全零；保护已有真实终态全零清理和 aggregate 零优先于 TTL 明细的契约。 |
| 同组缓存别名兼容 | 正数 `cached_tokens`、`prompt_tokens_details.cached_tokens`、`prompt_cache_hit_tokens` 同样构成真实 cache split。将流式别名规范到 canonical cache-read 桶，保留 canonical 非零优先及既有别名优先次序；没有 prompt 总量时，不把单独报告的正数 input 重算成零。补齐 alias-only delta 的返回 usage 与计费结果一致性；非流式 normalize 不改，不启用估算或中断收费。 |
| `a265dfd31`：公共设置失败退避 | 请求失败后普通调用 30 秒内不再重试；到期可重试，force 立即绕过退避。保留并发请求合并、已成功缓存和服务端注入配置；成功和 clear cache 重置退避，同步抛错与异步失败使用相同规则。后台保存后的 force 行为保持。 |

不修改个人收费价格、缓存 TTL 计费、上游声明倍率、生产账号、前端布局或冷灰蓝主题。

## 延期与排除

- **公共设置 HTTP/CDN 缓存延期。** `d455a8fb2` 的 `public,max-age=30` 会缓存注册、验证码、Passkey、OAuth、协议与隐私 gate。后台 `fetchPublicSettings(true)` 仍使用同一个 URL，不能证明穿透 HTTP/CDN 新鲜缓存。只采用独立的前端失败退避，不引入该响应头或对应测试预期。
- **监控固定 30% cache critical 规则延期。** `16b1ff835`、`e7181c7c6` 会把固定阈值叠加到本地可配置评分。现有 warning/critical cache 阈值默认 0，不惩罚 cache misses；有配置时按配置评分，保留操作者选择。Go 已为 1.27.4，不回退到源版 1.26.11。
- **受管代理/Mihomo 整组延期。** `90f90021c` 的静态节点导入、名称修正，以及源 proxy password schema/migration313 扩容服务于本地未引入的受管凭据链；不添加 ent、SQL、模块或闲置 helper。
- **IP 自动风控继续延期。** observe-only 默认、停 Key opt-in、共享 IP 峰值用户/累计 Key、Cloudflare ranges/policy 与配套 API/UI/fixture 更新均属于延期的自动探测/访问控制模块，不单独引入孤立字段或测试。
- **上游类型/余额链及图标延期。** 本地没有 `UpstreamKindBadge`/`UpstreamBalanceCell`；local-path glyph 修复无现有调用，不为图标重新引入余额/类型探测。既有手动开启的 Sub2API 声明倍率探测不变。
- **退役原生支付整组排除。** 包括资产市场价格、充值/USDT 导航、txlist native polling、gas median/floor、同 nonce 替换、finality/canonical receipt、stuck sweep 自动恢复及对应 mock、API、测试和页面。
- 源 VERSION/发行说明不覆盖个人历史；此前延期的仓库统计缓存、Auth Quiet/Nuxt/Dense、定时器、自动模型同步/catalogue、新 TTL 收费及已退役 Hosted/Ops/capture 等继续保持原决定。

## 验证

前端 34 项 store 测试、3 项 i18n 完整性测试、定向 ESLint、`vue-tsc -b`、Vite 生产构建及生产 CSS 资源路径检查通过。新增用例覆盖 29999/30000ms 边界、force/并发、同步异常、缓存恢复和注入配置。初次 build script 的嵌套 `pnpm` 命中了环境中的另一套包管理器并中止；改用 Corepack 固定项目 pnpm 9.15.5 分步执行相同四项检查后通过，没有变更依赖或全局配置。

后端 `urljoin`、`apicompat`、`securityaudit` 三包全量 unit 测试通过，共 640 个顶层测试、1134 项含子用例检查；apicompat 在缓存别名适配完成后重新执行。service 的 120 个定向顶层测试、565 项含子用例检查通过，覆盖实际 Messages/count/account-test/relay/Antigravity 请求、原生厂商版本路由、Cloudflare 严格边界、监控地址、流式输入与 cache/TTL 终态、转换 chat/responses 返回与计费用量、来源检查点。三份原有未跟踪用量测试未被选中执行。用量专项只读复核确认 alias 缺口关闭，没有遗留的可复现场景。测试只使用本机/内存 fixture，不调用生产设置或付费模型。

本轮没有界面布局变化，不另跑浏览器视觉验收；未执行全量后端、race、数据库集成或发布 CI，不能据本地检查声称线上已更新。gofmt、维护 JSON、104 路径分类、来源常量、`git diff --check`、个人 VERSION 和原有三份未跟踪测试 SHA256 核对通过，HEAD 保持同步前提交，无迁移、依赖或运行配置变化。最终结果汇总见 `.artifacts/fork-sync-20261113/verification-summary.json`。

## 发布前审查与修复

独立审查发现，新增缓存别名进入事件转换状态后仍需同步处理 provider `prompt_tokens` 总量、显式 `prompt_cache_miss_tokens` 和 `prompt_cache_hit_tokens:0`。例如总量 1200 含缓存 800 时，转换后的返回值曾为 2000，而本地计费为 1200；另一种终态已将 hit 清零但旧缓存 800 仍被保留。现在四个流式合并点遵循相同 canonical/alias 优先级、实际 total/miss 归一与零值清理，非流式 normalize 保持原契约。新增五个 Fidelity fixture 覆盖 total、miss 优先/零、含 TTL 的 total 和 hit 归零；CI race 选择同步覆盖占位零、版本化 URL 和安全审计回归。

浏览器实测 320px 英文管理端用量页文档宽度约 336px，根因是页面 Grid 的隐式 auto 列被日期工具栏 min-content 撑宽。将该页列定义为 `minmax(0, 1fr)` 后，内容收缩到可用宽度；359px 以下统计卡片将图标放到数值上方并微调金额字号，避免金额小数拆行。保留缓存浮层和日期/粒度/刷新/记录切换操作。发布前重新验证新增修复，源码未变的同步证据复用。服务器只读预检确认仍运行 v2.1.63，333 行迁移、安装密钥、运行配置和数据容器契约与上次验收一致；备份恢复与最终发布证据保存在仓库外 `development/diagnostics/sub2api-release-v2164-20261113/`。
