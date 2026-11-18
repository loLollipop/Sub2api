# 原版 v0.2.23 选择性同步（2026-11-18）

本轮将原版正式来源检查点推进到 `original/v0.2.23`（`5af9afb588c4b014b3df9db7d4073af1f8ae01b6`），并更正旧台账中已完成事项仍被写成延期的记录。个人 `VERSION` 保持 `2.1.67`；二改正式检查点仍是 v2.3.105（`3b3f324c3a8f4032aca7fa86ecafdd32747cffc8`）。观察到的原版 main `2af3f1e15c98c893a89255a037f551046883b8c0`不导入。

## 来源范围与清单

从 v0.2.22 commit `b0fa9f3e6eeab1968a53e1289b69daa4bb40ca2b` 到正式 v0.2.23 共 32 个 Git 可达提交（20 个非 merge、12 个 merge），涉及 90 个不同路径。旧摘要中的 34 个提交计数与真实对象图不符；按 `git rev-list` 和提交对象复核后，本记录及逐项清单采用 32。merge 提交标为范围 envelope，VERSION 提交排除。完整 32 条提交和 90 条路径逐条分类见[选择清单](original-selection-ledger-20261118-v0223.json)。审查工作材料保存在 `.artifacts/original-sync-20261118-v0223/`，不是提交台账。

## 本轮采用与适配

- GPT-6.1 Terra 与 GPT-6.2 Sol/Astra 的模型推理、采样、Codex 元数据、HTTP、WebSocket 和协议桥接已适配。Terra 保留 `none`；GPT-6.2 拒绝 `none`/`minimal`。Ultra Fast force gate、可配置渠道倍率和 Pro/chatgptpro workspace entitlement 已适配；渠道倍率 nil 继承，正值可用，零或负值拒绝。GPT-6.2 默认 Ultra Fast 价格倍率按请求时点计算：2027-03-01 08:00 UTC 前为 3x，之后为 6x；资格检查与价格倍率分别处理。
- 30 分钟/12 小时 cache usage 贯通转换器、存储、DTO、计费、账户统计和前端。只有两个上游桶同时存在、为非负整数且总和等于 aggregate 时才使用细分用量；不根据请求 TTL 推断账单。保留 0.15 倍率、显式零价和零用量。迁移 271–273 添加 force、Ultra Fast override、TTL token/price 字段；ent 生成完成，`go.mod` 未变，`go.sum` 增加 5 对 generator 工具 checksum。
- 缓存快照保留 aggregate、TTL 与显式零值；保留 prompt total，总量超过 TTL 明细的差额按 5 分钟处理。请求已出现费用或语义输出后不重放；断连后在固定期限内回收真实终态用量，不估算扣费。idle deadline 与首输出切换预算、原始流 flush、单遍 JSON 改写、本地 DeepSeek replay 和 Ollama 兼容继续使用本地边界。
- 保留历史 first-output spool、完整工具 schema 链、Gemini 同名并行工具、Antigravity thoughts、空 instructions/response echo。Messages prelude 暂存有界，超限释放后禁止重放；keepalive 不算语义输出，ping 后仍能补 SSE error。首输出前失败帧单独保留，供换号耗尽时匹配 passthrough rule。Messages 断连排空采用固定截止时间，上游持续心跳不会延长预算；关闭 idle timeout 时仍有默认上限。
- rollup 保留边界、主键有界归档/删除、规范 transcript 身份和 24 小时拒绝记录 UPSERT 保留 DB 原子扣费与幂等。迁移 274 修正 trigger 对 retained-from 边界、过期删除和保留内更新/删除的处理；旧迁移不修改。
- relay 隐私适配所有客户端出口，只清除客户端 relay 标识；Ops、usage 和财务观察保留原始值。前端补入 CSV、deleted-group 明示及金额、Accounts 编辑 shell、Groups 四 worker 即时打开和陈旧响应保护、usage cell 去 deepwatch；保留本地 Vue 主题、Ultra Fast Groups 输入和公开定价显示。
- 补齐 Fable/Mythos 5.2 的模型识别、Bedrock 映射、adaptive thinking、signed history、五档 effort、采样约束和协议桥接。标准价与 Fast 2x、Bedrock 不计 Fast、无长上下文加价，以及显式零价和 0.15 倍率已覆盖；CLI 版本依正式来源更新为 2.1.321，Mythos 配置提示不代表授予模型权限。
- 适配 `d05e8ef03`：统计、趋势和 Gemini 额度使用独立账号成本，客户实际扣费仍用 actual cost；管理员 endpoint 图表分列显示，用户接口和页面隐藏账号成本。同步补齐本地 rollup 读取和尾段合并，账号成本不会在历史小时数据中丢失；整小时范围也按 endpoint 合并，避免同一端点重复成行。

