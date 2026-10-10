# Sub2API 2.1.3 同步

- 本地输入基线 `c7c719eb5`；上游审查区间 `4dc2e58ffbeb3533a659f2de4d020bd362cfc4a6` → `2d478657a84b70e869e097343b3d15d51cbc23bb`，涉及 81 个 delta 文件。Release `v2.1.2` peeled commit 为 `8595e9980a545d4588341307d6f9fcb4b95e39bc`。
- 发布前上游更新至 `9bbe1e2bf249cdec2e77da9480d746ccb48bfff8`（v2.1.3 tag peeled commit 同值，tag object `89a0492f02aba0c8403866ab231f076a3e1ed9f8`）；追加审查 8 个提交。最终审查截止于此，不追逐发布过程中后续移动的 main。
- 按 `review-diff-then-squash` 选择安全 hunks；不导入上游历史。版本文件为 `2.1.3`，新 release workflow 仅接受数字 `vX.Y.Z`；历史 personal 版本读取、比较和回滚兼容保持。
- Cline / Command Code 平台常量、路由、配置、连接测试、前端入口与迁移 255 整体暂缓。审计发现连接测试向 Anthropic 发送凭据，映射 Claude 模型按原始模型选择错误协议。平台无关协议修复独立接受。
- Honeypot 模块、路由、拒绝原因和封禁 wiring 整体暂缓：合法查询会被关键字规则误拒。认证中间件保留原 `c.Next()`；在途展示单独挂载在鉴权之后。
- 在途展示补齐独立受锁元数据快照，后台不读取 Gin Request 或 response header map；请求线程发布模型、账号、实际路由组、reservation、stream 与端点。补齐最终每页 10 条之后的分页防溢出、显式 `include_inflight=false`、持久化请求 ID 前缀去重、Redis compare-and-delete / tombstone 原子化和前端 abort / generation / 筛选与列表版本隔离。
- 保留预留默认关闭及本地预授权费用计算。协议修复接受 legacy function ID、OAuth web_search history / Responses Lite、加密内容错误重试、WS 后续轮次同组价格刷新、Grok 空 completion failover。真实 refusal 文本算语义输出；terminal-only refusal 转换补齐流式与非流式一致性。
- 原 lock 的五项 deferred 原文保留，增加两项；不重新引入已删除的 DeepSeek inline parser，也不导入 summary-only 请求历史。保留 WS 原始模型先校验再去重、dedupe-before-media、UseNumber、管理员归属与缓存失败拒绝、充值 UI。
- 未新增迁移。数据库迁移验证、备份和生产受控切换仍须按既有流程完成；本记录不替代 CI、制品校验或部署验收。
- 2.1.3 增量：接受 HTTP Clipboard 降级、Gemini 映射真实目录与上游错误透传。请求 ID 采用支持现有索引的完整 ID 等值查询（含 upstream ID 和裸 client/local ID），通配字符按字面处理，前端提示完整 ID；延期片段包含搜索和索引迁移 256a/256b：缺扩展启动失败和无效并发索引重试未修复，不能先启用热表扫描。
- 最终审查发现 Opus 5.5 新校验只看公开模型名，渠道/账户映射存在绕过和反向误拦截；撤掉本轮新增校验器及 hooks（上游源码仍可恢复），整体延期到有效模型与原始请求联合回归通过。保留既有模型/参数校验和 raw-body 防护。
- 上游 sticky 删键与 inflight 前缀剥离不机械移植：本地能力感知 native replacement/continuation/transport/compact 保护及最终重绑已覆盖其意图；本地 inflight 与 billing 都使用规范命名空间 ID，保留完整 client:/local: 身份以防碰撞。已有在途缓存是 best-effort 展示：删除失败时最多保留末次心跳后 60 秒，并非数据库完成状态的全局查询。
