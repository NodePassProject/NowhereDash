# 开发环境

[English](../en/DEVELOPMENT.md) | 简体中文

## 环境准备

- Node.js 20+
- pnpm 10.23.0（建议通过 Corepack，与 `web/package.json` 保持一致）
- Go 1.23+（以 `go.mod` 为准）

SQLite 使用纯 Go 的 `modernc.org/sqlite` 相关实现，无需安装 C 工具链或 `sqlite-dev`。

```bash
corepack enable
corepack prepare pnpm@10.23.0 --activate
```

## 安装前端依赖

```bash
cd web
pnpm install --frozen-lockfile
```

Vite 构建产物会输出到 `cmd/server/dist`，后端会从该目录嵌入静态资源。

## 开发模式

在仓库根目录启动后端：

```bash
go run ./cmd/server --port 4000
```

全新仓库首次启动会进入 Setup 模式，直到 Web 初始化向导创建 `.env`。如果已有 SQLite 数据库或 PostgreSQL 配置，请把生成的 `.env` 放在仓库根目录，或在启动后端前导出同名环境变量。

另开一个终端启动前端开发服务：

```bash
cd web
pnpm dev
```

Vite 默认会把 `/api`、`/sub/portal` 和 WebSocket 流量代理到 `http://localhost:4000`。如需连接其他后端地址，可设置 `VITE_API_BASE`：

```bash
cd web
VITE_API_BASE=http://localhost:4001 pnpm dev
```

打开 Vite 输出的地址，通常是 `http://localhost:5173`。

## 生产构建

需要同时构建前端资源和发布二进制时，使用项目构建脚本：

```bash
./build.sh
```

脚本会执行 `pnpm install --frozen-lockfile`，将前端构建到 `cmd/server/dist`，并在 `release/` 下生成 Linux 和 Windows 二进制。

只构建本地二进制：

```bash
cd web
pnpm install --frozen-lockfile
pnpm build
cd ..
go build -o nowheredash ./cmd/server
```

## 测试

后端：

```bash
go test ./...
```

前端类型检查和构建：

```bash
cd web
pnpm build:check
```

Lint：

```bash
cd web
pnpm lint
```

## 常用运行参数

```bash
go run ./cmd/server --version
go run ./cmd/server --port 4000
go run ./cmd/server --log-level DEBUG
go run ./cmd/server --sse-debug-log
go run ./cmd/server --disable-sse-log
```
