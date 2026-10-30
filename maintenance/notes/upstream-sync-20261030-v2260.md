# 二改正式 v2.2.60 选择性同步（2026-10-30）

## 来源与范围

- 本地起点：已发布个人 v2.1.42，`020b77873e0662a34f0169c4a40f883cd3f1b521`。
- 二改旧正式检查点：v2.2.53，`636250da69e9b9d66a9abf9945292e9ccfd76441`。
- 本次正式目标：v2.2.60，`1edc1d3ed6f97981e7022fd3db3c5d1f396169ce`。观察 main：`b4dc02d5296195a230e21d04edbfee5d55753157`。
- 原版 main/正式检查点维持既有记录，本轮不扩展原版同步范围。

采用独立远端 release refs 核验 annotated tag 的 commit。v2.2.60 tag 中源 VERSION 仍写 2.2.59，main 仅补版本号，没有行为增量；检查点使用正式 tag 身份，不照搬源 VERSION。完整增量和逐文件基线在工作区 `development/diagnostics/upstream-v2260-20261030/`，不提交运行环境数据。

本轮仅本地同步，个人 VERSION 保持 2.1.42，未提交、push、发版或部署。保留本地橙色 shadcn 界面、按分组模型广场、0.15 计费、冻结价格与显式零价、Grok 拒绝收费及永久会话封禁的原子路径、最终模型映射、断连排空及禁止工具执行后重放。没有迁移、Ent、Go 或前端依赖变动；不恢复工单、无限画布、原生支付、渠道定价、Mixed 或 observer。

## 采用与适配

| 上游提交 | 本地行为与边界 |
| --- | --- |
| `eb1b01a09`、`f777c408d`、`18393acde` | 提炼独立 Responses 缺失工具结果修复，不引入 Hosted 模块。只在 HTTP Forward 最终原生 Responses 的 OpenAI/DeepSeek、非 compact 分支生效。实际保留服务端 continuation 时跳过；DeepSeek 和会删除 previous_response_id 的旧 OAuth HTTP 按无状态完整历史补全。按工具类型和字符串关联字段配对，已有旧结果不能回答后续同 ID 调用。原 item 字节、namespace、未知字段、大整数和媒体输出保留；明确返回 tool_output_missing，不执行工具、不增加请求，不改消息角色。 |
| `4037a5a03`、`6b3ff957e` 的 endpoint 子项 | 接入地址可分别设置最低累计充值可见门槛；缺省继续使用历史 1000，显式 0 全用户可见。后台验证有限数值及 0–10000000 范围，前端沿用本地设置布局；不导入 Ops 告警编辑器或支付功能。 |
| `a641da508` | 移动端用量详情支持点击打开、关闭和可滚动浮层，桌面位置限制在视口内；保留本地图片计费及分项成本展示。 |
| `a57ab11ea` | 既有错误日志加入客户端/上游请求 ID。直接从已有 header JSONB 快照派生，响应头优先 x-request-id、xai-request-id、x-oneapi-request-id，再回退请求头；无 schema 变更，不引入运维中心。 |

独立审查纠正上游统一输出格式的两个问题：原生 `local_shell_call_output` 使用 `id` 关联调用的 `call_id`，完整真实输出原样保留，缺失时生成合法 `id`；原生 `tool_search_output` 必需 `tools` 数组，不能用通用 `output` 字符串替代。本轮不伪造工具搜索成功或空目录，缺失的原生 search 输出保留原有明确错误；已经由既有适配器转为 function 的历史仍走 function 配对。专用原生形状 Forward 回归验证这些边界。真实入站 WebSocket `ProxyResponsesWebSocketFromClient` 不经过 HTTP Forward，本次没有扩张该路径的请求改写。

透传链补全放在既有 client-tool lowering 之后，避免生成的原生 shell 输出又被转换并丢失关联，也覆盖转换为 function 的 search 缺失结果。共享 lowering 对真实 shell 输出先把原生 `id` 转为 `call_id`，已有显式 `call_id` 优先，再做 item ID 清理；真实输出不替换。补充带 `local_shell`/`tool_search` 工具声明的真实 Forward 回归和转换器专项，确保 mapping、单次请求和配对均保留。

## 不采用与延期

