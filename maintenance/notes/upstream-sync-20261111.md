# 双上游选择性同步（2026-11-11）

本轮审查二改正式 v2.3.47 → v2.3.50、原版正式 v0.2.20 → v0.2.21。采用现有运行链路所需的修复，不合并上游历史。工作分支 `sync/dual-upstream-20261111`，起点 `5a2833f09`。同步完成后，用户授权提交 GitHub 并更新部署，个人版本推进至 v2.1.61；按 PR、CI、自有 Release/GHCR 和备份恢复校验流程发布，最终线上身份与验收证据另存。

## 来源

| 来源 | 原检查点 | 新正式检查点 | 审查范围 |
| --- | --- | --- | --- |
| 二改 `kiss-kedaya/sub2api` | `ea59efb6dd61d451b6dac66c4dd0690c3f1b508b`（v2.3.47） | `c21d39aec58d323f75014e276e96f1517b959bb6`（v2.3.50） | 15 commits / 18 paths |
| 原版 `Wei-Shaw/sub2api` | `dab829a4e9b7d1cc307189aea8b0319eb29e29d0`（v0.2.20） | `10ba8f9b4dc2e6023d4a9879ab77b21841babd10`（v0.2.21） | 82 commits / 200 paths |

正式版由 GitHub Releases API 核实，tag 分别 fetch 至 `refs/remotes/upstream/releases/v2.3.50` 和 `refs/remotes/original/releases/v0.2.21`，未覆盖本地已有 tag。原版此前另已审过 `6b4b37db43443b5e88e424fe3c69602cc3810dfb` 的版本记账提交。

本轮仅观察二改 main `cabd7dd5510a45a927f297e544bb9279fbca8b35`、原版 main `530c5e3a4f9c73ca2b0881191d0ef8e0d82c5484`。正式版之后的业务提交未导入；后台来源检查点代表选择性审查进度，不代表完整合并或个人发版。

## 采用与本地适配

