# 双源选择性同步（2026-11-18）

本轮完成本地选择性适配，个人版本保持 **2.1.66**。分支 `sync/dual-20261118-v23105-v0222`，起点 `2a6233c9a6e4ccc5d26c7e1351423cd3f8938806`；未 commit、push、创建 Release 或更新部署。

## 正式来源

| 来源 | 原检查点 | 本轮正式检查点 | 增量 |
| --- | --- | --- | --- |
| `kiss-kedaya/sub2api` | v2.3.89 / `9f7dc5e798e8054228e31feb6abaeae808d269cd` | [v2.3.105](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.105) / `3b3f324c3a8f4032aca7fa86ecafdd32747cffc8` | 156 个提交（含 merge），303 个路径 |
| `Wei-Shaw/sub2api` | v0.2.21 / `10ba8f9b4dc2e6023d4a9879ab77b21841babd10` | [v0.2.22](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.22) / `b0fa9f3e6eeab1968a53e1289b69daa4bb40ca2b` | 187 个提交（含 merge），340 个路径 |

GitHub Release API、正式 tag 与独立 remote ref 已核对；没有覆盖个人 tag。二改正式发布时间为 `2026-11-18T00:17:07Z`，原版为 `2026-11-17T02:48:12Z`。观察到的二改 main `a58e3d2eefccb39e8679d6d03435ef29e519b3a1`、原版 main `69d2fae91318359e4e25c0171eaf70364dad3629` 仅留档，未合入未发布 main。

完整提交、路径清单、diff、Release 元数据、采用的叶提交 patch 及验证证据保存在忽略目录 `.artifacts/dual-sync-20261118/`。路径台账覆盖二改 303 项（19 项选择性适配、221 项延期、63 项排除），原版 340 项（36 项选择性适配、3 项保留本地实现、295 项延期、6 项排除）。两边可能涉及同一路径，不能把路径数相加当成本地修改文件数。台账表示完整的来源清单及模块取舍；已选修复和下述具体冲突经过源码核对，**未宣称逐行审计所有延期代码**。同一文件内未选功能继续延期。

## 已适配的二改修复

| 叶提交 | 本地结果 |
| --- | --- |
| `cc00ea4b4`、`1055013d4` 的 CC Switch 部分 | 模型列表地址和导出的余额脚本不再重复 `/v1`；保留反向代理路径与 query，移除 hash。浏览器请求使用 URL 拼接；外部余额脚本只用字符串操作，不要求脚本引擎提供浏览器 `URL` 全局对象。覆盖根地址、`/v1/`、多尾斜杠、query/hash，实际解码并执行导出的脚本，验证余额显式 0 和禁用状态。支付部分未恢复。 |
| `d4b0308d4` | 渠道状态时间轴使用整个矩阵中最新真实 bucket 作为公共锚点，按 `coverage.bucket_seconds` 定位。新分组显示在右端 NOW，内部缺口留在原时间槽；未采集槽不绘色，已有 unknown 桶才绘灰。缺少有效定位信息时右对齐；质量历史按真实时间升序。保留本地主题、分组布局、状态颜色和不扩宽的焦点/悬停效果。 |
| `eda7d7c08` 的导出逻辑 | Excel 导出每页请求 1000 行，并接受后端返回更小的实际 page_size。每页最多 3 次尝试，只重试网络错误、408、429、5xx，间隔 400/800ms；400/401/403 直接失败。重试成功才追加该页，避免重复行；取消后不继续请求或生成文件，也不误报导出失败。保留现有列和入口，未引入 CSV 新入口。 |

## 已适配的原版修复

