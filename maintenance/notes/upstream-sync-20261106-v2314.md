# 二改 v2.3.14 审查与 GIF 数字字符串适配（2026-11-06）

## 采用 GIF 参数兼容修复，保留现有计费和界面

JSON Images 请求现在可以使用以下参数；API Key 转发时把字符串转换成 JSON 数字，OAuth/Responses 转换链使用解析后的整数。

```json
{
  "model": "gpt-image-2.6-sunburst",
  "prompt": "a looping lantern",
  "output_format": "gif",
  "frame_count": "8",
  "frame_duration_ms": "200"
}
```

现有数字、未设置控制参数的请求及 multipart 表单保持原有表示。生成和编辑两个 Images 入口均支持；其他请求字段不改写。仍拒绝负数、非整数、溢出、空字符串、布尔值、null 和容器类型；字符串不接受指数或小数形式。JSON 数字原有的精确整数表示仍有效，包括值为整数的指数和小数写法。

不采用上游数字路径直接调用 `gjson.Int()` 的实现，以免丢失本地非整数拒绝和溢出校验。沿用 `boundedJSONNonNegativeInt` 处理 JSON 数字，以 `strconv.Atoi` 检查新字符串形式；拒绝负号，包括 `"-0"`。格式和范围仍按原有契约交给上游，GIF 本地修正的帧数、时长、像素、合成与累计资源预算不变。

## 正式发布和指定 main 补丁分开记录

- 本地起点：个人 v2.1.52 `d9b33c30d1d51524da0751977fc7109d18be8528`，此前已完成使用记录自适应布局的未提交修改，本轮不改动这些文件。
- 旧正式检查点：v2.3.13 `4e3cd3c0a7b5af8d311a1418d7d797ae552d615f`。
- [正式 v2.3.14](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.14)：解引用提交 `ca2abf5c537d372084b8852c57a5883fcb52c804`，非草稿、非预发布，发布时间 `2026-11-06T04:56:52Z`（北京时间 12:56:52）。
- 观察到 main `a17cc8118ffdd1dc5931a3462aabf30685994630`；旧正式检查点至该 main 共 7 个提交、8 个路径，均为后端或 VERSION，没有前端更新。
- 采用 [GIF 修复 leaf `9c518bf7f`](https://github.com/kiss-kedaya/sub2api/commit/9c518bf7f14754a32284181c23b2bd0ab98fd9fe)。它晚于 v2.3.14；用户已在知悉该状态后明确要求选择性同步。这是对该补丁的单次授权，不改变仅监控正式 Release 的策略，也不以 main 替代正式检查点。

`maintenance/upstream.lock.json` 与 `update_source_baselines.go` 的正式审查检查点推进到 v2.3.14，所选未发布补丁独立记录。没有修改个人 VERSION、依赖、迁移、Ent、CI 或部署配置。

## 不导入估算计费修正

v2.3.14 的 `40fff03b9` 和 `308bdd3e7` 折叠大段 base64、把估算输入封顶 200 万 Token，并修复 Responses 估算的协议分派。它们只服务于此前明确排除的“断线立即关闭、缺 usage 时估算入账、95% 缓存计费”整组策略。本地继续有界读取真实终态用量；预授权只是临时冻结，不写作实际用量，不引入这一组估算入账文件。

## 验证

- Images/GIF、精确整数解析及更新来源检查点定向回归：159 个顶层测试、327 个测试及子测试通过，无失败或跳过。覆盖数字字符串、非法类型/值、OAuth 请求构造、GIF 合成和资源预算。
- Images 转发定向回归：38 个顶层测试、45 个测试及子测试通过，无失败或跳过。新增 API Key 生成/编辑入口测试使用模拟上游，确认两个控制字段实际转发为 JSON 数字，其他业务字段保留。
- `CGO_ENABLED=0 go build -p 2 -tags embed ./cmd/server` 通过，产物写入诊断目录，未替换运行中的服务。
- 修改的 Go 文件已通过 `gofmt` 格式检查，来源锁文件可解析，`git diff --check` 通过。

原始测试日志、构建日志和检查产物位于工作区 `development/diagnostics/upstream-v2314-gif-20261106/`。前端文件在 GIF 同步阶段没有修改，沿用此前使用记录自适应布局的有效验证结果。未发送真实模型或付费生图请求。

## 个人 v2.1.53 发布授权

用户随后明确要求提交 GitHub 并更新部署。本次将 GIF 兼容修复与此前已验收的管理端、用户端使用记录自适应布局一并发布：邮箱完整单行，Token、费用和延迟前移，列间距按容器宽度及可见列数自适应，保留列设置和次要信息横向滚动。

前端三个受影响组件共 100 项定向测试、ESLint、类型检查、生产构建及资源检查通过；实际两端桌面/手机、列设置、长 Token、明暗主题、排序与明细交互证据保存在 `development/diagnostics/usage-table-density-20261106/`。具体变化见 `docs/release-notes/v2.1.53.md`。

个人 VERSION 推进到 2.1.53，采用现有 PR/CI、自有 Release/GHCR、制品校验、数据库备份及真实隔离恢复验证、受控切换与回滚流程。依赖、迁移和部署配置不变；最终发布身份与线上验收记录在 `development/diagnostics/sub2api-release-v2153-20261106/`。