| 修复 | 来源提交 | 本地处理 |
| --- | --- | --- |
| Sol 工具请求选择具备 Responses 能力的账号 | 二改 `732318598`（#271） | 在有损协议转换前识别能力不匹配；可调度时换号，耗尽返回本地 400，不记账号健康失败。无调度器直调仍返回既有 400。仅支持本地已有 Sol，Terra 继续延期。 |
| 刷新令牌原子消费 | 原版 `7122efee9` | Redis Lua 原子 GET/DEL，只有一个并发刷新成功，存储故障不发新令牌；保留本地撤权 cutoff 和轮换后的复查。 |
| hydrate 失败释放账号槽位 | `4ac713c4d` | 加载完整账号失败时释放已获取槽位，正常返回仍把释放权交给调用方。 |
| Anthropic 转 Chat/Responses 的账号错误策略 | `090c58f66` | 优先执行账号错误策略，保留本地完整错误分类、财务错误隔离和既有重试约束。 |
| Gemini 非流式正文过滤 thought | `36484727c` | 思考摘要不再混入 Chat/Responses 正文，保留用量与已有思考控制。 |
| Antigravity 并行工具结果媒体排序 | `e2ce0b43d` | 连续输出 functionResponses 后再附加非 multimodal 媒体；Gemini 3+ 媒体继续嵌在所属结果中。 |
| Pro Argon 配额冷却隔离 | `7796a2d57` | Pro Argon 系列独立冷却；Ultra/未知计划沿用 Gemini 共享范围，图片模型边界不变。 |
| 远端定价 hash 与正文不一致后继续同步 | `92d35a694` | 仅在正文实际 hash 与远端一致时推进远端 hash 锚点，避免临时不同步后永不重试。 |
| Responses 工具循环会话 seed 稳定 | `85f43b640` | 从稳定的输入前缀派生，保留本地账号粘性及会话隔离。 |
| Codex 原生图片错误状态 | `cf1ba819b` | 带 `X-Codex-Image-Turn-Id` 的请求不提前发送空白 heartbeat 提交 200，晚到的上游错误保留正确 HTTP 状态。 |
| Codex metadata 的换号边界 | `e401c5805` | `codex.rate_limits` 和 `codex.response.metadata` 作为 preamble；它们不会让尚无模型输出的请求提前失去换号机会。 |
| 显式模型冷却优先于容量重试 | `ebf016808`、`88b7090a3`、`a1395eeaf` | 规则只匹配实际 error 字段，使用已映射模型；保留非 OpenAI 原生平台容量处理与请求范围语义。 |
| sealed function call 元数据 | `f2b0a32b1` | typed output、delta accumulator 及本地白名单 wire serializer 均保留 namespace 和 encrypted_function_args；区分缺省、null、空数组和非空数组。同时修复本地 SSE 转 JSON 后仍保留 SSE Content-Type 的问题。 |
| 忽略外来 CC Switch reasoning envelope | `552d1ab4b` | 增加外来签名前缀过滤，保留本地原生厂商 PreserveThinkingContent 支持。 |
| Responses 非法 UTF-8 | `63f117229` | 在正文转换前拒绝，防止 JSON 解码替换坏字节后语义改变。 |
| Codex Desktop 页面上下文 | `b2a695d61` | 将无 call_id 的合法页面快照按外部用户文本处理，先于现有 automation/delegation 归一化；适配本地 gjson 快速预判，保留重复键校验和大整数。真实工具结果仍须合法 call_id。 |
| OAuth WebSocket 搜索激活 | `142d22cc4` | 首次请求成功写入后记录搜索能力；后续激活搜索前重连，完整保存历史输出；历史缺失时安全关闭，保留 API Key 和已有搜索连接行为。 |
| WebSocket 后续轮次限流禁止重放首帧 | `0578fb160` | 后续轮次及重连握手限流返回当前轮次错误，不把第一条用户消息发给另一账号；首轮仍可换号。 |
| WebSocket TTFT | `fab3ec251` | relay 采用本地配置的首 token 模式，排除 codex.* 带外事件；沿用累计重试计时和真实终态用量。 |
| Messages 合成 effort 与最终校验 | `eb4ccd173`、`6f2faba29`、`aa245176a`、`f6ea845c5` | disabled 合成的 none 参与分组映射和上限，最终出站值再校验；客户端显式无效值仍直接拒绝。保留省略时的默认 medium、GLM 已配置降级和国产原生平台边界；count_tokens 不校验不会出站的 output effort。 |
| 异步安全审计立即唤醒 | `390ba9905` | 仅在发布队列成功后通知本进程 worker，启动先处理遗留任务，回收后通知；500ms 轮询继续覆盖跨实例任务与到期重试。保留本地日志脱敏。 |

## 已覆盖、延期和排除

- 原版 `508bb9560` 的 Sonnet 5.5 cache-read fallback 已是 `0.1e-6`，不重复修改价格。GLM 归一化在分组策略前执行的本地顺序继续保留，用新增 HTTP 矩阵验证最终出站值。
- 原版 `fe093d76b` 的 30m/12h cache-write 用量、计价、UI 和迁移整组延期；`7156213cc` 的 Fast 免费输入修复依赖这套新收费链，随其延期。
- 原版 `c004c962a`、`03bd718ab` 的请求体内存预算及 cgroup 读取整组延期；本地现有资源边界保留。
- Fable/Mythos 5.2 的能力和价格整组延期（`eb2da3448`、`3e8c2ee3f`、合并定价修订），不只摘名称或价格改变既有模型家族语义。
- Keys 异步 30 天统计及覆盖索引（`b8de5e170`）、刷新时保留行和额度（`4a22a75b3`）、Antigravity 强制查询按钮（`4ac1675b5`）、设置保存浮标（`5965ac4e9`）留待前端专项。本地账号自动刷新已有增量合并，手动加载和额度 skeleton 仍有源版所述改进空间；Keys 统计仍与列表共享取消控制器，不能只摘掉 await 而丢掉旧响应隔离。当前批量额度加载、缓存失效、主题与保存同行布局保留。
- 二改 `664e776c1` 针对 Nuxt 设置布局，本地是独立 Vue 设置组件，不套用；不引入其布局重构。模型自动同步尚未启用，`1207efdaa`/`d8843f47e` 一小时默认及测试继续延期。
- 二改 `858cd4da2` 的 sweep、原版 `137316a14`/`edd7db77f` 的原生支付变更对应已退役模块，不恢复。
- Hosted、Ops/capture/timer、自动白名单写入、catalogue、断线估算收费及此前其他延期项保持原决定。原仓库的 VERSION、发布、部署和只面向原版模型目录的图片说明不覆盖本地文件。

