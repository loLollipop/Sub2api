# 定制前端与个人 GHCR 镜像

本次界面更新参考 [qhongchen/sub2api 的 dev 分支](https://github.com/qhongchen/sub2api/tree/dev) 中的 `frontend-local`。移植的是导航和界面布局，不是用另一份旧前端/后端覆盖当前项目。

新界面直接维护在 `frontend/`：管理员端和用户端共用顶部导航、卡片、表格和移动端菜单。原有登录、接口、支付、工单、插件、模型广场、账号管理以及数据库保持当前实现，不需要为界面更新执行数据库迁移。源码开发/验证仍使用现有 `pnpm --dir frontend build`。

## 本地构建（不上传）

需要 Docker、buildx、Git。脚本继续使用本仓库根 `Dockerfile`，保留原有 Go/pnpm 版本、后端安全修改、前端嵌入方式和许可证，不依赖第三方镜像站。

```bash
IMAGE_NAME=ghcr.io/lolollipop/sub2api:frontend-local ./deploy/build_local_image.sh
```

默认 `PUSH=false`、`PLATFORM=linux/amd64`；镜像通过 `--load` 保留在本地。ARM64 可设 `PLATFORM=linux/arm64`。

## 上传自己的 GHCR

先使用自己的 GitHub 账号登录 `ghcr.io`；令牌需要对应镜像的 `write:packages` 权限。使用 `docker login ghcr.io --username loLollipop --password-stdin` 从标准输入提供令牌，切勿将令牌写入脚本、Dockerfile、Git 或构建参数。

确认测试通过并提交代码后，用不可变版本标签上传：

```bash
PUSH=true IMAGE_NAME=ghcr.io/lolollipop/sub2api:frontend-<版本> ./deploy/build_local_image.sh
```

`PUSH=true` 只允许 `ghcr.io/lolollipop/` 命名空间；GHCR 镜像名必须小写。多架构上传可增加 `PLATFORM=linux/amd64,linux/arm64`。只在成功构建后上传，不会自动登录、改变包可见性、清理已有镜像或全局构建缓存、更新 VPS。

支持显式覆盖 `NODE_IMAGE`、`GOLANG_IMAGE`、`ALPINE_IMAGE`、`POSTGRES_IMAGE`、`GOPROXY`、`GOSUMDB`、`NPM_CONFIG_REGISTRY`、`VERSION`；未设置时使用当前 Dockerfile 的默认值。构建标注源码提交；未提交的修改会带 `-dirty` 标记，不应作为正式发行版本。

## 现有 VPS

当前生产服务仍采用既有 systemd 部署。发布镜像不意味着把生产环境迁入 Docker，也不会启动第二套应用处理相同账号。更新生产界面应沿用当前部署方式：先备份运行配置和数据库、构建包含新前端的现有后端、验证，再原位替换并保留回滚版本；不要套用上游初始化或迁移脚本。
