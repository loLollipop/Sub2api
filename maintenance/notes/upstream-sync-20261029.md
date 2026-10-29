# 双上游正式版本选择性同步（2026-10-29）

## 来源与范围

- 本地起点：已发布 v2.1.41，`28d9e1068e8f5ce429b758cdcc324b8860962949`。
- 二改：上次正式 v2.2.42 `9aedba768005a6f6c0aa52f78a1fb931fee61587`；本次正式 v2.2.53 `636250da69e9b9d66a9abf9945292e9ccfd76441`；观察 main `bba11a3e2ac98a3320ba642e52c94a5111710607`。
- 原版：上次已审 main `5c185c5c5d4f5e409e2139806e0964ae944e03e8`，正式检查点 v0.2.18；本次正式 v0.2.19 `dece26153621959ccd7635814f4b2d08304e681a`；观察并分类 main `a3298b46ebe239fb0720b068a1eb3383955bdac2`。

检查点表示已审查并选择性适配，不表示完整合并。二改 annotated tag 的实际提交为 `636250...`，不是 tag object `fe199...`。没有覆盖本地重名 tag；正式版本使用独立远端 release refs 核验。源增量、基线 SHA256、修复前复现和本地验证日志保存于工作区外 `development/diagnostics/dual-upstream-20261029/`。

保留本地 shadcn 界面、0.15 计费、冻结价格与显式零价、Grok 拒绝收费及永久会话封禁的原子路径、最终模型映射、出站凭据边界、断连排空和禁止重放。未修改发行 VERSION、Go 模块、Ent 或任何已发布迁移。不恢复工单、无限画布、原生支付、渠道定价界面、Mixed 或 observer 角色。

## 采用的二改修复

- **DeepSeek Messages 思考续传**（`e268c6d5e`）：仅最终 DeepSeek 账号保留纯 assistant thinking；通用提供商维持原有行为。新增组合 options 入口，保留本地可配置 minimum output token floor API。
- **Anthropic 自动缓存**（`34f58a944`，参考 `4b3237bf9`）：原生 Anthropic 请求保留顶层意图，将其补到最后一个合法可缓存块；已有显式块标记优先。放在本地 rewrite 后、四断点限制与 TTL 归一化前。跳过 thinking、空文本、未知或畸形块，并保持幂等。
- **Grok relay 视频结果**（从 `ba268df58` 等视频修复提取独立解析）：兼容 `done/succeeded/success/completed/finished`，仍必须有非空字符串 asset；支持官方 `video.url` 及 relay data/output URL 形状。duration 兼容 `video.duration/seconds/duration`，排除非正数、非有限值和整数溢出，仍沿用本地时长归一化及创建时价格快照。重复轮询仍只结算一次。
- **账号窗口统计**（`d1d2f5c25`、`f527a2c9c`）：不同账号窗口起点通过参数化 VALUES join 一次聚合；保留单账号相同成本公式、无用量补零和旧 repository fallback，不导入账号定时功能。
- **账号列表虚拟化**（`075ca6387`）：复用本地已有 DataTable 机制，仅将阈值从 50 调至 20。
- **依赖修复**：不照搬 main-only `5d6328fba` 的临时漏洞豁免；将 vue-i18n 及相关 @intlify 升至修复版 11.4.16。保留 `legacy:false`、现有翻译与 SheetJS integrity。三个前端 CI workflow 改用 Node 24，与现有 Docker 一致；新增依赖要求 Node >=22。

## 采用的原版修复

| 来源 | 本地适配 |
| --- | --- |
| `f96528048`、`9aa16c8c6` | 分页 offset 与 offset+limit 整数溢出保护；内存 API key 分页复用统一计算，Ops 和提示词审核入口验证边界。保留本地可配置 page-size clamp。 |
| `de908ed6d` | 明确的 Coding Plan 频率型 429 在额度快照低于 90% 时使用原有短冷却；额度/余额文案优先。缺失或非法快照继续保守处理，额外拒绝坏类型、NaN/Inf 和负值快照。 |
| `917dbdc70` | Gemini 返回的 fileData/file_data 图片也计数，要求真实字符串数据或 URI；同 part 不重复计费，保留本地倍率。 |
| `cde6ddfd8` | OpenAI multipart 单 part 50 MiB，读 limit+1 后拒绝超限，避免静默截断。Grok 在现有整体 body 限制内完整读取，图片和 mask 保真；不导入无关日志。 |
| `88d9c43b3` | 禁用/删除管理员、最后管理员降级返回稳定 reason 的 HTTP 400，保留现有鉴权、TOTP 和管理员保护。 |
| `14a5fe305` | Antigravity malformed function call 返回 502，不能变为空成功；本地进一步保留并排空已发生用量，见下节。 |
| `b2b1bcf26` | Codex 模型目录进入 singleflight 后再次检查 fresh cache，避免另一刷新刚完成时重复请求；保留 stale/ETag 路径。 |
| `ba5b21879` | OpenCode 官方精确 hostname 且最终出站模型为 DeepSeek 时把 developer 改为 system；使用现有 OpenAI-compatible 账号，不新增不存在的平台。 |
| `7bce2cfb4` | Claude Code 指纹按 JavaScript UTF-16 码元下标采样并按 JS UTF-8 规则散列，保留 ASCII 行为，测试覆盖中文/emoji/代理对/NUL。 |
| `9eba61736`、`1352e899e` | 渠道 web_search_emulation 按平台合并，显式 feature 值 `{}`/null 仍可清空，其他 feature 保持整块替换；复制 map，不污染旧配置。 |
| `31564b2e8` | 刷新 Gemini 默认目录，连接测试默认 3.5 Flash；保留 Google One 保守列表、countTokens 方法、3.6/3.7/3.8/Argon 和自定义映射。前端候选同步并去重。 |

