# 二改正式 v2.3.1–v2.3.4 选择性同步（2026-11-03）

## 来源与范围

- 本地起点：个人 v2.1.46，`ca35e87fb6d977801af46ede21470e1438025ae2`。
- 二改旧检查点：v2.3.0，`bc7a99214368d9107d3262680244a4d2eee48dfb`。
- 本轮目标：正式 v2.3.4，`65db90acee6ea7983611ef1b9e248cf8c7663235`，Release 发布时间 `2026-11-03T01:50:57Z`，非草稿、非预发布。
- 中间正式 tag：v2.3.1 `755fc68d9523f83b88c1423feae97fcbd677a3d1`；v2.3.2 `83166cc6b606cee5c3bdc8317bca4107e7b54b1c`；v2.3.3 `988463632f7490e4f1d798403dac1995b1f3f8c8`。
- 固定观察到的 main 为 `c2d172150660e4416e8a49b80bbcff9ac2b09910`，领先正式 tag 三个提交。本轮不计作已同步，包括定时器未知状态确认的后续修复 `2ef7fa0b3`；本地没有该定时器组件及状态接口，未来引入时须一起审查。
- 正式增量 244 个路径，+14684/-1364；大量前端改动服务于源 Nuxt/Nitro SSR，不能直接覆盖本地 Vite Vue/shadcn SPA。

本轮在 `sync/upstream-v2304-footer-20261103` 修改与验证；用户随后授权代码审查、GitHub 提交及部署，个人 VERSION 准备为 2.1.47，原仓库检查点不变。沿用 PR/CI、自有 GHCR 与受控制品更新流程，最终源码、制品及线上验收另存发布证据。正式检查点只表示完成差异分类和选定行为适配，不代表整仓合并。

## 选择性适配

### DeepSeek 原生与跨协议兼容

选择 `cf3577f10`、`5a9055207`、`08789c3da`、`3e69d829e`、`221f25e05`、`567f2bf1c`、`2d65dd37d`、`8ade2685e`、`54c8f189d`、`5e4135c64`、`ed2f41765` 的必要行为：

- 原生 DeepSeek Responses 保留 store、previous_response_id、verbosity、summary、重复 call ID 和工具媒体，由供应商验证请求；400 不做删字段重试或 Chat fallback。已配置的 404/405 不支持端点回退保持原契约。
- 请求转换只将真实 reasoning_text 作为可重放内容；摘要不再新增写入回放缓存。响应按最终账号平台开启 DeepSeek 模式，区分 summary 与真实 thinking，覆盖流式 delta、done、terminal-only 与 buffered 恢复。
- DeepSeek 显式 none 在必要跨协议转换中保持 thinking.disabled，max 不降为 xhigh；普通 OpenAI/Anthropic 默认行为、配置 token floor 和 Claude 签名封装保持。
- 扩大本地回归后，让原生 DeepSeek 跳过通用缺失工具结果补造；OpenAI 原有修复继续执行。独立审查发现的两项边界也已修复：delta/done 的 item_id 用于匹配压缩后的终态，避免重复推理；真实 thinking 晚于正文到达时开启新的内容块恢复，避免丢失或向已关闭块写入。
- 工具参数尚在接收时暂存交错的真实 thinking，待工具块结束后顺序恢复；正常终态或 EOF 均排空。终态合成恢复事件保留完整类型，避免入队后被分派器忽略。回归检查工具 JSON 完整性、块开闭、逐 part 去重、一次签名及用量，默认非 DeepSeek 转换不启用该队列。

本地 `deepseek_native_reasoning_replay.go` 未修改，Key/对话/请求模型隔离与 opaque digest 绑定保留。不采用源缓存键简化，不引入 Hosted 自动工具执行、旧响应重写或真实收费测试。旧 Chat fallback 缓存键与历史值不迁移，仅禁止新的 summary 写入；这不等同于清洗历史缓存。

### 免费模型发现

适配 `c9c6edbae`，覆盖本地 OpenAI、根别名、Codex 和 Gemini/Antigravity 模型目录读取。仅 GET 列表或单模型信息免余额/订阅准入，不跳过认证、Key 状态/到期/配额、用户状态、IP ACL 和分组权限；生成类 POST 和 Gemini model action 不命中。Google 错误格式中间件单独配套，避免只修 OpenAI 路径。

与源实现不同，不将模型发现并入整体 `skipBilling`；原有 usage、任务结果查询与倍率自省行为保留。目录读取不加载或维护付费订阅。

### 模型广场自定义参考价

适配 `68d46b607`，使用本地计费解析器最终选中的 requested/mapped 模型和 `PricingSourceGroup` 决定自定义参考价。基础价来自同一阶梯表首档，前端显示“分组自定义价”与参考价说明，不把自定义价称为官方价，不乘分组倍率；非 token 价格不误标。保留平台定价隔离、官方长上下文次档、真实实付价及原模型名称。

本地额外修正源逻辑中的展示缺口：表格优先渲染 intervals，因此自定义参考价不仅更新平价字段，也更新首档；复制 memoized 参考价和区间，避免污染其他组的官方价。阶梯关闭时仍可查看官方次档参考信息。

### 智能路由 Key 分组筛选

`e00e45fc9` 修复的源前端“创建后本地插入”路径在本地不存在，本地创建后会重新加载列表。核查发现真实服务端筛选只匹配主 `group_id`，因此本地按相同业务意图修复公共列表查询：匹配主组或路由关联组，用 EXISTS 避免重复结果；count、分页及并发排序共用同一筛选，仍限定所有者和未软删记录，未分组筛选保持原语义。

