# 双源选择性同步：二改 v2.3.153 / 原版 v0.2.24

日期：2026-11-23。个人基线 HEAD `df3d4234caa23d02fb555246b5310470c9d1beb5`，VERSION 保持 `2.1.70`。仅本地工作树同步，未 commit、push、发布或部署。

## 正式来源与盘点范围

| 来源 | 起点 | 正式终点 | 终点 commit | 本轮范围 |
| --- | --- | --- | --- | --- |
| kiss-kedaya/sub2api | v2.3.117 | v2.3.153 | `35f6cd4f7f2d7670d9d4d7c1f763fed359cbaa33` | 333 commits，234 非 merge，1185 净路径 |
| Wei-Shaw/sub2api | v0.2.23 | v0.2.24 | `5c2982a81b46e2142d0143d2f83c1ce9c4da1d24` | 110 commits，65 非 merge，190 净路径 |

GitHub 正式 release 元数据、完整提交/路径清单及源 diff 保存在 `.artifacts/dual-sync-20261123/`。二改发布于 2026-11-22 23:37:50 UTC（北京时间11月23日07:37:50），原版发布于11月20日09:19:50 UTC。观察到的未发布 main 仅记录，不导入。

检查点表示已完成本轮盘点与选择性适配，不表示完整合并或对全部源代码做过安全审计。所有 ancestry/path 已列入两个 tracked ledger；仅采用项和明确列出的缺口完成源 patch/本地调用链深查，其余标明 inventory-only。

- [二改逐项清单](upstream-selection-ledger-20261123-v23153.json)
- [原版逐项清单](original-selection-ledger-20261123-v0224.json)

## 采用的行为

1. **Codex HTTP 上下文**：在真实工具输出校验前，归一化 `namespace=agent` 且没有有效 call_id 的 browser_info、goal_objective、untrusted_context、untrusted_content、ide_context、proposed_plan。保留页面快照、合法空内容/分片、历史顺序、原始数字精度与原归一化顺序；未知项、真实工具和重复字段仍校验。HTTP continuation 提示按 OAuth/SetupToken/API-key 实际支持说明，修复截图 `function_call_output requires call_id` 对 agent 上下文的误拒绝。
2. **登录 TOTP**：密码、邮箱 OAuth、OIDC、各 pending provider 的已有账号登录都使用严格开关读取；读取失败拒绝发 token。需要 TOTP 时仅返回一次性挑战，pending 原子消费，验证后才能发会话。LinuxDo 无 identity 的 synthetic 邮箱遗留快捷路径也先检查已有用户。passkey 尚无挑战 UI 时拒绝需 TOTP 的账号。登录验证码有独立 Redis SETNX marker，按用户/实际 counter/验证码摘要防重放，存储失败拒绝；一般 VerifyCode 的复用契约不变。前端 GitHub/Google/OIDC 回调接完整动态码流程。step-up 设置读取失败503，不能静默关闭门控。
3. **可信代理 IP**：仅可信 TCP peer 可提供转发头，XFF 从右向左剥离可信代理；自定义头列表中的 XFF、多值 XFF 同样使用可信链。直连不能伪造 ACL/限流/会话 IP。沿用现有 server.trusted_proxies 配置，更新示例和 EDGE_SECURITY 部署说明；没有写运行配置。
4. **OpenAI OAuth 刷新凭据**：上游轮换成功后用独立有界 context 保存，避免可选 enrichment 超时丢失不可逆结果。完整凭据+代理快照 CAS，排除 credential shadow；凭据和 scheduler outbox 同 SQL 原子更新。并发重授权保留新值，普通代理/非 token 设置编辑可在新快照重合并；保存失败停止旧 token 重试。统一和 direct refresh 两条路径都接入，分布式锁预算预留保存/清理时间。
5. **网关错误策略**：OpenAI alpha/search 工具套餐403不惩罚账号，明确 account/workspace access-state 仍处理；OpenAI 三处408按原策略换号。上游 request-body-budget503在分类、同账号重试、账号处罚、换号、Retry-After、客户端脱敏和前端标签中保持一致。AG bare429按一次请求最多冷却一个实际尝试账号，post-loop不再冷却跳过的账号；AG quota 使用同转发端点。
6. **协议兼容**：Gemini 非流式桥保留 cached usage，最后 function-response turn 的尾部普通 parts 拆至下一 user turn。Stop 与 Kimi dynamic tools 跨协议保留。Claude compaction 写 cache-control 断点使用5m。图像完成事件中 pretty usage 压成一行 SSE，保留本地 direct Images 对真实终态的约束。Claude/Bedrock 连接测试对拒绝 temperature 的型号省略字段，保留本地 custom prompt/max_tokens。
7. **ToolSearch 和空终态输出**：仅 Codex protocol、非 Grok 且模型声明 supports_search_tool 时使用 native ToolSearch；保留 forced tool choice、发现结果文本、deferred definitions、reserved-name映射，以及流式/非流式回程。空终态 output 按 output/part 索引恢复已完成 hosted items、文本、拒答和工具输入；已有真实 terminal output 不替换，usage/计费保留本地逻辑。重建输出不再被旧 accumulator 二次覆盖；本地 reasoning done-only 事件和真实 provider 思考仍完整保留。raw reasoning 与 display summary 继续分开，buffered reasoning 的多 part 仍用本地换行规则，不将 summary 当 DeepSeek 可回放思考。helper 的 typed hosted items 不是任意未知字段的透明 raw round-trip；原生透传链未替换。
8. **管理与前端**：Turnstile 部分保存省略 site key 时保留已存值；secret 空白仍表示保留旧值。Claude/Gemini 快照字段进入单元格及批量用量缓存键，旧请求结果/错误/finalizer不能改写新请求。保留移动端可见性懒加载、批量 request token、被动查询和本地主题。

