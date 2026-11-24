# 2026-11-24 双源选择性同步与原版图片画布

本次同步与画布接入以个人版本 2.1.72 为基线完成本地验收。用户随后授权提交 GitHub 并更新部署，个人 VERSION 升至 2.1.73。原有三份未跟踪用量测试、`account_test_models_test.go` 和旧 SQL 保持字节不变并排除无关改动。发布过程核对 PR/main CI、自有 Release/GHCR 制品、实际备份恢复与线上健康，最终结果记录于发布诊断目录。

## 固定来源

| 来源 | 正式检查点 | 提交 | 本次增量 |
|---|---|---|---|
| kiss-kedaya/sub2api | v2.3.158 | `22b47efd2062325e8a2924a6f006457858a9dd93` | 从 v2.3.155 起，14 commits / 98 paths |
| Wei-Shaw/sub2api | v0.2.25 | `348997267d756eab5729bb6b78a275a15d18796a` | 从 v0.2.24 起，15 commits / 91 paths |
| basketikun/infinite-canvas | 用户提供的真实源码 | `9cd3deacbcc598361807afc16c059a80dd18b6f9` | 原 React 图片画布及其依赖 |

逐项取舍见 [完整提交与路径清单](dual-selection-ledger-20261124-canvas.json)。两源未发布的 main 仅记录观察，不导入。本次是行为切片回移，检查点推进不表示所有变更已合并；延期路径也不声称逐行审计。

## 已接入的行为

原版：错误透传中的 invalid_encrypted_content、上下文容量类错误和管理员错误规则；type/code/param 只接收有限长度标量；上游主机名及凭据脱敏；compact SSE 保留错误 code，并适配可重试容量错误。保留本地财务隐私、状态码和可操作文案。

二改：Messages、Chat Completions、Responses、Gemini 的无账号错误不再直接回显内部调度错误；内容审核总开关关闭时不统计违规或自动封号，前后端同步约束；新配置 cyber_policy 默认不计入自动封号，已明确保存的旧策略仍受尊重。

画布：按用户后续决定撤回自制 Vue 画布，沿用二改的同源 iframe、显式创建 Key 和不自动轮换原则，嵌入 basketikun 的真实 React 界面。保留图片节点、文本提示、连线、参考图编辑、原参数面板、图片存储及导入导出；不开放视频、音频、Agent、文字模型生成和远程插件加载。

图片请求限定到本站 Images API，专用 Key 仅绑定一个当前有权使用的生图分组。Key 通过校验 origin/source 的消息握手保留在内存，不放 URL 或持久化配置。IndexedDB 和存储回退均按登录用户命名。图片失败不会自动重发收费请求，计费沿用现有网关。

资源依次构建控制台与画布，再随 Go embed、Docker、GoReleaser 和 release CI 打包。仅画布响应允许同源嵌入，以及读取本地 data/blob 图片；其他网络限制及父控制台 CSP 保留。静态代理不转发登录 Cookie、Bearer 和 Referer。窄屏默认收起原侧栏，原节点面板在窄屏固定显示，桌面沿用聚焦算法；切换画布会隔离组件状态并取消旧请求，避免旧结果写进新项目。

## 延期内容

不引入备份新体系、Nuxt UI、完整 Key seal/rotate/factor 流程、账号备注与 header identity 迁移、完整调度结构化计数、独立管理员审核邮件体系、新出站 safe-dial 基础模块和未完成的协议守卫扩展。model-sync SSRF 未包含在本轮修复声明中。旧 websocket 漏洞修复已由前一检查点覆盖。

## 验证与运行条件

源同步定向回归的 Go JSON 日志记录 1308 个测试/子测试通过，零失败；另有画布静态服务、嵌入资源、CSP 与 middleware 回归。控制台相关 Vitest 55 项通过，新增会话用例覆盖显式创建、单组绑定、撤权、失效、并发去重和账号切换。前后端定向 lint、Vue/React 类型检查和生产构建均记录在诊断目录。

浏览器验收使用实际编译的 Vue 宿主和原 React 画布，所有 API 均为本地隔离 fixture；不调用生产或付费模型。最终结果和截图见 `development/diagnostics/sub2api-dual-canvas-20261124/browser-real-canvas-result.json` 及同目录 `real-canvas-*`。目录中早期自制画布的截图、浏览器日志与构建结果已被本方案取代，不能作为本方案验收依据。

没有实付请求，实际账号的上游图片质量及特定模型参数未做付费实测。同步阶段本地没有执行 Docker 镜像构建或 GitHub workflow；发布阶段的远程 CI 和制品结果记录于 `development/diagnostics/sub2api-release-v2173-20261124`，不回写此前本地验收日志。画布在浏览器本地保存，跨设备不会自动同步，清理站点数据前应导出。使用方式、依赖安装、源码维护和许可见 [画布说明](../../canvas/README.md)。