| 叶提交 | 本地结果 |
| --- | --- |
| `197109ebc` | 登录邮箱身份回填失败返回 nil 时先判断，再读取 Metadata；不 panic，不把已属于他人的身份重复绑定或发放首次绑定权益。 |
| `d085bb1a3` | Chat Completions 转 Responses 的 JSON Schema 未指定 strict 时填 `false`，显式值保留。 |
| `6c79fdfc7` | Anthropic 会话中途的 system 消息转为 Responses developer，保留内容和顺序。 |
| `4584dcb39` | 缓冲聚合的 function call 保留 `encrypted_content`，沿用本地终态/item-done/delta 合并和完整参数快照，不替换为较弱的源 accumulator。 |
| `5846bfbbf` | Gemini Chat 兼容入口传递 `response_format`；JSON Schema 保留原字节、属性顺序和大整数。OAuth CodeAssist wrapper 以 raw JSON 嵌入 request，避免整体 map 解码损失精度。工具请求和图片输出模型沿用源排除条件。 |
| `8480cb0cd` | 既有 Antigravity Schema 清洗链只允许 array 携带 items，缺失/畸形 items 补 string schema；其他类型删除 items。不扩大到原生直通请求改写。 |
| `2b86d2f6a` | `count_tokens` API-key 透传默认请求头和调试读取使用已有 raw-cased helper，避免大小写不同的重复 content-type / anthropic-version。 |
| `1f2242436` | Chat → Anthropic 转换保留可解析的客户端 `metadata.user_id` 原始 JSON string token，不带入其他 metadata；OAuth Claude Code 伪装仍使用既有身份逻辑。 |
| `fd6487758` | Codex `additional_tools` 声明 ID 在过滤历史时保留，避免客户端工具发现被改成普通函数调用；其他历史 ID 处理不变。 |
| `74ba91aa6` | Codex 临时分叉携带父 session header / prompt-cache key 时使用父 session namespace；HTTP、Messages、Chat、透传及 WS 入口传入未出站改写的客户端 header，保留账号凭据/API-key 隔离。 |
| `f1394a1ab` | 注入站点设置的 HTML 各返回路径统一 `no-store` 与 CDN 禁缓存头。有每请求 CSP nonce 时即便 ETag 匹配也返回新 HTML；无 nonce 时允许原有 304。hash 静态资源缓存不变。 |
| `98a44ba3f` | 分组倍率同步复用本地已有事务/revision；RPM 同步的清空、删除、upsert 也进入同一事务。重复 user_id 以最后值为准，保留显式 0 和 nil 清除；仅 commit 成功后推进本地 revision。失败覆盖 begin、首条、后续写和 commit，防止半更新后错误失效缓存。 |

原版 `37a0f3870` 的跨分组 RPM 污染在本地通过 `CheckAPIKeyRouteEligibility` 复制 user 并重置目标组 loaded/override 状态处理；保留此实现，已运行 SmartBillingRouteEligibility 与 RPM 回归。Go 1.27.4 已在个人基线中，无需重复升级。前端新增的相关关键用例加入现有 Makefile 清单，后端新增测试沿用现有全包 unit CI 入口。

## 本轮未引入

- **错误请求整单免收策略。** 二改 v2.3.104–105 改变部分输出/usage 后以显式上游错误结束时的结算规则；后续正常完成且已交付回答、含正常 choices 的回答或空 error 对象不免收。WS、Bedrock、Antigravity、图片等覆盖也不同。本地继续采用既定真实终态与有界排空计费；整个策略及其 Hosted Search 观测 hook 延期，不能当作无行为变化的错误修补。
- **调度冷却成员恢复与 advisory fallback。** 原版 `39bac35a3` 将快照加载改用 `ListModelAvailabilityCandidates`；本地同名方法是专供模型支持诊断的短缓存投影，明确不加载完整凭据和瞬态冷却时间。直接套用会破坏认证与冷却判断。本轮没有声称修复“快照创建时已冷却的账号，到期后仍不在快照内”的情况；需完整适配独立账号池查询、outbox 新鲜度、退役 fencing、不可变副本及合并回源后再引入。二改 `a3bc6ff7a`、`d1bd6fcc1` 的模型冷却重选/同号重试联动也继续延期，保留本地持久故障隔离及不重放边界。
- **新计价、迁移及统计链。** Ultrafast、DeepSeek OCR/TTS/ASR、Gemini Carbon/Hydrogen/Vertex 区域加价、缓存 5m/1h 分档、DeepSeek 显式 override provenance、actual/account cost 分离均需要单独核对本地最终上游模型和显式零价。Rollup、共享统计 Redis 缓存、覆盖索引、live-balance outbox、去重归档保留此前延期。未追加源 251/315–318 迁移，未更新 pricing 资源。
- **上游监控及错误出口规则。** 本地没有已延期的 upstream_monitor 模块，告警 SQL cast 和重试不能孤立引入。二改新增 relay fingerprint/type 规范化跨 handler、SSE、WS 出口，需一起验证客户端脱敏和 Ops 原始观测；本轮保留现有隐私处理，未只导入 helper 而漏接调用方。
- **其他请求与运行时扩展。** Codex UUID ID 压缩、工具 Schema 正则删除和额外 context 改写；Bedrock 新错误语义、Gemini 并行同名工具调用、effort/keepalive/TCP 超时、首包 spool fallback、DeepSeek 大请求/等待上限、billing lane 与零拷贝等均未改动。保留本地原生路由、权限、智能路由、0.15 倍率、明确零值、真实终态用量、客户端断开后有界排空和不重放约定。
- **其他界面与退役模块。** CSV、已删分组统计、通配符白名单限制、quota 表格重构、新媒体文档，以及既有 Hosted Search、catalog/Plaza、自动风控、原文 dump、trace 和邮箱黑名单延期不变。退役支付/cryptopay/Mixed/工单不恢复；来源 VERSION、Release 工作流和 audit 豁免不覆盖本地发布、安全与主题约定。

