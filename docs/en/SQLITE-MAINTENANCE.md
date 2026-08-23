# Offline SQLite Compaction Guide

English | [简体中文](../zh-CN/SQLITE-MAINTENANCE.md)

This guide applies only to NowhereDash installations using SQLite. History cleanup deletes expired records, but SQLite normally keeps the released pages inside the database file for reuse. To return that space to the operating system, run `VACUUM` during an offline maintenance window.

## When to run it

Schedule compaction when all of the following apply:

- Reusable space in System Settings exceeds `256 MB`.
- Database history cleanup has completed and the retained data has been checked.
- An offline maintenance window is available.
- The filesystem containing the database has sufficient temporary space.

Do not run `VACUUM` after every history cleanup. It rewrites the entire database, can take a long time for large files, and blocks writes while it runs.

## Before you start

1. Install the `sqlite3` command-line tool.
2. Confirm the actual database path. The default is `db/database.db`; use the configured path when `DB_PATH` is set.
3. Keep approximately twice the database size available on the same filesystem. The backup and temporary `VACUUM` file both require additional disk space.
4. Run the maintenance commands as an account with read and write access to the database file and its directory.
5. Schedule a maintenance window and notify users.

For Docker Compose, use the path in the host-mounted directory, such as `./db/database.db`. Do not pass a container-only `/app/...` path to a `sqlite3` command running on the host.

The commands below assume the current directory is the NowhereDash working directory:

```bash
export DB_FILE="db/database.db"
export BACKUP_FILE="${DB_FILE%.db}-before-vacuum-$(date +%Y%m%d-%H%M%S).db"
```

Complete the remaining steps in the same shell session so these variables remain available.

## 1. Stop NowhereDash

For a systemd deployment:

```bash
sudo systemctl stop nowheredash
sudo systemctl status nowheredash --no-pager
```

For Docker Compose:

```bash
docker compose stop nowheredash
docker compose ps
```

For a directly launched binary, stop NowhereDash with its process manager. Before continuing, verify that no process still has the database open:

```bash
lsof "$DB_FILE"
```

No output means the file is not open by another process.

## 2. Create a consistent backup

Do not copy only `database.db` while the service is running. In WAL mode, committed data may still be present in `database.db-wal`, so copying the main file alone can produce an incomplete backup.

After stopping the service, create a backup with the SQLite Backup API:

```bash
sqlite3 "$DB_FILE" ".backup '$BACKUP_FILE'"
sqlite3 "$BACKUP_FILE" "PRAGMA quick_check;"
```

The second command must print:

```text
ok
```

Confirm that the backup exists and has a plausible size:

```bash
ls -lh "$BACKUP_FILE"
```

## 3. Record the pre-compaction state

```bash
sqlite3 -header -column "$DB_FILE" "
SELECT page_count * page_size AS database_bytes,
       freelist_count * page_size AS reusable_bytes
FROM pragma_page_count(), pragma_page_size(), pragma_freelist_count();
"

ls -lh "$DB_FILE" "$DB_FILE-wal" "$DB_FILE-shm" 2>/dev/null
```

The values mean:

- `database_bytes` is the total page size of the main SQLite database.
- `reusable_bytes` is the space occupied by released pages available for future writes.
- `database.db-wal` is truncated by a checkpoint; shrinking it is separate from compacting the main database.

## 4. Truncate WAL and compact the main database

```bash
sqlite3 "$DB_FILE" "
PRAGMA busy_timeout=30000;
PRAGMA wal_checkpoint(TRUNCATE);
VACUUM;
PRAGMA wal_checkpoint(TRUNCATE);
"
```

This operation:

1. Checkpoints committed WAL data into the main database and truncates the WAL file.
2. Rewrites the main database with `VACUUM`, retaining only pages that are in use.
3. Runs another checkpoint so the maintenance operation does not leave a large WAL file behind.

If SQLite reports `database is locked`, do not delete `database.db-wal` and retry. Verify again that NowhereDash and all other SQLite tools are stopped.

> **Never delete `database.db-wal` directly.** It may contain committed data that has not yet been checkpointed into the main database. Removing it can cause data loss.

## 5. Verify the result

```bash
sqlite3 "$DB_FILE" "PRAGMA quick_check;"

sqlite3 -header -column "$DB_FILE" "
SELECT page_count * page_size AS database_bytes,
       freelist_count * page_size AS reusable_bytes
FROM pragma_page_count(), pragma_page_size(), pragma_freelist_count();
"

ls -lh "$DB_FILE" "$DB_FILE-wal" "$DB_FILE-shm" 2>/dev/null
```

`quick_check` must print `ok`. After compaction, `reusable_bytes` should normally be close to `0`, and the physical sizes of the main database and WAL should be lower. If little reusable space existed before compaction, the main file may not change significantly.

## 6. Start and check the service

For systemd:

```bash
sudo systemctl start nowheredash
sudo systemctl status nowheredash --no-pager
journalctl -u nowheredash -n 100 --no-pager
```

For Docker Compose:

```bash
docker compose start nowheredash
docker compose ps
docker compose logs --tail=100 nowheredash
```

Finally, check the health endpoint and the administration UI:

```bash
curl -fsS http://127.0.0.1:4000/api/health
```

## Restore the backup

If the integrity check fails or the service cannot start, stop it again and restore the backup:

```bash
FAILED_FILE="${DB_FILE%.db}-failed-$(date +%Y%m%d-%H%M%S).db"

mv "$DB_FILE" "$FAILED_FILE"
cp "$BACKUP_FILE" "$DB_FILE"
rm -f "$DB_FILE-wal" "$DB_FILE-shm"
sqlite3 "$DB_FILE" "PRAGMA quick_check;"
```

Restore the original file ownership and permissions if necessary, then start NowhereDash again. Remove WAL/SHM sidecar files only while the service is stopped and after verifying that the backup is valid.
