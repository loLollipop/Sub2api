# 二改正式 v2.3.18 选择性同步与柔和主题（2026-11-07）

## 来源与采用范围

[正式 v2.3.18](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.18) 于 `2026-11-07T06:24:04Z` 发布，非草稿、非预发布；标签解引用提交为 `935e24168cb37c720bd5264be4ca290fb7841611`。从 v2.3.17 的 `ad1a9ea8b446b9a73613d58e7f568aed9f55816b` 到该标签共 5 个提交、15 个路径，738 行增加、22 行删除。观察到的 main 为 `6330f0f462512e0045d8e9e19fe7e4bcab37ddba`，比正式标签多一次 VERSION bookkeeping，不作为本轮正式检查点。

从 `d1191f6a52be929455711cea7a9278f7b49b2e71` 仅适配已有账户模型同步中的三个独立修正：

- DeepSeek 内置补全的最大输出由 `384000` 改为 `393216`（384 × 1024），上下文仍为 `1048576`。
- `deepseek-v4.1-flash-1017` 使用精确显示名称 `DeepSeek V4.1 Flash 1017`。
- `gpt-image-medium` 使用精确显示名称 `GPT Image Medium`，不扩展到任意同前缀 SKU。

三项都作用于现有已识别 ID，不新增发现条目、不改变匹配范围、能力或价格。元数据继续按缺失字段补全，已有上游或 registry 的非空字段优先；本轮没有引入权威覆盖，因此不宣称已修正已有 reseller 的错误非零值。

## 继续延期的模型元数据

正式新增的 `4067236bfea657ab5b2c36af84fc2892b3535ed9`、`d1191f6a52be929455711cea7a9278f7b49b2e71` 没有修复上一轮的完整集成缺口：

- `model_metadata_catalog.go` 的 resolver 接收 platform，但查找候选不使用 platform。真实 key、模型 ID 与 canonical alias 仍进入同一索引；相同排序条件下可能继承 map 遍历顺序。新增点号到连字符改写也没有解决不同 SKU 的宽松匹配问题。
- API 模型报价与 smart-routing 的有效价、倍率、最终计费模型缺口仍在；正式差异没有修改相应 matcher、smart router 或 gateway ETag。模型广场源码按 `m.Name` 查询参考价，本地已按实际选中的 `pricingModel` 查询，不能直接覆盖。
- Kimi K2.7 Pro/K2.8 的 `262144/65536` 与模态来自相邻 K2 型号的共享 envelope 推断，源注释也明确说明这一点，却标记 `Authoritative` 并按前缀匹配。等待针对这些型号的直接供应商证据，不把推断作为权威值覆盖。

因此不导入通用 catalog、`Authoritative` DTO/覆盖规则、`/v1/models` 元数据/报价增强、模型广场 catalog 注入和配套 UI。保留上一轮 API Key 额度预占延期，以及断连估算收费、95% 缓存收费、原生支付、Hosted/Nuxt、Ops/capture/timers 等既有排除项，详见 [v2.3.17 同步记录](upstream-sync-20261107-v2317.md)。

## 本地界面调整

删除首页“从对话到创作，用熟悉的 API 接入主流模型。统一管理密钥、余额与每一次调用。”两句说明，同时移除中英文 `orbitDescription` 和无用样式。保留站点标题、`Keep Running` 副标题与原有入口。

共享主题使用近白微暖背景、白色卡片和小面积陶橙强调色，深色使用较亮的中性暖炭灰与柔和暖杏；首页、登录、控制台和传送到 body 的弹窗共用颜色变量。首页中央标题及左上角品牌名使用浅色陶橙到金棕、深色浅珊瑚到杏金的静态文字渐变，小字品牌名采用稍深的浅色渐变以保持清晰；去除雾状光晕，背景猪英雄纹样使用更淡的暖红和砂金线条；图表普通系列保留收敛后的多色，趋势主色与强调色一致，成功、错误等语义继续保留。沿用既有品牌、布局、表格和操作功能，思考强度仍是紧随模型的独立列。

