# 二改正式 v2.2.42 选择性同步（2026-10-28）

## 来源与范围

- 本地起点：已发布 v2.1.40，`b7e35af65cb1d4c7020030e7b363c7c4db8b72f1`。
- 上次检查点：二改 v2.2.39，`75f65fd910a23af1d503d0b9feb048aa3b4f715a`。
- 本次正式发布：v2.2.40 `f67276ccde69885bdfc3294cfda1724fe1bb8b34`、v2.2.41 `e6f5ec72d37e96984edf1e20c0f4806adcd8e6d3`、v2.2.42 `9aedba768005a6f6c0aa52f78a1fb931fee61587`。
- v2.2.42 发布时间：`2026-10-28T01:15:09Z`；观察 main 为 `0ff60acabffe06a8de2d82f8b0ba657f5551512f`，与正式标签仅有上游 VERSION 差异。

本轮增量为 793 个文件、89,963 行新增和 918 行删除，包含一批新运维子系统。仍按行为审查后选择性适配，不做全量合并。正式 Release 元数据、标签对象与实际提交、增量、基线文件 SHA256 和验证日志保存在本地 `development/diagnostics/upstream-v2242-20261028/`。

## 选取的修复

### 原生 DeepSeek Responses 工具续轮

采用 `f25abe49c4aea37cd8f38f704e81ea006b28d160`、`1afa00814b8ec17c2a7c6f5180b8d7a60a6fab8d`、`a019acb42` 的最终行为。重复分支中的 `c9dafe233`、`bf315f024`、`57b248616` 不重复导入。

客户端在工具续轮仅回传 reasoning item ID、摘要及 opaque 字段时，补回网关实际收到过的完整 `reasoning_text`。不使用摘要构造思考，不关闭 thinking，也不增加上游请求。

缓存绑定鉴权 API key ID、会话及请求模型；每轮重算身份，opaque 值的存在和缺失同样参与绑定。只填补缺失、null 或空 content 数组，不覆盖客户端提供的内容。沿用现有 Redis 接口和七天 TTL，使用独立命名空间及哈希键，不读取旧的无作用域 Chat fallback 项。

同一 key、会话和请求模型允许跨已获准的 DeepSeek 原生账号续轮，保持调度连续性；没有把 account ID 加入缓存键。真实 mapped model 或上游账号间的思考可移植性未通过付费请求验证。不同最终用户若共用同一 API key 和 session/thread，本实现不额外建立用户边界。

读取和写入均有 128 项、2 MiB 及累计两秒预算；Redis 首次写失败后停止该响应的后续写入。只观察已有响应，不修改既有 token 计量、财务、错误分类、断连 drain 和禁止重放逻辑。Forward 接线放在本地 Codex 限制检查通过后，避免被明确拒绝的请求访问缓存。

本地额外收紧普通、无 opaque 绑定 item 的写入：流式成功终态后才提交，失败、未完成或中断尝试不能用重复 ID 覆盖此前成功的内容；有 opaque 绑定的完整 item 保留源实现的快照写入方式。该边界有独立失败/成功终态回归。

独立审查还发现三个源实现/适配边界，均新增测试先复现再修复：JSON 显式错误和 SSE 失败/未完成状态不能冒充成功终态；非流式 SSE 缓存解析复用现有按事件合并多行 data 的解析器，支持仅 `event:` 指定类型；无显式 session/thread 时，仅为此缓存统一字符串 input 与等价用户消息（包含单个 input_text）的锚点表示，不改其他路由种子或原始请求形状。错误识别检查所有重复信封字段，避免重复 error/status 隐藏失败。

本地未启用 Hosted Search/Fetch，删除源文件中仅为内部 Hosted 递归保留作用域的例外及对应测试，测试 fixture 改为自包含 Responses JSON/SSE。没有引入 Hosted 类型占位、自动工具注入或分组专用搜索凭据。

### TypeSafe 和 Gemini 协议修复

从 `d4ad8277ff24d303928cdaf539513071eb8ed773` 选择两个独立修复：

- TypeSafe 地址已经带 `/v1` 时，仅追加 `systemone`，同时保留部署前缀；原来的 origin 地址仍生成 `/v1/systemone`。本地响应解析、禁止重定向带走凭据、计费和审核逻辑保留。
- Google 风格错误将 HTTP 503 标识为 `UNAVAILABLE`，HTTP 状态保持 503；429 仍是 `RESOURCE_EXHAUSTED`，500/502 仍是 `INTERNAL`。

新回归在基线 overlay 上复现 TypeSafe 三种 versioned base 路径失败及 503 映射失败，修复后相关完整小包测试通过。证据：`utilities-before.jsonl`、`utilities-tests.jsonl`。

### Go 依赖

参照 `1b3ee6b947c8266ff6052be6197e912aa12588fb`，通过 `go get golang.org/x/net@v0.62.0` 更新相容依赖集合：x/net 0.62.0、x/crypto 0.58.0、x/sync 0.24.0、x/term 0.47.0、x/sys 0.49.0、x/text 0.43.0、x/tools 0.51.0。保留 Go 1.27.3 及其他本地依赖；`go mod tidy` 只移除对应旧校验和，没有增加新业务模块。

