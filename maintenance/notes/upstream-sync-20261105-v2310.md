# 二改 v2.3.10 选择性同步与控制台样式适配（2026-11-05）

## 来源和采用范围

从正式 v2.3.5 `51748099a2b82dbb16eb8f080cf92efe4a2722b2` 审查至[正式 v2.3.10](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.10) `2746b74a38922ad763cb6ad5626a6441780a931d`，共 350 个路径、+36,209/-6,565。Release 发布时间为 `2026-11-05T09:35:29Z`。观察到的 main `9c561fa05e316735addb0888fbaec445544aa0e5` 含后续未发布变化，不纳入正式检查点。

- `41a8df815`：管理员余额调整与 `admin_balance` 来源记录使用同一事务；提交后才同步缓存、失效认证缓存和计提返利。来源写入失败回滚余额，返利失败保持既有不撤销充值契约。仅适配通用调账，不恢复已删除的原生支付退款模块。
- 修正直接适配源事务逻辑时暴露的连接池问题：真实 `UserRepository.GetByID` 使用基础 client，在事务占满连接池时会等待自身持有的连接。将用户元数据读取放在开启事务前，金额仍来自事务中原子 `BalanceChange` 的新旧余额。
- `f01772586`：分组用户倍率支持批量设置指定值，复用现有草稿、保存、权限与请求隔离机制。保留原乘法批调；继续执行本地倍率上限 `999999.9999` 和 6 位小数处理，不照搬源代码仅检查正数的条件。
- 两端仪表盘参考源 `DashboardStat`、`dashboardUi`、`DashboardQuickLinks`、`DashboardChartCard`：统一连接式指标网格，时间控制位于上方，快捷入口位于指标下方。用户端按源依次纵向展示趋势和模型分布，简洁模式保持七指标；快捷入口按创建密钥、查看使用记录、邀请记录、兑换码排列，有权限的批量生图放最后。邀请记录复用 `/affiliate` 与现有功能开关、简洁/后端模式可见性条件。
- 两端使用记录按源文件的模型、分组、端点、趋势顺序展示四图网格。用户端保留横向图标汇总条、独立筛选卡、标签/操作行及独立表格；管理端保留四张汇总卡、标题区日期控件和合并明细工作区。筛选、错误记录、管理员排行、导出、清理与分页保持完整，管理端宽表在局部横向滚动。
- 公告采用源项目的右侧时间线抽屉、居中详情和全局必读弹窗交互。使用本地 `BaseDialog`、DOMPurify 和现有接口，不引入 Nuxt UI。必读公告必须成功提交已读后才切换下一条，失败原地重试；保持会话隔离、键盘和焦点管理。
- 保留本轮开始前的共享浏览导航修复：管理员从模型广场/排行榜点控制台返回管理仪表盘；管理员主动进入用户工作区仍遵循当前工作区。

## 排除与延期

原生 crypto/payment、无限画布及相关安全策略已移除，不恢复。运维、监控、渠道扩展与源 migrations 307–310，以及分组倍率历史/NEW 信号与 migration 311 作为完整模块延期。requestcapture 和可调度定时器缺少本地模块，相关补丁随模块延期。

账号明文 Key reveal 扩大凭据读取面；本地 step-up 可配置关闭，不能沿用源“始终 2FA 保护”的前提，因此不引入。账号编辑立即打开 skeleton 是独立体验改动，延期。智能路由 Key `+N` 本地已有；源 router5/Transition 白屏与随主题变化的 tooltip 对比度问题不适用于本地 router4/固定深色 tooltip。

不导入 Nuxt SSR/BFF、源主题设置、CI、安装发布脚本、源 VERSION 或文档。保留本地 Vue/Vite SPA、原创橙色/奶油/深炭主题、原生协议和计费口径。本轮无数据库迁移、依赖或部署配置变化；原版上游检查点和个人 VERSION 不变。公告 summary/detail 懒加载等接口重构继续延期，本轮适配展示与交互。

## 前端源码对照

按固定源提交的组件、调用方及主题依赖核对，而非仅比截图。组件映射及适配原则如下：

