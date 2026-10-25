# 二改选择性同步 — 2026-10-25

基于本地 v2.1.35 / main@86f64fd38006372c7f9f210353f9854521bdfedf，审查 kiss-kedaya/sub2api 正式 v2.2.25–v2.2.29，在分支 `sync/fork-v2.2.29-20261025` 选择性适配视频查询和 Anthropic 缓存修复。用户后续授权提交 GitHub 并更新部署，项目 VERSION 递增为 2.1.36；发布 CI、制品身份及线上验收另行记录。

## 来源和检查点

- 原正式检查点 v2.2.24：`7d83b16e7230e33ebee91a591a7da5217cf45be0`；原已审 main：`b7e8d034ec75e3f2a9dc5a902241732f72912219`。
- v2.2.27：`015c196f1714d47d3ee65559046e54bbf09682d3`，视频来源查询修复，主要提交 `9fa60cad5`。
- v2.2.28：`539ad9e471d881f8c1bc833649899309c35c10f0`，缓存转换修复，提交 `a388738ca`、`43abd82d3`、`7286b33bd`、`9d4286738`。
- 最新正式 v2.2.29：`6add01cc1cbc2b89cf557aab1154503aa07cd297`；已审 main：`63c1a4e66fe79b780f81bec3053efb6df8140130`（仅 VERSION 同步）。
- GitHub releases API 已确认这些版本非 draft/prerelease。运行时继续仅跟踪正式 releases；检查点代表已审和选择性适配，不代表全量合并。

## 视频任务查询

区分任务不存在、来源永久失效和临时故障。任务属于当前 user 与 API key 后，原 Grok 账号被删除或改成其他平台返回 410 `video_task_unavailable`；账号、绑定或账单快照临时读取失败返回 503 并提供 `Retry-After: 2`；不存在或归属不符继续返回固定 404。

本地进一步优先读取当前账号仓库，避免调度缓存尚未失效时仍用创建时的凭据；仓库失败不回退旧账号。legacy 智能路由同样保留 404/410 错误分类。查明任务归属后保留原始账号的运维归因；失败分支不重建视频、不查询其他账号、不消耗结算 claim，也不调整历史余额。既有持久化任务和幂等结算保持不变。

## Anthropic 缓存转换

适配 Chat Completions / Responses 转 Anthropic 时的缓存标记传播，包括请求级自动缓存、system 分块、消息尾、工具、工具调用和结构化工具结果。工具结果内部标记归并到该 `tool_result` 外层；普通 OpenAI Responses 转换过滤 Anthropic 扩展。

独立审查发现源实现同样遗漏文件型工具结果，本地补齐内联 PDF、UTF-8 文本文档及混合文本的转换，保留附件内容、标题和缓存标记。不能共享的上游文件 ID、不支持的文件格式或损坏内容在转发前明确报错；不抓取外部文件。

保留本地转换顺序和配置：先使用可配置的 `min_output_tokens`，再写入最终映射模型，最后转换到 Anthropic，继续按最终模型执行 thinking 处理。保留 signed-thinking 与本地 usage 字段，不照搬无关 reasoning 重构。

转换路径遵循既有密钥级/全局缓存 TTL 策略；原生 Anthropic builder 统一执行断点配额、TTL 顺序及所需 1h beta。顶层自动缓存占用一个断点槽，最终显式 1h 标记与自动缓存目标一致时协调 TTL，防止上游拒绝。

## 不引入

- v2.2.25、v2.2.26、v2.2.29 的链上充值扫描、RPC、Gas、归集、Bark 与支付临时授权，属于本项目已移除的原生支付范围。
- 配套 crypto grant 的登出/会话撤销、审计头和前端不单独引入。
- 不恢复工单、无限画布、原生支付或渠道定价；不更换本地 shadcn 界面。
- 不修改已应用迁移、模块依赖或公开原仓库检查点；本项目发布版本单独递增。

## 验证

- 主代理使用 Go 1.27.3：`go test -json -tags unit ./internal/pkg/apicompat -count=1 -timeout=10m` 全包 780 个测试/子测试通过（含审查后新增的文件工具结果16场景）。
- 文件修复后 service/handler 定向扩展回归 2277 个测试/子测试通过，覆盖 Grok 视频、计费幂等和归属、Anthropic native/converted、缓存和 TTL、模型映射/thinking、上游检查点和更新提醒。两组合计3057项，无跳过、无失败；不与首轮或子代理重复运行数量相加。
- `go build -tags embed -o NUL ./cmd/server` 通过；未在项目保存新的发行二进制或打包文件。
- `gofmt -l` 改动 Go 文件无输出，`git diff --check` 通过。仓库敏感信息扫描4888个 tracked 文件通过，另用同一 scanner 检查5个新增文件，无发现。文件修复后再次检查全部21个改动/新增文件，无发现。
- 视频范围独立 deep_auditor 未确认缺陷；缓存独立 reviewer 发现的文件型工具结果丢失已修复并增加回归，独立狭窄复核确认修复且未确认新缺陷。
- 主代理复核实际 diff 与关键代码；共享 Git 数据和历史 backing 工作树未提交状态保持不变。
- 原始验证证据保留在本地 `development/diagnostics/fork-v2229-20261025/`；实现阶段定向日志在 `.artifacts/fork-v2229-20261025/`。

测试使用本地 mock/fixture，不调用真实付费模型接口，不操作生产数据库或服务器。未运行完整后端全包、真实 PostgreSQL/Redis 集成、Go race 或 Linux 发布 CI；本轮无前端源码改动，未重复前端全量构建/浏览器验证。

现有上游聚合缓存写入 usage 的默认 5m 分类及账号/全局 usage override 属于既有计费策略，本轮不根据请求 TTL 猜测改价；上游提供真实 5m/1h 明细时继续保留并使用。没有明细时不宣称可准确恢复混合缓存时长。
