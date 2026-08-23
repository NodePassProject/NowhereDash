<div align="center">
  <img src="web/public/nowhere.png" alt="NowhereDash" height="80">
</div>

**Language:** English | [简体中文](docs/zh-CN/README.md)

![GitHub license](https://img.shields.io/github/license/NodePassProject/NowhereDash)
![GitHub release](https://img.shields.io/github/v/release/NodePassProject/NowhereDash?include_prereleases)
![GitHub downloads](https://img.shields.io/github/downloads/NodePassProject/NowhereDash/total.svg)
![Docker image](https://img.shields.io/badge/docker-ghcr.io%2Fnodepassproject%2Fnowheredash-blue?logo=docker&logoColor=white)

NowhereDash is a modern web dashboard for managing [Nowhere](https://github.com/NodePassProject/Nowhere) Portal instances through the [OpenCtrl](https://github.com/NodePassProject/OpenCtrl) master API. It ships as a single Go binary (Gin + GORM + SQLite/PostgreSQL) with an embedded React (Vite + TypeScript + HeroUI) frontend, and streams runtime state through SSE/WebSocket.

This project is Portal-only. It does not include legacy client/server instance modes, service assembly, or compatibility fields from earlier dashboard formats.

## Highlights

- **Portal-focused management**: create, edit, start, stop, restart, rename, sort, and monitor Nowhere `portal://` instances.
- **OpenCtrl endpoint control**: manage multiple OpenCtrl `/api/v2` endpoints from one dashboard.
- **Complete Portal editor**: handle network mode, TLS, certificates, ALPN, rate limits, dialing, SOCKS, next-hop, carriers, pools, SNI, pinning, and log level.
- **Metadata preservation**: keep OpenCtrl `meta.tags` and `meta.peer` independent from the Portal URL.
- **Vector output**: generate a matching `vector://` URL and QR code for every Portal.
- **Managed subscriptions**: publish selected running Portals through token-authenticated `/sub/portal?token=...` feeds.
- **Real-time telemetry**: stream status, traffic, connection, latency, and log updates through SSE and WebSocket.
- **Traffic and history tools**: view runtime metrics, clean historical data, and compact SQLite during maintenance windows.
- **Portable runtime**: run as Docker, systemd service, or a standalone binary with an embedded frontend.
- **Database choice**: initialize with SQLite or PostgreSQL from the web setup wizard.
- **Security options**: built-in setup flow, password reset, OAuth2-only login mode, TLS flags, and token rotation for subscriptions.
- **Mobile-friendly workflows**: QR code and `anywhere://add-proxy` helpers for importing into Anywhere.

## Quick Start

- **Docker:** [docs/en/DOCKER.md](docs/en/DOCKER.md)
- **Binary + systemd:** [docs/en/BINARY.md](docs/en/BINARY.md)
- **Development:** [docs/en/DEVELOPMENT.md](docs/en/DEVELOPMENT.md)

On first start, NowhereDash enters Setup mode when no database configuration exists. Open the web UI, choose SQLite or PostgreSQL, accept the compliance notice, and create the first administrator. The setup wizard writes `.env`; restart the service after setup if your process manager does not do it automatically.

## Documentation

- **Migration Guide:** [MIGRATION.md](docs/en/MIGRATION.md)
- **Docker Guide:** [DOCKER.md](docs/en/DOCKER.md)
- **Binary Guide:** [BINARY.md](docs/en/BINARY.md)
- **Development Guide:** [DEVELOPMENT.md](docs/en/DEVELOPMENT.md)
- **Offline SQLite Compaction:** [SQLITE-MAINTENANCE.md](docs/en/SQLITE-MAINTENANCE.md)

## Portal Configuration

NowhereDash manages the current Nowhere Portal parameters:

```text
portal://<shared-key>@<listen-host>:<port>
?net=mix|tcp|udp
&tls=1|2
&crt=...
&key=...
&alpn=...
&rate=...
&etar=...
&dial=auto|IP
&socks=none|endpoint
&next=none|shared-key@host:port
&up=tcp|udp
&down=tcp|udp
&pool=0..256
&sni=...
&pin=<lowercase SHA-256>
&log=none|debug|info|warn|error|event
```

`socks` and `next` are mutually exclusive. Refer to the [official Nowhere configuration reference](https://github.com/NodePassProject/Nowhere/blob/main/docs/configuration.md) for runtime behavior and defaults.

The generated Vector URL uses `127.0.0.1:1080` as its local SOCKS5 listener. When a Portal binds a wildcard address, NowhereDash uses the OpenCtrl endpoint hostname as the public Portal host.

## Portal Subscriptions

The Subscription menu publishes selected existing Portals through `/sub/portal?token=...`. The response is generated from current, running Portal data on every pull and contains one or more `nowhere://` URLs; it never includes the separate `vector://` credentials shown on Portal pages.

Subscriptions support expiry, traffic limits, carrier preferences, traffic reset, content preview, token rotation, and one-click import into Anywhere through `anywhere://add-proxy`. A subscription URL is a bearer secret: deploy it over HTTPS and redact its `token` query parameter in reverse-proxy, CDN, and observability logs.

## CLI Flags

```bash
./nowheredash --help
./nowheredash --version
./nowheredash --port 4000
./nowheredash --log-level INFO
./nowheredash --cert /path/to/cert.pem --key /path/to/key.pem
./nowheredash --disable-login
./nowheredash --sse-debug-log
./nowheredash --disable-sse-log
./nowheredash --demo
./nowheredash --resetpwd
```

Common environment variables include `PORT`, `LOG-LEVEL`, `TLS_CERT`, `TLS_KEY`, `DISABLE_LOGIN`, `SSE_DEBUG_LOG`, `DISABLE_SSE_LOG`, `DEMO_MODE`, `DB_DRIVER`, `DB_PATH`, and the `PG_*` PostgreSQL variables written by the setup wizard.

## Development

Requirements: Go 1.23+, Node.js 20+, and pnpm 10+.

```bash
cd web
corepack enable
corepack prepare pnpm@10.23.0 --activate
pnpm install --frozen-lockfile
pnpm build
cd ..
go run ./cmd/server
```

Run backend tests:

```bash
go test ./...
```

Run the frontend during development:

```bash
cd web
pnpm dev
```

## Data Compatibility

NowhereDash uses a Portal-only schema and backup format. Legacy dashboard tunnel fields and service records are intentionally unsupported. Recreate existing instances as Nowhere Portal instances or import a NowhereDash Portal-only backup.

## License

GNU General Public License v3.0. See [LICENSE](LICENSE).

## Disclaimer

This project is provided "as is", without any express or implied warranties. You are responsible for complying with local laws and regulations and using it only for lawful purposes. The authors are not liable for any direct, indirect, incidental, or consequential damages. The authors reserve the right to modify features and this statement at any time.

## Support

- Issues: https://github.com/NodePassProject/NowhereDash/issues
- Nowhere: https://github.com/NodePassProject/Nowhere
- OpenCtrl: https://github.com/NodePassProject/OpenCtrl

---

Copyright 2026 NodePassProject.
