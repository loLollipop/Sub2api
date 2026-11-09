# 二改正式 v2.3.40 选择性同步（2026-11-09）

## 来源

[正式 v2.3.40](https://github.com/kiss-kedaya/sub2api/releases/tag/v2.3.40) 的标签对象为 `631c8f48f9b70f0b39bf78bfe9ddc75cd168de1d`，解引用提交为 `59136801e026eca4397ec9fe64bea791ddc6a33f`。从已审查 v2.3.19 `a16b937d6afea658434aa2c16049114c91984867` 到该正式标签共 118 个提交、128 个变化路径。本次观察到的 main `17036fca1f3d1728ddf0cad2dec83f8b829cc867` 仅多 VERSION bookkeeping，不作为运行时同步来源。

只按正式版审查并选择性适配，不合并上游历史或 VERSION。原版检查点仍为 `6b4b37db43443b5e88e424fe3c69602cc3810dfb`，原版正式版本仍为 v0.2.20；同步时个人 VERSION 为 2.1.58，随后为已授权发布推进至 2.1.59。

## 采用与本地适配

- `5e163e2f5`：编辑账号保存时，显式白名单 identity 清除同源旧映射，保留无关映射、空名单语义及全局 combined precedence。发布前审查另发现通配符白名单条目会误删同源映射，而 builder 不生成通配符 identity；清理只匹配可保存的精确模型，避免意外变成不限模型。API Key、OAuth、Vertex、Bedrock 的白名单界面显示隐藏映射数量，补齐中英文文案。
- `ee41365ae`、`703fa51f0`、`053c6aa78`、`4a472cfd0`：DeepSeek 缺失或空思考内容仅补单空格。原生 Responses 先恢复原有按 key、会话、模型及 opaque 标识隔离的真实思考；Anthropic 仅在最终映射 DeepSeek 模型的普通 assistant tool_use 回合补 thinking。保留真实、redacted 和未知供应商块；纯文本、server_tool_use 和显式 disabled 不补，不把 summary 当作真实 CoT。
- `e640ef269`、`23d5ee213`：在共享 Chat 出站入口配对并排序工具结果，删除孤儿、重复结果和未回答调用。重复 call ID 关联最近在前的声明，不能借用后续回合的结果。使用 raw JSON 保留未知字段与大整数，合法序列不改写；仅 DeepSeek 平台或精确 `api.deepseek.com` 目标生效。Chat 显式 disabled 仍修复配对，但不补 reasoning_content。
- `9e711d899`：Grok 支持思考等级的模型仅 trim 后原样透传，不再把 minimal/max/ultra/xhigh 改写为 low/high；模型能力门保留。
- `bed6a8919`：适配撤销所有会话同时撤销 access JWT、注册及额外通知邮箱验证码原子消费。注册防邮箱枚举此前已覆盖，不重复引入。新 JWT 和 refresh 共用签发时读取的 cutoff epoch；撤销 Lua 即使遇到相同时间或时钟回拨也严格递增。旧 JWT 按 iat、旧 refresh 按 CreatedAt 兼容核对。先持久化撤权再删 refresh 集合，刷新前后复核原凭据，删除失败不能复活旧会话；普通用户、管理员 REST/WS 和 OAuth 绑定 cookie 共用验证。
- `3128b2763`：分组模型白名单在选取 model 前按 handler 相同规则处理 BOM 和控制字节，并回填请求体；使用配置的请求预算。精确识别 multipart，同时处理 JSON 伪装 multipart Content-Type。独立复核发现并修正两处边界：共享 normalizer 一次去除完整连续 BOM 前缀，避免两次处理逐层暴露 model；带 JSON 样式 MIME preamble 的真实 multipart 先识别 part，保留上传二进制和原始长度预算，仅对完整 JSON 采用修复结果。
- `63bc94ca7`、`9aa319c57`：API Key 会话释放与注册条件一致；启动默认不清理其他进程的 Redis 并发槽，仅受控单实例重启显式 `SUB2API_STARTUP_SLOT_CLEANUP=enabled` 执行。自定义首页 HTML 经 DOMPurify 清理，外部 URL 经协议校验、sandbox 隔离及 no-referrer；无效 URL 不产生空 src iframe。保留本地首页布局、主题和渐变品牌。
- `da9a1501b`、`8315067ff`：安装下载使用 HTTP 失败即失败模式；校验清单不可达、缺少精确文件名条目或 SHA-256 不匹配均中止，校验成功后才替换二进制。配置目录 0700、已有 sub2api.env 0600，保留本地 GitHub token 传递机制。

## 延期与排除

- `b36b911f5` 的 account/model 冷却没有成功观测复位接线，失败惩罚可能持续保留；超时、断流等观察覆盖不完整。`d02e88307` 的健康度排序依赖这套链路，暂不导入。
- `0b87219e6` 按全组、全模型的一小时质量统计静默重排显式 RouteGroupIDs，可能改变用户指定主组及实际计费分组，保留本地有序路由。
- `7a86e799f` 的余额耗尽邮件忽略通知开关与额外通知邮箱，去重为无界进程内 map；本地现有投递台账也是检查、发送、写标记，不能原子排除并发重复投递。因此暂不挂接高频鉴权拒绝路径。
- `0830c3d5d`、`82dbf1f42` 的 catalog 测试隔离和 seed 可用性修复继续随整组延期。它们没有解决 provider 忽略、别名冲突、Kimi 参数推断、实际计费/ETag 集成与每六小时刷新 main 的审查边界。先前记录的白名单修复现已进入正式范围并独立采用。
- channel 凭据 reveal 模块本地不存在，不引入；本地账号导出已有 step-up。邮箱枚举隐藏已覆盖 exact/alias，复用已有行为。
- 退役原生支付、crypto、sweeper 及 migration312 全部排除。Hosted、Ops/capture/timers、Nuxt SSR、MCP、估算断线用量和 95% 缓存收费等旧排除项不恢复。Docker 基础镜像、CI 和线上部署配置变更不在本次适配范围。

## 验证

- 白名单 helper 与账号弹窗发布前最终 107 项测试通过，双语完整性 3 项通过。覆盖不同账号类型、同源旧映射、无关映射、空白名单和保存重开。
- Home 定向 38 项测试通过，覆盖 HTML 清理、sandbox/no-referrer、无效 URL 无 iframe 及现有首页功能。
- Edge 实际操作浅色 1440 和深色 390 宽度：白名单加入冲突模型、保存、重开、再保存，identity 与无关映射均正确，隐藏数量从 2 降至 1，无页面溢出或 pageerror。另实测首页恶意 HTML 不执行、外部 URL sandbox 属性和无效 URL 不建 iframe。使用合成预览数据，无真实凭据。
- Git Bash 下安装脚本语法、GitHub token 回归和 7 个下载校验场景通过；失败场景保留旧二进制。Windows 未实际验收 Linux 文件权限效果。
- repository、service、middleware、routes、handler 共 538 项定向测试（含子用例）通过，含 DeepSeek/Grok、会话释放、启动清理、模型白名单、持久撤权和验证码并发。覆盖 168h JWT 在 49h 后仍撤销、refresh 清理失败/轮转竞态、同纳秒撤销后立即登录和缓存不可用拒绝认证；更新来源检查点与跟踪另 40 项通过。
- 安全复核后的 multipart/重复 BOM 最终回归另通过 207 项（含子用例），涵盖 httputil、requestmodel、middleware、routes。旧逻辑的 MIME preamble 文件字节改写已由失败用例复现；修复后含引号/控制字节文件及紧贴原长度预算均保持字节一致。JSON 与伪 multipart 在 Messages/Responses/Chat 入口的重复 BOM 均正确准入或拒绝。
- 前端 vue-tsc、九个改动文件 ESLint、Vite 生产构建和 Go embed server 构建通过。gofmt 与 diff 空白检查通过；依赖及三份原有未跟踪测试的 SHA-256 与开始时一致，原版检查点、个人 VERSION、迁移和部署配置未变。

race 未运行：当前 Windows 环境无 C 编译器；Redis 原子性由真实 Lua/miniredis 定向回归验证，不等同于线上多实例演练。未做外部供应商实网或生产发布验收。独立安全合同复核已通过：上述 P1 重复 BOM 绕过和 P2 multipart 字节改写修复后，无未解决阻断项。

证据位于本地 `.artifacts/upstream-sync-20261109/`，包括来源元数据、提交/路径列表、测试日志和浏览器验收截图。

## 边界与交付

保留原生厂商、发现与凭据目的地、上传者权限隔离、真实终态用量、有界排空和已交付/计费后不重放；保留 0.15 倍率、冻结价格、显式零价、有序路由和现有图表/主题，思考强度仍是紧随模型的独立列。

无依赖、数据库迁移或部署配置变化。开始时已有的三份未跟踪 Go 测试保留原内容且不纳入本轮交付，不启用 `billing_audit_deferred`。用户随后授权审查通过后提交 GitHub 并更新部署，个人版本为 v2.1.59；发布按 PR/CI、自有 Release/GHCR 和既有备份恢复校验流程执行，最终身份与线上验收另存证据。CI 另补本轮撤权、验证码、准入和协议路径的 Linux race 回归及安装下载校验。
