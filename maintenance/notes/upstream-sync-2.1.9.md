# Sub2API 2.1.9 同步

- 本项目基线 `0ca4d164e54f989524e4eb74bb3e00b593b6e708`（2.1.6）；冻结上游 `f743ca8e23ff036f628010f4272ab13cbd7f61b1` → `9e2f733aa93d95acee551a5cd272708c9ea544b5`，9 个提交、69 个 delta 文件。review-diff-then-squash，不追逐后续 main。
- 导入 Grok 普通文本快速路径与等价性/回退测试，流式最终错误统计、嵌套 524 分类与提示脱敏。最小移植取消跟踪器的 timer.Stop/doneOnce 清理；不替换本地 UsageInflightProgress 快照架构或鉴权流程。
- 2.1.7/8 请求 ID 搜索仅导入点击筛选、trim/空值归一化和三个 Ops handler 的参数透传；完整 ID、client/local 命名空间和特殊字符保持精确 SQL 比较。ILIKE 前缀和 contains 缺少可靠索引，继续暂缓 256a/256b，不导入新的模式语法或错误说明。保留本地日志/在途列表的异步 stale-response 防护。
- 暂缓整个 `95db8e604` 质量模块重构：runOnePlan 将旧 Pelican 质量计划直接送入 runChanshuiPlan，normalize 只禁用副本且发送前不检查该标志，可能把密钥未经确认发送到外部服务；同时移除旧 compare-and-clear 暂停恢复。保留旧 provider 分派、视觉检测、CAS 恢复及结果窗口修复。生产只读聚合检查当前无计划、无 legacy quality pause；不以当前零数据替代升级兼容性保障。
- 原 9 项 deferred 保留并新增质量迁移一项。0.15 倍率、余额、充值界面、owner 隔离、逐跳下载安全、sticky/continuation、金融错误与终止帧保护均不变。
- 无迁移及依赖变更；生产仍 validate/update disabled/preauthorization false。仅部署本仓库校验固定的数字版本制品；发布及数据一致性证明见本次 CI 和部署验收。
- 独立审查补齐上游遗漏：客户端取消后的错误尾帧只保留已观测用量，不标记 provider SLA；嵌套 524 的返回错误使用已净化消息；HTTP 524 不再复用原始 JSON 提示。部署更新器的恢复/stop/reload 任一步失败均禁止 start，并停止未验证实例，新增故障注入回归。
- 修复已有 AccountsView 全量选择测试的 debounce 清理：在 mocks 重置前结清 timer/request，并保证组件 unmount，避免全量 vitest 通过断言但因迟到 promise 拒绝失败；产品行为不变。

## 2026-10-12 质量模块补同步

后续按用户澄清导入上游 `95db8e604` 的完整质量模块设计，并补旧 provider 关闭与外发前授权检查。此前的暂缓是当时同步结果，不再表示这部分仍保留旧视觉检测；具体兼容边界和旧暂停人工恢复步骤见 [质量模块补同步记录](upstream-quality-default-off-20261012.md)。其他九项 deferred 保持不变。
