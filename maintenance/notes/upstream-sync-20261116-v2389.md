# 二改 v2.3.89 选择性同步（2026-11-16）

同步阶段完成本地选择性适配及验证，个人版本当时保持 **2.1.65**。用户随后授权提交 GitHub 并更新部署，个人版本推进至 **2.1.66**，按既有 PR、CI、自有 Release/GHCR 与受控部署流程发布。

## 来源与范围

| 项目 | 检查点 |
| --- | --- |
| 二改仓库 | `kiss-kedaya/sub2api` |
| 原正式检查点 | v2.3.76 / `f3956f1a7f31254e47eaf8cbb6cd9d83f2998b1d` |
| 本次正式目标 | [v2.3.89](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.89) / `9f7dc5e798e8054228e31feb6abaeae808d269cd` |
| 正式发布时间 | `2026-11-16T05:47:50Z` |
| 观察到的 main | `cf769dbac37dfc8ddc2de57dd584e4968e065bc3`，仅观察 |
| 增量 | 66 个提交（含 merge），49 个非 merge 提交，96 个变化路径 |
| 原版检查点 | v0.2.21 / `10ba8f9b4dc2e6023d4a9879ab77b21841babd10`，不变 |
| 本地分支 | `sync/fork-20261116-v2389`，起点 `b451bcfcab698993ddef072bb5a7701d681bddcc` |

通过 GitHub Release API 与独立 remote release ref 核对正式版本，不覆盖个人 tag，不合并未发布 main。完整来源 diff、提交清单、发行元数据、初始状态及逐路径分类保存在忽略目录 `.artifacts/fork-sync-20261116/`。96 个路径逐项记录为 **16 项适配、2 项本地已有、41 项延期、37 项排除**；分类按来源路径统计，适配项中的无关子功能仍排除。

## 已适配

| 来源 | 本地行为 |
| --- | --- |
| `045df7029`：注册条款输入门禁 | 未同意协议时可填写邮箱、密码、邀请码和优惠码，也可切换密码可见性；注册提交和 OAuth 仍受协议限制。设置未加载或提交期间继续禁用输入。覆盖 checkbox 和 modal 两种协议模式。 |
| `57509a60f`：可关闭 transport 冷却 | 增加 `gateway.disable_openai_transport_error_cooldown`，环境变量为 `GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN`，默认 **false**。仅跳过 OpenAI 网关路径新增的 transport-failure 冷却；保留 failover、先前 block、HTTP 错误处理与本地自定义错误码策略。YAML、环境变量及四套 Compose 示例完整传递同一默认值，未改运行配置。 |
| `e4afee4a0`、`52241ce8e`：密钥批量编辑 | 复用本地 DataTable、BaseDialog、Select 和现有逐 Key 更新 API，支持分组、状态、额度、5h/1d/7d 限额、有效期及 IP 白名单。只提交勾选字段，保留显式 0 和空白名单；数值必须非空、有限且非负，有效期必须为未来时间或留空。只接收当前可见行的 ID，重载清空选择，打开弹窗时复制目标 ID；保存期间锁表单与关闭入口，防止重复提交，逐条失败后继续并汇报成功/失败数。 |

批量编辑同时纠正两处本地契约差异：上游“有效期留空会清空”文案没有对应清除 payload，本地明确发送 `expires_at: ''`，复用后端已有清除语义；本地智能路由不能仅靠 `group_id` 替换，因此发送 `group_ids: [id]`，双语说明明确**替换所有分组路由，包括智能路由**。未勾选分组时不改变路由集合。

`f6cc7ee49` 的 Antigravity `/hub` User-Agent 本地已实现，并支持配置版本和校验；保留本地实现，运行现有回归，没有重复修改。

## 延期与排除

- **邮箱精确黑名单与批量禁用整组延期。** `b37920582` 的仓储事务无条件更新 `mcp_oauth_grants`，本地没有该表或 MCP OAuth 模块，直接引入会导致批量禁用回滚。后续 `e8eedd37f`、`e02d58fe8` 删除相关集成用例，不能证明本地兼容。连同 auth/OAuth、设置、API、UI 和文案一起延期，后续须按本地 schema、会话撤销、缓存及禁用语义完整适配。此判断来自源码与本地依赖核对，本轮未运行该模块的 PostgreSQL 集成测试。
- **新版 DeepSeek reasoning/Schema 修补延期。** `cfbf70ff1` 等改动无条件剥离调用者的 JSON Schema，改变结构化输出约束；`ca0fa5b59` 生成 `rs_sub2api_N` 时不避开已有同名 ID；placeholder helper 只要整段历史出现任一 reasoning 即提前返回，不能修复其他缺失 turn；按 served-model 前缀扩大处理范围也与本地 native 保留边界不一致。保留现有原文保留和隔离实现，不引入这组请求改写。
- **原始内容 dump 继续延期。** 新增错误场景落盘不再完全受 dump opt-in 控制，仍属于既定延期的请求/响应原文捕获链，不恢复 handler 或网关 hook。
- **连接 trace 清理继续延期。** `1f69e37e3`、`c56f3c6d7` 的 14 天保留及协议 metadata 去重依赖本地不存在的诊断模块/表，不新增孤立功能。
- **退役支付及无关外观改版排除。** 不恢复 cryptopay、EasyPay、支付页签排序/默认页签、支付组件测试；全局 squircle 样式不覆盖本地主题。源 VERSION 和发行说明不替代个人发布历史。
- Rollup 首日 clamp/SQL315、自动风控、余额/类型探测、目录同步、原文捕获及此前全部延期决定继续保留；不修改迁移、依赖、原版检查点或生产账号。