## 验证

按包和测试名合并最终有效结果，共 3706 项测试及子测试通过、10 项跳过；保留首次失败日志，修复后重跑受影响路径，输入未变的有效结果直接复用。

| 范围 | 通过 | 结果与证据 |
| --- | --- | --- |
| service 选定回归 | 1865 | 令牌轮换/撤权、调度与粘性、Sol 工具能力、Messages effort/GLM、最终计价、count_tokens、Codex 元数据、图片、定价同步，以及现有 WS 会话/重放/用量；`service-tests.jsonl` + `service-retry-tests.jsonl`。 |
| handler 选定回归 | 359 | 包含 UTF-8、页面上下文、分组 policy、工具请求换号，以及真实本地 WebSocket 首轮换号和后续用户消息/工具结果/重连握手限流；`handler-tests.jsonl`。 |
| repository | 1 | miniredis Lua 并发消费，16 个请求仅一个成功；`repository-tests.jsonl`。 |
| apicompat 全包 | 876 | 包含 CC Switch signature、sealed wire metadata 和既有协议转换；`protocols-tests.jsonl`。 |
| Antigravity 全包 | 312 | 媒体保真、旧模型连续 functionResponses 和 Gemini 3+ 嵌套归属；`antigravity-retry-tests.jsonl`。 |
| openai_ws_v2 全包 | 74 | TTFT callback 和既有双向 relay；`protocols-tests.jsonl`。 |
| securityaudit 全包 | 219 | 启动 drain、发布后唤醒、无阻塞通知和原有 worker/脱敏；`protocols-tests.jsonl`。 |

10 项跳过来自未配置的 OpenAI 实际 API、真实 Redis/PostgreSQL 集成环境。没有执行全量后端、race、线上数据库或部署验收；本轮不新增迁移或依赖，令牌原子性另由 miniredis Lua 和并发服务测试覆盖。

前端 i18n 完整性、`vue-tsc -b`、Vite 生产构建及制品路由校验通过（`frontend-build.log`）。前端源码没有变化，因此未额外运行浏览器交互测试。

后端 `go build -p 1 -tags embed -trimpath -ldflags "-s -w" ./cmd/server` 通过，执行本地制品 `-version` 确认为 2.1.60（`backend-build.log`）。制品仅供本地验证，未发布或运行服务。`gofmt`、维护 JSON 解析及 `git diff --check` 通过。

来源记录与后台常量一致性测试通过；同步阶段个人 VERSION 为 2.1.60，后续获授权发布时推进至 2.1.61。CI 增补刷新令牌原子性、槽位释放、WebSocket 搜索/TTFT、Messages 思考策略和审计唤醒的 race 回归。三份原有未跟踪用量测试 SHA256 与初始记录逐一一致，本轮未执行这些延期测试。本轮未改动依赖、迁移、部署配置、前端源码或其他 worktree。

证据保存在 `.artifacts/dual-upstream-20261111/`，包括初始状态与三份未跟踪测试 SHA256、双方完整 diff、测试选择清单、JSON 测试输出和构建日志。源码、维护锁和后台来源常量需一致通过检查。
