# 二改正式 v2.2.39 选择性同步（2026-10-27）

## 来源与范围

- 本地起点：已发布 v2.1.39，`c77f1e67cee2ee0a909a6767ccb192dc177b7844`。
- 二改 `kiss-kedaya/sub2api` 最新正式发布：`v2.2.39`，`75f65fd910a23af1d503d0b9feb048aa3b4f715a`，发布时间 `2026-10-27T13:52:31Z`。
- 本次观察 main：`bad13f1767ee8791878c9986a0b9b8c7bc1fe6f7`。只选择正式发布中的行为，不按未发布 main 变更推进功能。
- 已审正式范围：v2.2.33、v2.2.35–v2.2.39。v2.2.34 为上游标记不推荐的预发布，不单独同步；其后正式版本中的修复按最终源码审查。

上游重写了 Git 历史。原已审 v2.2.32 `f72c80a72ef5a4fb45772ffd389e3fe58d1485ba` 与新历史 `084790c5523fe810f68c47184a75a324d95d7557` 的 tree 均为 `6733cb20779ba5fad483d2ff72adc255bd31e980`，源码差异为空。本轮新增提交按等价的新历史起点审查，未把重写前的功能误当新改动。

来源标签、发布状态、提交清单和差异记录保存在本地 `development/diagnostics/upstream-v2239-20261027/`。本轮是选择性适配，不代表全量合并上游；原仓库检查点不变。

## 选取的修复

`75f65fd91`：引入客户端错误诊断脱敏工具，覆盖 JSON、SSE、HTTP/WebSocket 错误输出及普通用户的历史错误详情，清除上游地址、代理、账号和凭据等信息。只处理客户端输出副本，内部原文继续用于错误分类、财务/拒绝处理、重试判断和运维记录；成功模型文本与搜索来源 URL 保留。响应头过滤不能通过额外允许列表恢复私密头；人工写回的关联 ID 同样检查，Codex opaque turn-state 保留。

本地适配还修正了来源及相邻调用方的遗漏：错误识别逐一访问重复的 `error/type/status/response` 字段，避免首值或末值掩盖错误；Gemini 列表、账号别名及分组别名在目录投影前保留原始错误信封，最终写出时清洗副本，不把成功模型元数据当作诊断。Anthropic 的实际流式终态识别同样采用该规则。Raw Chat Completions、Embeddings、图片、媒体和 token 计数的 JSON 写出边界接入相同保护；流式和 WebSocket 输出仍保留内部分类原文。人工写回的 Content-Type 按媒体类型和参数分别验证，危险或畸形值回退为协议默认类型，合法 `application/vnd.*+json` 不因点号被误删。媒体二进制内容不进行文本脱敏。

`2b87345eb`、`2849e8d92`、`edf3359ac`：Anthropic 透传流内错误以明确的失败终态结束，而非进入成功路径。保留错误前已观测用量，不消费错误后的伪造用量，不重放请求；已成功发送错误帧时禁止 handler 再追加通用错误。命名 error 事件设 1 MiB 专用上限，普通模型输出继续遵守原有响应限额。

`75f65fd91`：仅 DeepSeek API-key 账号、无代理、连接被拒绝时缩短临时不可调度期至 5 秒。DNS、代理和其他平台的持久故障仍用原有时长；已发出插件请求的禁止重放保护保留。

`b5d42a574`、`2b87345eb` 中的独立 Search/Fetch 切片：

- 省略 model 时，从当前密钥获准使用的 DeepSeek 分组目录挑选模型，优先 flash；不再写死已可能过时的模型名。不扩大到其他平台或未绑定分组，不因冷目录扫描数据库；显式 model 保持不变，仍由调度器验证权限。
- 只有尚未转发请求、且当前候选没有账号时才尝试下一模型；实际请求失败后不跨模型重放。
- 仅 HTTP 200 且 search 有非空 results 数组、fetch 有非空 content 字符串才产生工具费。空结果、畸形/未知结构、201/204 或带错误的结果不产生工具费。成功响应仍原样转发；上游将来若更换成功结构，需要更新可计费识别规则。
- 沿用本地 `SearchOnly`、`SearchCount`、每千次单价和独立 money-event ID，不引入 Hosted Search 字段。工具费倍率固定为 1，余额预留与最终结算一致，显式零价仍免费。普通 token、Codex alpha/search 和附加搜索费的倍率规则保持。

## 延期与排除

