<div align="center">
  <img src="docs/logo.png" alt="NowhereDash" width="320">

  <h1>NowhereDash</h1>

  <p><strong>A focused control plane for Nowhere Portal infrastructure.</strong></p>
  <p>Manage multiple OpenCtrl endpoints, operate Portal instances, inspect live telemetry, and publish private subscriptions from one dashboard.</p>

  <p>
    <a href="LICENSE"><img src="https://img.shields.io/github/license/NodePassProject/NowhereDash" alt="License"></a>
    <a href="https://github.com/NodePassProject/NowhereDash/releases"><img src="https://img.shields.io/github/v/release/NodePassProject/NowhereDash?include_prereleases" alt="Release"></a>
    <a href="https://github.com/NodePassProject/NowhereDash/releases"><img src="https://img.shields.io/github/downloads/NodePassProject/NowhereDash/total.svg" alt="Downloads"></a>
    <a href="https://github.com/NodePassProject/NowhereDash/pkgs/container/nowheredash"><img src="https://img.shields.io/badge/docker-ghcr.io%2Fnodepassproject%2Fnowheredash-blue?logo=docker&logoColor=white" alt="Docker image"></a>
  </p>

  <p>
    <a href="#quick-start">Quick Start</a> ·
    <a href="#portal-subscriptions">Subscriptions</a> ·
    <a href="#documentation">Documentation</a> ·
    <a href="#development">Development</a>
  </p>

  <p><strong>English</strong> · <a href="docs/zh-CN/README.md">简体中文</a></p>
</div>

NowhereDash ships as a single Go binary with an embedded React frontend. It uses Gin, GORM, and SQLite or PostgreSQL on the backend, with Vite, TypeScript, and HeroUI in the web application. Runtime state is delivered through SSE and WebSocket.

> [!IMPORTANT]
> NowhereDash manages Nowhere Portal instances only. Legacy client/server modes, service assembly, and compatibility fields from earlier dashboard formats are intentionally unsupported.

## At a Glance

| Area                      | What NowhereDash provides                                                                                                             |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| **Portal lifecycle**      | Create, edit, start, stop, restart, rename, sort, and monitor `portal://` instances.                                                  |
| **OpenCtrl endpoints**    | Manage multiple OpenCtrl `/api/v2` endpoints from one dashboard.                                                                      |
| **Complete editor**       | Configure network mode, TLS, certificates, ALPN, rate limits, dialing, SOCKS, next hop, carriers, pools, SNI, pinning, and log level. |
| **Live operations**       | Stream status, traffic, connections, latency, and logs through SSE and WebSocket.                                                     |
| **Operations data**       | Inspect runtime metrics, clean historical data, and compact SQLite during maintenance windows.                                        |
| **Managed subscriptions** | Publish selected running Portals through token-authenticated feeds with expiry, traffic limits, previews, and token rotation.         |
| **Security controls**     | Use the guided setup, password reset, OAuth2-only login, TLS, and subscription token rotation.                                        |
| **Portable deployment**   | Run with Docker, systemd, or a standalone binary; initialize SQLite or PostgreSQL from the browser.                                   |
| **Mobile workflows**      | Generate QR codes, `nowhere://` URLs, and `anywhere://add-proxy` import links.                                                        |

OpenCtrl metadata remains intact: `meta.tags` and `meta.peer` are stored independently from the Portal URL.

## Quick Start

Run the latest container:

```bash
mkdir -p db logs

docker run -d \
  --name nowheredash \
  --restart unless-stopped \
  -p 4000:4000 \
  -v "$(pwd)/db:/app/db" \
  -v "$(pwd)/logs:/app/logs" \
  ghcr.io/nodepassproject/nowheredash:latest
```

Open [http://localhost:4000](http://localhost:4000).

> [!TIP]
> On first start, the Setup wizard lets you choose SQLite or PostgreSQL, accept the compliance notice, and create the first administrator. It writes the resulting configuration to `.env`; restart the service afterward if your process manager does not do so automatically.

| Installation     | Guide                                       | Recommended for                 |
| ---------------- | ------------------------------------------- | ------------------------------- |
| Docker           | [Docker guide](docs/en/DOCKER.md)           | Containers and quick evaluation |
| Binary + systemd | [Binary guide](docs/en/BINARY.md)           | Long-running Linux hosts        |
| Source           | [Development guide](docs/en/DEVELOPMENT.md) | Contributors and custom builds  |

## Portal Subscriptions

The Subscription menu publishes selected Portals through `/sub/portal?token=...`. Every request is rendered from the current running Portal state and returns one or more `nowhere://` URLs.

Subscriptions support expiry, traffic limits, carrier preferences, traffic reset, content preview, token rotation, light/dark icons, and one-click Anywhere import through `anywhere://add-proxy`.

> [!WARNING]
> A subscription URL is a bearer secret. Use HTTPS in production and redact its `token` query parameter from reverse-proxy, CDN, and observability logs.

## Configuration

<details>
<summary><strong>Common CLI flags</strong></summary>

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

</details>

Common environment variables:

| Group          | Variables                                                                 |
| -------------- | ------------------------------------------------------------------------- |
| Server         | `PORT`, `LOG-LEVEL`, `TLS_CERT`, `TLS_KEY`                                |
| Authentication | `DISABLE_LOGIN`                                                           |
| Runtime        | `SSE_DEBUG_LOG`, `DISABLE_SSE_LOG`, `DEMO_MODE`                           |
| Database       | `DB_DRIVER`, `DB_PATH`, and the `PG_*` values written by the Setup wizard |

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

Validation and local frontend development:

```bash
go test ./...

cd web
pnpm dev
```

## Documentation

| Topic                         | Guide                                                  |
| ----------------------------- | ------------------------------------------------------ |
| Deployment with Docker        | [DOCKER.md](docs/en/DOCKER.md)                         |
| Binary and systemd deployment | [BINARY.md](docs/en/BINARY.md)                         |
| Development environment       | [DEVELOPMENT.md](docs/en/DEVELOPMENT.md)               |
| Migration                     | [MIGRATION.md](docs/en/MIGRATION.md)                   |
| Offline SQLite compaction     | [SQLITE-MAINTENANCE.md](docs/en/SQLITE-MAINTENANCE.md) |

## Compatibility

NowhereDash uses a Portal-only schema and backup format. Recreate legacy dashboard instances as Nowhere Portal instances, or import a NowhereDash Portal-only backup.

## License and Support

NowhereDash is licensed under the [GNU General Public License v3.0](LICENSE).

| Resource | Link                                                                                        |
| -------- | ------------------------------------------------------------------------------------------- |
| Issues   | [NodePassProject/NowhereDash/issues](https://github.com/NodePassProject/NowhereDash/issues) |
| Nowhere  | [NodePassProject/Nowhere](https://github.com/NodePassProject/Nowhere)                       |
| OpenCtrl | [NodePassProject/OpenCtrl](https://github.com/NodePassProject/OpenCtrl)                     |

<details>
<summary><strong>Disclaimer</strong></summary>

This project is provided "as is", without any express or implied warranties. You are responsible for complying with local laws and regulations and using it only for lawful purposes. The authors are not liable for any direct, indirect, incidental, or consequential damages. The authors reserve the right to modify features and this statement at any time.

</details>

<div align="center">
  <sub>Copyright 2026 NodePassProject.</sub>
</div>
