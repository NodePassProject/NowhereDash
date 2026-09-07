<div align="center">
  <img src="../logo.png" alt="NowhereDash" width="320">

  <h1>NowhereDash</h1>

  <p><strong>专注于 Nowhere Portal 基础设施的统一控制面板。</strong></p>
  <p>在一个面板中管理多个 OpenCtrl 端点、操作 Portal 实例、查看实时状态并发布私有订阅。</p>

  <p>
    <a href="../../LICENSE"><img src="https://img.shields.io/github/license/NodePassProject/NowhereDash" alt="许可证"></a>
    <a href="https://github.com/NodePassProject/NowhereDash/releases"><img src="https://img.shields.io/github/v/release/NodePassProject/NowhereDash?include_prereleases" alt="版本"></a>
    <a href="https://github.com/NodePassProject/NowhereDash/releases"><img src="https://img.shields.io/github/downloads/NodePassProject/NowhereDash/total.svg" alt="下载量"></a>
    <a href="https://github.com/NodePassProject/NowhereDash/pkgs/container/nowheredash"><img src="https://img.shields.io/badge/docker-ghcr.io%2Fnodepassproject%2Fnowheredash-blue?logo=docker&logoColor=white" alt="Docker 镜像"></a>
  </p>

  <p>
    <a href="#产品展示">产品展示</a> ·
    <a href="#快速开始">快速开始</a> ·
    <a href="#文档">文档</a> ·
    <a href="#开发构建">开发构建</a>
  </p>

  <p><a href="../../README.md">English</a> · <strong>简体中文</strong></p>
</div>

NowhereDash 以单个 Go 二进制发布，并内嵌 React 前端。后端采用 Gin、GORM 和 SQLite 或 PostgreSQL，Web 应用采用 Vite、TypeScript 与 HeroUI，运行状态通过 SSE 和 WebSocket 实时传递。

> [!IMPORTANT]
> NowhereDash 仅管理 Nowhere Portal 实例。旧版 client/server 模式、服务组装功能和历史面板兼容字段均不再支持。

## 能力概览

| 领域         | 核心能力                                                      |
| ------------ | ------------------------------------------------------------- |
| **Portal**   | 在多个 OpenCtrl 端点之间统一管理 Portal 实例。                |
| **可观测性** | 实时监控流量、连接数、延迟、运行状态和日志。                  |
| **订阅**     | 发布受保护的订阅，并提供二维码和移动端导入链接。              |
| **安全**     | 提供初始化向导、OAuth2 登录、TLS 和 Token 轮换。              |
| **部署**     | 使用 Docker、systemd 或单二进制运行，支持 SQLite/PostgreSQL。 |

## 产品展示

|                                                  |                                                 |                                                      |
| ------------------------------------------------ | ----------------------------------------------- | ---------------------------------------------------- |
| ![初始化向导](../screenshots/00-initialtion.png) | ![登录页面](../screenshots/01-login.png)        | ![仪表盘概览](../screenshots/02-dashboard.gif)       |
| ![订阅管理](../screenshots/03-subscription.gif)  | ![Portal 管理](../screenshots/04-portal.gif)    | ![Portal 详情](../screenshots/05-portal-details.gif) |
| ![节点管理](../screenshots/06-node.gif)          | ![节点详情](../screenshots/07-node-details.gif) | ![设置](../screenshots/08-settings.gif)              |

## 快速开始

运行最新容器：

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

打开 [http://localhost:4000](http://localhost:4000)。

> [!TIP]
> 首次启动时，Setup 向导会引导你选择 SQLite 或 PostgreSQL、确认合规声明并创建第一个管理员，最终配置会写入 `.env`；如果进程管理器不会自动重启，请在初始化完成后手动重启服务。

| 安装方式         | 指南                       | 适用场景              |
| ---------------- | -------------------------- | --------------------- |
| Docker           | [Docker 部署](DOCKER.md)   | 容器环境与快速体验    |
| 二进制 + systemd | [二进制部署](BINARY.md)    | 长期运行的 Linux 主机 |
| 源码             | [开发环境](DEVELOPMENT.md) | 参与开发与自定义构建  |

## 配置

<details>
<summary><strong>常用命令行参数</strong></summary>

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

常用环境变量：

| 分类     | 变量                                                      |
| -------- | --------------------------------------------------------- |
| 服务     | `PORT`、`LOG-LEVEL`、`TLS_CERT`、`TLS_KEY`                |
| 身份验证 | `DISABLE_LOGIN`                                           |
| 运行状态 | `SSE_DEBUG_LOG`、`DISABLE_SSE_LOG`、`DEMO_MODE`           |
| 数据库   | `DB_DRIVER`、`DB_PATH`，以及 Setup 向导写入的 `PG_*` 变量 |

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

运行测试和前端开发服务：

```bash
go test ./...

cd web
pnpm dev
```

## 文档

| 主题                  | 指南                                           |
| --------------------- | ---------------------------------------------- |
| Docker 部署           | [DOCKER.md](DOCKER.md)                         |
| 二进制与 systemd 部署 | [BINARY.md](BINARY.md)                         |
| 开发环境              | [DEVELOPMENT.md](DEVELOPMENT.md)               |
| 迁移                  | [MIGRATION.md](MIGRATION.md)                   |
| SQLite 停服压缩       | [SQLITE-MAINTENANCE.md](SQLITE-MAINTENANCE.md) |

## 数据兼容性

NowhereDash 使用 Portal-only 数据模型和备份格式。旧面板中的实例需要按 Nowhere Portal 重新创建，或导入 NowhereDash 的 Portal-only 备份。

## 许可证与支持

NowhereDash 使用 [GNU General Public License v3.0](../../LICENSE) 许可证。

| 资源     | 链接                                                                                        |
| -------- | ------------------------------------------------------------------------------------------- |
| Issues   | [NodePassProject/NowhereDash/issues](https://github.com/NodePassProject/NowhereDash/issues) |
| Nowhere  | [NodePassProject/Nowhere](https://github.com/NodePassProject/Nowhere)                       |
| OpenCtrl | [NodePassProject/OpenCtrl](https://github.com/NodePassProject/OpenCtrl)                     |

<details>
<summary><strong>免责声明</strong></summary>

本项目按“现状”提供，不附带任何明示或暗示担保。使用者需自行遵守所在地法律法规，并仅将其用于合法用途。作者不对任何直接、间接、偶发或后果性损失承担责任，并保留随时调整功能和声明的权利。

</details>

<div align="center">
  <sub>Copyright 2026 NodePassProject.</sub>
</div>
