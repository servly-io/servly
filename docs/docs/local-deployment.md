---
id: local-deployment
title: Local deployment
---

A local deployment is one servly process on a machine in the restaurant: a mini PC, a NAS or a Raspberry Pi 4/5. It runs with SQLite, needs no internet after installation, and needs no account anywhere.

Status: the server, health endpoints and migrations work. Ordering features arrive in Phases 2-4.

## Requirements

* Linux on amd64 or arm64. One CPU core and 512 MB RAM are enough.
* A persistent directory for data, such as `/var/lib/servly`.
* The tablets and phones reach the machine over the LAN. Customers' phones must reach it too, so the QR codes must point at an address they can open.

## Option A: container

```bash
podman run -d --name servly --restart=always \
  -p 8080:8080 \
  -v servly-data:/data \
  ghcr.io/servly-io/servly:latest
```

Docker works the same way. The image runs as a non-root user and keeps everything in `/data`. With a bind mount instead of a named volume, the host directory must be writable by uid 65532.

### With Podman Desktop

[`deploy/podman/servly.yaml`](https://github.com/servly-io/servly/blob/main/deploy/podman/servly.yaml) describes the same setup as a Kubernetes pod. In Podman Desktop, open **Pods**, then **Play Kubernetes YAML**, and paste the file. From a terminal, run `podman kube play deploy/podman/servly.yaml`.

It uses a pinned image version, port 8080 on every network interface, the `servly-data` volume (the same one as above), and a read-only root filesystem. Set `SERVLY_HTTP_PUBLIC_BASE_URL` in the file to the address guests' phones use. `podman kube down deploy/podman/servly.yaml` stops it and keeps the data.

## Option B: binary and systemd

```bash
sudo useradd --system --home-dir /var/lib/servly --shell /usr/sbin/nologin servly
sudo install -m 0755 servly /usr/local/bin/servly
sudo install -m 0644 deploy/systemd/servly.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now servly
journalctl -u servly -f
```

The unit restarts servly on failure and starts it at boot. It allows writes only to `/var/lib/servly`.

## First start

1. The server creates the database and applies migrations (`migrate_on_start` is true by default).
2. `curl localhost:8080/readyz` returns `{"status":"ok"}`.
3. Copy the setup token from the log. It is in the line `first-run setup: open servly in a browser and enter this setup token`:

   ```bash
   podman logs servly 2>&1 | grep setup_token      # container
   journalctl -u servly | grep setup_token         # systemd
   ```
4. Open `http://<host>:8080` in a browser. It goes to the setup, which asks for the token, then creates the owner account and the restaurant. Every restart prints a new token until setup is done.

   Everything about the restaurant happens in the browser from here on: menu, tables, staff and settings. The `servly` command is only for running the installation: configuration, migrations, upgrades and recovery ([ADR 0006](adr/0006-cli-scope.md)).

## Configuration

Settings resolve in this order (highest wins): command-line flag, environment variable, config file, default.

| Key (YAML) | Environment | Flag | Default |
|---|---|---|---|
| `listen_addr` | `SERVLY_LISTEN_ADDR` | `--listen-addr` | `:8080` |
| `data_dir` | `SERVLY_DATA_DIR` | `--data-dir` | `./data` |
| `database.driver` | `SERVLY_DATABASE_DRIVER` | `--database-driver` | `sqlite` |
| `database.url` | `SERVLY_DATABASE_URL` | `--database-url` | `<data_dir>/servly.db` for SQLite; required for PostgreSQL |
| `migrate_on_start` | `SERVLY_MIGRATE_ON_START` | `--migrate-on-start` | `true` |
| `log.level` | `SERVLY_LOG_LEVEL` | `--log-level` | `info` |
| `log.format` | `SERVLY_LOG_FORMAT` | `--log-format` | `json` |
| `http.cookie_secure` | `SERVLY_HTTP_COOKIE_SECURE` | - | `auto`: `Secure` session cookie when the request came over HTTPS; `true` or `false` to force |
| `http.trust_proxy_headers` | `SERVLY_HTTP_TRUST_PROXY_HEADERS` | - | `false`; `true` only behind a reverse proxy that sets `X-Forwarded-For`/`-Proto` |
| `http.public_base_url` | `SERVLY_HTTP_PUBLIC_BASE_URL` | - | empty: taken from the request; the address guests use, for QR codes, e.g. `http://192.168.1.20:8080` |
| `secret_key` | `SERVLY_SECRET_KEY` | - | empty: online payments off. 32 random bytes, base64 (`openssl rand -base64 32`), that encrypt payment provider credentials. Keep it with your backups: without it, connected providers must be connected again |
| `payments.sandbox` | `SERVLY_PAYMENTS_SANDBOX` | - | `false`; `true` offers a fake payment provider for demos and tests. Never for real guests |

The config file is optional. servly reads the first file it finds among `--config`, `$SERVLY_CONFIG`, `~/.config/servly-io/servly.yaml` and `/etc/servly-io/servly.yaml`. An unknown key in the file is an error, so typos do not pass silently.

```yaml
# /etc/servly-io/servly.yaml
listen_addr: ":8080"
data_dir: /var/lib/servly
log:
  level: info
  format: json
```

Inspect what servly will use:

```bash
servly config path   # which file is read
servly config view   # the resolved settings, passwords redacted
```

## Data directory

```
/var/lib/servly/
├── servly.db        SQLite database (WAL mode)
├── servly.db-wal    write-ahead log: part of the database, never delete it
├── servly.db-shm
└── assets/          uploaded images, one directory per restaurant
```

Back up the database and `assets/` together. See [Backup and restore](backup-restore.md).

## Forgotten password

On the server:

```bash
servly user set-password anna@example.com   # asks for the new password; ends all of that user's sessions
```

## Upgrades

1. Take a backup ([Backup and restore](backup-restore.md)), and keep the previous one.
2. Replace the binary, or pull the new image.
3. Restart. Migrations run at startup. Check `servly migrate status` and `/readyz`.

To control migrations by hand instead, set `migrate_on_start: false` and run `servly migrate up` after the backup. Until you migrate, `/readyz` reports `503`.

Downgrading a binary below the schema version is not supported. Restore the backup taken before the upgrade instead.

## Network and offline operation

* Nothing in the order flow calls the internet. The binary embeds the frontend, fonts, timezone data and QR generation.
* Online payments (optional) do need the internet: servly talks to the payment provider, and the guest's phone opens its payment page. Without a connection, guests pay staff as usual, and open payments are confirmed once the connection is back. Webhooks need servly reachable from the internet over HTTPS. Without them, servly checks open payments every few seconds, which is enough.
* Plain HTTP on a LAN means anyone on that network can watch staff sessions. Put staff devices on a separate network from guest Wi-Fi, or terminate TLS in a reverse proxy with a local certificate.
* If the connection drops, the staff board shows it and reloads the state when the connection returns. Orders are safe as soon as the customer sees the confirmation.

## Health

* `GET /healthz`: liveness. Use it for container restarts.
* `GET /readyz`: database reachable and schema current. Use it for load balancers and monitoring.