DeepSeek Pro 定价、响应模型冲突审计、prompt_cache_hit_tokens、图片按次时保留 token、模型广场 group 定价优先及 x/net 0.62.0 已由本地实现覆盖，没有重复导入。

## 独立审查后修正的两处边界

**Antigravity 错误不能漏记用量。** 原版直接在 `MALFORMED_FUNCTION_CALL` 返回空结果，会丢失同帧、此前和后续 usage，并停止排空。本地改为保存该终态错误，继续在既有读错误/间隔超时边界内收集。转换器、收集器、Claude Forward 和 Chat/Responses 适配器逐层保留部分结果及普通错误，沿现有 handler 错误部分结果路径记账；客户端仍收到 502，没有成功完成响应，也不转换为 failover。

新增真实 Forward 回归覆盖三种协议、usage 位于错误前/同帧/错误后、跨 channel 缓冲区的后续帧排空、只调用上游一次及 read error。修复前 3 个顶层测试、11 项 tests/subtests 失败；修复后通过。另保留底层 transformer 错误时返回已知 usage 的断言。

**通用 Chat 请求不能泄漏 Anthropic 专属字段。** 二改 `4b3237bf9` 的无条件顶层 cache_control 透传会让严格的 OpenAI-compatible 提供商拒绝请求，本地不采用这一子项。通用转换器保持不携带该顶层字段；原生 Anthropic 请求保留并展开自动缓存。真实 fallback 测试验证 OpenAI/Kimi/智谱/Command Code/DeepSeek 出站隔离，同时检查 thinking 与 token floor。新断言在修复前失败，修复后通过。

## 延期与不采用

- 运维、采集、连接时间线、账号定时/通知、ID1/Bark、Hosted Search/Fetch、上游整套 UI：继续按用户决定延期；源 migration 306 不移入。
- 二改 `7b2a1caf8` 的 key+item-only reasoning 缓存放宽不移入，保留本地 API key+会话+模型及 opaque 绑定。稳定 session/alias 变化下的兼容可单独设计，不能以弱化隔离换取命中。
- 二改 DeepSeek image filter（`b99a5284b`、`9e7e3a5bc`）会按客户端原模型静默删图，未满足本地最终映射边界，暂不移入。现有 vision catalog 保留。
- Grok creating 超时自动退款、expired 自动退款、无 asset 强行结算、扩大 reconciliation 自动轮询不移入。未知上游执行/收费结果继续保留预留，不因本次状态兼容改变财务政策或既有 24h 自动轮询上限。
- 原版 scoped admin keys、image_count_billing 迁移/开关、受管 Grok catalog CAS/observer/诊断、Gemini 显式 cache runtime 继续延期，不摘孤立字段/迁移。
- GPT-6.1 Terra 新能力、Gemini 同账号可配置重试、所有 mapping 即存在证明的目录并集属于第二批功能适配，本次维持既有行为。
- 原版正式 tag 后 main 的 OAuth 非流式 Responses 修复组 `6b2fdfcac`、`9b5407449`、`4db8402b5` 已审查，按 official-releases-only 整组延期，不拆开导入。
- 不导入上游 VERSION、无关 CI runner/release 策略、支付/渠道 UI、Sponsors 和历史 merge；既有延期仍有效。

## 验证与状态

- 7 个相关小包全量测试及 service/handler/admin/repository 的较广定向回归通过，审查修正后重跑相关协议与检查点。三份最终日志去重为 **2,737 个顶层测试、5,708 项 tests/subtests**，不是全后端测试。
- 前端全量 **3,141 项通过、1 项跳过、0 失败**；目录更新另跑 20 项专项。新增 5 项语言切换测试、typecheck、全量 ESLint、Vite production build、console CSS 资产校验、frozen-lockfile 安装通过。
- 生产依赖审计零漏洞，没有新增豁免。Go server build、改动 Go 文件 gofmt、git diff --check 和本轮修改/新增文件敏感信息检查通过。
- 基线 4,956 个跟踪文件及 3 个既有未跟踪测试通过逐文件 SHA256 核对；所有未改文件和 688 个受保护文件（迁移、Ent、Go 模块等）保持原样。HEAD 未变。
- 本地证据以 `verification.json`、`final-packages.jsonl`、`final-integration.jsonl`、`audit-fixes-final-v2.jsonl`、`backend-build-final.log` 及前端日志为准；早期失败/overlay 日志仅为复现和阶段验证，不当作最终通过。

未运行全后端套件、Linux race、真实 PostgreSQL 执行计划/迁移、Redis 集成、付费模型请求或生产操作。统计 SQL 有参数化 sqlmock 与 handler/service 路径回归，没有声称真实数据库性能测试。

仅保留本地分支 `sync/dual-upstream-20261029`。未 commit、push、发布或部署；发行 VERSION 保持 v2.1.41，现网不变。共享 Git 数据库和三个此前已有的未跟踪测试保持原样。

用户随后授权提交 GitHub 并更新部署。本次准备个人发行 v2.1.42；通过 PR、合并提交 CI、正式制品与自有 GHCR 校验后进行受控部署。运维、采集和上述延期项继续不做，最终发布与现网验收另行记录。