### 表格格式化缓存与首页

从 `85ec3769e` 移植独立 Intl formatter 缓存；数字和日期缓存各上限 64，按语言/货币/精度/日期选项区分，日期选项排序后生成稳定键。保留所有原有格式化辅助函数与显示语义。

首页底部版权和法律链接改为竖排居中，风险说明保持居中；不修改共享法律链接组件。保留原创猪英雄底纹、明暗主题和八个法律链接，不恢复 GitHub 跳转。桌面/手机、明暗和中英文共八种组合已通过浏览器测量，无横向溢出或脚本错误。

## 不适用与延期

- Nuxt SSR/BFF、cookie 会话、passkey/OAuth SSR 绑定、客户端导航和源主题修复不直接导入；本地 SPA 认证、路由与 BaseDialog Teleport/dialogStack 继续使用现有实现。源 SSR 发行包、workflow 改动不适用于自有 Go/GHCR 发布流程。
- `5991959d6` 的 TOTP 延后消费依赖源共享 replay store。该 store 随原生支付安全模块一并移除，本地 TOTP 验证没有该过早消费机制，因此不复制 VerifyLoginCode/ConsumeLoginCode，也不恢复支付耦合。现有代码不能据此宣称具备通用原子 TOTP 防重放；若增加该能力，需独立实现和验证并发消费、下游失败重试及成功后重放。
- 同提交的 `x/crypto` 伪版本单独核对：v0.58.0 到 `2f1f834330b2` 只有一个上游提交，唯一变更为新增 `openpgp/go.mod`，拆分已弃用 OpenPGP 模块；本地仅直接使用 bcrypt、curve25519、nacl/box，未使用 OpenPGP。本轮保留稳定 v0.58.0，不引入没有运行时收益的伪版本或审计豁免。证据 `crypto-source-review.json`。
- 新 KeyUsageDialog/API 范围摘要、渠道/用户活跃信号及倍率历史需要本地 UI 消费与额外查询；现有 Key 批量用量和每日统计保留，本轮不加新入口。
- 白名单自动同步 runner/JSONB CAS、运维、采集、Hosted、定时器状态、上游 Ops JSON 导入按既有模块边界延期。
- DataTable 源 UTable DOM 同步、Groups 异步编辑和 Accounts 详情预取属于可独立适配的性能优化；本轮只移植格式化缓存，避免与本地虚拟表格、编辑竞态保护和弹窗加载契约混改。Keys 默认折叠和兑换结果弹窗是源 UI 选择，保留本地交互。
- 原生/crypto 支付入账深度、充值默认标签、支付 forwarded origin 及归集修复均不适用，不恢复已移除模块。工单和无限画布保持移除。
- 正式差异无迁移或 Ent schema 新增。本轮不改迁移、Ent、依赖、计费/退款、Grok 拒绝收费与永久会话封禁、断线用量排空或部署配置。

## 验证

已完成的本地检查：

- 主代理目录权限、Key 查询回归 285 项 tests/subtests；模型广场、handler/可见性与更新检查点 64 项，共 349 项通过，无失败或跳过。Key 筛选使用真实 SQLite count/分页查询，既有 PostgreSQL ANY 路由 hydrate 不在此 SQLite 用例中执行。
- 发布前完整 apicompat 包 858 项 tests/subtests、扩大协议回归 4504 项全部通过，无失败或跳过。扩大范围包括 DeepSeek、Responses/Chat/Anthropic 转换、Grok、原生 replay 隔离、opaque 绑定、断线用量排空及工具历史；新增事件顺序回归已先复现失败再修复。各套件有重叠，最终 JSONL 按 package/test 名去重为 5033 项，包含父测试与子测试，不能简单相加。
- 前端最终 12 文件、170 测试通过；完整 vue-tsc、ESLint、Vite 生产构建和 console 资源检查通过。格式化工具全目录初轮 40 文件/309 测试中仅新增弹窗断言使用了错误容器，已改为检查 Teleport 真实 body，相关用例及最终套件重跑通过。
- Go 服务含前端 embed 的验证构建通过，无依赖或迁移变更。
- 首页浏览器八场景通过，桌面/390px、明暗、中英文；版权、法律链接、风险说明中心偏差小于 1px，八法律链接完整，无 GitHub footer 链接、横向溢出或脚本错误。
- 独立目录/定价/筛选/前端审查发现两项：参考次档误取实付阶梯、无官方目录价别名的测试误断言。已分别修正为复制官方次档、仅自定义参考必需非空，回归通过。
- 协议独立复核发现的 item_id、晚到 thinking、工具参数交错和终态合成事件类型问题均已修正并补充回归。最终窄复核确认终态补全不再遗漏正文或签名，目标测试独立重复十轮通过，无未处理阻断；完整协议包和扩大协议回归由主代理重跑。
- 发布前独立审查另发现默认非 DeepSeek 模式的晚到摘要可能写入已关闭内容块。恢复逻辑现仅写入当前开放的 thinking 块，保留既有签名与关闭契约；正文/工具 sibling 与 summary.done/item.done 四种组合先复现失败，修正后通过。
- 752 个受保护 tracked 文件及三份旧 untracked 测试逐字节未变；4997 个 tracked 文件敏感资料扫描通过，新增文件另行扫描通过。`git diff --check` 通过。

证据目录：`development/diagnostics/upstream-v2304-footer-20261103/`。仅本地 mock/unit；不调用真实模型或收费搜索。本机无 Docker，未运行全后端、Linux race、真实 PostgreSQL/Redis 集成与生产验收；正式发布 CI、制品和线上验收仍是后续独立门禁。
