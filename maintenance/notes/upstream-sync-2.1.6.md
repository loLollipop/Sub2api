# Sub2API 2.1.6 同步

- 本地发布基线 `97e7b4941777321d9b37faaa394ea38ee14f0d73`（2.1.3）。上游审查区间 `9bbe1e2bf249cdec2e77da9480d746ccb48bfff8` → `f743ca8e23ff036f628010f4272ab13cbd7f61b1`，9 个提交、43 个 delta 文件。冻结 v2.1.6 peeled commit，不追逐后续 main。
- 按既有 review-diff-then-squash 策略导入安全增量：Anthropic API-key 缓存 TTL 改写与 beta 补齐，鉴权查询的 TTL 字段投影，完整终止帧后的流式早退，以及间隔超时可配置上限 3600 秒（默认值不变）。
- beta helper 按 JSON 解码值检测 TTL，避免合法 Unicode 转义使原始字节快速匹配漏判。普通路径仍尊重 beta drop 策略；账号级显式 header override 仍最终生效，可主动覆盖注入 beta。
- 请求 TTL 注入与 usage 计费类别覆写是不同设置。保留上游实际返回的 5m/1h 用量、已有账号计费覆写和全局优惠规则；不凭请求猜测兼容上游的缺失 TTL 用量，不在同步时改变收费规则。`cache_ttl_overridden` 继续表示计费类别覆写，不等同于请求 TTL 注入标记。
- 补齐上游遗漏的 Anthropic/OpenAI 透传终止帧收尾，并加入保持 body 打开的回归测试。错误帧、金融错误保护、权威 response.failed、客户端断连及 usage 收集规则保持。
- 真实上游域名夹具和注释脱敏、私密本地目录忽略规则导入。Cline 文件和契约 fixture 变更不适用；本地 comma-ok lint 修复已存在；保留 legacy HTTP2 GOAWAY 类型回归及 nolint 说明。
- 原 9 项 deferred 保留；不导入 Cline/Command Code、迁移 255/256 或已暂缓的 DeepSeek parser、honeypot 和 Opus 公共模型校验。0.15 倍率、余额、充值界面、owner 隔离、逐跳下载安全、sticky/continuation、inflight 保护保留。
- 无新增数据库迁移；生产保持 validate、在线更新禁用及余额预扣关闭。发布仅使用本仓库数字标签和校验固定的构建制品；本说明不替代 CI 和部署验收。
