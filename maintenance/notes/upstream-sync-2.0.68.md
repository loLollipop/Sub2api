# Sub2API 2.0.68 同步

- 个人基线 `edf577657ac8d1b2d245e772a0172b550b9d7c53`；上游差异从 `8a2ddd52668a5bff824c3eadb0c7c8aec09c212b` 到 cutoff `4dc2e58ffbeb3533a659f2de4d020bd362cfc4a6`，共 36 个文件。
- `refs/tags/v2.0.68` 实际指向 `2b518c1eaacdcaa7039e4b318b6299f1510d2be2`；后续 `26fe7a19a` 是 VERSION 同步提交，cutoff 还包含 Go 1.26.9 和标准库 HTTP/2 维护更新。
- 按 review-diff-then-squash 策略仅导入审查后的文件差异，不接入不相关的上游历史。个人版本为 `2.0.68-personal.1`；个人 CI 仅变更 Go 版本断言。
- 导入 DeepSeek 结构化工具 call_id 去重与 HTTP / WS 请求接入，保留第一次调用和第一次结果，不再给重复工具改名。JSON 解码仍使用 UseNumber，以保留大整数精度。
- WebSocket 后续轮次先校验原始 JSON 的重复模型及白名单候选，再执行 DeepSeek 去重，避免 JSON 重建折叠重复键后绕过已有准入校验。
- 导入 messages 分发开关、生图重复模型拒绝，以及配置限额时 Redis 计数 / Key 用量读取失败拒绝的行为和上游回归。补齐标准模式 Key 创建限额及 RPM 限额启用但计数缓存缺失时的拒绝；不限额及 simple 模式 Key 创建保留既有行为。
- 导入 Go 1.26.9、相关 `golang.org/x` 依赖和出站标准库 HTTP/2 配置；不改默认平台传输模式与官方 OpenAI 首字节阶段的空闲 PING 策略。服务端 h2c 迁移暂缓：上游会把独立的 h2c IdleTimeout 写入 HTTP/1 Server.IdleTimeout；保留旧配置并补独立超时回归。
- 异步生图多分组入口放宽暂缓：当前 Submit 沿用主组平台，主组审计完成标记也可能跳过目标组策略。保留入口平台 / 生图权限拒绝，待目标平台和审计覆盖后再接入。
- `openai_gateway_response_handling.go` 的 47 行 DeepSeek 回复重写接入完全暂缓：依赖此前未导入的 parser / bridge / passthrough。三项原有暂缓决定保持完整，结构化请求去重不解除 DSML 延期。
- 个人充值 UI、账户归属、MIME 模型守卫、会话延续和工具转换改动保留。无数据库迁移；部署继续使用既有受控升级和备份流程，最终测试与发布证据由本轮验收记录补充。
