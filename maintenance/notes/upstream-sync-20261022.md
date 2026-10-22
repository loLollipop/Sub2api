# 双上游选择性同步 — 2026-10-22

本轮仅在本地选择性适配有用更新；没有合并上游历史、修改个人 VERSION、提交、推送或部署。开始时的 470 个已有修改路径已在开发诊断目录保存快照。保留 shadcn 风格前端、工单/无限画布/原生支付移除，以及链动小铺和兑换码入口。

## 来源与检查点

| 来源 | 已审查范围 | 正式发布检查点 |
| --- | --- | --- |
| [原版](https://github.com/Wei-Shaw/sub2api) | `b0a02a3304a0a78cdd003ddda1dd23ba02c03084..dba770d56a0ee0ae5bad1b3dc5e11614d323163c` | 仍为 v0.2.17，`060d0160417782cd962a0c7f460f6cd4565d4916` |
| [二改](https://github.com/kiss-kedaya/sub2api) | `6c624219955f467114edb5ae91dc1e3904c1c878..b60f6160135f90c708f58a4cf5d9228d6eced218` | v2.2.18，`b60f6160135f90c708f58a4cf5d9228d6eced218` |

观测到二改 main 为 `450ca5b9ea2e2ecacfd5daf8fa9e760805f870e6`，相比正式 tag 只有版本同步。原版 main 的新增代码审查与正式 release 检查点分开记录；运行时仍仅监控正式发布。这些检查点表示差异已审查，不表示全量合并。

## 采纳与本地适配

### 安全、流处理和用量

- 二改 `0bec57406`：管理员关闭本人 TOTP 必须提供当前动态码；原子绑定 secret/enabled，禁用时校验角色、secret、有效状态；管理员降级及关闭全局 TOTP/step-up 走敏感操作门控。省略或提交未变化的安全开关不能回滚并发更新。保留最后管理员、通知字段和局部更新规则。前端同步动态码输入与重复提交保护。
- 二改 `58338b4da`、`6ce8f3c6f`、`b60f61601`：Anthropic 流错误按 payload 判定，显式整数 400–599 优先，`api_error/server_error` 为 500；已输出与否不改变错误含义。Ops 在确认匹配当前终止错误时保留发送端状态，显式终止状态优先，不能借用旧账号尝试或 WebSocket 轮次。独立审查另发现新 Chat 兼容错误帧缺少终止类型，补齐顶层 `type:error`，避免 Ops 将真实失败记为 200，并以真实转换器和 Ops 中间件联合回归。
- 原版 `94c7d6b02`、`4355236e0`：Anthropic Chat/Responses 兼容流在客户端断开后停止写入及 flush，继续读取上游以收集用量。保留财务错误 reader 和行大小限制；按配置限制读间隔，未配置时断开后启用 180 秒空闲上限。缺少终止事件、超长行和异常读错不能补成成功终止。
- 原版 `2b1f12938`：Gemini 取消时保留中断前观测用量；丢弃非 EOF 错误附带的残行。原生图片只在写出成功后计入已交付图片，不把未交付图片当作可计费输出。保留本地 PricingAt、Messages 补扣和取消上下文规则。
- 原版 `9c41b3269`：Gemini/Antigravity 输入 token 扣除缓存后下限为零，缓存用量仍独立保留。
- 原版 `ab71e12ec`：异步图片下载限制 URL 协议、凭据、私网主机、实际直连 IP 和每次重定向；走代理时额外检查本地 DNS。声明与内容嗅探均非图片时拒绝转存。代理最终 DNS/出口属于受信代理边界，不能声称本地检查解决代理侧 DNS rebinding。

### 模型、权限和价格

- 原版 `47cb84b8b`：Gemini 生图按分组权限在选号前和账号映射后校验，包括请求 IMAGE 模态及图片模型别名；countTokens 不受生成门禁影响。拒绝为本地 400，不换号、不处罚账号。新建 Gemini/Antigravity 分组默认启用，Composite 保持原默认值。
- 本地独立审查发现原版检查位置不足以覆盖当前定制版的 OpenAI 兼容分流和 composite 模型改写，已补齐客户端原模型及最终账号模型检查。Messages 的两个协议转换分支在推理档位后缀归一及最终映射完成后再次校验，覆盖 `gpt-5.4-xhigh→gpt-5.4→gemini-3-pro-image`。所有新增服务层图片门禁返回同一可识别的本地拒绝错误，会话/用户 affinity 不记为上游失败；真实上游失败仍清理失效偏好。对应兼容路由及缓存副作用回归随本轮修复保留。
- 原版 `c7bfcd93a`：工具 schema 按节点语义清理，不删除合法同名属性或改动注释/default 对象；保留本地整数 exclusiveMinimum 转换与 mixed scalar enum 行为。
- 原版 `e0fa4b94f`：结构化 Cloudflare 1010/browser_signature_banned 及限定错误字段中的 1010 文本不再误罚各平台账号。不能把任意 request/tool 回显或普通权限错误当成 Cloudflare 豁免；原有较宽 WAF 规则仍限制在原平台范围。
- 原版 `e068c3d02`、二改 `a308f2586` 的前奏部分：`response.queued`、`response.metadata` 不算有效首输出，保留 HTTP/WebSocket 的输出承诺与重试边界。
- 原版 `1cdecf4a8`：补齐 Grok 4.7 Fast 价卡及 provider 前缀匹配；输入达到 200k 后使用整次长上下文价格。保留已有价卡布局、显式零价格及本地 0.15 结算。

### 额度、性能和前端

- 原版 `d6120b5d8`：Vertex service_account 配额贯通 DTO、调度、结算、通知与管理前端；账号池模式仍保持原类型限制。删除账号跳过账号计数但不撤销用户扣费；没有实际增量不通知。保留本地 Cloudflare 额度表单及 Vertex 实时用量。
- 原版 `67dca9687`：按日/周维度选择四条固定配额 SQL；维度提示不符时回退完整语句，不进入幂等指纹。保留事务、去重和 outbox 边界。
- 原版 `38b72d734`：模型限流快照对 Extra 和嵌套 limits 写时复制，避免污染共享快照。
- 原版 `d5dbb5c36`：单 Antigravity 账号检查延迟到需要时执行，并在请求内缓存结果；保留本地调度模式，上游 WithoutCancel 检查额外设置两秒截止时间。
- 原版 `7fcaa32db`：管理账号页记住合法的平台/状态筛选；坏 JSON、未知值、storage 异常均容忍，不持久化搜索内容，保留当前前端样式。

## 已覆盖与排除

| 变更 | 决定 |
| --- | --- |
| `89654873f` 映射签名热路径 | 本地顺序无关签名与 atomic memo 已等效，避免重复改写。 |
| `cacc80b86`、`4c940c3cb`、`59100d324` | 当前依赖、Go 1.27.3、CI/Docker 入口已覆盖。 |
| `53002f677` scheduler snapshot memo | 暂缓：与本地账号 atomic memo/共享 map 修改边界不同，需要 mutator、Redis 和 race 验证。 |
| `afbef7c8c` DeepSeek 重复图片 | 本地没有源变更针对的旧 alias helper，不引入无关输入归一化子系统。 |
| 二改 `a308f2586` 额外收费/封禁 | 排除基础设施 IP 错误及 HTTP 200 内嵌 403 的收费、永久封禁；保留本地已有原子收费及明确拒绝语义。 |
| 二改 `0bec57406` 可选离线恢复 CLI | 暂缓：独立审查确认，清理 refresh token 集合与在途轮转有竞争，旧请求仍可能签发新会话；不在本轮新增会话代次架构。 |
| 二改 `3940f16a5`、版本与赞助文档 | 无本地对应实现或与本轮功能无关，不移入。 |

## 迁移和数据边界

仅追加 `269_enable_gemini_image_generation_groups.sql`，将源仓库编号 246 的行为适配到本地下一空闲编号，绝不覆盖已存在的 246 或其他已应用迁移。迁移为既有 Gemini/Antigravity 分组启用该图片权限字段，与升级前行为一致；Composite 不回填。

所有数据库验证使用本轮专属回环 PostgreSQL 17.6，未连接生产数据库，未调用真实付费上游。没有恢复已移除模块。部署前仍需按现有流程备份、迁移排练和受控发布。

## 验证记录

日志目录：`development/diagnostics/upstream-sync-20261022/`（开发工作区外层目录，不属于发布源码）。

- 前端全量：363 文件、2994 测试通过；ESLint、vue-tsc、Vite 生产构建、控制台 CSS 资源检查通过。
- 管理员 TOTP 安全定向六包通过，覆盖 service、handler/admin、repository、middleware 和离线 integration。
- 网关流处理/Ops/Gemini 取消/下载安全定向 service 和 handler 通过；图片权限/延迟 probe 定向通过。
- 真实 PostgreSQL：15 种配额状态 × 4 条 SQL 的等价性、结算回归通过；16 个唯一结算命令各重复两次并发，余额、额度、去重及单次 crossing outbox 断言通过。通过临时只读 overlay 使用专属 PostgreSQL，未运行依赖 Docker/Redis 的完整 integration harness。
- 敏感资料扫描复用仓库 `scan()` 和原有合成 fixture 哈希，仅排除 Git 已删除路径并加入未忽略新增文件。原始 `--all` 因已删除未暂存文件读取失败，不能记为通过；最终当前工作区扫描 4865 文件无发现。
- 后端全量 `go test -p 3 -tags unit ./... -timeout 360s` 通过；最终新增的 Messages 归一后映射门禁又单独运行 handler/service 相关回归通过。嵌入前端的 web 测试与 `CGO_ENABLED=0 go build -tags embed -trimpath -o NUL ./cmd/server` 通过。
- 独立审查覆盖管理员安全、配额 SQL/前端、图片权限、流处理和下载安全；发现的可选恢复 CLI 撤销竞争已通过排除该子功能处理，其余本轮发现均修复。追加门禁修复经主代理读取实际代码并回归验证。
- 本轮修改的 Go 文件 gofmt、完整工作区 `git diff --check` 通过。原有 470 路径无意外丢失或恢复，main.go 与 VERSION 相对本轮开始保持相同，旧 SQL 迁移无改动；最终增量以开始时快照为基准记录，避免将此前前端改造混入本轮统计。

后端命令及结果见 `backend-final-unit-tests.log`、`final-normalized-model-gates-tests.log`、`ops-compat-cc-failure-tests.log`、`embed-web-tests.log` 和 `embed-server-build.log`。前端结果见 `frontend-all-tests.log`、`frontend-typecheck.log`、`frontend-sync-lint.log`、`frontend-build.log`、`console-assets.log`；数据库结果见 `quota-postgres-tests.log` 和 `quota-concurrency-postgres.log`。保留已修复失败的早期日志，不把它们作为最终成功证据。

race 验证受本机缺少 gcc 的限制，单独记录失败结果，不把普通并发测试表述为 race 通过。专属 PostgreSQL 测试实例已停止；自动审批拒绝删除本轮两处临时目录（返回 `blocked by policy`），保留在 `D:/DevCache/Temp/sub2api-sync-pg-20261022` 与 `D:/DevCache/Temp/sub2api-sync-pg-tools-20261022`。嵌入构建输出至 Windows NUL，无额外本地可执行产物。未提交、未发布、未部署。
