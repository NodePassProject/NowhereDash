# 统一二进制部署

`scripts/install.sh` 可以在 Linux/macOS 上部署组件，`scripts/install.ps1` 用于 Windows。安装目标可以是 NowhereDash，也可以是由 OpenCtrl 管理的 Nowhere 节点。

| 安装目标 | 安装内容 |
| --- | --- |
| `dash` | NowhereDash |
| `nowhere` / `openctrl` | OpenCtrl + Nowhere 运行时 |
| `all` | 上述全部组件 |

`nowhere` 与 `openctrl` 是同一个安装目标。该目标不会创建固定的单 Portal 服务；OpenCtrl 将 Nowhere 作为运行时，之后由 NowhereDash 通过 OpenCtrl API 创建和管理 Portal/Vector 实例。

## 环境要求

- Linux：使用 systemd，需要 root 权限；节点支持 x86_64 和 arm64。
- Linux 节点始终下载静态链接的 `unknown-linux-musl` 版 Nowhere，不依赖宿主机 glibc 版本。
- macOS：当前仅支持 Apple Silicon (arm64) 节点，使用 launchd，需要 root 权限。NowhereDash 暂无 macOS Release。
- Windows：当前支持 x86_64 节点和 Dash，使用管理员 PowerShell 和 Windows 计划任务。
- 服务器需要能够访问 GitHub Releases。

Linux 脚本会安装缺少的基础工具，并为服务创建独立的非登录用户。macOS 节点使用系统 `nobody` 账户，Windows 任务使用 `SYSTEM` 账户。

## 下载安装器

建议先下载检查，再使用参数直接安装：

```bash
curl -fsSL https://raw.githubusercontent.com/NodePassProject/NowhereDash/main/scripts/install.sh \
  -o /tmp/nowheredash-install.sh
chmod +x /tmp/nowheredash-install.sh
sudo /tmp/nowheredash-install.sh --help
```

不带参数会打开交互菜单：

```bash
sudo /tmp/nowheredash-install.sh
```

Windows 使用原生 PowerShell 安装器：

```powershell
Invoke-WebRequest `
  https://raw.githubusercontent.com/NodePassProject/NowhereDash/main/scripts/install.ps1 `
  -OutFile $env:TEMP\nowheredash-install.ps1
Set-ExecutionPolicy -Scope Process Bypass
& $env:TEMP\nowheredash-install.ps1 status all
```

## 安装 Nowhere + OpenCtrl

以下三个命令等价，都会同时安装 OpenCtrl 与 Nowhere：

```bash
sudo /tmp/nowheredash-install.sh install nowhere
sudo /tmp/nowheredash-install.sh install openctrl
sudo /tmp/nowheredash-install.sh nowhere
```

默认配置为：

- 监听 `0.0.0.0:10101`
- API 路径 `/api/v2`
- OpenCtrl TLS 模式 `1`（临时自签证书）
- 最新 OpenCtrl 和 Nowhere 正式版

建议显式填写 Dash 连接节点时使用的公网域名或 IP：

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --openctrl-public-host node.example.com \
  --openctrl-port 10101
```

使用已有 PEM 证书：

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --openctrl-public-host node.example.com \
  --openctrl-port 443 \
  --openctrl-tls 2 \
  --openctrl-cert /etc/letsencrypt/live/node.example.com/fullchain.pem \
  --openctrl-key /etc/letsencrypt/live/node.example.com/privkey.pem
```

`--openctrl-tls 0` 会使用明文 HTTP，只适合可信内网或前置 TLS 反向代理。脚本不会申请证书，也不会停止现有 Web 服务。

### macOS Apple Silicon

macOS 使用同一个 Bash 安装器，自动匹配 `openctrl_*_darwin_arm64.tar.gz` 与 `nowhere-aarch64-apple-darwin.tar.gz`，并注册 `/Library/LaunchDaemons/com.nodepass.openctrl.plist`：

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --openctrl-public-host node.example.com
sudo nowhere-ctl status
```

launchd 以非特权账户运行节点，因此 macOS 上的 OpenCtrl 端口必须不小于 1024。Intel Mac 和 macOS 上的 Dash 会得到明确的不支持提示，因为当前 Nowhere/NowhereDash Release 没有相应产物。

### Windows x86_64

在管理员 PowerShell 中安装节点。安装器会匹配 `openctrl_*_windows_amd64.tar.gz` 与 `nowhere-x86_64-pc-windows-msvc.zip`，并创建开机启动的 `OpenCtrl` 计划任务：

```powershell
& $env:TEMP\nowheredash-install.ps1 install nowhere `
  -OpenCtrlPublicHost node.example.com
& $env:TEMP\nowheredash-install.ps1 status nowhere
```

Windows 上安装 Dash：

```powershell
& $env:TEMP\nowheredash-install.ps1 install dash -DashPort 4000
& $env:TEMP\nowheredash-install.ps1 update all
```

安装成功后会输出与 npsh 相同类型的连接信息：

```text
API URL: https://node.example.com:10101/api/v2
API KEY: 0123456789abcdef0123456789abcdef
URI: np://master?url=...&key=...
```

`API URL:` 与 `API KEY:` 文本可以直接粘贴到 NowhereDash 的节点导入框。系统已安装 `qrencode` 时，脚本还会输出 URI 二维码。

非交互安装示例：

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --non-interactive \
  --openctrl-public-host 203.0.113.10
```

需要 GitHub 下载代理时，使用前缀地址；末尾的 `/` 可省略。该设置会同时用于 GitHub API、Release 资产、校验文件和后续 `nowhere-ctl update`：

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --non-interactive \
  --github-proxy https://ghproxy.com/
