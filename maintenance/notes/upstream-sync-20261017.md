# 双上游同步 — 2026-10-17

基线为本仓库 `origin/main@1f048a6fe`，在 `codex/sync-both-upstreams-20261017` 上选择性适配，不导入上游提交历史。

| 来源 | 上次检查点 | 本次检查点 | 对应 Release |
| --- | --- | --- | --- |
| Wei-Shaw/sub2api | `1b2243a4f50621508e79aef4b498a2937a6238ee` | `b0a02a3304a0a78cdd003ddda1dd23ba02c03084` | v0.2.17 (`060d01604`) |
| kiss-kedaya/sub2api | `f7bde184592d6855d4b27d1f705a8aad13f49e3f` | `ce66c457c779653414b8776126144e9bd8148027` | v2.1.17 (`c11182075`) |

## 适配范围

- Responses 普通 HTTP/WS/JSON 出口补齐缺失或 null 的 `created_at`，同一流复用时间；透传只观察原帧。合成失败终态带时间及序号，保留本地空成功响应切换、usage 保护、524 和财务错误脱敏。
- Gemini Chat/Messages 兼容入口转发支持的思考预算/档位、流式返回思考摘要，移除历史思考内容；签名降级重试保留客户端意图。记录实际发送的档位，沿用本地 0.15 结算倍率。
- GPT-5.5 退休仅针对 OpenAI OAuth/setup-token/凭据影子账号。API Key 及显式映射到可用替代模型保持支持。适配本地调度快照、短缓存、空目录、复合分组、compact、公开别名投影及最终 ETag。迁移说明见 [GPT-5.5](../../docs/OPENAI_GPT55_RETIREMENT.md)。
- compact 目标意图贯穿两套调度器的初筛、抢槽、负载查询失败回退和等待；多分组 allowlist 保留权威空目录。Live/Realtime 的空模型占位符仍按端点能力选号。模型别名规范化对无空白输入跳过切分，保留既有缓存命中零分配约束。
- OpenAI API Key `/v1/messages` 搜索模拟复用共享配置；先保留原生 Anthropic 转发，再判断模拟；OAuth/setup-token 排除。编辑账号时保留其他 OpenAI extra 字段。
- 自定义错误停调改为 30 天临时不可调度，覆盖原自定义处理路径、选中 402/529，使用独立 reason 防止余额探测提前清除。与二改源实现一致，认证、普通限流、显式模型规则及供应商特殊策略的既有优先级保留；未把所有 4xx 强制改成 30 天。
- 支付补支付宝退款查询、审计时间及读取错误处理、易支付查询参数与返回值兼容和通知校验、履约类型校验、Stripe 同订单本地密钥恢复、return URL 主机校验。

## 本地实现优先项

支付宝保留稳定 `orderID-refund` 请求号，防止重试重复退款；旧交易号形式的 refund ID 不猜测查询。查询异常维持 pending，无记录须等待 10 分钟。保留本地退款保留扣款、终态事务认领、失败单次归还和 live balance 更新；不导入上游较弱的第二事务失败兜底。

易支付继续在入账前按订单固定的实例配置对账，不再叠加上游 handler confirmer。缺失 code、错误订单号、非成功 HTTP 状态继续拒绝；缺失网关交易号保持空值。查询 URL 的商户密钥从传输错误中剥离。

本地充值中心/商城、0.15 计费、原生厂商、默认关闭的外部检测和财务脱敏保留。既有 LinuxDo referral、多邀请码以及两个 maintenance 台账中的其他排除/暂缓项未重新启用。未新增迁移、依赖升级或项目发版号。

## 验证

- 后端：`go test -p 2 -tags=unit ./...` 全部通过，service 包耗时 225.791 秒；`go vet -p 2 -tags=unit ./...` 及 `go build -p 2 -tags embed -trimpath ... ./cmd/server` 通过。
- 本地适配定向回归已通过：两套 compact 调度器、批量与非批量、透传与普通账号、负载查询失败回退和无槽位等待；Live 空模型能力选号；多组 allowlist/Composite/短缓存空目录；WebSearch 原生协议边界；模型支持缓存零分配；终态完整帧与断连 usage。
- 前端全量 385 个文件、3066 项测试通过；类型检查、全量 ESLint、最终 locale ESLint 与 i18n 完整性检查通过；Vite 生产构建及 console 资源校验通过。
- 完成独立支付数据完整性审查、网关及兼容行为审查，并修正本次回归问题；暂存差异检查通过。详细执行日志保留在本地忽略目录 `.artifacts/upstream-sync-20261017/`。

验证环境为 Windows、Go 1.27.2、CGO 关闭；未运行 race、golangci-lint、Docker、真实 PostgreSQL/Redis 集成或 Linux 生产镜像构建。所有支付及网关回归使用本地桩或测试服务器，不调用真实支付/付费接口。

本记录只对应源码同步；发布、真实 PostgreSQL/Redis/Docker 集成和生产部署仍由独立流程验证。

## 既有后续问题

独立财务审查发现：整份订阅退款先撤销订阅、网关返回 pending、后续确认退款失败时，现有归还逻辑无法读取已软删除的订阅，订单会持续 pending。该路径在同步前基线中已存在，本次没有改写订阅撤销/恢复机制，应单独修复并补退款失败恢复回归。
