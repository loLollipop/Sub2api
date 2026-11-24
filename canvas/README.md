# 图片画布

界面来自 `basketikun/infinite-canvas`，固定源码提交为 `9cd3deacbcc598361807afc16c059a80dd18b6f9`。保留原 React 画布的图片、文本、生成配置、分组节点、连线、参考图、图片编辑及导入导出。原 MIT 许可证保存在 [LICENSE](LICENSE)。`VERSION` 和 `CHANGELOG.md` 是来源信息，不是 Sub2API 的发布版本。

接入沿用二改的 `/infinite-canvas` 页面和 `/canvas/` 同源 iframe。仅提供图片生成和编辑；视频、音频、Agent、文字模型生成及远程插件加载不开放。源项目中未被入口引用的页面、工作台、生成接口和部署脚本不随本集成发货。保留的类型中可能仍有旧媒体字段，用于原画布数据结构；它们不会开放对应生成请求。

## 使用

登录控制台，打开「无限画布」，选择有权限的 OpenAI/Grok 生图分组。第一次点击「创建并进入」，系统创建只绑定该组的专用 Key。之后复用有效 Key；停用、过期、额度耗尽或撤销分组访问均不会自动换 Key。Key 明文缺失时到 API 密钥页处理。

创建画布，添加「图片」节点，输入提示词，选择分组公开的生图模型、尺寸、质量与张数。已有图片可作为参考编辑，也可以上传 PNG/JPEG/WebP/GIF。手工文本节点可组织提示词。模型实际支持的参数仍由现有网关和上游决定，错误会显示在节点中；不会自动重发付费请求。

按现有生图分组、模型和质量计费；多张图片分别提交请求。离开生成中的画布可能丢失结果，已提交的请求仍可能产生费用。画布项目、参考图和生成图片保存在当前浏览器的 IndexedDB，按登录用户隔离；不同设备不会自动同步，清除站点数据会删除本地画布。需要迁移时先导出画布文件。

## 构建

在仓库根目录执行：

```sh
pnpm --dir frontend install --frozen-lockfile
npm --prefix canvas/web ci --ignore-scripts --no-audit --no-fund
pnpm --dir frontend run build
make build-backend
```

控制台构建会先生成 Vue 静态文件，再检查并构建画布，输出到 `backend/internal/web/dist/canvas`。次序不能颠倒，Vue Vite 会清空整个 dist。Go `-tags embed` 将两个前端一起打进二进制；Docker 和 release CI 已安装两套依赖并沿用此构建次序。Go embed 构建前不能只构建控制台 Vite。

运行时优先检查二改兼容的本地静态目录（`INFINITE_CANVAS_STATIC_DIR`，默认 `/opt/infinite-canvas/html/canvas`），然后使用内嵌画布；没有内嵌资源时才使用 `INFINITE_CANVAS_UPSTREAM` 代理。外置运行时需要自行兼容本项目的消息握手，新安装无需设置这些变量。

画布在收到已验证的父页面会话后才初始化数据库和模型配置。Key 只通过严格同源、校验发送窗口的 `postMessage` 传递并保留在内存，不放进 URL、导出项目或持久化配置。图片请求只允许本站 `/v1/images/generations` 和 `/v1/images/edits`，模型读取使用 `/v1/models`。静态代理不向外部上游转发 Cookie、Authorization 和 Referer。

后续同步原项目时保留上述入口、存储命名空间、请求限制和构建适配，重新验收实际界面及参考图编辑。源码版本与 Sub2API 个人版本分别维护。
