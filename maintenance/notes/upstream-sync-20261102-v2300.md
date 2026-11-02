# 二改正式 v2.2.67–v2.3.0 选择性同步与原创主题（2026-11-02）

## 来源与范围

- 本地起点：个人 v2.1.45，`16e161823f86c6dfc9c6483383ca020d98c83853`。
- 二改旧检查点：v2.2.66，`bbb057ebf308d983cc3201fd3441793691a9af54`。
- 本轮正式检查点：v2.3.0，`bc7a99214368d9107d3262680244a4d2eee48dfb`；中间正式 v2.2.67 的 peeled commit 为 `03f72e7a7f840e26fbbb513136d894a3bc225a9e`。
- Release API 确认 v2.3.0 非草稿、非预发布，发布时间为 `2026-11-02T00:14:11Z`。观察到 main 为 `185dfaf184c1f24fc910a07ad21702fdbd5bb55e`，领先正式 tag 27 个提交；依现有正式 Release 策略，这些未发布提交不计作已审查或同步。
- 正式增量共 642 个路径，backend 为 41 个路径；没有 Go 模块或数据库迁移变更。前端大部分为 Nuxt UI/SSR 迁移，不能直接覆盖本地 shadcn 界面。

同步阶段在 `sync/upstream-v2300-ggbond-20261102` 本地适配，初始个人 VERSION 保持 2.1.45。用户随后授权提交 GitHub 和更新部署，按个人 v2.1.46 准备 PR、CI、自有 GHCR 和受控制品流程。检查点表示正式源码差异已分类并选择性适配，不代表完整合并。原仓库检查点不变。

## 采用的修复与优化

### DeepSeek 原生推理参数

`290c26c53`：在现有 HTTP Forward 链增加两个限定判断，DeepSeek `responses/adaptive` 原生请求保留显式 `reasoning.effort=none`，并跳过 OpenAI 兼容用的 `minimal -> none` 改写。保留本地按用户 Key、对话和请求模型隔离的 reasoning replay、原生输出与工具历史、模型映射和计费逻辑，不整文件覆盖、不引入 Hosted 自动执行。

### 分组实时容量未知状态

`ce7a129fe` 的独立子集：后端返回 `runtime_known`，前端对未知使用量显示 `—` 并提供说明，保留配置总容量。空分组实时使用量仍是已知的零；依赖缓存缺失或读取失败时不能伪装成真实零。按组判断所需的并发、session、RPM 数据，避免某一限额维度失败将不依赖该维度的其他组一起标为未知。批量与单组 fallback 保持一致，并处理缺少并发服务的情况。独立审查进一步发现真实 session cache 会吞掉 pipeline 单项错误，只返回成功账号；本地增加返回结果完整性判断，缺失账号仅令其所在分组未知。新增回归在修复前于 batch/sequential 两个分支复现失败，同时覆盖共享账号、真实零、空组和无需 session 限额的组。

### 用户列表平台配额按需批量加载

`ce7a129fe` 的独立子集：平台配额列可见时，在用户列表请求中显式启用内联批量加载，替代当前页逐用户发起的查询。后端 repository、service port/adapter、admin handler 与本地前端消费端配套更新，保留原单用户配额 API 和默认列表契约。配额加载失败与成功返回空记录分别处理，保留取消请求、旧分页响应隔离和编辑后刷新。

## 本地原创主题与首页

- 新增共享 `GGBondBackdrop.vue`，以原创内联 SVG 绘制猪英雄头盔、猪鼻护盾、闪电和能量折线，不下载外部图片、不覆盖站点自定义头像或 Logo。
- 浅色使用奶油底与半透明红金细线，保留现有橙色交互、卡片和 shadcn 字体层级。首页、compact 首页、登录及控制台共用同一视觉语言。
- 根据用户对初稿暗色背景的反馈，移除大面积灰棕光晕，改为中性深炭黑 `#111318`，光晕与底纹分开控制；按用户后续反馈恢复徽记与折线的清晰度，取消大片填色，保留可见细线。浅色方案保持。
- 装饰层不参与交互或无障碍树，没有持续动画或额外图片请求；移动端裁切在装饰层内，高对比和打印模式隐藏装饰。
- 删除默认首页底部 GitHub 跳转及无用 prop/常量，保留版权、法律链接与风险说明。不修改管理端个人菜单中的源码入口。

