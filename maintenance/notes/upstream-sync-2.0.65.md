# Sub2API 2.0.65 同步

- 个人基线 `b8f1e136d3e335516f1ce449c8b5de7e20d125c0`；本轮审查上游 `0710c3e6a7d39089b4edda5530ca5faf481ba3c1` 至固定 cutoff `6278f0e2bd09d7710a96729a8adeb21ec788f249`，正式 v2.0.65 release commit 为 `1557adff351fcafb02f78cae13c497b0a4aedfd1`。
- 导入普通 Responses 会话由 Chat Completions 转换账号向合格原生账号让位的调度变更。个人版同时覆盖 legacy 和 advanced 调度；没有合格原生候选时保留有效的转换账号会话。强 `previous_response_id` 绑定、guardian parent、既有调度开关、模型/组/传输/财务准入和 compact/fill 优先级仍按原有规则。
- 新增 DSML 解析、桥接/透传接线和全局 analysis/summary 标签识别暂缓，不视为已接入。独立审计发现：残缺工具标记与正文代码示例会合成为可执行调用，工具索引/事件顺序/终态身份不一致；原生 DeepSeek 正常 Forward 路径无法进入新增透传接线，该接线内部还破坏完整 SSE 帧边界并漏过生成输出的预扣观察点。安全接入需要 provider 能力边界及实际 Forward 级测试，不能仅通过 helper 单测放行。
- 新增字面协议回归测试：普通 HTML、analysis/summary 标签、完整 DSML local_shell 代码示例和未闭合 invoke 必须原样留在正文。暂缓前四个用例均复现失败，暂缓后全包通过。保留现有 think/thinking 分离、双思考通道去重、reasoning-only 可见回落和结构化工具处理。
- 历史暂缓的 Grok 拒答计费与 summary-only 明文请求思考历史不变。同步范围和原因均记录在 `maintenance/upstream.lock.json`，上游原版说明文件仅作为来源存档，个人发布说明描述实际行为。
- 保留五档独立小铺商品链接、模型广场透传展示、安全 Grok 下载和滚动时显示的滚动条。无数据库迁移、依赖或部署脚本变更；不修改客户余额、倍率、账号归属、小铺链接及 updater 策略。
- 本文件记录源码同步决策；确切 SHA 的 CI/security、发布资产与生产验收证据保存在本轮运维目录。
