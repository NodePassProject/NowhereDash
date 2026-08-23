# Development

English | [简体中文](../zh-CN/DEVELOPMENT.md)

## Prerequisites

- Node.js 20+
- pnpm 10.23.0 (via Corepack, matching `web/package.json`)
- Go 1.23+ (see `go.mod`)

SQLite uses the pure-Go `modernc.org/sqlite` stack, so no C toolchain or `sqlite-dev` package is required.

```bash
corepack enable
corepack prepare pnpm@10.23.0 --activate
```

## Install Frontend Dependencies

```bash
cd web
pnpm install --frozen-lockfile
```

The Vite build writes static assets to `cmd/server/dist`, where Go embeds them.

## Dev Mode

Run the backend from the repository root:

```bash
go run ./cmd/server --port 4000
```

On a fresh checkout, the backend enters Setup mode until `.env` is created by the web setup wizard. If you already have a SQLite database or PostgreSQL settings, keep the generated `.env` in the repository root or export the same variables before starting the backend.

Run the frontend dev server in another terminal:

```bash
cd web
pnpm dev
```

Vite proxies `/api`, `/sub/portal`, and WebSocket traffic to `http://localhost:4000` by default. Override the backend URL with `VITE_API_BASE` when needed:

```bash
cd web
VITE_API_BASE=http://localhost:4001 pnpm dev
```

Open the URL printed by Vite, usually `http://localhost:5173`.

## Production Build

Use the project build script when you want frontend assets and release binaries:

```bash
./build.sh
```

The script runs `pnpm install --frozen-lockfile`, builds the frontend into `cmd/server/dist`, then creates Linux and Windows binaries in `release/`.

For a local binary only:

```bash
cd web
pnpm install --frozen-lockfile
pnpm build
cd ..
go build -o nowheredash ./cmd/server
```

## Tests

Backend:

```bash
go test ./...
```

Frontend type-check and build:

```bash
cd web
pnpm build:check
```

Lint:

```bash
cd web
pnpm lint
```

## Useful Runtime Flags

```bash
go run ./cmd/server --version
go run ./cmd/server --port 4000
go run ./cmd/server --log-level DEBUG
go run ./cmd/server --sse-debug-log
go run ./cmd/server --disable-sse-log
```