| 源文件 | 本地适配 |
| --- | --- |
| `dashboard/DashboardToolbar.vue`、`layout/PageToolbar.vue` | 日期与粒度为左侧轻量控件，单一刷新图标位于右侧；保留本地布局容器和完整刷新事件。 |
| `dashboard/DashboardStat.vue`、`dashboardUi.ts` | 桌面四列连体指标，手机两列独立卡片；管理端总用户为主值，今日新增为次值，三种费用显式标注。 |
| `dashboard/DashboardChartCard.vue`、两端 `DashboardView.vue` | 小标题和区间合计；管理端按趋势、模型/排行、最近用户用量排序。用户端按源改为趋势在上、模型在下，模型卡内图左表右、手机纵向排列。 |
| `user/dashboard/UserDashboardModels.vue` | 模型列表补齐模型、真实 Token 数、占比三列。 |
| `dashboard/DashboardQuickLinks.vue` | 紧凑图标与文字的导航卡片，保留本地可用入口和权限。 |
| `views/user/UsageView.vue`、`views/admin/UsageView.vue`、`console-workspace.css` | 四图默认展开，核对 template 与 scoped/global 样式后的实际分组；用户端筛选卡、标签/操作行、表格分开，管理员保留合并明细工作区。 |
| `common/AnnouncementBell.vue`、`common/AnnouncementPopup.vue` | 紧凑标题/时间同排列表，摘要单行；正文元数据放标题下，保留抽屉叠层和确认失败重试。 |

源 `pnpm-lock.yaml` 锁定 `@nuxt/ui` 4.12.0；已核对该版本 [modal 主题](https://github.com/nuxt/ui/blob/v4.12.0/src/theme/modal.ts) 和 [slideover 主题](https://github.com/nuxt/ui/blob/v4.12.0/src/theme/slideover.ts)：两者遮罩为 `bg-elevated/75`，右侧抽屉为 `sm:max-w-md`，正文和必读弹窗由源调用方分别限定为 `sm:max-w-3xl` / `sm:max-w-2xl`。本地将这些尺寸和明暗色遮罩映射到现有主题，公告层保留轻模糊；没有增加 Nuxt 依赖。

## 验证与交付

- Go 1.27.3：定向运行 `internal/service` 的管理员余额调整、`internal/repository` 的来源记录事务与原子余额相关回归通过。最终服务测试另覆盖单连接池读取和上游检查点与维护锁文件一致性；来源写入失败回滚和余额/来源一起提交均通过。
- 前端定向组件与页面测试通过，覆盖两端仪表盘/使用记录、公告抽屉与必读弹窗、导航和分组倍率。新增邀请入口的 href、顺序、功能开关和模式可见性回归通过；中英文 key 完整性、修改文件 ESLint 与 `vue-tsc --noEmit` 通过。
- 最终 Vite production build 与 `check-console-assets.mjs` 通过。浏览器覆盖两端浅色 1600px、深色 1440px 和手机 390px；无页面异常或整页横向溢出。检查日期/粒度请求、仪表盘完整刷新、用户趋势在上/模型在下、使用记录筛选/标签/列设置与手机高级筛选。
- 公告浏览器验收覆盖抽屉保留、朦胧遮罩、详情关闭后的滚动和焦点恢复，以及最后一条未读变为已读后的 DOM 稳定性；必读弹窗的 Escape/外部点击限制、键盘焦点、失败重试和队列推进通过。管理员从模型广场/排行榜返回管理控制台通过。
- 邀请入口在上述三种用户端视口下顺序正确，点击实际加载 `/affiliate`；已刷新最终用户仪表盘截图。浏览器记录为 `browser-final-checks.json`、`browser-usage-final-checks.json`、`browser-followup-checks.json` 与 `browser-invite-final-checks.json`，production 日志为 `frontend-final-build.log`。

浏览器使用 `127.0.0.1:4317` 的本地演示 API，截图为演示数据。测试、截图和脚本保存在仓库旁 `development/diagnostics/upstream-v2310-ui-20261105/`。

本轮只修改本地源码；未提交、推送、发布或部署。真实 PostgreSQL/Redis 与生产验收由后续明确授权的发布流程负责。