```

例如上面的配置会把 `https://github.com/...` 请求转换成 `https://ghproxy.com/https://github.com/...`。

NowhereDash 的 `/endpoints` 页面可通过“复制安装脚本”下拉菜单进入“引导添加”。页面会签发一个 10 分钟有效、成功注册后立即失效的一次性令牌，并生成带以下参数的完整命令：

```text
--register-url https://dash.example.com/api/endpoints/register
--register-token <一次性令牌>
```

OpenCtrl 启动后，安装器会把 API URL 和 API Key 自动提交到 NowhereDash，无需再次手动添加节点。注册令牌不会写入 `/etc/openctrl` 或其他持久化配置。

指定版本时可带或不带 `v` 前缀：

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --openctrl-version v2.0.1 \
  --nowhere-version v1.7.0
```

## 安装 NowhereDash

```bash
sudo /tmp/nowheredash-install.sh install dash --port 4000
```

安装后访问输出的地址，通过 Web 初始化向导配置数据库和管理员。

使用已有 PEM 证书启用 Dash 内置 HTTPS：

```bash
sudo /tmp/nowheredash-install.sh install dash \
  --port 443 \
  --https \
  --cert /path/fullchain.pem \
  --key /path/privkey.pem
```

`--cert`/`--key` 也会自动启用 HTTPS；现有安装可用 `--http` 切回 HTTP。生产环境也可以让 Dash 监听普通 HTTP 端口，再由 Nginx/Caddy 提供 TLS。

版本选项：

```bash
sudo /tmp/nowheredash-install.sh install dash --stable
sudo /tmp/nowheredash-install.sh install dash --beta
sudo /tmp/nowheredash-install.sh install dash --version v4.0.6
```

## 同机安装全部组件

```bash
sudo /tmp/nowheredash-install.sh install all \
  --dash-port 4000 \
  --openctrl-port 10101 \
  --openctrl-public-host node.example.com
```

Dash 与 OpenCtrl 的端口必须不同。脚本会先完成两套配置和端口校验，再开始下载和安装。

## 管理和更新

```bash
# OpenCtrl + Nowhere
sudo nowhere-ctl status
sudo nowhere-ctl logs
sudo nowhere-ctl info
sudo nowhere-ctl tui
sudo nowhere-ctl update

# NowhereDash
sudo nowheredash-ctl status
sudo nowheredash-ctl logs
sudo nowheredash-ctl reset-password
sudo nowheredash-ctl update
sudo nowheredash-ctl switch-version
```

也可以直接调用安装器：

```bash
sudo /tmp/nowheredash-install.sh update nowhere --non-interactive
sudo /tmp/nowheredash-install.sh update dash --non-interactive
sudo /tmp/nowheredash-install.sh status all
```

更新会保留配置和数据。新服务未通过启动/API 检查时，脚本会尝试恢复上一版二进制。

Windows 使用相同的 `update`、`status`、`uninstall` 动作，例如：

```powershell
& $env:TEMP\nowheredash-install.ps1 update nowhere
& $env:TEMP\nowheredash-install.ps1 uninstall nowhere -Yes
```

## 卸载

卸载 Nowhere 节点时会同时彻底删除 OpenCtrl、节点状态、API Key、配置和安装器生成的证书：

```bash
sudo /tmp/nowheredash-install.sh uninstall nowhere --yes
```

卸载 Dash 时默认保留 Dash 数据：

```bash
sudo /tmp/nowheredash-install.sh uninstall dash --yes
sudo /tmp/nowheredash-install.sh uninstall all --yes
```

显式添加 `--purge` 可同时删除 Dash 数据：

```bash
sudo /tmp/nowheredash-install.sh uninstall all --purge --yes
```

## 文件位置

| 内容 | 路径 |
| --- | --- |
| Dash 程序和数据 | `/opt/nowheredash` |
| Dash 服务 | `/etc/systemd/system/nowheredash.service` |
| OpenCtrl / Nowhere 二进制及状态 | `/opt/openctrl` |
| OpenCtrl 配置和连接信息 | `/etc/openctrl` |
| Nowhere Portal 证书目录 | `/etc/nowhere/certs` |
| OpenCtrl 服务 | `/etc/systemd/system/openctrl.service` |

API Key 和 OpenCtrl 状态包含敏感信息；不要把 `/etc/openctrl` 或 `/opt/openctrl/bin/gob` 暴露给非受信用户。

macOS 节点沿用 `/opt/openctrl`、`/etc/openctrl` 与 `/etc/nowhere/certs`，服务定义位于 `/Library/LaunchDaemons/com.nodepass.openctrl.plist`。Windows 使用 `%ProgramData%\OpenCtrl`、`%ProgramData%\OpenCtrlConfig` 与 `%ProgramData%\NowhereDash`，敏感目录的 ACL 仅允许 `SYSTEM` 和管理员访问。

## 手动运行 NowhereDash

从 GitHub Releases 解压 `nowheredash` 后，可以直接运行：

```bash
mkdir -p /opt/nowheredash/bin
cd /opt/nowheredash
./bin/nowheredash --port 4000
```

常用参数包括 `--port`、`--cert`、`--key`、`--resetpwd`、`--disable-login`、`--sse-debug-log` 和 `--disable-sse-log`。对应的主要环境变量包括 `PORT`、`TLS_CERT`、`TLS_KEY`、`DISABLE_LOGIN`、`SSE_DEBUG_LOG` 与 `DISABLE_SSE_LOG`。
