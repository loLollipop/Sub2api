# 版本更新与上游跟踪

本项目把“可部署的项目版本”和“上游待审查的正式发布”分开检查。不同仓库的版本号不会互相比较，因此本项目版本号领先上游时，也不会漏报上游变化。

## 默认：跟踪本项目发布

`UPDATE_CHECK_MODE=release`（默认值）只检查 `loLollipop/Sub2api` 的正式 GitHub Release。其他人部署本项目后，看到的是本项目的更新，不是原版或二改的安装包。

## 维护者：额外跟踪两个上游

维护者实例可以在部署环境中设置：

```dotenv
UPDATE_CHECK_MODE=upstream
```

这个模式仍检查本项目正式版本，同时分别检查两个上游最新的正式 GitHub Release：

- `kiss-kedaya/sub2api` 的正式 Release（二改上游）。
- `Wei-Shaw/sub2api` 的正式 Release（原版上游）。

后台版本提示展示发布标签、发布时间、发布提交、Release 页面与比较链接。只解析正式 Release 的 `tag_name` 对应提交（支持轻量和 annotated tag）；忽略 `main` 后续提交、草稿和 prerelease，不使用 `target_commitish` 或分支头。正式 Release 标签或提交不同于已审查发布检查点时，才显示待跟进提示，不受本项目版本号影响。没有正式发布、检查或标签解析失败时显示状态未知，不提示可更新。三处仓库的检查相互独立。

Docker Compose 的三个模板都已传入此变量。修改 `.env` 后需要在计划部署时重建应用容器，仅编辑文件或重启旧容器不会更新其环境。二进制/systemd 部署需要把变量写入服务的持久环境配置，并在计划部署时重启进程。此文档不表示已经修改现有服务器。

## 检查周期和检查点

管理员界面约每分钟确认缓存是否过期；远端检查结果缓存 20 分钟，也可以手动刷新强制检查。这是后台界面的更新提醒，不是邮件、Webhook 或源码自动合并。

已审查正式发布标签和对应提交记录在 `backend/internal/service/update_source_baselines.go`，并由回归测试核对 `maintenance/upstream.lock.json` 和 `maintenance/original-backports.json`。维护者完成源码比较后才更新检查点；这表示历史已审查，不代表所有功能都已合并。具体移植与暂缓内容以两个 maintenance 文件为准。

2026-10-20 正式发布检查点：原版 `v0.2.17` / `060d0160417782cd962a0c7f460f6cd4565d4916`，二改 `v2.2.7` / `24fa1a78b6f81117f1ce4ece982e355aeb11c94b`。正式发布检查点独立于已审查的较晚 `main` 提交，避免重复提醒旧发布。本项目发行版本不随上游版本号自动变更。

## 安装安全边界

上游跟踪只是审查提醒，不改变安装包来源，也不解除现有禁用在线自替换的保护。发布、迁移验证和部署仍使用本项目的受控发布流程。任意仓库 URL 不受支持；非法检查模式会退回默认发布检查并提示警告。

可选 `UPDATE_GITHUB_TOKEN` 只用于访问 GitHub API；发布资源下载不会携带它。无令牌时仍可检查，但受 GitHub 匿名 API 限额约束。