## 验证与交付状态

以下本地定向检查全部通过；没有重跑全仓测试，也没有调用真实模型、付费接口或生产管理员操作。

- 后端 `go test -p 2 -tags=unit -json ./internal/service -run '^(TestBuiltin(DeepSeek|Catalog)ModelMetadata|TestSyncUpstreamModelCatalog|TestInheritUpstreamModelVariantMetadata|TestMatchModelsDevProvider|TestUpdate(Source|Tracking|Service))'`：59 个顶层测试、118 个测试及子测试通过，无失败、跳过或 stderr。覆盖精确显示名、相邻自定义 ID 不扩展、缺失字段补全与 snapshot 原有字段优先，以及正式来源检查点。
- 前端 typecheck、增量 ESLint、完整生产构建通过；构建包含 locale 完整性、类型检查、Vite 打包和控制台静态资源检查。受影响的 25 个现有测试文件共 256 个去重测试通过，图表旧色值断言已更新并复测。暖中性修订只调整配色与装饰，复用输入未变的功能测试，重新运行生产构建和增量 ESLint。
- Playwright + headless Edge 先完成 44 个结构/交互场景，涵盖完整/compact 首页、登录、用户与管理端仪表盘/使用记录、API Key 弹窗、浅深主题及 1440/390/320 宽度。暖中性定稿重新运行 16 个重点截图场景及 2 个登录按钮悬停场景，涵盖浅深主题的 1440/390 首页、登录和管理端使用记录，桌面控制台及密钥弹窗。无页面 JavaScript 错误和整页横溢，定稿正文/按钮最低抽测对比度为 4.82；登录按钮悬停的浅/深对比度为 5.25/8.17。操作覆盖主题与 API 示例切换、登录密码显示、图表 tabs、密钥弹窗分组 portal 与 Escape，以及管理员独立思考强度列菜单。首页两句文案消失、`Keep Running` 保留，并查看实际截图。浏览器使用本地合成数据。
- 修改的 Go 文件格式、`git diff --check`、锁文件与正式 tag 一致性通过；历史延期项与正式发布监控策略未改变。同步前三份未跟踪 Go 测试逐项 SHA-256 一致，依赖、迁移、部署配置、个人 VERSION 与 HEAD 均保持。
- 最后仅对首页中央标题、左上角品牌名补充静态文字渐变，覆盖完整/compact 首页和控制台共用品牌样式。增量 ESLint、Vite 经 PostCSS/Tailwind 的实际 CSS 编译渲染及 8 个 Edge 场景通过；主题切换、首页入口与品牌链接正常，标题完整，无页面错误或横溢。大标题渐变浅/深最低对比度为 4.11/7.782，小字号品牌使用更深的浅色渐变，逐段对比度均高于 4.5。高对比与打印均回到可读实色。纯文字样式调整沿用前次功能测试和生产构建证据，没有重跑完整生产构建或广泛测试。

来源 Release JSON、完整正式差异、审查摘要、同步前保护摘要和后端测试原始日志位于 `.artifacts/upstream-v2318-20261107/`。前端原有测试和结构/交互证据保存在 `.artifacts/calm-theme-20261107/`；暖中性基础主题的构建、增量日志和截图位于 `.artifacts/warm-theme-20261107/`。最终标题/品牌文字渐变的精简差异、源码摘要、浏览器结果与截图位于 `.artifacts/gradient-title-20261107/`；当前预览为该目录下的 `home-light-1440.png`、`home-dark-1440.png` 和 `brand-title-preview.png`。

用户随后授权提交 GitHub 并更新部署，个人版本升为 `2.1.56`。发布补跑最终渐变样式的生产构建，并通过 PR/CI、自有 Release/GHCR、制品身份核验及既有备份恢复校验流程交付；最终构建与线上验收另存发布证据，不在此预记部署成功。没有依赖、迁移或部署配置变更；三份开始时已有的未跟踪 Go 测试保持原内容并排除在本轮提交之外。
