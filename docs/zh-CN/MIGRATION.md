# 迁移与升级指南

[English](../en/MIGRATION.md) | 简体中文

NowhereDash 是一个通过 OpenCtrl 管理 Nowhere 的 Portal-only 面板。包含 NodePass 隧道、client/server 实例或服务组装记录的旧面板数据，与 NowhereDash 当前数据模型不兼容。

## 升级前准备

1. 在 NowhereDash 的数据导入/导出页面导出 Portal-only 备份（如当前版本提供该入口）。
2. 停止服务，并复制持久化数据目录。
3. 使用 SQLite 时，请备份完整 `db/` 目录，不要只复制 `database.db`，因为 WAL 模式可能把最近提交的数据保存在辅助文件中。
4. 使用 PostgreSQL 时，请通过常规 PostgreSQL 工具创建数据库 dump。
5. 如果 `.env` 由容器或安装脚本之外的方式管理，请一并保存。
6. 记录 OpenCtrl 端点 API URL 和 API Key。

## 升级 Docker 部署

```bash
docker compose pull
docker compose up -d
docker compose logs --tail=100 nowheredash
```

随后打开界面确认：

- 不会意外进入 Setup 模式。
- OpenCtrl 端点在线。
- Portal 列表、实时状态和日志正常更新。
- 订阅地址 `/sub/portal?token=...` 仍能正确输出内容。

## 升级二进制/systemd 部署

如果通过 `scripts/install.sh` 安装：

```bash
sudo nowheredash-ctl update
sudo nowheredash-ctl status
sudo nowheredash-ctl logs
```

也可以直接调用安装器：

```bash
sudo /tmp/nowheredash-install.sh update dash --non-interactive
```

手动二进制部署时，请先停止服务，替换 `nowheredash` 二进制，再重新启动。保留原工作目录、`.env`、`db/` 和 `logs/`。

## 从旧面板迁移

包含旧 tunnel/client/server/service 字段的导出文件不能直接导入 NowhereDash。建议按以下流程迁移：

1. 在每个节点安装或升级 Nowhere 与 OpenCtrl。
2. 将 OpenCtrl `/api/v2` 端点添加到 NowhereDash。
3. 将每个需要保留的业务重新创建为 Nowhere `portal://` 实例。
4. 校验生成的 `nowhere://` URL 与二维码。
5. 基于新的 Portal 列表重新创建订阅。
6. 确认流量和订阅拉取正常后，再停用旧面板。

## 数据库迁移

NowhereDash 支持 SQLite 和 PostgreSQL，数据库类型在 Setup 阶段选择。当前没有自动在线转换数据库的命令。

推荐流程：

1. 从旧实例导出 Portal-only 备份。
2. 部署一个新的 NowhereDash 实例。
3. 在 Setup 向导中选择目标数据库。
4. 导入 Portal-only 备份。
5. 重新创建或核对 OpenCtrl 端点和订阅。

生产环境迁移时，建议旧实例先停服保留，直到新实例通过登录、端点、Portal、订阅等检查。

## 回滚

只有在拥有升级前匹配备份时，回滚才是安全的。

- Docker：停止容器，恢复 `db/` 和 `.env`，固定到旧镜像 tag 后重新启动。
- systemd/二进制：停止服务，恢复旧二进制和数据备份后重新启动。
- PostgreSQL：将升级前 dump 恢复到干净数据库。

除非发布说明明确支持，不要让旧二进制连接较新的数据库。

## 排错

- 升级后进入 Setup：检查 `.env` 是否存在，且包含 `DB_DRIVER`。
- SQLite 数据缺失：恢复完整 `db/` 目录，包括备份时存在的 `database.db-wal` 和 `database.db-shm`。
- 端点离线：检查 OpenCtrl API URL、API Key、TLS 模式和防火墙规则。
- 订阅返回空内容：确认关联的 Portal 正在运行，且未超过到期时间或流量限制。

## 技术支持

- NowhereDash Issues: https://github.com/NodePassProject/NowhereDash/issues
- Nowhere Issues: https://github.com/NodePassProject/Nowhere/issues
- OpenCtrl Issues: https://github.com/NodePassProject/OpenCtrl/issues
