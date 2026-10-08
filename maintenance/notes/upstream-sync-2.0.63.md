# Sub2API 2.0.63 同步

- 在个人基线 `231d1040a6243d17f7492efb324d8257f78fce7c` 上导入上游 `1b464c0da7ca27cbc7f5435dd812aa691fcf7195` 至 `0710c3e6a7d39089b4edda5530ca5faf481ba3c1` 的增量；正式 release commit 为 `ad8de5545b176768c78a99c650a82006969f6fec`，其后同步 VERSION。
- Grok 非官方 CDN 元数据地址改为回落带鉴权的 content 端点；官方 HTTPS vidgen.x.ai 签名地址仍直接下载。保留个人独立下载器的逐跳地址校验、IP 固定、跨 origin 请求头隔离、HTTPS 降级阻断和代理策略，以及账号覆写前捕获下载头的回归测试。
- Responses HTTP/WebSocket 请求优先选择无需提前转换为 Chat Completions 的账号；保留 sticky、compact、排除账号及既有调度开关。该优先级是上游行为调整，不启用实验调度器。
- 导入前导 think/thinking 标签的流式与非流式分离及 local_shell_call 至 Anthropic 工具调用转换。补修双思考通道重复输出和 Anthropic 仅思考回复的可见文本降级；流式以首个实际输出的思考通道为准，同一首帧同时含两者时显式 reasoning 优先，后到的另一通道只移除标签，不重复输出。
- 暂缓通用请求转换器的 summary-only 明文 reasoning 历史变更：转换器固定 store:false，而既有 OpenAI 规范化会删除没有 encrypted_content 的 reasoning item。保留之前的 assistant-message 历史和原始空白，避免未验证目的端兼容性的格式切换；此项单独列入 lock，输出思考分离已导入。
- 补修上游 native 分类中的 Anthropic 误判、非 batch 选择中的映射排序覆盖，以及高级调度 fill 被外层 native 分区打乱的 Priority 层次；增加真实选择器与 fill 回归。
- 保留五档独立小铺商品链接、模型广场透传展示修复和滚动时显示的滚动条。无数据库迁移、依赖或部署脚本变化；不修改客户余额、倍率、账号归属及小铺配置。Grok 拒答计费仍暂缓。
- 此记录描述源码同步；运行检查、发布和生产验收证据保存在本次运维目录。
