# 二改正式 v2.3.5 检查点同步（2026-11-03）

## 结论

v2.3.5 的业务修复只涉及尚未引入的可调度定时开关模块，另两项改动稳定同样延期的 request-capture 诊断测试。本轮没有适用于本地的业务补丁；更新审查记录和后台更新检测检查点，不引入这些模块。用户随后授权审查后提交 GitHub 并更新部署，个人 VERSION 准备为 v2.1.49。

## 来源

- 本地起点：个人 v2.1.48，`f90eaddbf3ec36104eb1a21a6bcaab785860328c`。
- 旧检查点：正式 v2.3.4，`65db90acee6ea7983611ef1b9e248cf8c7663235`。
- 目标：[正式 v2.3.5](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.5)，`51748099a2b82dbb16eb8f080cf92efe4a2722b2`；GitHub API 确认非草稿、非预发布，发布时间 `2026-11-03T04:36:17Z`（北京时间 12:36:17）。精确抓取上游 tag 并解引用至提交，未以移动 main 或 tag 对象 ID 代替。
- [完整差异](https://github.com/kiss-kedaya/sub2api/compare/65db90acee6ea7983611ef1b9e248cf8c7663235...51748099a2b82dbb16eb8f080cf92efe4a2722b2)：四个提交、六个路径，+51/-9。
- 观察到 main 为 `a0e9e0f3493667963319cf79bfb64592ea043cee`，正式 tag 后另有四个提交，涉及 VERSION 记账、SSR/BFF HA 模板和 CI；不计入本轮正式检查点。

## 六个路径的处理

### 可调度定时开关：继续随模块延期

`2ef7fa0b3d430cb572b46163c33dbbf617e40c73` 修改源 `AccountsView.vue`，新增 `schedulableTimerState.ts` 及其测试，共三个路径。源实现只有在定时状态明确为 false 时才跳过取消计划确认；正在加载、读取失败或已存在计划时均要求确认，覆盖单账号和批量开关。

本地没有 `timerScheduleState`、定时开关面板或对应状态/取消接口。已沿前端单个和批量开关、accounts API、管理路由、handler 与 service 核查：本地直接更新 schedulable，单个入口为 `SetAccountSchedulable`，不取消源实现中的周期或单次计划。因此不新增无消费者的工具函数，也不为不存在的取消行为添加确认框。

本地现有 `ScheduledTestsPanel` 和 scheduled-test-plans 属于独立质量检测，不是源可调度定时开关模块，保持原功能。将来若引入该定时模块，需要同时纳入未知状态确认修复及其回归。

### request-capture 诊断：继续随模块延期

`c2d172150660e4416e8a49b80bbcff9ac2b09910` 和 `51748099a2b82dbb16eb8f080cf92efe4a2722b2` 仅修改 `backend/internal/requestcapture/id1_diagnostic_test.go`，共一个路径。改动等待异步诊断任务进入 running 状态，并排空前一请求后再建立 WebSocket 会话，避免测试与 worker 竞争。

本地没有 `backend/internal/requestcapture` 包、诊断任务管理器及其测试，保持既有延期边界；未来引入模块时应采用这两个测试修复。

### 源 VERSION 和发行说明：不覆盖个人版本

其余两个路径是 `82cef8149b35ff0aab0816d35fcd24ca6fd54f41` 的 `backend/cmd/server/VERSION` 记账及源 `docs/release-notes/v2.3.5.md`。正式 tag 内的源 VERSION 仍为 2.3.4，后续 main 才写入 2.3.5；本轮检查点按正式 Release/tag 的精确提交确定，不用该文件替换个人 VERSION，不复制源安装、SSR 部署与回滚说明。

## 本地改动

- `maintenance/upstream.lock.json`：推进正式审查/观察检查点，单独记录“已审查、无适用业务改动”及模块延期理由，保留历史采用记录。
- `backend/internal/service/update_source_baselines.go`：与锁文件同步 v2.3.5 及完整提交 ID；编译后的上游检测不再将已审查的同一正式 Release 视为待审查更新，后续正式发布仍会提示。
- `LOCAL_FORK.md` 与本记录：记录本轮选择与实际范围。

无数据库迁移、Ent、依赖、部署配置或界面改动；本地计费、原生协议及推理隔离保持现有实现。本轮在 `sync/upstream-v2305-20261103` 分支工作；发布元数据追加个人 VERSION 2.1.49 和对应发行说明。沿用 PR/CI、自有 GHCR 和受控更新流程，最终发布及线上验收另存证据。

## 验证

- 使用现有 Go 1.27.3 执行 `go test -tags=unit ./internal/service -run '^(TestUpdate|TestNormalizeUpdate|TestOrchestratedRollback)' -json`：170 项 tests/subtests 通过，0 失败、0 跳过。其中更新检测、正式检查点、缓存身份与更新/回滚保护相关 56 项；较宽的 TestUpdate 前缀也执行了现有账号/用户更新测试。
- 检查点与维护锁文件一致性、正式 Release 而非 main 的提示策略、已审查同版不再提示、后续发布提示、缓存失效及禁止直接安装上游制品的现有回归均通过。
- 锁文件 JSON 解析、`gofmt -l` 和 `git diff --check` 通过；初轮差异为上述四个文件，发布阶段另加两份版本/发行说明文件。三份既有 untracked 用量测试不属于本轮改动。
- 初轮未运行全量前后端测试、界面渲染或生产验收；本轮只改审查/更新检测元数据，没有界面或业务补丁。后续完整 CI、制品核验及生产验收作为发布门禁，结果另存发布证据。

原始差异与测试 JSONL 保存在本地忽略目录 `.artifacts/upstream-v2305-20261103/`。