## 验证与边界

- **后端定向测试通过。** Go 1.27.4，覆盖 apicompat、antigravity、repository、service、web，去重后 **841 个顶层测试、1172 个子用例**通过。首轮 707/946；补测 Codex 父会话/声明 ID、Gemini OAuth Schema 字节、raw header 调试、Antigravity 与目标组 RPM；新增 nil/zero 去重 SQL 参数和 RPM 成功/失败 revision 断言后复验仓储；`-tags=embed` 实际覆盖 HTML/nonce/304；来源维护 JSON 与运行时检查点一致性、Release-only 通知测试通过。各轮结果和去重规则见 `verification-summary.json`，未将重复执行算作额外测试。
- **前端定向测试通过。** 7 文件有效结果 **155 项**；首轮 151 项，KeysView 原 40 项被新增脚本执行测试后的 44 项结果取代，其他未变文件复用。覆盖时间槽/缺口/未知桶、卡片和页面连线、质量历史、CC Switch、导出限次重试/不重复/永久错误/取消。既有 fixture 的 i18n composite key 与 GroupBadge 警告不影响通过结果。i18n 完整性、`vue-tsc -b`、修改文件 ESLint、Vite 生产构建和 console 静态资源路由检查通过，未改依赖。构建仍提示既有 Browserslist 数据陈旧及 Node DEP0190，不影响本轮产物。
- **普通及 embed 后端构建通过。** 分别输出忽略目录内 `server.exe`、`server-embed.exe`，后者嵌入本轮 Vite 产物；未启动应用连接生产。
- **真实浏览器验收通过。** Edge headless，320/390px × 中英文 × 亮暗主题，24 项检查，无页面 JavaScript 错误或文档横向溢出。实际验收公共锚点、新分组右端、内部缺口、键盘 focus 和鼠标 hover 的 tooltip 边界，并查看 320px 中英文代表截图。仅访问 `127.0.0.1` 演示 API，测试响应中构造单点/缺口组，未登录生产或调用付费模型。
- **工作区边界核验通过。** `git diff --check`、两个维护 JSON、来源 643 项清单总数、个人 VERSION、起点 HEAD、空暂存区和三份原有未跟踪 usage 测试 SHA256 已核对；三文件保留且未修改。依赖、迁移、Ent schema、部署文件、运行配置及生产账号无变更。

本轮未执行全量测试、race、真实 PostgreSQL 集成、远端 CI 或部署验收；仓储事务验证为 SQL mock，不代替真实数据库的后续发布门禁。已选修复的验证不代表延期模块已经完成适配。

## 个人 v2.1.67 发布准备

用户随后授权提交 GitHub 并更新部署，个人版本推进至 **2.1.67**。独立审查发现导出重试读取了 Axios 原始 `response.status`，而本地 `apiClient` 实际抛出顶层 `status`；已修正并把 fixture 改为真实错误结构，增加经 `adminUsageAPI.list` 和实际拦截器的契约测试。修复后两文件 76 项通过，有效前端覆盖合计 8 文件 192 项，修复后的前端/embed 产物重新构建。其他未变实现的定向回归和浏览器证据复用，复核后无未解决阻断项。PR/主分支 CI、安全检查、自有 Release/GHCR、生产只读预检、备份实际恢复及线上验收记录在 `development/diagnostics/sub2api-release-v2167-20261118/`；此处不预先声明后续门禁已完成。