## 验证

- **后端定向回归通过。** Go 1.27.4，config、antigravity 和 service 三包共 **89 个顶层测试、193 个子用例**通过。覆盖开关默认值/YAML/环境变量优先级、开关与 passthrough 组合、保留 failover 和既有 block，以及现有 transport、自定义错误码、DeepSeek/CN provider 和 User-Agent 回归。命令：`go test -json -tags=unit -p=2 -timeout=8m ./internal/config ./internal/pkg/antigravity ./internal/service -run 'TestLoadOpenAITransportErrorCooldown|TestOpenAITransportCooldown|TestHandleOpenAIUpstreamTransportError|TestTempUnscheduleOpenAITransportError|TestBuildUserAgent|TestGetUserAgentForContext|TestCN|TestDeepSeek|TestNormalizeDeepSeekResponsesRequestBody|TestClassifyUpstreamTransportError' -count=1`。证据：`backend-tests.jsonl`。
- **前端相关回归通过。** BulkEditKeysModal、KeysView、RegisterView、DataTable 四文件最终有效结果共 **80 项**通过。首轮唯一失败为新 KeysView 用例选择器错误；修正为实际 `Refresh` aria-label 后该文件 40 项通过，其他未变文件的通过结果直接复用，未重复计数。`pnpm run typecheck` 和修改文件定向 ESLint 通过。证据：`frontend-tests.log`、`keys-tests-final.log`、`typecheck.log`、`eslint.log`。
- **生产构建通过。** i18n 完整性 3 项、`vue-tsc -b`、Vite 和 console 静态资源路由检查全部通过，后端普通构建及嵌入本轮前端的 `go build -tags embed -p=2 ./cmd/server` 通过。主机 PATH 中较新版 pnpm 干扰 `pnpm run build` 的嵌套调用，首轮在依赖目录确认前退出；随后用项目指定的 Corepack pnpm 9.15.5 顺序执行相同构建步骤，没有重装或改动依赖。证据：`frontend-build.log`（保留失败原因）、`i18n.log`、`type-build.log`、`vite-build.log`、`console-assets.log`、`backend-build.log`、`backend-embed-build.log`。
- **实际浏览器验收通过。** Edge headless，320/390px × 中英文 × 亮暗主题共 **25 项**检查，无文档/弹窗横向溢出或页面 JavaScript 错误。实际操作确认额度显式 0、留空清除有效期、分组替换、空 IP 白名单、单条失败后继续且显示部分成功，以及协议勾选前可输入、勾选后允许提交。测试只访问本机演示 API；匿名注册夹具仅在测试响应中启用注册/协议并阻止演示自动登录。已人工查看代表截图。证据：`ui_acceptance.py`、`ui-acceptance-results.json`、`ui-acceptance-final.log` 及截图。
- **配置与工作区检查通过。** Compose Gateway 环境变量一致性和安全脚本通过。逐路径分类覆盖 96 项，维护 JSON、`git diff --check`、版本/HEAD、迁移/依赖/生产配置边界及三份原有未跟踪测试的 SHA256 核验见 `verification-summary.json`。

本轮没有执行全量测试、race、真实 PostgreSQL 集成、远端 CI 或部署验收，也没有访问生产设置或调用付费模型。未接纳模块的延期依据属于静态审查；本轮构建和定向测试不替代后续发布门禁。

## 个人 v2.1.66 发布审查

用户授权发布后，独立审查核对真实后端所有权/分组权限、批量字段更新契约、协议提交/OAuth 门禁及冷却开关分支，未发现阻断项。复用实现未变的定向测试、类型检查、ESLint、生产前端构建和 25 项浏览器验收；发布前补齐运行时 `update_source_baselines.go` 与维护 JSON 的 v2.3.89 检查点一致性，以及 Makefile 中新批量弹窗测试的关键 CI 清单，现有来源追踪回归与新版 Go embed 构建通过。三份原有未跟踪测试继续保留且不纳入提交。

线上只读预检确认上一版 v2.1.65 制品、运行配置、数据容器、333 行迁移历史和安装密钥契约一致；受控部署脚本沿用既有流程，仅绑定本次版本、来源与基线，并通过 23 项故障注入回归。PR/主分支 CI、安全扫描、Release/GHCR 身份、部署前备份及实际隔离恢复和线上验收，另存 `development/diagnostics/sub2api-release-v2166-20261116/`，本记录不预先声明这些后续步骤已通过。
