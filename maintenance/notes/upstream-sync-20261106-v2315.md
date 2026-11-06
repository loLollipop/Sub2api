# 二改正式 v2.3.15 选择性同步（2026-11-06）

## 同步结果

二改本次正式发布的 GIF 数字字符串兼容修复，已在本地 v2.1.53 中采用。本轮推进正式来源检查点到 v2.3.15，并核对现有实现；图片处理逻辑无需重复修改。

- [正式 v2.3.15](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.15)：非草稿、非预发布，发布时间 `2026-11-06T11:19:24Z`（北京时间 19:19:24）。
- 标签对象 `07c344f522fe383ee420a59144e249f90e52b932`，解引用提交 `a17cc8118ffdd1dc5931a3462aabf30685994630`。
- 上次正式检查点 `ca2abf5c537d372084b8852c57a5883fcb52c804`（v2.3.14）到本次正式标签共 3 个提交、3 个路径：来源 VERSION、Images 实现及其测试。
- 其中业务修复为 `9c518bf7f14754a32284181c23b2bd0ab98fd9fe`；此前已单独采用，证据见 [v2.3.14 同步记录](upstream-sync-20261106-v2314.md)。另外两个提交是来源 VERSION 记录与合并提交。
- 观察到 main 为 `f035fdc0eb1aa9cebc816227703285f54505a6ed`，比上轮已审 main 只多一条 VERSION 更新；正式检查点仍使用标签解引用提交。

## 核对的现有行为

- JSON 的 `frame_count`、`frame_duration_ms` 接受数字和十进制整数字符串。
- API Key 生成、编辑请求向上游发送 JSON 数字；OAuth 请求构造使用解析后的整数。既有数字与 multipart 表示继续有效。
- 本地保留精确整数、非负数及溢出检查，以及 GIF 合成和资源预算；没有用来源较宽松的 `gjson.Int()` 替换这些校验。
- `n`、`output_compression`、`partial_images` 不扩展为接受数字字符串。

本轮修改 `maintenance/upstream.lock.json`、`update_source_baselines.go` 和本记录。正式 Release 监控策略、原版仓库检查点、既有计费策略与模块排除项保持原决策；个人 VERSION、依赖、迁移、部署配置没有变化。

## 已有改动与验证

同步前保存了 31 个已有修改路径的补丁、内容副本和 SHA-256，位于本地 `.artifacts/upstream-v2315-20261106/`。这包含紧凑时间范围、模型后独立的思考强度列、搜索和错误列表竞态修复、无调用代码清理，以及开始时已有的三份未跟踪 Go 测试。同步只新增来源检查点修改，不覆盖这些内容。

- `go test -p 2 -tags=unit -json ./internal/service -run '^TestUpdate(Source|Tracking|Service)'` 通过：32 个顶层测试、54 个测试及子测试，无失败或跳过，覆盖检查点一致性、正式 Release 提醒和更新服务。
- 来源锁文件可解析，正式标签和观察 main 的记录与实际 Git 提交一致；既有暂缓项和正式 Release 监控策略核对一致。修改的 Go 文件通过 `gofmt` 检查，`git diff --check` 通过。
- 31 个已有修改路径逐项 SHA-256 比对通过，包含已删除路径仍保持删除。两份检查点文件的原内容也已留存副本。
- 未改变的 GIF 行为复用本轮之前已通过的后端全量 unit 验证；前端没有改动，沿用此前界面、相邻列、类型、lint 和浏览器验证。本次没有重复全量测试，也没有发送真实模型或付费生图请求。

本次测试原始事件日志为 `.artifacts/upstream-v2315-20261106/update-tests.jsonl`。

本轮在 `sync/upstream-v2315-20261106` 分支完成适配。用户随后授权提交 GitHub 并更新部署；与已验收的使用记录修复和代码清理一并按个人 v2.1.54 发布，最终 CI、制品身份与线上验收另存发布证据。
