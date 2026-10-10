<p align="center"><img src="web/public/icon.svg" alt="servly" width="96"></p>

<p align="center">Lightweight order management for restaurants.</p>
<p align="center"><a href="https://servly-io.github.io/servly/">Documentation</a></p>

Guests scan a QR code at the table and order from their phone. Staff see the order on a live board on a tablet or PC and work it through to completion. The owner manages the menu, tables and staff in the browser.

servly runs **locally** on one small machine with SQLite, with no cloud account and no internet needed for ordering. It also runs **managed** for many restaurants on PostgreSQL. Both use the same single binary, which serves the API and the web app.

> **Status:** early development. The foundation is in place: server, configuration, both databases, migrations, health endpoints, web app shell, container image. Menu administration, ordering and the staff board arrive in the next phases of [MASTER_PLAN.md](MASTER_PLAN.md).

## Quick start

```bash
make run                      # builds the web app and the server, serves on :8080 with SQLite in ./data
curl localhost:8080/readyz    # {"status":"ok"}
```

In a container:

```bash
make image
podman run --rm -p 8080:8080 -v servly-data:/data servly:$(git describe --tags --always --dirty)
```

Configuration: flags, `SERVLY_*` environment variables, or `servly.yaml`. See `servly config view` and [Local deployment](docs/docs/local-deployment.md).

## Documentation

* [Architecture](docs/docs/architecture.md), [Domain model](docs/docs/domain-model.md), [API](docs/docs/api.md), [Security](docs/docs/security.md)
* [Local deployment](docs/docs/local-deployment.md), [Cloud deployment](docs/docs/cloud-deployment.md), [Backup and restore](docs/docs/backup-restore.md)
* [Decision records](docs/docs/adr/index.md)

## Security

Report security issues through [GitHub's private vulnerability reporting](https://github.com/servly-io/servly/security/advisories/new).

## Contributing

Building and testing: [Contributing](docs/docs/community/contribution.md).

## License

MIT. See [LICENSE](LICENSE).
