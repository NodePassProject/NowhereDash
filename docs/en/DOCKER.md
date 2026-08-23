# Docker Deployment (NowhereDash)

This guide deploys NowhereDash using Docker. NowhereDash runs as a **single container** (Go API + embedded Web UI) on **one port** (default `4000`).

## Requirements

- Docker Engine + Docker Compose (`docker compose`)
- A directory to persist data (`db/`) and file logs (`logs/`)

## Quick Start (docker compose)

1) Create a working directory and prepare volumes:

```bash
mkdir -p nowheredash && cd nowheredash
mkdir -p db logs
```

2) Create `docker-compose.yml` (example):

```yaml
services:
  nowheredash:
    image: ghcr.io/nodepassproject/nowheredash:latest
    container_name: nowheredash
    ports:
      - "4000:4000"
    volumes:
      - ./db:/app/db
      - ./logs:/app/logs
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:4000/api/health"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s
```

3) Start:

```bash
docker compose up -d
```

## First Setup

On first start, NowhereDash enters Setup mode when no database configuration exists. Open `http://localhost:4000` in your browser and complete the wizard.

Choose SQLite or PostgreSQL, accept the compliance notice, and create the first administrator. The wizard writes `.env` inside the container working directory. Because the compose file persists `./db`, the default SQLite database is retained on the host.

For PostgreSQL deployments, keep the generated database settings in your compose `environment:` block or another persistent environment source so container recreation does not lose them.

After setup, restart the container if it does not restart automatically:

```bash
docker compose restart nowheredash
```

Once initialized, the health endpoint should return `{"status":"ok"}`:

```bash
curl -fsS http://localhost:4000/api/health
```

## Add an OpenCtrl Endpoint

NowhereDash manages Nowhere through OpenCtrl endpoints. After logging in, open **Endpoints** and add an OpenCtrl `/api/v2` URL with its API key.

If you use the bundled installer on a node, the **Guided Add** flow on the Endpoints page can generate a registration command. The installer starts OpenCtrl and Nowhere, then registers the endpoint back to NowhereDash with a one-time token.

## Common Options

You can pass configuration either as CLI flags (recommended) or via environment variables.

### Ports

- Default port: `4000`
- CLI: `./nowheredash --port 8080`
- Env: `PORT=8080`

### TLS (HTTPS)

Provide both cert and key to enable HTTPS:

```bash
./nowheredash --cert /path/to/cert.pem --key /path/to/key.pem
```

In Docker, mount the certificate files and pass the flags via `command:`:

```yaml
services:
  nowheredash:
    image: ghcr.io/nodepassproject/nowheredash:latest
    ports: ["443:443"]
    volumes:
      - ./db:/app/db
      - ./logs:/app/logs
      - ./certs/fullchain.pem:/certs/fullchain.pem:ro
      - ./certs/privkey.pem:/certs/privkey.pem:ro
    command: ["./nowheredash","--port","443","--cert","/certs/fullchain.pem","--key","/certs/privkey.pem"]
```

### Disable Password Login (OAuth2 only)

```bash
./nowheredash --disable-login
```

If you enable this, make sure OAuth2 is configured in the UI first; otherwise you may lock yourself out.

### Disable SSE Log File Recording

By default, NowhereDash records Portal/OpenCtrl SSE event logs to the `logs/` directory. If disk space is limited or you don't need to keep log files, you can disable it:

```bash
./nowheredash --disable-sse-log
```

Or in docker-compose.yml:

```yaml
services:
  nowheredash:
    image: ghcr.io/nodepassproject/nowheredash:latest
    environment:
      - DISABLE_SSE_LOG=true
    # or use command
    command: ["./nowheredash","--disable-sse-log"]
```

**Note:** When disabled, SSE logs will still be pushed to the frontend in real-time, but won't be saved to files.

## Backup / Restore

- Backup: copy the `db/` directory (SQLite) and optionally `logs/`. If you keep a manually managed `.env` outside the container, back it up too.
- Restore: stop the container, restore directories, then start again.

```bash
docker compose down
# restore ./db (and ./logs)
docker compose up -d
```

## Upgrade

```bash
docker compose pull
docker compose up -d
```

If you pin to a version tag, update the tag in `docker-compose.yml` first.

## Troubleshooting

- Check health: `curl -fsS http://localhost:4000/api/health`
- View logs: `docker logs -f nowheredash`
- Reset admin password (requires restart after): `docker exec -it nowheredash ./nowheredash --resetpwd`
