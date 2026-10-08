# Sub2API 2.0.66 同步

- 个人基线 `27b45ec8c3095998d393aec8380100c10bef4877`；审查上游锁定提交 `6278f0e2bd09d7710a96729a8adeb21ec788f249` 到 cutoff `8a2ddd52668a5bff824c3eadb0c7c8aec09c212b`。正式 v2.0.66 对应 `eff38f3ac79b9467a772d2fc14725933f9cf5bff`。
- 导入 `eff38f3ac` 的重复模型拒绝逻辑及对应回归测试。复用既有 JSON/multipart 模型候选提取，在 OpenAI 兼容 HTTP 入口、Messages、Grok 媒体和 Responses WebSocket 首帧/后续轮次中拒绝不同模型候选；相同非空候选不判定为冲突。
- 发布前独立审计发现伪造 multipart Content-Type 可使整份有效 JSON 得到空候选，绕过新增冲突校验和分组白名单。个人补丁仅在声明 multipart 但正文为整份有效 JSON 对象时使用 JSON 候选；真正 multipart 不执行部分 JSON 扫描，保留顶层 model/session 和前导段语义。新增包级、实际 Gin 准入与 Responses/CC 无上游/无 usage 回归，以及白名单关闭时 WS 首帧/第二轮两种转发模式的精确关闭原因回归。
- 依赖 lenient normalization 的 BOM/原始控制字符请求不在该 MIME 补丁的严格 JSON 覆盖承诺中；后续仍需单独核验准入与 normalization 的顺序，不将此补丁表述为所有历史解析差异均已修复。
- 上游改动与个人充值页、账号归属和会话调度补丁不重叠；按 review-diff-then-squash 策略保留维护者历史，不导入上游 Git 历史。
- 之前暂缓的三类功能保持原有决定与原因，不因提升上游版本号视为已接入；锁文件仍完整记录。
- 同批发布充值页一屏布局修复：桌面顶部合并、低高度紧凑规则、原生可访问说明折叠；小铺跳转和支付逻辑保持不变。
- 无数据库迁移、依赖或部署文件变更。VPS 继续 validate-only 迁移策略和禁用应用自更新，余额/倍率/归属/商品链接不做转换。
- 精确发布 SHA、CI/security、资产摘要、备份和生产验收证据保存在本轮运维记录中；发布前需通过这些门禁。
