# 二改正式 v2.2.32 选择性同步与用量标签修复（2026-10-26）

## 来源与范围

- 二改仓库：`kiss-kedaya/sub2api`，正式发布 `v2.2.32`，发布时间 `2026-10-25T20:14:08Z`。
- tag 剥离后的 release commit：`f72c80a72ef5a4fb45772ffd389e3fe58d1485ba`。
- 上次已审 main：`3af71dc396fd9f84647c43e95293e07a27d78d52`；本次观察 main：`41a4df17ffd7e6594d1ea7e7aa1aa724e8692c82`。后者仅同步上游 VERSION，不作为 release commit。
- 本地起点：已发布 v2.1.38，`a6bd5b3db9e9970d1015824c8e7d739efb9d1d6e`。

用户将本轮范围收窄为用量页滚动条和二改同步，取消额外的中转计费用量审计。按行为选择性适配，不进行整仓合并、付费模型调用或生产操作。

## 选取的改动

`0b7b6fc186841c5061f7e7013ba0dce12b213231`、`d9328f9da6092958c6b11c02a88f632295a0ae9b`：Sonnet 5.5 缓存读取基础价由每百万 token 0.20 美元修正为 0.10 美元，覆盖两处代码回退及远程/回退目录中的精确条目。按本地查价顺序，额外覆盖已有 `-high/-medium/-low` 目录别名；不修改其他模型和缓存写入 TTL 价格。

定价优先级是目录数据 → Sonnet 基础价修正 → 显式 `pricing.override_file`。自定义 `remote_url` 的 Sonnet 目录同样经过基础价修正，需要不同价格时使用显式 override。这是同步二改正式版的基础价行为，本轮没有独立核验厂商价表。

`f72c80a72ef5a4fb45772ffd389e3fe58d1485ba` 中选择两个切片：

- 用户最大并发 `0` 与现有前端/API 约定一致，表示不限；负数仍按 1。不限用户仍在 Redis 原子记录实际槽位，改回正数时计入既有请求；Live 与普通请求合计，账号和 API Key 的独立限制保持。等待队列仍保留最小容量。
- 在途快照增加账号和分组名称。选号与换组时在本地 `UsageInflightProgress` 锁内更新 ID/名称，ID-only 选号清除旧名称；普通用户仅增加分组摘要，账号名称仅投影到管理员 DTO。不增加请求热路径数据库查询，不让后台刷新读取可变 Gin 上下文。

## 本地界面修复

管理端用量明细标签容器原为 `overflow-x: auto`，Y 轴计算为 auto，标签的负下边距造成 1px 纵向溢出，出现右侧小滚动条。移除负边距、明确禁止纵向滚动并隐藏原生滚动条；保留窄屏横向滑动和键盘焦点自动滚入视野。

浏览器检查同时发现 320px 英文环境下三个分布图标题/按钮撑宽页面，标题容器允许换行。没有替换上游的整套在途面板或本地主题。

## 明确延期与保留

上游实时钱包 outbox 批处理涉及 SQL 批量领取/确认、worker/recovery 顺序和 Redis 原子水位，本轮延期。该 outbox 是仍在使用的通用余额同步机制，并非已移除的原生支付模块；继续使用本地原有逐事件实现。

不恢复原生支付、工单、无限画布、渠道定价界面或 observer 副管理员角色。不引入上游 VERSION、CI 和整页布局；现有迁移、依赖版本不变。

## 验证

Go 1.27.3 定向回归通过：

```text
go test -p 1 -tags=unit ./internal/service ./internal/repository ./internal/handler/dto ./internal/handler ./internal/server/middleware -run 'Test(Sonnet55CatalogCacheReadPrice|NewModelPricingCatalogFallbackAndContext|PricingService_AntigravityClaude55TiersUseBaseModelPricing|AcquireUserSlot|UserConcurrency|LiveLease|CalculateMaxWait|UsageInflight|TrackUsageInflight|UsageLogFromInflight|SetOpsSelectedAccount|KeyRouteBindingUsesSelectedPricing)' -count=1
go test -p 1 -tags=unit ./internal/service ./internal/repository ./internal/handler ./internal/server/middleware -run 'Test(PricingOverride|PricingHotReload|GetModelPricing|NewModelPricingAliases|NewModelPricingChannel|AcquireAccountSlot|TrackAPIKey|ConcurrencyHelper|AcquireUserSlotWithWait|TryAcquireUserSlotForAPIKey|FilterUsageInflight|PageUsageInflight|LiveLease)' -count=1 -timeout=10m
```

第二条 middleware 包没有匹配测试；其相关测试已由第一条执行。覆盖 Sonnet aliases/分层目录/显式 override/实际费用，Redis 并发获取与释放、0 改正数、Live 合计与账号限制，在途 DTO 隐私及换组换号名称更新。

前端 UsageView、ModelDistributionChart 和 GroupDistributionChart 共 36 项回归、四个修改文件 ESLint、vue-tsc 检查通过。Playwright mock 预览验证 1600px 浅色中文、1280px 深色中文、390px 浅色中文和 320px 深色英文：三个标签切换、键盘可达、无纵向溢出、窄屏横向滚动有效、页面不被撑宽，无 JavaScript 错误。截图和测量结果保存在本地 `development/diagnostics/upstream-v2232-usage-tabs-20261026/`。

完整前端生产构建通过：i18n 完整性 3 项、`vue-tsc -b`、`vite build` 及 `dev/check-console-assets.mjs`。本机通用 pnpm 指向不同版本，会触发依赖重装校验并在无 TTY 时终止；实际构建直接使用现有 node_modules 逐项执行项目 build 脚本的等价步骤，未重装或改动依赖。

`go build -p 1 -tags embed -o NUL ./cmd/server` 通过，没有生成发行二进制或发布包。29 个修改/新增 Go 文件的 gofmt 检查、35 个本轮同步文件的敏感信息扫描、JSON 来源检查点一致性及 `git diff --check` 均通过。

独立 critical_reviewer 对生产 diff、四个新增回归及必要调用上下文完成只读复核，未发现新增阻断问题。两个换组入口的名称分支未单独新增端点测试，已有路由回归和选号辅助函数名称回归通过。本机 CGO 关闭且无 gcc，本轮不声明 race、全量后端测试、真实 PostgreSQL/Redis 集成、Linux CI 或生产验收通过。浏览器使用本地 mock，不代替线上验收；预览进程已停止。

取消审计生成的三个未跟踪测试保留为 `unit && billing_audit_deferred`，不参与本轮 unit 测试，不纳入同步文件。其删除被自动审批策略拒绝，未尝试其他删除方式。

同步完成时先保留本地，随后用户于 2026-10-26 授权提交 GitHub 并更新部署，按本项目 v2.1.39 准备发布。新增 CI 定向 race 回归覆盖用户零并发、Sonnet 目录和在途 DTO/名称；最终 CI、制品和生产验收证据独立记录。检查点表示已审来源及选择性适配，不表示上游所有变更已合并。
