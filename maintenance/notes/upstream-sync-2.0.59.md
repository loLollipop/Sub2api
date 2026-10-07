# Sub2API 2.0.59 源码同步

- 按 `review-diff-then-squash` 审查旧导入树 `6dac7d1726557ce6a969b515e90ad6f252a7a03c` 到固定上游 `d40387ff6346c8f7073d28aff94d8374632df56e` 的差异。上游重写过历史，因此依据 tree delta 导入，不导入上游作者或提交历史。
- 包含 2.0.59 安全更新、后续 build-tag 和开发预览 DTO 修复。加强账号池权限、数值/分页契约、支付查单及回放防护、初始化邮件/管理员校验、重复 model 字段拒绝。
- 账号池权限由邮箱切换为 `security.account_pool_owner_user_id` / `SECURITY_ACCOUNT_POOL_OWNER_USER_ID`。正整数指定的管理员可管理全池；其他管理员只能管理自己的账号。0 保留未配置部署的全池可见性。
- 生产启动前必须从原邮箱配置解析同一 active 管理员 ID 并设置新变量。当前只有一个 active 管理员；不得以默认 0 短暂运行，也不靠邮箱继续授予权限。
- 修复同步审查发现的 Spark 影子归属缺陷：影子继承经过权限校验的母账号上传者，legacy nil 归属不被仓储改写为创建者；母账号所有者修改代理能继续传播到影子。客户端没有上传者字段入口。
- 修复上游 CI 的 6 项 lint 报错：分页 context 类型断言使用 comma-ok，拒绝异常类型/空 loader 导致 panic；Go 格式化；支付日期错误信息小写，并保留回归检查。未降低 lint 或测试标准。
- 前端按三方差异同步 DTO、分页和输入约束，保留个人 SettingsView 模块/卡片；affiliate 分页回写在独立卡片控制器中实现。保留首页 compact 透明头部、共享 AccountMenu、主题工单面板、品牌字体、个人构建脚本及 source-map-js override。Vue 范围更新为 ^3.5.43。
- 保留个人更新器校验和、Release 仓库限制、生产 `UPDATE_STRATEGY=disabled` 和 schema validate-only 策略。Grok moderation refusal fixed-charge 继续 deferred，两个 gateway handler 仅导入 duplicate model 校验。
- 无新增数据库迁移；已有 2.0.58 易支付真实查单与历史未绑定订单处理约束继续有效。

本文件记录源码候选，不表示生产已部署。发布/部署由主线程在最终验证、数据库及配置备份后执行。

## 本地验证

- Go 1.26.6、unit 标签下的 service/repository/admin handler/response/config/setup/middleware 聚焦权限、分页、支付、计费、初始化、数值、重复模型与影子账号回归通过；新增影子归属、代理传播和 legacy nil 仓储回归通过。
- pnpm 9.15.5 frozen-lockfile 安装、前端 typecheck、dev TypeScript 检查通过。24 个变更及个人 UI 测试文件的 371 个测试通过，独立 affiliate 分页回归 1 个测试通过。
- 完整生产构建及内嵌 console assets 验证通过，i18n 检查 3 个测试通过；修改 Go 文件 gofmt 检查和 git diff whitespace 检查通过。
- Linux 全量 CI、独立审查和生产验收仍由主线程完成；本地结果不代替这些检查。