## 已有覆盖与保留条件

admin/user/WS 的 token 撤权已有统一 ValidateTokenContext + 纳秒 cutoff 和读取错误拒绝逻辑，不再加源不存在的 IsAccessTokenRevoked 接口。DeepSeek 原生思考占位已由 ensureDeepSeekThinkingReplayPlaceholders 覆盖，不叠加二次修复。multipart RFC2231 extended-name拒绝及原版v0.2.24 HTTP/WS duplicate-field安全切片已在此前采用。Fable/Mythos5.2、Gemini4、GPT6.2、UltraFast、可配置 Gemini retry 和 vue-i18n11.4.16保持既有实现。

本地原生厂商路由、实际终态/cache分桶计费、自定义倍率、主管理员 user ID 隔离和冷灰蓝 Vue UI 保留。既有331份 SQL、三份起点未跟踪用量测试、VERSION、依赖清单与锁文件受 hash 保护；没有引入源 SQL、依赖升级或运行配置修改。

## 明确延期与验证边界

- 二改 scoped admin keys、hash-only API key/redeem及一次性receipt、blind index、at-rest encryption、全套 session/admin step-up/免重复验证策略没有整组导入；支付、crypto、deposit、退役 ticket/canvas runtime不恢复。不能用本轮登录修复宣称覆盖源全部认证/权限更新。
- 管理出站 SSRF `51a3b1a189` **未修复**：本地仍有由管理员配置目标的 transport/private-network风险；需同时适配 DNS固定拨号、重定向、SMTP/S3等真实入口及现有私网部署契约。入站代理链修复不覆盖此风险。头像 SVG/MIME/真实字节检查和自定义 Key 可预测性规则同样未采用，清单中保留具体缺口。
- 原版 foreign-model过滤须与本地显式厂商/渠道映射一起适配；[1m]归一化、metadata长上下文模型列表、audio capability整链、每轮 reasoning effort、Grok usage成功搜索计数/五图编辑、管理员新用户默认quota、Exa/source结果输出和估算窗口成本等未引入。批量透传filter源修复没有对应本地UI/写链，不把缺模块称为已覆盖。
- 本轮无真实 PostgreSQL/Redis 集成、跨进程锁/并发和 race instrumentation；CAS原子性由SQL mock/代码审查检查，TOTP marker由单测Redis fixture检查。Linux/amd64是交叉编译，未在Linux执行。浏览器使用本地preview+route fixtures，未调用真实OAuth/付费上游。

## 验证记录

最终验收汇总见 `.artifacts/dual-sync-20261123/final-acceptance-results.json`。中间态失败日志保留，不能算最终通过证据；采用的最终日志在汇总中逐项列明，子测试/顶层及重复范围不能相加。

- 前端账户快照96项、认证回调66项、i18n3项、错误标签/明细11项通过；类型检查、lint、生产构建和console资产检查通过。
- Playwright32项通过，覆盖320/1280、明暗主题、GitHub/Google/OIDC挑战的错误停留与成功跳转、发token前状态和账户刷新，无page error；已查看窄屏挑战和账户页实际截图。
- 后端8个受影响包完整测试及服务定向回归通过；有效证据按package/test去重，共4019个顶层、8201个测试/子测试通过事件。Linux/amd64嵌入前端构建通过；339份保护文件hash、HEAD与index均不变。
- 独立认证/凭据数据完整性审查发现的自定义XFF绕过已修；最终enrichment超时验证另有明确通过记录。

## 后续发布授权

用户随后明确要求提交 GitHub 并更新部署，个人版本准备推进至 v2.1.71；本页和同步验收 JSON 保留同步时的版本及边界。发布阶段复用未变的实现验证，另核对 PR/CI、自有 Release/GHCR 制品身份、真实备份恢复、迁移只读校验和线上运行。最终发布证据位于 `development/diagnostics/sub2api-release-v2171-20261123/`，不将待执行步骤记成已成功。
