# Migration and Upgrade Guide

English | [简体中文](../zh-CN/MIGRATION.md)

NowhereDash is a Portal-only dashboard for Nowhere managed through OpenCtrl. Legacy dashboard data that describes NodePass tunnels, client/server instances, or service assembly records is not compatible with the NowhereDash schema.

## Before Upgrading

1. Open NowhereDash and export Portal-only backups from the data/import-export page if available.
2. Stop the service and copy the persistent data directory.
3. If using SQLite, back up the full `db/` directory, not only `database.db`, because WAL mode may keep recent committed data in sidecar files.
4. If using PostgreSQL, create a database dump with your normal PostgreSQL backup tooling.
5. Save `.env` if it is managed outside the installer or container.
6. Record the OpenCtrl endpoint API URLs and API keys.

## Upgrade Docker Deployments

```bash
docker compose pull
docker compose up -d
docker compose logs --tail=100 nowheredash
```

Then open the UI and verify:

- Setup mode does not appear unexpectedly.
- Endpoints are online.
- Portal list, real-time status, and logs update correctly.
- Subscriptions still render through `/sub/portal?token=...`.

## Upgrade Binary/systemd Deployments

If installed with `scripts/install.sh`:

```bash
sudo nowheredash-ctl update
sudo nowheredash-ctl status
sudo nowheredash-ctl logs
```

Or call the installer directly:

```bash
sudo /tmp/nowheredash-install.sh update dash --non-interactive
```

For a manual binary deployment, stop the service, replace the `nowheredash` binary, then start it again. Keep the existing working directory, `.env`, `db/`, and `logs/`.

## Migrating From Legacy Dashboards

Legacy dashboard exports that contain old tunnel/client/server/service fields cannot be imported directly. Use this process instead:

1. Install or upgrade Nowhere and OpenCtrl on each node.
2. Add the OpenCtrl `/api/v2` endpoint to NowhereDash.
3. Recreate each required workload as a Nowhere `portal://` instance.
4. Verify each generated `nowhere://` URL and QR code.
5. Recreate subscriptions from the new Portal list.
6. Retire the old dashboard only after traffic and subscription pulls are confirmed.

## Migrating Between Databases

NowhereDash supports SQLite and PostgreSQL, selected during Setup. There is no automatic live database conversion command.

Recommended path:

1. Export Portal-only backups from the old installation.
2. Deploy a fresh NowhereDash instance.
3. Complete Setup with the target database driver.
4. Import the Portal-only backup.
5. Recreate or verify OpenCtrl endpoints and subscriptions.

For production moves, keep the old instance stopped but available until the new instance has passed endpoint, Portal, subscription, and login checks.

## Rollback

Rollback is only safe with a matching backup from before the upgrade.

- Docker: stop the container, restore `db/` and `.env`, pin the previous image tag, then start again.
- systemd/binary: stop the service, restore the previous binary and data backup, then start again.
- PostgreSQL: restore the pre-upgrade dump to a clean database.

Do not mix a newer database with an older binary unless the release notes explicitly say it is supported.

## Troubleshooting

- Setup appears after upgrade: check that `.env` is present and contains `DB_DRIVER`.
- SQLite data is missing: restore the complete `db/` directory, including `database.db-wal` and `database.db-shm` if they existed at backup time.
- Endpoints are offline: verify the OpenCtrl API URL, API key, TLS mode, and firewall rules.
- Subscriptions return no entries: confirm the linked Portals are running and not over expiry/traffic limits.

## Support

- NowhereDash Issues: https://github.com/NodePassProject/NowhereDash/issues
- Nowhere Issues: https://github.com/NodePassProject/Nowhere/issues
- OpenCtrl Issues: https://github.com/NodePassProject/OpenCtrl/issues
