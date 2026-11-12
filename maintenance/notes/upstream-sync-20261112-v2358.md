# 二改 v2.3.58 选择性同步（2026-11-12）

正式 v2.3.54 → v2.3.58，工作分支 `sync/fork-20261112-v2358`，起点为个人 v2.1.62 的 `f77a633224f6a0cad90382543224069338cd58ec`。同步阶段仅本地选择性适配，不导入二改 Git 历史，个人 VERSION 保持 2.1.62。用户随后授权审查后提交 GitHub 并部署，发布阶段推进至个人 v2.1.63；PR、CI、自有 Release/GHCR、备份恢复和最终上线证据单独保存。

## 来源与范围

- 二改仓库 `kiss-kedaya/sub2api`，原正式检查点 `e30c61bb257b0c5840260b9d2cabcde0c3bd6369`（v2.3.54），新正式检查点 `28ff1b3d41dddb2959892e66f4b410b064a266d4`（[v2.3.58](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.58)）。GitHub Releases API 记录发布时间为 `2026-11-12T11:47:32Z`。
- 共 39 个提交（含 merge）、29 个非 merge 提交、121 个变化路径。正式 tag 独立 fetch 至 `refs/remotes/upstream/releases/v2.3.58`；本地 tag 不覆盖。
- main 观察到 `c9d72b46bff8f7fcf596473d05f9125fc6f5a8ec`，只作观察记录，正式版之后的变化未纳入检查点。继续仅跟踪正式 Release；原版检查点 v0.2.21 / `10ba8f9b4dc2e6023d4a9879ab77b21841babd10` 不变。
- 原始发布元数据、完整 diff、提交清单、逐路径分类、初始状态及验证材料保存在 `.artifacts/fork-sync-20261112-v2358/`。121 个路径均有明确分类，无遗漏：适配 3、使用本地验证替代源测试变更 2、本地已覆盖 1、延期 62、排除 53。
- 检查点表示差异已审查并记录取舍，不表示整版功能全部导入，也不等于个人版本已发布。

## 采用与适配

| 项目 | 来源提交 | 本地处理 |
| --- | --- | --- |
| 声明倍率直接显示数字 | `6a51d9bea` | 同步倍率与账号倍率相同时也显示 `0.15x` 等实际数字，不再替换成“同账号倍率”。保留现有时效、峰值重算、失败但仍新鲜的数据、过期/异常状态及手动探测；保留 `accountRateVisible` prop 的调用兼容。 |
| 缓存明细浮层窄屏越界 | `af6ec7264`（部分） | 两端共享组件的浮层由居中改为右对齐。真实浏览器复现：320px 时管理员/用户浮层右缘分别达到 364.8px / 347px，调整后为 295.7px / 272px，显示浮层不再增加文档宽度。本地两端统计容器已有无裁切布局，源版删除 `overflow-hidden` 的部分无需重复同步。 |

不覆盖本地冷灰蓝主题、表格布局、原生平台路由、凭据目的地、真实终态用量或收费规则；不修改生产账号的倍率/探测设置。

## 延期与排除

- **仓库层统计缓存 `e1b17b171` 延期。** 源版默认启用 Redis 聚合缓存，TTL 60 秒，写入不失效；覆盖用户、Key、账号、模型及筛选趋势等查询。key 包含查询维度、实体、时间、时区与筛选条件，Redis 出错可回源，但新鲜度是另一项契约。本地管理端 `usage_query_cache.go`、`dashboard_query_cache.go`、`snapshot_cache.go` 及 snapshot-v2 已缓存重叠统计，并提供管理端 `nocache`；用户 stats/trend/models 当前直接查询。全局包进 repository 会叠加管理端缓存，并让用户统计默认滞后最多 60 秒。没有数据库负载证据支持这项变化，保留现有路径；配置、构造器、wire 和源测试整组不引入。
- **共享 IP 自动风控整组延期。** `832307f25`、`a89fc310b`、`c2d658e05`、`ff6ce64cd` 和配套 contract/UI 变更引入多用户共享出口检测、外部机房分类、自动追加 IP 提示/限流、停用 API Key、risk marker 与密钥轮换，改变实际访问控制。与此前已延期的 IP 分类/自动封禁保持一致；`e1b17b171` 的 Lua 写合并和 `99559e55a` 的 ZRangeArgs 替换只服务该新 detector，本地没有对应调用，不单摘性能或弃用修复。
- **上游类型/余额探测继续延期。** `a3e79bc61`、`83b0c1942`、`d48232b7d` 的 New API 指纹/1e8 sentinel、同源重定向、只有已用金额的显示、自定义余额提取 fallback、固定十分钟重试，均属于本地尚未引入的余额链。`customUsageExplicitlyConfigured` 也仅供新链调用，不增加闲置 helper。本地显式启用的 Sub2API 声明倍率探测保持原目的地和 unsupported 退避。`UpstreamBalanceCell` 不存在，其刷新按钮修复不适用；本地声明倍率刷新按钮已有 `type="button"`。
- **Auth Quiet 和定时器改版延期。** `b636d85f9` 的登录/注册整套主题、三步注册需要独立适配；`3480d346b`、`ef4638ca7`、`ee94dd4ff` 的定时器模态框和触发器只适用于延期模块。不覆盖现有 Vue SPA 或认证交互，不恢复定时器。
- **原生链上充值整组排除。** ETH/BNB 收款、动态价格、扫描/归集及源 migration314 属于已退役 cryptopay；不引入 SQL、支付配置、后台和用户界面。源版 v2.3.56 的 migrate-only 部署说明不适用于本次本地适配。
- 源 VERSION、发行说明、未发布 main、此前延期的 Dense/Nuxt、自动模型同步/catalogue、TTL 收费/Chat usage，以及 Hosted/Ops/capture 等退役模块均保留原决定。