- 分组“强制返回请求模型名”：全协议写出层、缓存投影和数据库变更需要单独适配。上游 migration268 与本地工单移除迁移冲突，且其缓存触发器缺少本地倍率谓词，不复制。
- Hosted Search/Fetch：依赖此前延期的分组专用凭据、自动执行和多轮流式状态机，继续整体延期。
- 用量页关联错误徽标：上游按时间和 caller request ID 临时关联 Ops，成功账单可能被邻近的无用量失败请求误标，暂不采用。
- 上游完整公共文档页：包含本地未实现的 Hosted 工具说明并耦合 observer 角色，不覆盖本地主题和现有指南。
- 原生支付私钥导出不适用。工单、无限画布、原生支付、渠道定价界面、observer 角色均不恢复。此前延期的实时钱包 outbox 批处理等保持原决定。

迁移、依赖及前端没有变化。本轮不进行用户已取消的额外中转计费用量审计，不进行付费模型调用。

## 验证与发布状态

最终本地验证（2026-10-28，记录文件沿用本轮开始日期）：

- 使用 Go 1.27.3，`go test -json -p 1 -parallel 1 -tags=unit` 对 `clientprivacy`、`responseheaders`、`service`、`handler` 执行所选行为和相邻协议回归。525 个顶层测试、共 1,367 项 tests/subtests 全部通过，四包均 pass，无失败。完整过滤表达式与统计记录在本地 `verification.json`、`final-regression-v5.jsonl`、`regression-summary-v5.json`；未声称运行全量后端测试。
- `go build -p 1 -o NUL ./cmd/server` 成功，日志 `backend-build-final-v5.log`。本轮前端未改动，此为普通后端构建，未重新生成已清理的前端 dist，也未构建发布镜像。
- 修改的 Go 文件 `gofmt -l` 无输出，`git diff --check` 通过；67 个本轮文件（含未跟踪新增文件）的敏感信息扫描无命中。未扩大敏感扫描豁免列表。
- 补充实际接口回归先在修复前复现四个顶层失败（60 项 tests/subtests），最小修复后新增用例全通过。覆盖 Anthropic 重复错误信封、终态前后用量与不重放，Gemini 原始错误及 native/account/channel alias 列表和详情，Raw CC 与真实 Embeddings 写出。证据为 `adjacent-boundaries-before.jsonl`、`adjacent-boundaries-after.jsonl` 和 `adjacent-boundaries-summary.json`。
- 独立 critical reviewer 确认重复信封、Gemini 投影、Raw CC/Embeddings 等已知阻断关闭；其后发现的 OpenAI 透传流使用脱敏副本解析 usage 已另行动态复现并修复。现在三处 usage 解析均使用预先保留的原始字节及事件类型，managed/passthrough × safe/private diagnostic 四组用例均维持原 11/3 用量，不受重复键重编码为 999/999 的影响。独立 deep auditor 追踪字节所有权并确认该剩余阻断关闭。前后日志为 `privacy-usage-before.log`、`privacy-usage-after.log`；最终 v5 还显式执行了 `TestPluginRequestSentErrorDoesNotFailOver`。
- 本机没有 gcc，未执行 Linux race；新增 CI race 步骤只是后续运行配置，不代表已通过。未执行真实数据库升级、生产验收、付费模型调用。只能提供无返回值 `Flush` 的 writer 无法报告底层 flush 错误；已验证可访问的 `FlushError/Unwrap` 路径。

发布审查还发现 `/v1/responses` 两条流式路径命中自定义错误透传规则后会提前写回原始 message。发布补丁只在最终客户端写出时脱敏，保留规则匹配和内部错误证据；新增 managed/passthrough × safe/private 四组实际流回归，修复前两组 private 用例失败。完整 CI 同时检出了四个旧测试与本次脱敏掩码、Anthropic 失败返回类型不再一致，已更新预期并补强已发送终态、不重放和保留内部记录的断言；另修正一处 staticcheck tagged-switch 提示。后续完整 CI 与部署验收单独记录。

三个此前延期的 `billing_audit_deferred` 未跟踪测试保留原样，不纳入本轮同步或普通 unit。Git 共享数据库和历史工作区未删除。

同步开发阶段仅保留本地，未提交、未推送、未部署，基线发行版本为 v2.1.39。用户随后于 2026-10-28 授权提交 GitHub 并更新部署，本次发行版本设为 v2.1.40；最终发布和上线结果以对应 GitHub Actions 与部署验收记录为准。维护检查点表示已审来源和已记录的选择性适配，不代表全量合并上游。
