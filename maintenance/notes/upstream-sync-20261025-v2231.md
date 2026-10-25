# 二改正式 v2.2.30–v2.2.31 选择性同步（2026-10-25）

## 来源与范围

- 二改仓库：`kiss-kedaya/sub2api`，仅以正式发布为同步依据。
- 已有检查点：v2.2.29 `6add01cc1cbc2b89cf557aab1154503aa07cd297`；上次已审 main 为 `63c1a4e66fe79b780f81bec3053efb6df8140130`。
- v2.2.30：正式发布于 `2026-10-25T08:27:16Z`，release commit `b175daaf142177257a96bca7a6f2c08a53fafb85`。
- v2.2.31：正式发布于 `2026-10-25T10:02:53Z`，release commit `a3401d78c7d774042ddcb43c4c005945b249160f`。

本轮选择性适配 v2.2.30 的 Anthropic 最终累计用量和缓存时长明细。参考 `bb39d2f0a`、`d1ae31bd5`、`40defffdd`、`9f728c580`、`12ccb50f0`、`42f68aa31`，按当前代码改造，不整文件覆盖或直接 cherry-pick。

字段 presence 区分省略/null 与明确数字 0；最终零值可以替换早期累计量，四个 buffered 路径无条件使用最终累计 usage。保留并贯通 nested cache creation 和本地扁平 5m/1h 字段。

v2.2.31 的 `50f3bb2eb`、`07d16bf35` 仅修复原生链上支付归集前的余额刷新及地址保留额；该支付模块已按用户要求移除，本轮排除全部 crypto 及其前端改动。

## 保留的本地行为

本地已有 `anthropicCompatStream` 和 native pump，支持客户端断开后停止下游写入、继续读取同一次上游响应及最终用量，并保留超时和禁止重放语义。保留该实现，不替换成上游新加的较简化排水逻辑。

保留 Kimi、GLM、DeepSeek 的 prompt/cache 别名、互斥计费桶、真实 5m/1h 明细及账号/全局 usage override；保留最终模型映射、`repriceOpenAIFinalAttempt`、配置的输出下限、signed thinking、文件工具结果和错误终止处理。不更改路由、调度、缓存策略或单价。

上轮排行榜 66 个改动和新增文件已在本轮开始前按原字节备份并记录 SHA-256。本轮不改排行榜、现有界面、已应用迁移或依赖，不恢复工单、无限画布、原生支付或渠道定价。

## 验证与发布状态

独立 deep_auditor 指出上游合并语义还有两个边界：显式聚合 0 搭配部分/空/null TTL 对象时可残留旧的计费明细；没有聚合更新的空/null TTL 对象又可能把旧聚合重算为 0。新增跨路径用量与实际缓存写入费用断言，修复放在本轮用量适配层，通用单价和 legacy detail-only 计费策略不改。

主代理新增回归确认修复前上述边界失败，完成修复后由新的独立 deep_auditor 狭窄复核，未发现新增缺陷。最终验证（Go 1.27.3）：

- `go test -p 1 -json -tags unit ./internal/pkg/apicompat -count=1 -timeout=10m`：全包 787 个测试/子测试通过，无跳过。
- service/handler 的 Anthropic、SSE usage、缓存、Kimi、Claude 5.5、检查点、排行榜及实际计费回归：2353 个测试/子测试通过，无跳过；与上一项共 3140 项，不重复累计修复前轮次或子代理结果。准确命令在本地 `final-verification.json`。
- 覆盖 Gateway/native × Chat/Responses × buffered/stream 的最终 wire/result 用量一致性；明确零、缺失/null、混合与部分 TTL、空/null TTL、实际缓存写费用、国内供应商 aliases；断连后同一次上游末尾 usage、请求次数为 1、失败后不再写下游。
- 17 个本轮改动/新增 Go 文件的 `gofmt -l` 无输出；`git diff --check` 通过。
- 86 个改动和新增文件使用项目敏感信息 scanner 检查，无发现；原有 66 个文件 SHA-256 与同步前完全一致。
- `go build -tags embed -o NUL ./cmd/server` 通过；未在项目生成发行二进制或打包文件。

正式发布/API/tag、源差异与测试证据保存在本地 `development/diagnostics/fork-v2231-20261025/`。测试使用本地 mock/fixture，不调用真实付费模型或生产数据库。未执行全量后端全包、race、Linux CI 或生产验收；本轮前端源码未改，未重复前端构建与浏览器检查。

本轮仅保留本地；不提交、不推送或部署。本项目 VERSION 保持 2.1.36，源检查点与本项目发布版本分开记录。
