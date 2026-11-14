# 二改 v2.3.76 选择性同步（2026-11-14）

本轮先完成本地选择性适配，个人版本当时保持 **2.1.64**。用户随后授权审查通过后提交 GitHub 并更新部署，个人 `backend/cmd/server/VERSION` 推进为 **2.1.65**，按既有 PR、CI、自有 Release/GHCR 与受控部署流程发布。

## 来源与范围

| 项目 | 检查点 |
| --- | --- |
| 二改仓库 | `kiss-kedaya/sub2api` |
| 原正式检查点 | v2.3.66 / `00e1d64b7e5a12375507f8a31958297d7f6f99b8` |
| 本次正式目标 | [v2.3.76](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.76) / `f3956f1a7f31254e47eaf8cbb6cd9d83f2998b1d` |
| 正式发布时间 | `2026-11-13T23:28:13Z` |
| 观察到的 main | `6a8b8d1197cf2e48cb99cc31dc0a1d8c89bec5d1`，仅观察 |
| 增量 | 60 个提交（含 merge），45 个非 merge 提交，60 个变化路径 |
| 原版检查点 | v0.2.21 / `10ba8f9b4dc2e6023d4a9879ab77b21841babd10`，不变 |
| 本地分支 | `sync/fork-20261114-v2376`，起点 `a862b50e338719ccde7bf9acdd39ec6574d5f0a9` |

正式版本通过 GitHub Release API 与独立 remote release ref 核对，不覆盖个人 tag，不合并未发布 main。完整 diff、提交清单、来源元数据、初始工作区及逐路径分类存于忽略目录 `.artifacts/fork-sync-20261114/`。60 个变化路径逐项分类为 19 项适配、26 项延期、15 项排除；适配以本地现有调用链为落点。

## 已适配

| 来源 | 本地行为 |
| --- | --- |
| `bfc0bdc38`：平均输出速率 | 使用 `output_tokens / displayDuration(row)` 表示整请求平均速率，不依赖 TTFT。保留至少 20 tokens、至少 500ms 的展示门槛及现有耗时来源。双语说明明确包含首 token 等待；不修改 Ops 的生成窗口速率、真实用量、计费或持久数据。 |
| `03edaac0d`、`409bd4476`、`4a07d7bd5`：余额卡 | 管理仪表盘末张卡改为有余额用户的余额总额和人数。现有用户统计同次扫描计算 `balance > 0` 的 count/sum，排除软删除用户，沿用现有全体用户口径（不另按角色或状态过滤）。补齐 handler、共享统计类型、本地卡片、双语文案和演示数据。 |
| `1687505ff`：分组状态点 | 密钥创建、编辑及行内分组选择按需获取用户侧 90 分钟监控矩阵，倍率后显示最新真实桶的可用率颜色。复用时间轴分色函数；已有 unknown 桶灰色，无桶不画点，提供可访问的状态说明。每个 KeysView 实例独立缓存 60 秒、合并并发，空结果和失败也退避，失败清除旧色，避免跨会话复用和重复请求。 |
| `e15e34504`：监控空槽 | 尚未聚合的位置仅保留间距，不画灰条；真实 unknown 桶继续灰色。保留本地时间轴布局、主题及既有交互。 |

## 延期与排除

### Rollup 清理优化整组延期

`ab4d1348d`、`659a33ad3` 旨在减少清理后整窗重建，但源 Go clamp 与 SQL315 会把 `affected_date <= retained_date` 的变更整体忽略。本地 `retained_from` 来自 `MIN(created_at)`，并非可丢弃历史的策略截止。以下反例来自完整调用链静态复核，未声称运行过 PostgreSQL 集成测试：

- `closed_before=08-14`、首日为 `08-12` 时，部分删除 12 日费用行会被跳过；剩余首日仍在有效日桶范围，费用继续读旧值。当天水位相等即跳过同步，次日只增量发布，也不保证重建该日。
- 同批 INSERT 包含 11 日与 13 日时，全批 `MIN=11日` 被忽略，同时漏掉 13 日变动；跨首日边界的 UPDATE 对 OLD/NEW 先取 `LEAST` 也会漏掉窗口内旧桶或新桶。只修正等号不足以关闭缺口。
- 本地分区删除仅以月初传入 invalidation helper，再 `DROP TABLE`，不会触发行级 DELETE。若保留窗首日为月中，源 clamp 会忽略跨界分区中仍然有效的日桶。
- 源迁移写死 `Asia/Shanghai`，与本地可配置时区和 `timezone_name` 不一致；历史补录、时区切换和发布锁后的重新判定都需要保持。