## 明确排除与延期

- Nuxt/Nitro SSR、新 BFF、Node 页面服务及相应部署/构建链延期；保留现有 Vite Vue SPA、认证和 Go 自有 GHCR 部署。源站点主题设置服务于新 UI，不与本地纯视觉背景一起导入。
- usage compact 会将列表关联字段替换为 `*_summary`，需要完整迁移本地表格消费端；当前用量查询已经批量 hydrate，本轮不改变契约。
- `7d1249b10` 公告 summary/popups/detail 懒加载作为独立性能适配延期。源 summary 仍加载完整公告与可见性数据，summary 与 popups 还可能重复查询；本地需同时保留未读、弹窗队列和会话隔离，不能只导入缩减 DTO。现有公告功能与兼容接口保留。
- `505c1a4dd` BSC 扫描容错属于已移除的原生支付，不恢复扫描器、路由、页面或定时器。链动小铺与兑换继续使用现有链路。
- 不恢复工单、无限画布、原生支付或此前延期的运维、采集、Hosted 等模块；不修改计费、退款、Grok 拒绝收费/会话封禁和断线用量排空。
- 不修改已有迁移、Ent 或 Go/前端依赖清单，也不以源 VERSION 覆盖个人版本。

## 验证

- 主代理最终后端定向回归：service/repository/admin 三包 **229 项 tests/subtests 通过，无失败、无跳过**，覆盖 DeepSeek native effort/replay、其他 provider 的 none 过滤、容量与缓存缺失、Grok refusal、平台配额、用户列表与更新检查点。详细 JSONL 及计数分别见 `final-backend-tests.jsonl`、`backend-summary.json`。
- 最终前端定向回归：HomeView、ConsoleSignal、ConsoleAtmosphere、UsersView、admin users API、GroupCapacityBadge 与 locale completeness，**7 文件、95 测试通过**；完整 lint、类型检查和生产构建通过，构建后的静态资源路由检查通过。
- Go `build -p 2 -tags embed ./cmd/server` 通过，包含最终前端资源。验证二进制仅用于本地构建验收，不是正式发布制品。
- 浏览器检查覆盖桌面/390px 手机、首页/控制台/管理端/模型广场/登录、明暗主题、头像下拉和侧栏折叠/抽屉。无页面脚本错误、无横向溢出，装饰层不拦截交互；compact 首页、系统主题/按钮切换、forced-colors 和打印隐藏也通过。
- tracked 4991 文件敏感资料扫描通过，新增未跟踪文件另行扫描通过；`git diff --check` 通过。保护校验确认三份原有未跟踪测试及迁移、Ent、Go/前端依赖和 VERSION 共 691 个受保护 tracked 文件逐字节未变。
- 独立常规主题审查无阻断项；深审计确认 DeepSeek 与批量配额边界，并发现真实 session cache 吞错导致容量误报，已增加缺失成员判断和修前失败/修后通过回归。最终修复另经只读复核。

证据目录：`development/diagnostics/upstream-v2300-ggbond-20261102/`，包含源码基线、Release 身份、测试/构建日志及浏览器截图。测试仅使用本地 mock 与虚构凭据，不调用真实模型或收费搜索。未运行全量后端套件、Linux race、真实 PostgreSQL/Redis 故障注入或生产部署；新增 repository Docker integration 用例因本机无 Docker 尚未执行，不将单元回归等同于数据库集成或正式发布 CI。

发布阶段只追加个人 VERSION、发行说明和发布状态；上文验证保留同步阶段的时间与范围边界。v2.1.46 最终源码、PR/main CI、发行包/GHCR 一致性、备份实际恢复和线上验收证据另存 `development/diagnostics/sub2api-release-v2146-20261102/`。
