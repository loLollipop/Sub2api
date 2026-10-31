# 二改正式 v2.2.61–v2.2.62 选择性同步（2026-10-31）

## 来源与范围

- 本地起点：已发布个人 v2.1.43，`c1910d0c3e707986395f07b7f1a02e58db1b3f9b`。
- 二改旧正式检查点：v2.2.60，`1edc1d3ed6f97981e7022fd3db3c5d1f396169ce`。
- v2.2.61 正式 tag：`efea7f20d164d3f62b527caf8694adee740ab049`。
- 本次目标 v2.2.62 正式 tag：`b459e8662bc2c7cfeb6cc70a7610570c59a6e20a`。观察 main：`824414ca18d5e98a1b1f09e0ecbc8e74a357e5ee`。

Release API 确认为非草稿、非预发布，tag 以独立远端 release refs 核验 annotated tag 的实际 commit。main 比正式 tag 只多 VERSION 同步；不将未发布分支作为行为检查点。全增量为五个文件、两个行为修复，其余是版本及合并记录。原版检查点不变。

初始同步仅保留本地，分支 `sync/upstream-v2262-20261031`，当时个人 VERSION 保持 2.1.43。用户随后授权代码审查、提交 GitHub、部署及本地缓存清理；本次准备个人 v2.1.44，最终 CI、制品身份和生产验收证据另存发布目录。

## 已有覆盖

`fc624b76917908362cf07dd00e1811374c83a322`（v2.2.61）在错误日志列表的上游请求 ID SQL 表达式末尾加入空字符串，避免扫描 SQL NULL 到 Go string 时整页报错。

本地 `ops_repo.go` 已在上一轮同步中提供该兜底，并包含响应头优先、Grok `xai-request-id`、`x-oneapi-request-id` 和请求头回退。现有 `TestListErrorLogsUpstreamRequestIDQueryAndScan` 检查实际查询中的候选顺序、末尾空串，以及空 ID/非空 ID 的扫描结果。因此不重复覆盖实现或缩减本地候选。

这里的 sqlmock 用例验证 SQL 文本与 Go 扫描，不代替 PostgreSQL 对 JSONB/COALESCE 的实际执行验证；本轮不声称运行了真实数据库集成。

## 采用与适配

`b3ad123c16a0f24aa38849392141aceeaf763bf1`（v2.2.62）修复已有掺水质量检测的状态识别：

- `queued/running` 继续保留检测 ID 并延迟轮询；`done/partial/error/failed` 识别为可处理终态。未知或非字符串状态仍拒绝，不扩大协议接受范围。
- `done/partial` 使用已有停止条件和配置摘要判断；部分探针失败不等于整份报告失效。相关证据不足、探针失败或配置变化时仍返回 `unknown`，不据此恢复账号。
- `error/failed` 保存脱敏的终态报告并结束本轮，结果为 `unknown`，不标记检测成功，也不根据其中可能残留的 verdict 停用或恢复账号。这是对源实现保留 `completed` 的小幅安全适配。
- 创建响应也接受上述终态，先持久化检测 ID，后续沿用既有 GET 获取最终报告，不在本轮重复创建检测。保留同源轮询、发送前重读同意、提交意图与不确定接受时禁止重提的原有保护。
- helper 按“已识别状态”命名，避免把包含 `queued/running` 的集合误称为全部终态；不采用上游对外部 JSON 字段直接强制断言 string 的写法。

仅修复已存在且默认关闭的质量模块，不引入延期的运维、采集、定时器扩展或任何新模块，不打开计划。报告先持久化再应用按计划所有权的停调/恢复及清理检测状态的顺序保持原样。

## 验证

- 新增终态回归在旧实现上复现 **11 个子项失败**；包含父测试的日志为 13 fail、1 pass。修复后新增用例全部通过。
- `go test -p 2 -tags unit ./internal/chanshui ./internal/service ./internal/repository ./internal/handler/admin -run 'Test(.*Chanshui|Scheduled|UpdateSource|UpdateTracking|ListErrorLogsUpstreamRequestID|DefaultFingerprint|CustomStop|ConfigurationAndPoll|RetryAfterAndRedaction)' -count=1 -timeout=5m -json`：**160 项 tests/subtests 通过，0 失败、0 跳过**，四个包均通过。包括质量默认关闭、每次外发前重读同意、逐计划停调所有权、报告脱敏、状态持久化失败、上游 ID 查询及更新提示检查点回归。
- Go 1.27.3 普通服务端 `go build -p 2 -o ... ./cmd/server` 通过；这是本地非 embed 构建，不是 Linux 发布制品。前端未变，不重新生成已清理的前端资源。
- 三个变动 Go 文件的 `gofmt -l` 为空，`git diff --check` 通过；仓库敏感信息扫描覆盖 4,986 个跟踪文件并通过。
- 独立只读状态机与安全审查未发现本轮引入的问题。需要精确保留既有策略语义：必要 verdict 证据缺失或相关 probe 明确失败时不恢复；仅缺少 probes 元数据，不自动推翻有效 verdict。
- 4,986 个跟踪文件逐文件 SHA256 核对通过：除五个预期跟踪修改和新增同步记录外，没有范围外变化；既有三个未跟踪测试哈希、个人 VERSION、迁移、Ent 与依赖文件保持不变，HEAD 未变。

所有 HTTP 测试使用 fake transport 与虚构凭证，不请求实际检测服务或收费模型。未运行全后端套件、Linux race、真实 PostgreSQL/Redis 集成、生产检查或部署。本机首次测试从清理后的冷 Go 缓存重建；中止了不必要的独立 trimpath 编译，最终普通服务端构建通过，未把中止命令当作成功。

保留本地 shadcn 界面、模型分组、本地计费、冻结/显式零价、Grok 拒绝收费及会话封禁、推理隔离和断连排空；没有数据库迁移、Ent、依赖、前端或个人版本变更。原有三个未跟踪测试与历史工作树不属于本轮修改范围。

证据位于工作区 `development/diagnostics/upstream-v2262-20261031/`：`baseline.json`、正式 release JSON、完整增量、`backend-before.jsonl`、`backend-final.jsonl` 及对应 summary、`backend-build.log`、`preservation.json`、`sensitive-scan.log` 和独立审查记录。不提交环境证据或构建产物；验证用可执行文件 `server-validation.exe` 留在该目录并记录哈希，供后续统一清理。

发布阶段仅追加个人 VERSION 及本版发行说明；不扩展上述同步范围。既有本地检查结果为同步阶段证据，v2.1.44 的最终提交、PR/main CI、Release/GHCR 二进制一致性、备份实际恢复、受控部署及缓存清理，以 `development/diagnostics/sub2api-release-v2144-20261031/` 及对应清理记录为准。