因此不局部套用 Go clamp，也不导入 SQL315 或修改既有迁移。后续若独立优化，必须明确清理与历史写入的不同语义、逐候选日期/分区区间求交，并覆盖首日、混合批、跨日期更新、历史补录、非上海时区与并发发布的真实 PostgreSQL 回归。

### 其余取舍

- **原文 dump 延期。** v2.3.71–73 新增失败请求全文、响应和转换步骤的本地落盘与后台下载。该链会保留用户提示词、URL query 和部分凭据头，尚无有界保留/脱敏契约；不恢复此前延期的请求捕获链，也不引入孤立 handler、wire 或网关 hook。
- **自动 IP/停 Key 风控继续延期。** 共享用户统计、自动条目及 UI 文案均依赖同一延期模块。
- **New-API 类型/余额自动探测继续延期。** 本地现有手动开启的声明倍率刷新保持原契约，不增加源版账户余额推断或额外探测端点。
- **退役原生支付排除。** 扫链限流重试、自家 gas 补给过滤及相关测试不恢复。
- 源 VERSION 和发行说明不覆盖个人发布历史；仓库统计缓存、公共设置 HTTP 缓存、固定 cache critical 阈值、受管代理、模型目录/自动同步、新 TTL 计费及此前所有延期项继续保持原决定。

## 验证

- **后端定向测试通过。** repository 与 admin handler 共 17 个顶层测试、4 个子用例通过，覆盖余额 count/sum 和零值输出，以及既有 dashboard 聚合、清理与用户统计回归。执行 `go test -json -tags=unit -p=2 -timeout=6m ./internal/repository ./internal/handler/admin -run 'TestDashboardEntityStats|TestDashboardHandler|TestDashboardAggregationRepository|TestGetUserDashboardStats' -count=1`；日志为 `go-dashboard.jsonl`。共享 `usagestats` 包测试及来源基线两文件定向测试也通过。
- **前端相关回归通过。** 7 个 Vitest 文件最终共 118 项通过：UsageTable、DashboardView、GroupOptionItem、ChannelMonitorV3Timeline 的 unknownBar/qualityStrip、groupStatusDot、KeysView。末次修改后补测受影响的两个文件，其余有效结果复用；没有把初测与补测重复累计。相关 18 个文件定向 ESLint 及末次修改后的补查、`pnpm run typecheck` 均通过。
- **生产构建通过。** pnpm 9.15.5；i18n 完整性 3 项、`vue-tsc -b`、`vite build` 和 `node dev/check-console-assets.mjs` 全部通过。日志为 `i18n.log`、`type-build.log`、`build.log`、`console-assets.log`。
- **实际浏览器验收通过。** 本机 Edge headless，320/390px × 中英文 × 亮暗主题，检查 Dashboard 余额卡、管理用量速率、密钥创建分组选择和 Monitor，共 32 项页面检查，无文档横向溢出或页面 JavaScript 错误。对照演示 API 返回的 token/总耗时核验实际显示速率；操作平台切换确认正常、低可用率、unknown 和无桶状态点。监控验收把本地响应裁为 17 个真实桶，确认 18 个槽位保留间距且仅真实 unknown 桶画灰条。代表截图已人工查看；脚本及结果为 `ui_acceptance.py`、`ui-acceptance-results.json`。另有 1440px 默认中文暗色初验截图。
- **工作区检查通过。** `git diff --check`、维护 JSON 解析、Makefile 中相关测试路径及来源基线一致性通过。`HEAD` 保持起点，个人 VERSION 仍为 2.1.64；三份原有未跟踪测试 SHA256 与初始状态一致且仍未跟踪。没有迁移、依赖或运行配置变化。

以上同步阶段验证只使用本地代码和演示数据，没有访问生产设置或调用付费模型；汇总见忽略目录 `.artifacts/fork-sync-20261114/verification-summary.json`。该阶段未执行全量后端/前端测试、race、真实 PostgreSQL 集成、远端 CI 或部署验收；余额 SQL 使用 sqlmock 覆盖查询和扫描契约，rollup 延期反例仍为静态复核。

## 个人 v2.1.65 发布审查

用户授权发布后，复用本轮源码未变的有效测试、类型检查、生产构建和 32 项窄屏浏览器结果；补充独立发布审查、嵌入前端的后端构建及提交内容检查。三份原有未跟踪测试继续保留且不提交。发布元数据只推进个人版本，不改变正式来源检查点或延期决定。远端 PR/主分支 CI、安全检查、Release/GHCR 制品身份、数据库备份实际恢复及线上验收，另存 `development/diagnostics/sub2api-release-v2165-20261114/` 的发布证据；本记录不预先声明这些步骤已经通过。