- `af159e739`、`e8a629c91`、`01f23893c`、`b27cf7545`：缓存 miss、故障或 opaque 不匹配时删除 reasoning 并把真实 assistant 改成 user，会改变历史语义；省略 type 的标准消息状态机及本地 effort:none 前置过滤也存在冲突。不采用这些降级，保留现有明确错误及原历史。
- 不覆盖 `deepseek_native_reasoning_replay.go`。保留 API key + conversation + requested model 作用域、opaque 摘要绑定及非成功响应不得污染缓存的保护；不退回上游 key+item-only 缓存。
- `4b27cd084`、`bb6562cc7`：仅适用于未引入的 Hosted 合成历史。不能仅按 `[Previous ` 文本前缀改真实消息角色，继续延期。
- `1a5b47dba`：未知视频任务 404/410 不能证明上游未执行或未收费，不自动退款；维持既有 reconciliation 及冻结价格路径。
- `87a093d86`、`cca1effd0`：Terra 新能力整体延期，避免在既有正式客户端能力门控之外引入半套模型支持。
- `c0ae692f7`、`817ecf96c`、`b66c94701`：价格对比 UI 不引入；本地模型广场已使用实收价格，上游再次乘倍率会双乘且不能保留显式零价。
- `d47f4ab63` 运维中心、`2797deda5` 定时器、采集及通知继续延期。`6b3ff957e` 只采用 endpoint 子项，不导入无关告警和 sidebar。
- 工单 `e92c08fa3`、`d6eac651c`、`52d7ec34c` 不恢复；全站 Chanshui 主题 `351ad0cca`、`81d6a180c` 不覆盖本地 UI。
- `5d6328fba` 的 vue-i18n 9.x 安全豁免不适用，本地已升级 11.4.16；不新增豁免。版本流水、merge 拓扑和上游发行流程不导入。

## 验证

- 最终后端日志按包和测试名去重：**1,141 个顶层测试、2,125 项 tests/subtests 全部通过**。覆盖完整 apicompat 包，以及 service、handler、admin、repository 受影响专项；包含真实 Forward 的原生/转换工具配对、OAuth/API-key/compact 边界、DeepSeek reasoning 与媒体账务回归。不是全后端测试。
- 前端全量 **373 个文件，3,156 项通过、1 项跳过、0 失败**。前端 typecheck、全量 ESLint、生产构建和 console CSS 资产校验通过；Go embed server 构建通过。
- Chromium 本地演示验证 320px 深色、390px 浅色、1440px 桌面，Token/费用六组浮层均在视口内；底部定位、关闭按钮、Escape、无页面横向溢出及无浏览器错误通过。截图与 `browser-results.json` 保存于本轮诊断目录。
- 独立 UI/settings/ID 审查无待处理问题；协议审查发现的原生输出 schema 和 lowering 顺序问题已修正，并由新的只读审查验收。未把首次 helper 单测通过当作完整链路验收。
- 4,979 个跟踪文件基线逐文件 SHA256 核对通过；未改动文件、691 个受保护文件（迁移、Ent、依赖、VERSION 等）以及 3 个既有未跟踪测试保持原样。HEAD 不变。所有本轮变更/新文件敏感信息扫描、gofmt 和 `git diff --check` 通过；跟踪文件全量敏感扫描通过。

证据：`verification.json`、`backend-protocol-v3.jsonl`、`final-tool-adapter.jsonl`、`backend-final-v2.jsonl` 的端点/ID 专项、`frontend-tests.json`、`frontend-build-final.log`、`backend-build-final.log`、`preservation.json`。保留真实 Forward 修复前失败与阶段验证日志，不当作最终通过。

本机 `pnpm build` 脚本内的嵌套 pnpm 命中了运行环境附带的新版本包管理器，安装检查因无 TTY 中止；没有放开依赖重装。随后按原 build 脚本顺序，分别以 `corepack pnpm` 执行 check:i18n、vue-tsc、Vite，再运行资产校验，全部通过；依赖文件和依赖版本未变。

未运行全后端套件、Linux race、真实 PostgreSQL/Redis 集成、付费模型请求或生产操作。SQL 优先级与 Scan 使用 sqlmock 验证，不声称真实数据库执行验证。仅保留本地分支 `sync/upstream-v2260-20261030`，未 commit、push、发布或部署。

用户随后授权提交 GitHub 并更新部署。本次准备个人发行 v2.1.43，通过 PR 和合并提交 CI 后发布正式制品及自有 GHCR；核验相同二进制身份、备份及隔离恢复后再受控部署。最终发布与生产验收证据另存发布目录，上述延期与排除范围保持不变。

发布前独立审查补充覆盖 OAuth 非透传的原生 shell：Codex transform 规范化调用关联 ID 时，输出的原生 `id` 必须同步规范化并保留，不能按普通 item ID 清理。修正对缺失结果和已有真实结果都生效，真实 output 和扩展字段保留；以实际 Forward 的单次请求验证两种历史。同步阶段的 2.1.42 及保护报告为当时快照，最终 2.1.43 提交、增量验证和制品身份以发布目录的独立证据为准。
