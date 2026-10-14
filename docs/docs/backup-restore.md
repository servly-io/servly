---
id: backup-restore
title: Backup and restore
---

servly stores everything in the database plus the uploaded-assets directory. A backup is both, taken together.

Status: the procedures below work with standard tools today. A built-in `servly backup` command, with retention and a pre-upgrade backup, is Phase 5 work.

## What to back up

Uploaded images live in the database too ([ADR 0009](adr/0009-images-in-database.md)), so the database is all the data there is.

| | Local (SQLite) | Cloud (PostgreSQL) |
|---|---|---|
| Database | `<data_dir>/servly.db` (with its WAL) | the servly database |
| Config | `/etc/servly-io/servly.yaml`, `/etc/servly-io/servly.env` (contains secrets) | deployment config |

## SQLite: safe backups

Do not copy `servly.db` while servly is running. The newest writes may still be in `servly.db-wal`, and the copy can be torn. Use SQLite's own backup instead, which is safe while servly serves:

```bash
sqlite3 /var/lib/servly-io/servly.db ".backup '/var/backups/servly-io/servly-$(date +%F-%H%M).db'"
```

`VACUUM INTO '/path/backup.db'` works too. Alternatively, stop servly, copy `servly.db` together with any `-wal` file, and start servly again.

Check a backup:

```bash
sqlite3 /var/backups/servly-io/servly-2026-10-10-0300.db "PRAGMA integrity_check;"   # prints: ok
```

### Retention

Keep at least 7 daily and 4 weekly backups. Keep at least one copy off the machine, such as a USB disk taken home or remote storage when internet is available. Never let a backup job overwrite the only good backup: write to a new file, verify it, then prune old ones.

### Restore (local)

1. Stop servly: `systemctl stop servly`, or `podman stop servly`.
2. Move the current files aside. Do not delete them:

   ```bash
   mv /var/lib/servly-io/servly.db{,.broken}; rm -f /var/lib/servly-io/servly.db-{wal,shm}
   ```
3. Copy the backup in as `servly.db`, and unpack the assets archive.
4. Check ownership: `chown -R servly:servly /var/lib/servly`.
5. Start servly. It applies any newer migrations. Check `/readyz` and the last orders.

Orders placed after the backup are lost. Tell staff to check open orders against paper or terminal receipts.

## PostgreSQL

* Use the provider's automated backups with point-in-time recovery, or `pg_dump --format=custom` on a schedule.
* Restore into a new database, point a servly instance at it, and verify before switching traffic.
* Before a migration, take a backup or note the PITR timestamp.

## Restore test

A backup is only proven by a restore. Phase 5 adds an automated test: create data, back up, restore on a clean instance, and verify orders and payments. Until then, rehearse a restore by hand once per installation.
