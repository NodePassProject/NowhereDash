# Docker 部署（NowhereDash）

本指南使用 Docker 部署 NowhereDash。NowhereDash 以 **单容器**方式运行（Go API + 内置 Web UI），默认使用 **单端口**（`4000`）。

## 环境要求

- Docker Engine + Docker Compose（`docker compose`）
- 用于持久化的数据目录（`db/`）与文件日志目录（`logs/`）

## 快速开始（docker compose）

1）准备目录：

```bash
mkdir -p nowheredash && cd nowheredash
mkdir -p db logs
```

2）创建 `docker-compose.yml`（示例）：

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

3）启动：

```bash
docker compose up -d
```

## 首次初始化

首次启动时，如果没有数据库配置，NowhereDash 会进入 Setup 模式。在浏览器打开 `http://localhost:4000` 并完成向导。

在向导中选择 SQLite 或 PostgreSQL，确认合规声明，并创建第一个管理员。向导会在容器工作目录写入 `.env`；compose 示例已持久化 `./db`，默认 SQLite 数据库会保存在宿主机。

如果使用 PostgreSQL，请把生成的数据库配置同步到 compose 的 `environment:` 或其他持久化环境变量来源，避免容器重建后丢失。

初始化后，如果容器没有自动重启，请手动重启：

```bash
docker compose restart nowheredash
```

初始化完成后，健康接口应返回 `{"status":"ok"}`：

```bash
curl -fsS http://localhost:4000/api/health
```

## 添加 OpenCtrl 端点

NowhereDash 通过 OpenCtrl 端点管理 Nowhere。登录后进入 **Endpoints**，添加 OpenCtrl `/api/v2` 地址和 API Key。

如果节点使用项目内置安装脚本部署，可以在 Endpoints 页使用 **Guided Add**。页面会生成带一次性注册 Token 的命令，安装器会启动 OpenCtrl 与 Nowhere，并把端点自动注册回 NowhereDash。

## 常用配置

可通过命令行参数（推荐）或环境变量配置。

### 端口

- 默认端口：`4000`
- CLI：`./nowheredash --port 8080`
- Env：`PORT=8080`

### TLS（HTTPS）

同时提供证书与私钥即可启用 HTTPS：

```bash
./nowheredash --cert /path/to/cert.pem --key /path/to/key.pem
```

Docker 中可挂载证书并通过 `command:` 传参：

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

### 禁用用户名密码登录（仅 OAuth2）

```bash
./nowheredash --disable-login
```

启用前请先在界面中配置好 OAuth2，否则可能会无法登录。

### 禁用 SSE 日志文件记录

默认情况下，NowhereDash 会将 Portal/OpenCtrl 的 SSE 事件日志记录到 `logs/` 目录。如果磁盘空间有限或不需要保留日志文件，可以禁用：

```bash
./nowheredash --disable-sse-log
```

或在 docker-compose.yml 中：

```yaml
services:
  nowheredash:
    image: ghcr.io/nodepassproject/nowheredash:latest
    environment:
      - DISABLE_SSE_LOG=true
    # 或者使用 command
    command: ["./nowheredash","--disable-sse-log"]
```

**注意：** 禁用后，SSE 日志仍会实时推送到前端界面，但不会保存到文件中。

## 备份与恢复

- 备份：拷贝 `db/`（SQLite 数据库），需要的话也可备份 `logs/`。如果你在容器外手动维护 `.env`，也应一并备份。
- 恢复：停止容器，恢复目录，再启动。

```bash
docker compose down
# restore ./db (and ./logs)
docker compose up -d
```

## 升级

```bash
docker compose pull
docker compose up -d
```

如使用固定版本 tag，请先更新 `docker-compose.yml` 中的镜像 tag。

## 排错

- 健康检查：`curl -fsS http://localhost:4000/api/health`
- 查看日志：`docker logs -f nowheredash`
- 重置管理员密码（重置后需要重启容器）：`docker exec -it nowheredash ./nowheredash --resetpwd`