- 补齐正式原版祖先 `d70a23f84` / `32e70f3d4` 的 Gemini 4 Carbon 协议、thinking、模型列表及白名单，与 Carbon/Hydrogen 静态价卡。裸 Carbon 原样映射，裸 Argon 继续映射 high。按 `b32ad6dfe` → `68071ddde` → `2164ea487` 补齐 Vertex us/eu 端点、缓存资源同区路由及实际区域价格因子；仅默认模型价乘 1.1，分组/渠道最终价和自定义账号规则不叠加。上述提交属于正式原版祖先，不属于二改 v2.3.105 祖先。源代码没有按日期自动切价，不添加自造定时器。
- 适配 `4197fa620`：`gateway.max_same_account_retries_gemini` 默认 4、范围 0–10，0 表示只发首次请求；Native、Messages、Chat 的 HTTP/transport 错误共用预算，签名降级重试独立计数。补齐 Gemini Messages 非空 partial result 禁止 failover，真实 handler 验证部分输出或收费结果后只调用一次、结算一次。
- 补齐 `457602c8f` → `c004c962a` → `03bd718ab` 的完整 HTTP 请求体内存预算链：chunk、连续副本、解压/Zstd、JSON normalize 的临时及最终内存统一预留，handler 完成、取消和失败后释放；预算耗尽返回对应协议 503 与 `Retry-After: 1`。未显式设置时按 RAM/cgroup/GOMEMLIMIT 最小限制的四分之一计算，最低 1 GiB；显式 0 保留。cgroup 路径用 `os.Root` 约束；已升级 WebSocket 帧继续保持独立生命周期。
- 适配 `4ac1675b5` / `4a22a75b3`：Antigravity 主动查询绕过两次额度缓存检查并保留 singleflight；各状态提供 Query。后台刷新保留账号行与已有额度，表格在刷新时 busy/inert。适配 `b8de5e170`：Keys 列表就绪与独立用量请求解耦，旧响应及 finally 不覆盖新请求；追加迁移 275 建覆盖索引 `INCLUDE(actual_cost)`，runner 重试前处理 INVALID 索引。
- 从 `ce7a129fe` 抽取 compact admin usage：Ent 只投影显示字段，跳过 subscription，不查 API Key、credentials、密码哈希及大定价；summary DTO 支持表格、删除标识、导航和导出。账号摘要只进入管理员响应，用户/详情旧契约保留。CSV 文本列转义公式前缀，数值/负成本和 XLSX 字符串保持原语义。
- 从 `7d1249b10` 抽取公告懒加载：登录、路由和 focus 只刷新角标；铃铛打开才加载无正文 timeline/excerpt，详情按需读取；登录弹窗每批最多五条正文，确认失败保留，成功后续批次。summary 与 popup 筛选使用不选正文的 metadata SQL；timeline 仍读取正文生成 excerpt，不把它宣称为无正文 SQL。详情/已读使用同一可见性授权；前端隔离跨会话、陈旧响应、revision、已读确认及登录延时请求。

## 旧延期纠正与仍需验证

fork 台账中此前仍标为 deferred 的 glass modal、Gemini group OpenAI catalog、Cline/Command Code、request-ID trigram index、Opus 5.5 strict validation 已移入 resolved；trigram 和 Cline 的 PostgreSQL fixture 仍列 verification backlog。用户取消的 affiliate per-code 统计标为 cancelled；TypeSafe/System One 项拆成已实现 native System One、默认关闭 moderation、取消 affiliate 与撤回的专用 journal/recovery。TOTP consumption、Anthropic disabled thinking omission、no-op JSON validation 标为 not applicable / preserve local contract。v2.3.10 和 v2.3.4 混合延期中删除已完成的 Accounts shell、Groups hydration 与 usage cell 项，只保留未完成范围。旧 note 的历史取舍文字不改，新 `resolution_20261118_v0223` 字段指明覆盖范围。

原版台账中已实现的 moderation cleanup、monitoring picker/details/compact filters、SheetJS 固定依赖和 Gemini 3.6 已移出延期；moderation 并发删除/保存/回滚及真实 PostgreSQL schema、race、Redis/Docker、跨批次验证归入 verification backlog。`afbef7c8c` DeepSeek alias 修复因本地缺少来源 alias 前置而标为 not applicable。拆分 v0.2.21 遗留延期后，Fable/Mythos 5.2（`eb2da3448`）及 pricing 修正（`3e8c2ee3f`、`d216e4215`）由本轮适配，后端 414 项、前端 66 项通过。

