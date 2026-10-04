# VPS 发布更新

此更新器只负责把 `loLollipop/Sub2api` 已发布并经过校验的 Linux amd64 成品切换到现有 `sub2api.service`。它不会编译源码、不会执行数据库迁移，也不会下载 `kiss-kedaya/sub2api` 的制品。

## 安装

以 root 执行：

```bash
install -m 0755 deploy/vps-release-update.sh /usr/local/sbin/vps-release-update.sh
install -m 0600 deploy/vps-release-updater.conf.example /etc/sub2api-release-updater.conf
install -m 0644 deploy/sub2api-release-update.service /etc/systemd/system/sub2api-release-update.service
systemctl daemon-reload
```

安装前检查配置中的 `ENVIRONMENT_FILE`、`WORKING_DIRECTORY`、`READY_URL` 与现有生产服务一致。生产环境必须继续设置：

```text
DATABASE_MIGRATION_MODE=validate
UPDATE_STRATEGY=disabled
```

## 更新流程

1. 在后台版本徽标查看 `kiss-kedaya/sub2api` 的更新说明。
2. 审查源码差异、迁移、依赖与本地补丁。
3. 将接受的变更同步到 `loLollipop/Sub2api`，更新 `maintenance/upstream.lock.json`，运行 CI。
4. 创建 `vX.Y.Z-personal.N` 个人版本标签和正式（非 prerelease）GitHub Release，确认其中存在 `checksums.txt` 与 Linux amd64 归档。GoReleaser 配置会显式设置 `prerelease: false`，使 GitHub `/releases/latest`、VPS 更新器与个人回滚列表采用同一发布口径。
5. 手动部署个人仓库最新正式 Release：

   ```bash
   systemctl start sub2api-release-update.service
   journalctl -u sub2api-release-update.service -n 200 --no-pager
   ```

   也可以锁定明确标签：

   ```bash
   /usr/local/sbin/vps-release-update.sh v2.0.50-personal.1
   ```

更新器在停止旧服务前完成下载、SHA-256、归档、平台、版本和只读迁移检查。切换后必须连续通过 `/readyz`、MainPID 实际二进制路径和单进程检查；失败会恢复原 systemd drop-in 并重新启动旧程序。数据库不会被自动回放或降级。

没有安装 timer：发现上游新版不会自动同步，也不会自动部署。
