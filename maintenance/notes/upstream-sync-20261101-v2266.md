# 二改正式 v2.2.63–v2.2.66 选择性同步（2026-11-01）

## 来源与范围

- 本地起点：个人 v2.1.44，`7ef2de4fd62f4d9de089e704d3b7f9624d3798c4`。
- 二改旧检查点：v2.2.62，`b459e8662bc2c7cfeb6cc70a7610570c59a6e20a`。
- 本次正式检查点：v2.2.66，`bbb057ebf308d983cc3201fd3441793691a9af54`，与本次观察的 main 一致。
- 四个 Release 均非草稿、非预发布；正式 tag 的实际 commit 以独立远端 refs 验证。增量共 46 个路径，没有数据库迁移、Ent 或 Go/前端依赖清单变更。

本轮先在 `sync/upstream-v2266-20261101` 本地适配并验证，初始个人 VERSION 保持 2.1.44。用户随后授权提交 GitHub 和更新部署，按个人 v2.1.45 准备 PR、CI、自有 GHCR 和受控制品发布。更新检查点表示差异已分类并选取适用部分，不表示完整合并上游。原仓库检查点不变。

## 采用：Gemini Flash 显式思考预算上限

`0f4db7fcc733ea471afcb91528070c9889e2c4ff` 修复 Antigravity 转换链只给 Gemini 2.5 Flash 限制思考预算的问题。新版 Flash 在高推理预算下可能收到超过 24,576 的 `thinkingBudget`，从而被上游以 400 INVALID_ARGUMENT 拒绝。

- 在现有转换器中将显式正预算上限应用到所有 Gemini Flash 世代，保留 `gemini-` 前缀约束，避免像源补丁一样把任意名称含 `flash` 的非 Gemini 模型纳入限制。
- 沿用已有最终映射模型选择；原请求映射到 Flash 时限制，原请求名含 Flash 但实际发给 Pro 时不限制。内置搜索最终降级到 2.5 Flash 时仍适用原上限。
- 小预算、恰好等于上限的预算、动态 `-1`、未指定预算的 adaptive、Pro、Argon 和 Claude 保持原行为。先限制预算再计算输出 padding，不修改调用者的原始请求。
- 保留已有思考签名隔离、无效签名处理、模型映射、最大输出限制、推理内容和计费逻辑。

## 已有覆盖与明确排除

- `21bd7341d` 的质量检测状态安全断言，在个人 v2.1.44 已使用 checked assertion 处理；不重复变更。现有 `partial/error/failed` 安全终态判断保持。
- 上游给 GHSA-mrwp-f3q8-3g9p 添加 follow-redirects 漏洞豁免，本地此前已升级到修复版 1.16.2；保留空豁免列表，不导入豁免或降级依赖。
- v2.2.63–v2.2.65 的加密支付归集阻塞对账、真实错误提示和逐链 gas 报价缓存属于已经移除的原生支付/归集模块，不恢复服务、路由、页面或定时器。
- 上游薄荷、粉白、水墨樱花及 Nuxt UI 风格变更不采用；保留本地 shadcn 界面、橙色配色、模型分组和顶部导航。仅服务新主题的纹理资源检查放宽也不采用。
- 原生支付的支付宝/微信标签顺序调整不适用；继续使用链动小铺与本地兑换入口。

### DeepSeek Hosted 搜索格式修复的边界

`b6852a02a` 将 raw Chat Completions 中 hosted `web_search` 改写成网关自有 function，并依赖 group injection 开关、响应执行器和自动搜索收费闭环。本地没有且已明确延期这套 Hosted 模块；单独导入会向模型声明本地无法执行的工具，因此不采用，不将其记为完整“已有覆盖”。

本地 Responses→Chat 与 Anthropic→Chat 转换器已经丢弃不支持的 hosted 搜索声明及关联的工具选择，保留普通客户端 function；本轮验证已有转换用例。直接把 Responses 的 `type=web_search` 塞入原始 Chat 请求仍属不支持的协议输入，不宣称为其提供自动执行或新增前置验证。独立 `/v1/tools/search`、`/v1/tools/fetch` 以及现有按结果计费保持，不新增自动搜索、专用凭据、重试或全局 call-ID 扩张。

## 验证与证据

- 新增预算回归在旧实现上复现 8 个叶子用例失败（计入父测试为 10 个失败）；覆盖新版 Flash、adaptive 显式预算、输出 padding 和最终模型映射。
- 修复后 `go test -p 2 ./internal/pkg/antigravity -count=1 -timeout=5m -json` 整包 **112 项 tests/subtests 通过**，无失败、无跳过。新增用例同时覆盖动态预算、非 Flash 模型与调用者请求不变。
- `go test -p 2 -tags unit ./internal/service -run 'Test(.*Antigravity|GeminiThinking|ResolveGeminiThinking|.*Chanshui|UpdateSource|UpdateTracking|ParseDeepSeekTools|DeepSeekTools|ForwardDeepSeekTools)' -count=1 -timeout=5m -json`：**512 项 tests/subtests 通过**，覆盖现有网关桥接、更新检查点、质量检测和 standalone DeepSeek 工具边界。
- Responses/Anthropic→Chat 的 hosted 工具及 tool_choice 转换专项 **3 项通过**。以上三组共 **627 项通过，无失败、无跳过**。
- 独立只读审查认可预算限制、最终模型选择、输出 padding、请求不变和检查点表述；DeepSeek 独立调查确认 Hosted 补丁依赖当前延期模块。服务端构建、源码保护和敏感资料扫描结果保存在本轮本地证据目录。

所有测试使用本机或 fake transport 和虚构凭据，不调用真实模型或收费搜索。本轮前端源码未改，不生成前端构建资源；没有运行全后端套件、Linux race、真实数据库集成或生产部署，针对性检查不等于已通过正式发布 CI。

证据位于工作区 `development/diagnostics/upstream-v2266-20261101/`，包含源码基线哈希、正式 Release JSON、远端 tag refs、完整增量、测试 JSONL 与摘要、构建和敏感资料扫描结果及独立审查。原有三个未跟踪测试和历史工作树不属于修改范围；源码保护校验以该目录的 `preservation.json` 为准。

发布阶段仅追加个人 VERSION、发行说明和发布状态记录，不扩大功能范围。同步阶段的本地验证保留其原有时间与范围边界；v2.1.45 的最终源码身份、PR/main CI、Release/GHCR 二进制一致性、备份实际恢复及线上验收另存 `development/diagnostics/sub2api-release-v2145-20261101/`。