真实延期仍包括：scheduler membership 缺 credentials/transient time/retirement fencing；整单 error 免收会改变真实终态财务策略；DeepSeek caller schema/ID collision/native scope 与 Hosted Search 的收费结果、DSML 依赖；whitelist 失败清理人工 mapping 和 stale `fetchErr` retry；可变 main catalog provider/alias/ETag；public settings CDN/auth/captcha；自动 IP/key disable；account blacklist 缺 `mcp_oauth_grants`；大型 DataTable patch 与本地 virtualization 冲突；outbox batching 和 60 秒统计缓存一致性；固定 30% critical 阈值覆盖用户零阈值；DeepSeek 48K/20MB、30–65 分钟策略缺本地证据；Auth Quiet/SSR 产品重构；Mihomo、upstream monitor、raw dump、trace 本地模块缺失。Gemini 可配置同账号 retry 已适配；mapping/pricing union 在个人基线已等效实现，设置页 sticky 保存入口也已满足 save dock 的用途。Gemini explicit-cache CRUD/bound-cache pool runtime 尚未完整接线，`a20741237` 随该链保留；MCP credential、account-size、penalty 和 image-size stickiness 仍有真实缺口。DeepSeek 最大输出 393216 和精确型号名已实现；额外 OutputModalities/ToolCall/Family/authoritative catalog 当前没有 DTO 消费者且会改变 primary-wins，继续排除。Kimi `4067236b` 的 262144/65536 是邻族推断，不作为官方依据。

## 验证边界

已保存的有效证据包括 cache apicompat 895 项、cache service 229 项、模型 complete/service/regression/Terra bridge 回归、privacy targeted 与确认日志、instructions echo、messages handler fixed、前端 11 文件 229 项、公开前端 3 文件 35 项、lint，以及 SQL/PGlite 19 项迁移检查。复用 `.artifacts/original-sync-20261118-v0223/` 中这些最终证据，不把中间失败或过期日志记为通过。

新增有效验收：`claude52-final-backend.jsonl` 后端 414 项，`claude52-frontend.log` 前端 66 项，最后权限提示断言的 `claude52-final-usekey.log` 32 项；`messages-bound-and-account-cost.jsonl` 123 项覆盖 Messages 固定断连期限及费用分离；`account-cost-frontend.log` 28 项。实际浏览器 `ui-acceptance-results.json` 26 项覆盖 Ultra Fast 保存、清空继承、免费价格、CSV 下载、Accounts 即时 shell；`endpoint-cost-ui-results.json` 8 组覆盖管理员/用户、320/1280px、英文亮暗主题、实际扣费排序、账号成本分列/隐藏及嵌套展开，均无文档横向溢出和浏览器错误。

最终有效验收：`accepted-backend-tests-summary.json` 覆盖 20 个包、1,268 个顶层测试、3,018 个测试及子测试通过，0 失败；其中包含公告 metadata、授权、详情、已读与 migration275 runner 回归。后端 `go build ./...` 与包含生产前端资源的 `go build -tags=embed ./...`、前端 lint、locale 完整性、vue-tsc、Vite 生产构建、console assets 和 `git diff --check` 全部通过，结果见 `final-acceptance-results.json`。公告定向前端 58 项回归通过，见 `announcements-lazy-frontend.log`，本地 mock API 24 项通过；真实浏览器 `announcements-lazy-browser-results.json` 四组覆盖 320/1280px 英文亮暗主题、首次无 timeline、popup 正文复用、确认失败保留、Escape、详情加载/失败/重试，无横向溢出和页面异常。账号、compact 导出与 Keys 另有 `account-keys-compact-ui-results.json` 四组交互验收通过。

Gemini4/Vertex 有 754 项后端、27 项前端回归，包含 27 组真实转发至计费测试；Gemini retry/escaped cache resource 最终 604 项通过。HTTP body budget 639 项、AG Query 52 项、usage cell 56 项、Accounts 刷新 17 项、Keys 47 项、compact backend 30 项、UsageTable 56 项以及最终 CSV/导出 48 项通过。这些定向证据与最终回归重叠，不能相加作为独立用例数。中间失败日志保留诊断用途，以各最终 passing 日志为准。

本机没有 Docker 或 PostgreSQL，真实 PostgreSQL 并发、升级/回滚及 Redis 集成 fixture 未执行；PGlite 的迁移271–274共19项和迁移275共5项 SQL 检查不能代替 PostgreSQL 并发建索引/恢复证据。一个 cgroup symlink 子测试因 Windows 创建权限跳过，Windows junction 防逃逸通过；Linux amd64 仅交叉编译，未原生运行。最终边界检查确认 HEAD、暂存区、旧迁移、个人 VERSION 及三份受保护未跟踪测试保持原状。以上是同步阶段的验收边界。用户随后授权提交 GitHub 并更新部署，个人版本推进至 v2.1.68；按 PR/CI、自有 Release/GHCR、备份恢复及真实迁移验证流程发布，实际发布结果另存证据。