## 验证

| 范围 | 结果 | 边界 |
| --- | --- | --- |
| 前端定向回归 | 3 个文件、30 项通过 | 声明倍率单元 8 项、缓存统计卡 2 项、用户用量页 20 项；包含相同倍率与账号列显隐、峰值、失败/过期、异常数据、按钮和现有用量交互。 |
| i18n、类型与构建 | 3 项 i18n 完整性、`vue-tsc -b`、Vite 生产构建、生产 CSS 资源路径检查通过 | Corepack 固定项目 pnpm 9.15.5，执行 build 的四个等价步骤，不修改依赖或全局配置。日志为 `i18n.log`、`typecheck.log`、`build.log`、`console-assets.log`。 |
| 定向 ESLint | 通过 | 仅两个修改的 Vue 组件及倍率现有测试，`lint.log`。 |
| 来源检查点 | 2 项通过 | 显式文件运行 `go test -json -tags unit update_source_baselines.go update_source_baselines_test.go`，核对两源锁文件、正式 Release 与 main 分离以及部署检查策略；`source-checkpoint-tests.jsonl`。 |
| 实际浏览器 | 见下文 | Playwright + headless Edge，全部使用本机 mock，无生产凭据和真实探测/付费调用。 |

浏览器证据：

- 深色主题，两端 320、390、768、1440px 共 8 个布局场景，分别操作悬停和键盘聚焦；浮层明细完整、在视口内，隐藏/显示不增加横向滚动。`ui-before.json` 与 `ui-after.json` 保存修复前后几何，截图逐项保存。
- 浅色主题，两端 390、1440px 共 4 个布局场景，同样验证悬停、聚焦和横向边界；`ui-after-light.json`。
- 账号页在浅/深主题与账号倍率列显/隐四种组合中，使用浏览器响应 fixture 验证同步的 `0.15x` 数字、可聚焦详情、Escape 关闭及刷新按钮。四次按钮操作只调用本地拦截 mock，返回后数字保持；`ui-rates.json` 和 `rate-*.png`。所有场景无页面 JavaScript 异常，未访问外部域名。

`gofmt`、维护 JSON、121 路径覆盖、`git diff --check`、个人 VERSION 与三份原有未跟踪用量测试 SHA256 最终核验通过。三份测试未改动、未纳入执行或提交范围。本轮后端仅改变来源常量，没有运行全量后端、race、数据库集成或服务器构建；不据本地验证声称发布 CI 或线上验收通过。无迁移、依赖及运行配置变化。

## 发布前审查

发布使用个人 v2.1.63。同步阶段已有的 30 项前端回归、3 项 i18n、2 项来源一致性测试、类型/lint/构建及 16 个浏览器场景在对应源码未变化时复用；发布阶段另行验证最终提交的完整 CI、安全检查、Release/GHCR 和线上身份。只读预检确认生产仍运行 v2.1.62 / `f77a63322`，333 行迁移、安装密钥、运行配置和数据容器符合上次部署验收。独立审查和部署 helper 故障注入结果、最终上线验证保存在仓库外 `development/diagnostics/sub2api-release-v2163-20261112/`。
