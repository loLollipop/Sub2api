# 二改 v2.3.13 选择性同步与使用记录补齐（2026-11-06）

## 正式源码与范围

- 来源：[kiss-kedaya/sub2api v2.3.13](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.13)，发布时间 `2026-11-06T02:59:29Z`。
- 从已审查 v2.3.11 `8bbb31374967e20ac70ddceaf30e23ec2be96144` 至正式 tag `4e3cd3c0a7b5af8d311a1418d7d797ae552d615f`，20 个提交、50 个路径、+3383/-480。使用 `upstream/tags/v2.3.13`，保留本地既有同名历史 tag。
- 观察到后续 main `cc6abfb553a6732c88b549bba1465fe22c8bf8bf` 只补源 VERSION。正式 tag 内 VERSION 仍为 2.3.12；检查点按 Release 身份记录，不复制源版本文件。
- UI 以该正式版本的完整源码为依据，包含早于本次版本增量、之前未完整适配的结构。

## 本轮采用

### 补齐两端使用记录

直接核对源 `views/admin/UsageView.vue`、`views/user/UsageView.vue`、`UsageStatsCards.vue`、`UsageFilters.vue`、`InflightUsagePanel.vue`、`DistributionPanel.vue` 和 dashboard toolbar。

- 四项连体统计：统一图标、数字、分隔线及小屏 2×2 布局；保留真实 Token/缓存详情、账号成本和用户端标准价划线语义。
- 图表独立标题栏与内容栏；doughnut 依据卡片容器宽度切换上下/左右布局，不以整页宽度误判半宽卡片。
- 日期、粒度与右侧刷新复用本地 `AnalyticsToolbar`，去除厚重按钮外框；窄屏完整显示“按小时”。管理端筛选日期入口和优先刷新契约不变。
- 管理端筛选不照搬源 `flex-wrap` 造成右侧留白的缺点，改为全宽 6×2 网格，中容器三列、手机前三字段配“更多筛选”。字段、模式、显式零/false 和搜索选项均保留。
- 用户端筛选、用量/错误 tabs、刷新/重置/列设置/导出、在途请求与日志收进一张工作区，不让操作栏漂在卡片外。
- 两端在途请求采用源紧凑居中折叠方式，展开显示计数、刷新、分页与 loading/空状态。增加真正的失败提示与重试，避免请求失败显示“暂无请求”；保留已加载分页行及原有过期响应隔离。
- 保留管理错误/用户排行、用户错误可见性、下钻、排序、权限、完整筛选、Excel/CSV 导出与本地主题。

### 设置页与其他板块盘点

核对正式 `SettingsView.vue` / `settings/SettingsSection.vue`：采用标签与保存同行、正文 `max-w-3xl`、独立标题/说明/操作配轻表单卡片的结构。保留原保存函数、加载失败禁用、TOTP step-up 和备份页独立保存语义；不迁移 Nuxt 组件。

已核对 Users、Accounts、Groups、Keys：用户管理已有命令栏/筛选/批量选择/表格结构；账号今日统计去重、Grok 拒绝费预览、Key 限额窗口已在此前同步完成。本轮不以重复替换整页的方式覆盖本地交互。

### 原生 Images GIF 结果归一化

选择 leaf `ec799ee447161b018ae6df34a66a95df1829e98a`，接入 API Key 非流式、OAuth native 非流式和 native completed 事件。对实际 GIF 按显式请求归一化帧数、10 ms 粒度时长、逐帧尺寸，并保持实际尺寸元数据、URL 转换和原 usage 路径。

适配时复现并修正源缺陷：

- 巨大目标尺寸乘法溢出可能触发 `NewPaletted` panic：采用防溢出计算并限制 GIF 16-bit 尺寸。
- 丢帧时透明快照残影、不透明动画背景丢失：正确合成 Background/Previous disposal、逻辑背景和 local palette。
- 发布前审查补充发现，扩帧重复原 disposal 也会令依赖前帧的透明/局部帧丢失像素；增减帧统一从合成画面重采样，并补 None/Background/Previous 六组逐像素回归。
- 限制单动画帧数、源/目标像素及 CatmullRom 中间缓冲区；进一步让整份响应、整条 native 流共享累计输入/输出帧数（各 1024）、像素（各 96 Mi）及 resize 工作（96 Mi）预算，不按图片或 completed 事件重置。单次 resize 中间缓冲区峰值仍为 8 Mi 像素。

无控制参数、非 GIF、已匹配或无效数据保持原字节；超资源预算不重编码后续结果。需要合成且不透明颜色并集占满 256 色时保留源字节，不做有损量化。

## 没有导入的更新

- `737361ba9` / `96983dcc6` / `7c6897947` / `503f2b970`：将“断线即关闭上游、缺终态估算 usage、95% 缓存输入计费”作为一整组策略排除。它改变财务口径，并非界面同步的附带修复；本地继续有界 drain，读取真实终态 usage。
- `39eac6b42`：brotli 1.2.7、klauspost/compress 1.20.2 修复已在本地 v2.1.51 具备，不重复升级。
- `7d0fbe612`：Hosted Search 首次失败切换账号；本地该模块延期，不恢复整个模块。
- `18e2e65f4` 与相关 theme/scene/chartTheme/Home/KeyUsage：源 View Transitions 与 Nuxt token 变更不覆盖本地主题。保持已有明暗主题和降低动态效果设置。
- `340b5cbde` 及源公共 Docs/data：模块路径和 API 说明与本地不同，不整页复制。实际 GIF 行为与本地限制记录于本节。
- 源 VERSION、merge 和分支同步提交仅作历史检查。无迁移、Ent、CI、部署或依赖文件变化。

## 验证与边界

- 使用记录/筛选/在途面板：61 个定向测试通过；统计卡片、图表及两端仪表盘相关原有回归通过。
- 设置页既有 60 个测试通过，新增 toolbar/备份保存可见性测试通过；原有保存和二次验证路径保留。
- GIF helper、三返回点、响应/流共享预算、URL/实际尺寸和 API Key/OAuth/direct 断线 drain 定向测试通过。逐像素比较覆盖 disposal、透明/不透明与 local palette。
- 上游检查点与维护记录一致性：2 个定向测试通过。
- 中英文 i18n 完整性 3 项通过；改动文件 ESLint 通过。
- 浏览器使用本地预览演示 API：两端 1600 浅色、1440 深色、390 手机，另 320 窄屏检查；没有整页横向溢出或页面异常。覆盖日期/粒度/刷新/请求 ID、tabs/排行/列选择、导出，在途失败/重试/空状态/10+2 分页、手机全部筛选入口；仪表盘上下图表与标题布局回归正常。
- 设置页浏览器为演示环境补充只读合成设置响应；检查常规/网关/用户/邮件/备份切换与响应式保存入口，没有发送 SMTP 或保存真实设置。
- 生产 typecheck、Vite 构建和 console 资产路由检查通过。常规 `pnpm run build` 内层命中系统新版 pnpm 的工具链问题已通过项目 corepack 9 逐项执行相同链路解决，没有安装/改写依赖。未运行无关全套、真实模型付费请求或生产流量验证。
- 实际截图和浏览器记录位于仓库外 `../development/diagnostics/upstream-v2313-ui-20261106`，使用本地演示数据。

实现阶段个人版本保持 2.1.51，仅完成本地修改与截图预览。用户随后于 2026-11-06 授权提交 GitHub 并更新部署，准备个人 v2.1.52；采用 PR/CI、自有 Release/GHCR 及既有备份、实际隔离恢复验证和受控切换流程。最终源码、制品与线上验收身份保存在仓库外的受控发布记录中。
