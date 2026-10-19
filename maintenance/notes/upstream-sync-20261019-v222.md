# 二改 v2.2.2 同步 — 2026-10-19

本地基线 `c26d1187f2af791bde1056a0929e942a5816b1a9`（个人 v2.1.23）。检查二改 `f645453e36c1514563055e64704e493e412c3263..648c17cc14846c901726f866fa76059d61aacc75` 两个提交。正式 v2.2.2 Release 指向 `da633759c0d65ecd3ca928c214ac79a77fecb9c9`，main 另含 VERSION 提交。个人目标版本 v2.1.24，原版检查点保持不变，不导入来源 Git 历史。

## 范围

- `da633759c`：OpenAI Responses 出站将 `tools[].type` 和对象型 `tool_choice.type` 中的 `web_search_preview`、合法 `_YYYY_MM_DD` 日期变体改为 `web_search`。覆盖普通 HTTP、HTTP passthrough 以及 WebSocket 首轮和后续请求。
- 通过局部 JSON 字段修改保留其他选项、输入、数字精度及无改写请求的原始字节，支持 JSON 转义的旧工具名；匹配时仍验证 JSON，不修复非法请求。
- 本地只对 OpenAI 平台启用，不扩展到其他原生厂商。未知前缀、函数名称、字符串型 tool_choice 及历史 web_search_call 不改写。保留现有搜索计数、token 结算及分组价格。
- `648c17cc1` 的来源 VERSION 使用个人版本。Release 中关于来源账号配置的说明属于对方部署数据，无相应源码变更，不复制账号或修改生产配置。
- 无数据库迁移、依赖或环境变更，保留此前暂缓项。

## 验证和发布门

新增 helper、HTTP 普通/透传实际出站、WebSocket 兼容与实际透传测试；覆盖日期、混合工具、tool_choice、数字/字节保真、JSON 转义、非法 JSON 和原生厂商隔离。新增 Linux race 验收入口，并回归既有搜索价格测试。

发布前验收包括完整后端单元测试、Go vet、前端生产构建及嵌入服务构建、敏感资料检查、独立审查和 GitHub 集成/race/安全检查。部署使用已验证的个人 Linux amd64 制品，备份数据库与配置、持锁切换并验收健康和实际版本。源码检查点不代表发布或部署完成；不调用真实付费上游。