源提交提到的 `tdewolff/parse/v2` HTML parser 不在本地依赖链中，不为修复未使用的依赖而导入它。`govulncheck` 没有发现当前代码调用到的漏洞或所导入包的漏洞；仍提示 x/crypto 模块中未被导入的 openpgp `GO-2026-5932`，没有修复版本。项目现有 `--fail-on-fixable` 检查通过，未豁免漏洞。

## 延期与不适用内容

- v2.2.40 API-key 工具注入开关：依赖仍延期的 Hosted 自动注入，本地独立 Search/Fetch 不受此功能控制，暂不添加无实际作用的字段和 UI。
- v2.2.41 连接时间线、传输量、请求正文采集、ID1 持续诊断：涉及新中间件、持久化、正文保留、2FA、稳定部署槽位及清理进程，作为单独功能适配。v2.2.42 的 JSON attempts 搜索修复只适用于该新采集模块，本地没有对应路径，不单独移入。
- Key 独立并发/队列、Gemini 显式缓存生命周期、账号定时调度/通知和运维控制：需要完整的准入、恢复和数据库适配，不能只拣字段或路由。源迁移编号 269/270 与本地含义不同，Gemini cache 表也已分叉，禁止覆盖已发布迁移。
- 分组缓存测试页面：有诊断价值，但使用真实模型调用和计费，需要单独适配本地 shadcn 页面及明确触发方式，本轮不增加页面或执行付费测试。
- 安全惩罚：与现有审核/自动封禁重叠，源表持久保存完整 prompt，不照搬；受管 Mihomo、凭据巡检、地区网关、账号自动化/OAuth 准入、Excel/BPS、Copilot SDK 和新增视频提供商均作为独立功能延期。
- 邮箱别名拒绝开关：本地已有 alias 归一化去重，保留允许合法邮箱格式注册的行为。
- WebSocket close-error 构造器具体返回类型仅服务新准入代码；本地已有 `errors.As` 保留实际关闭类型，不为未引入模块改动签名。

不恢复工单、无限画布、原生支付、渠道定价界面和 observer 角色；不覆盖本地 shadcn UI。不进行用户已取消的额外中转计费用量审计。此前记录的延期项继续有效。

## 验证与发布状态

- Go 1.27.3 下六个相关包的选择性回归全部通过：531 个顶层测试，合计 1,439 项 tests/subtests。包含真实 Forward/response-handler 路径、原生提供商与目录权限、TypeSafe、独立 DeepSeek tools、错误脱敏、断连用量 drain、WebSocket、Grok/财务保护和更新检查点。命令及结果在 `verification.json`，完整日志 `final-regression-v2.jsonl`；未声称执行全量后端测试。
- DeepSeek 专项共 13 个顶层测试、47 项 tests/subtests 通过。新增三项审查回归在修复前复现 3 个顶层、11 个子项失败；移除新增 hooks 的基线 overlay 还复现了两项真实调用路径、八个子项失败。日志 `audit-boundaries-before.jsonl`、`audit-boundaries-after.jsonl`、`reasoning-before.jsonl`。
- `go build -p 2 -o NUL ./cmd/server` 成功；修改的 Go 文件 gofmt 和 `git diff --check` 通过，16 个本轮文件（包括新增未跟踪文件）的敏感信息扫描无发现。
- `go mod tidy -diff` 无变化；`govulncheck` 及项目已有模块级可修复漏洞检查通过，唯一未调用的 openpgp 模块告警如上所述。独立审计和后续收口核验确认上述三项阻断均已关闭。
- 基线全部 4,950 个跟踪文件和三个历史未跟踪文件逐一核验：前端、全部既有迁移、Ent、发行 VERSION、延期测试及 HEAD 未改变，差异仅为本轮允许范围。

本轮验证结果以本地 `verification.json` 和相关日志为准；Linux race 尚未在本机执行，新增 CI 检查不等于已通过。未变更前端或数据库迁移，不需要生成前端 dist；未执行真实数据库升级、后台登录、生产变更、真实 Redis 集成或付费模型调用。

仅保留本地。没有 commit、push 或部署；本项目 VERSION 保持 v2.1.40，现网继续运行已验收的 v2.1.40。三个此前延期的 `billing_audit_deferred` 未跟踪测试及共享 Git 数据库保持原样。

用户随后授权提交 GitHub 并更新部署。本次发行版本设为 v2.1.41；运维和采集等上述延期模块继续不做。最终发布、CI、制品身份与现网验收以独立发布记录为准。

发布前额外核验发现，现有共享 go-redis 客户端未开启 ContextTimeoutEnabled，新增缓存 context 的两秒预算不能保证覆盖 socket I/O。此次为 reasoning 读写接入独立、由 Wire 和退出清理管理的 Redis 连接池，在创建前开启 deadline 支持并取消空闲预热；其他缓存、计费与锁继续使用原共享客户端。TLS 故障测试确认调用方按预算返回；go-redis 内部后台 TLS 拨号仍可能继续到配置的 DialTimeout，未声称握手同步终止，也没有应用层异步写入。新增真实 go-redis 与本地卡住 socket 的握手、读、写、TLS 和连接池等待回归；最终结果及修复前复现见本地发布证据。没有异步遗留写入或修改共享 Options。
