# SQLite 停服压缩指南

[English](../en/SQLITE-MAINTENANCE.md) | 简体中文

本指南仅适用于使用 SQLite 的 NowhereDash。历史清理会删除过期记录，但 SQLite 通常会把释放的页面留在数据库文件中供后续写入复用，不会立即把空间归还给操作系统。需要缩小物理文件时，应在停服维护窗口执行 `VACUUM`。

## 何时执行

建议同时满足以下条件时再安排压缩：

- 系统设置中的“可复用空间”超过 `256 MB`。
- 已执行数据库历史清理，并确认需要保留的数据仍然完整。
- 可以安排停服维护窗口。
- 数据库所在磁盘有足够临时空间。

不要在每次历史清理后执行 `VACUUM`。它会重写整个数据库，对大文件耗时较长，并在执行期间阻塞写入。

## 开始前

1. 安装 SQLite 命令行工具 `sqlite3`。
2. 确认实际数据库路径。默认路径为 `db/database.db`；如果配置了 `DB_PATH`，请使用对应路径。
3. 确认数据库所在文件系统至少还有约两倍数据库大小的可用空间。备份和 `VACUUM` 临时文件都会额外占用磁盘。
4. 使用对数据库文件及其所在目录具有读写权限的账号执行维护命令。
5. 安排维护窗口，并提前通知使用者。

Docker Compose 部署应使用宿主机挂载目录中的文件路径，例如 `./db/database.db`，不要把只在容器内存在的 `/app/...` 路径直接传给宿主机上的 `sqlite3`。

以下命令假设当前目录是 NowhereDash 工作目录：

```bash
export DB_FILE="db/database.db"
export BACKUP_FILE="${DB_FILE%.db}-before-vacuum-$(date +%Y%m%d-%H%M%S).db"
```

请在同一个 shell 会话中完成后续步骤，避免变量丢失。

## 1. 停止 NowhereDash

systemd 部署：

```bash
sudo systemctl stop nowheredash
sudo systemctl status nowheredash --no-pager
```

Docker Compose 部署：

```bash
docker compose stop nowheredash
docker compose ps
```

直接运行二进制时，请通过对应的进程管理工具停止 NowhereDash。继续之前，应确认没有进程仍然打开数据库文件：

```bash
lsof "$DB_FILE"
```

没有输出表示当前没有进程占用该文件。

## 2. 创建一致性备份

不要在服务运行时只复制 `database.db`。WAL 模式下，尚未合并的数据可能仍在 `database.db-wal` 中，单独复制主文件可能得到不完整备份。

停服后使用 SQLite Backup API 创建备份：

```bash
sqlite3 "$DB_FILE" ".backup '$BACKUP_FILE'"
sqlite3 "$BACKUP_FILE" "PRAGMA quick_check;"
```

第二条命令应输出：

```text
ok
```

确认备份文件存在且大小合理：

```bash
ls -lh "$BACKUP_FILE"
```

## 3. 记录压缩前状态

```bash
sqlite3 -header -column "$DB_FILE" "
SELECT page_count * page_size AS database_bytes,
       freelist_count * page_size AS reusable_bytes
FROM pragma_page_count(), pragma_page_size(), pragma_freelist_count();
"

ls -lh "$DB_FILE" "$DB_FILE-wal" "$DB_FILE-shm" 2>/dev/null
```

其中：

- `database_bytes` 是 SQLite 主数据库的页面总大小。
- `reusable_bytes` 是已释放、可被后续写入复用的页面大小。
- `database.db-wal` 需要通过 checkpoint 截断，它与主数据库文件的压缩是两件事。

## 4. 截断 WAL 并压缩主数据库

```bash
sqlite3 "$DB_FILE" "
PRAGMA busy_timeout=30000;
PRAGMA wal_checkpoint(TRUNCATE);
VACUUM;
PRAGMA wal_checkpoint(TRUNCATE);
"
```

该操作会：

1. 把 WAL 中已提交的数据合并回主数据库并截断 WAL 文件。
2. 使用 `VACUUM` 重写主数据库，只保留正在使用的页面。
3. 再次执行 checkpoint，避免维护操作留下较大的 WAL 文件。

如果出现 `database is locked`，不要删除 `database.db-wal` 后重试。应重新确认 NowhereDash 和其他 SQLite 工具都已停止。

> **禁止直接删除 `database.db-wal`。** WAL 文件可能包含已经提交但尚未合并到主数据库的数据，直接删除可能造成数据丢失。

## 5. 校验结果

```bash
sqlite3 "$DB_FILE" "PRAGMA quick_check;"

sqlite3 -header -column "$DB_FILE" "
SELECT page_count * page_size AS database_bytes,
       freelist_count * page_size AS reusable_bytes
FROM pragma_page_count(), pragma_page_size(), pragma_freelist_count();
"

ls -lh "$DB_FILE" "$DB_FILE-wal" "$DB_FILE-shm" 2>/dev/null
```

`quick_check` 应输出 `ok`。压缩后 `reusable_bytes` 通常接近 `0`，主数据库和 WAL 的物理大小应相应下降。如果压缩前可复用空间很少，主文件大小可能不会明显变化。

## 6. 启动并检查服务

systemd 部署：

```bash
sudo systemctl start nowheredash
sudo systemctl status nowheredash --no-pager
journalctl -u nowheredash -n 100 --no-pager
```

Docker Compose 部署：

```bash
docker compose start nowheredash
docker compose ps
docker compose logs --tail=100 nowheredash
```

最后检查健康接口和管理页面：

```bash
curl -fsS http://127.0.0.1:4000/api/health
```

## 恢复备份

如果完整性检查失败或服务无法正常启动，请再次停服后恢复备份：

```bash
FAILED_FILE="${DB_FILE%.db}-failed-$(date +%Y%m%d-%H%M%S).db"

mv "$DB_FILE" "$FAILED_FILE"
cp "$BACKUP_FILE" "$DB_FILE"
rm -f "$DB_FILE-wal" "$DB_FILE-shm"
sqlite3 "$DB_FILE" "PRAGMA quick_check;"
```

必要时恢复数据库文件原有的所有者和权限，然后重新启动 NowhereDash。只有在服务已经停止且备份文件通过完整性检查时，才能删除 WAL/SHM 辅助文件。
