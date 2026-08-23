<div align="center">
  <img src="../../web/public/nowhere.png" alt="NowhereDash" height="80">
</div>

**语言:** [English](../../README.md) | 简体中文

![GitHub license](https://img.shields.io/github/license/NodePassProject/NowhereDash)
![GitHub release](https://img.shields.io/github/v/release/NodePassProject/NowhereDash?include_prereleases)
![GitHub downloads](https://img.shields.io/github/downloads/NodePassProject/NowhereDash/total.svg)
![Docker image](https://img.shields.io/badge/docker-ghcr.io%2Fnodepassproject%2Fnowheredash-blue?logo=docker&logoColor=white)

NowhereDash 是一个通过 [OpenCtrl](https://github.com/NodePassProject/OpenCtrl) Master API 管理 [Nowhere](https://github.com/NodePassProject/Nowhere) Portal 实例的现代 Web 面板。它以单个 Go 二进制运行（Gin + GORM + SQLite/PostgreSQL），内嵌 React（Vite + TypeScript + HeroUI）前端，并通过 SSE/WebSocket 展示实时状态。

本项目仅支持 Nowhere Portal。旧的 client/server 实例模式、服务组装功能和历史兼容字段均不再保留。

## 亮点

- **Portal 专用管理**：创建、编辑、启动、停止、重启、重命名、排序和监控 `portal://` 实例。
- **OpenCtrl 端点控制**：在一个面板内管理多个 OpenCtrl `/api/v2` 端点。
- **完整 Portal 编辑器**：覆盖网络模式、TLS、证书、ALPN、速率限制、拨号、SOCKS、Next Hop、载波、连接池、SNI、Pin 和日志级别。
- **Metadata 保留**：将 OpenCtrl `meta.tags` 与 `meta.peer` 独立于 Portal URL 保存。
- **Portal 导入输出**：为每个 Portal 生成匹配的 `nowhere://` URL 与二维码。
- **托管订阅**：将选定的运行中 Portal 发布为带 Token 鉴权的 `/sub/portal?token=...` 订阅。
- **实时遥测**：通过 SSE 和 WebSocket 展示状态、流量、连接数、延迟与日志。
- **流量与历史工具**：查看运行指标、清理历史数据，并在维护窗口执行 SQLite 停服压缩。
- **便携运行**：支持 Docker、systemd 服务或单二进制运行，前端已嵌入后端。
- **数据库可选**：首次启动通过 Web Setup 向导选择 SQLite 或 PostgreSQL。
- **安全选项**：支持初始化向导、密码重置、OAuth2-only 登录、TLS 参数和订阅 Token 轮换。
- **移动端友好**：提供二维码和 `anywhere://add-proxy`，便于导入 Anywhere。

## 快速开始

- **Docker：** [DOCKER.md](DOCKER.md)
- **二进制 + systemd：** [BINARY.md](BINARY.md)
- **开发环境：** [DEVELOPMENT.md](DEVELOPMENT.md)

首次启动时，如果没有数据库配置，NowhereDash 会进入 Setup 模式。打开 Web UI，选择 SQLite 或 PostgreSQL，确认合规声明，并创建第一个管理员。向导会写入 `.env`；如果你的进程管理器不会自动重启，请在初始化完成后手动重启服务。

## 文档

- **迁移指南：** [MIGRATION.md](MIGRATION.md)
- **Docker 部署：** [DOCKER.md](DOCKER.md)
- **二进制部署：** [BINARY.md](BINARY.md)
- **开发环境：** [DEVELOPMENT.md](DEVELOPMENT.md)
- **SQLite 停服压缩：** [SQLITE-MAINTENANCE.md](SQLITE-MAINTENANCE.md)

## Portal 参数

NowhereDash 管理当前 Nowhere Portal 参数：

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
&pin=<小写 SHA-256>
&log=none|debug|info|warn|error|event
```

`socks` 与 `next` 互斥。完整规则和默认值以 [Nowhere 官方配置文档](https://github.com/NodePassProject/Nowhere/blob/main/docs/configuration.md) 为准。

生成 Vector URL 时，本地 SOCKS5 监听默认为 `127.0.0.1:1080`。如果 Portal 监听空地址或通配地址，NowhereDash 会使用 OpenCtrl 端点的 hostname 作为外部可达地址。

## Portal 订阅

订阅菜单可将选定的已有 Portal 发布到 `/sub/portal?token=...`。服务端会在每次拉取时根据当前运行中的 Portal 动态生成正文，正文包含一行或多行 `nowhere://` URL。Portal 导入与订阅使用统一的 URL scheme。

订阅支持到期时间、流量上限、传输偏好、流量重置、正文预览、Token 轮换，以及通过 `anywhere://add-proxy` 一键导入 Anywhere。订阅 URL 属于 Bearer Secret，生产环境应使用 HTTPS，并在反向代理、CDN 与可观测性日志中隐藏 `token` 查询参数。

## 命令行参数

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

常用环境变量包括 `PORT`、`LOG-LEVEL`、`TLS_CERT`、`TLS_KEY`、`DISABLE_LOGIN`、`SSE_DEBUG_LOG`、`DISABLE_SSE_LOG`、`DEMO_MODE`、`DB_DRIVER`、`DB_PATH`，以及 Setup 向导写入的 `PG_*` PostgreSQL 变量。

## 开发构建

需要 Go 1.23+、Node.js 20+ 和 pnpm 10+。

```bash
cd web
corepack enable
corepack prepare pnpm@10.23.0 --activate
pnpm install --frozen-lockfile
pnpm build
cd ..
go run ./cmd/server
```

运行后端测试：

```bash
go test ./...
```

启动前端开发服务：

```bash
cd web
pnpm dev
```

## 数据兼容性

NowhereDash 使用 Portal-only 数据模型和备份格式，不接受旧面板的隧道字段和服务记录。已有配置需要按 Nowhere Portal 重新创建，或导入 NowhereDash 的 Portal-only 备份。

## 许可证

[GNU General Public License v3.0](../../LICENSE)

## 免责声明

本项目按“现状”提供，不附带任何明示或暗示担保。使用者需自行遵守所在地法律法规，并仅将其用于合法用途。作者不对任何直接、间接、偶发或后果性损失承担责任，并保留随时调整功能和声明的权利。

## 支持

- Issues: https://github.com/NodePassProject/NowhereDash/issues
- Nowhere: https://github.com/NodePassProject/Nowhere
- OpenCtrl: https://github.com/NodePassProject/OpenCtrl

---

Copyright 2026 NodePassProject.
