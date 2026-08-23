# Unified Binary Deployment

`scripts/install.sh` deploys NowhereDash or a Nowhere node managed by OpenCtrl.

| Target | Installed components | Service |
| --- | --- | --- |
| `dash` | NowhereDash | `nowheredash.service` |
| `nowhere` / `openctrl` | OpenCtrl and the Nowhere runtime | `openctrl.service` |
| `all` | All components above | Both services |

`nowhere` and `openctrl` are aliases for the same target. This target does not create one fixed Portal service. OpenCtrl uses Nowhere as its managed runtime, and NowhereDash creates and manages Portal or Vector instances through the OpenCtrl API.

## Requirements

- Linux with systemd and root access.
- x86_64 or arm64 for the node; the installer selects the glibc or musl release automatically.
- See NowhereDash Releases for supported Dash architectures.
- Outbound access to GitHub Releases.

The installer installs missing base utilities and creates a separate non-login user for each service.

## Download the Installer

Download and inspect the script before running it with an explicit target:

```bash
curl -fsSL https://raw.githubusercontent.com/NodePassProject/NowhereDash/main/scripts/install.sh \
  -o /tmp/nowheredash-install.sh
chmod +x /tmp/nowheredash-install.sh
sudo /tmp/nowheredash-install.sh --help
```

Running it without arguments opens the interactive menu:

```bash
sudo /tmp/nowheredash-install.sh
```

## Install Nowhere and OpenCtrl

These commands are equivalent and install both OpenCtrl and Nowhere:

```bash
sudo /tmp/nowheredash-install.sh install nowhere
sudo /tmp/nowheredash-install.sh install openctrl
sudo /tmp/nowheredash-install.sh nowhere
```

The defaults are:

- Listen on `0.0.0.0:10101`
- API path `/api/v2`
- OpenCtrl TLS mode `1` with an ephemeral self-signed certificate
- Latest stable OpenCtrl and Nowhere releases

Specify the public hostname or IP that Dash should use to reach the node:

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --openctrl-public-host node.example.com \
  --openctrl-port 10101
```

Use existing PEM files for trusted TLS:

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --openctrl-public-host node.example.com \
  --openctrl-port 443 \
  --openctrl-tls 2 \
  --openctrl-cert /etc/letsencrypt/live/node.example.com/fullchain.pem \
  --openctrl-key /etc/letsencrypt/live/node.example.com/privkey.pem
```

`--openctrl-tls 0` uses plaintext HTTP and should only be used on a trusted network or behind a TLS reverse proxy. The installer does not issue certificates or stop an existing web server.

On success, it prints npsh-style connection details:

```text
API URL: https://node.example.com:10101/api/v2
API KEY: 0123456789abcdef0123456789abcdef
URI: np://master?url=...&key=...
```

The `API URL:` and `API KEY:` lines can be pasted directly into the NowhereDash endpoint import dialog. If `qrencode` is already installed, the installer also prints a QR code for the URI.

Non-interactive example:

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --non-interactive \
  --openctrl-public-host 203.0.113.10
```

Use a prefix URL when GitHub downloads require a proxy. The trailing `/` is optional. The setting applies to the GitHub API, release assets, checksum files, and future `nowhere-ctl update` runs:

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --non-interactive \
  --github-proxy https://ghproxy.com/
```

For example, this converts a `https://github.com/...` request to `https://ghproxy.com/https://github.com/...`.

On the NowhereDash `/endpoints` page, open **Guided Add** from the **Copy Install Script** dropdown. The page issues a one-time token that expires after 10 minutes or immediately after successful registration, then generates a complete command with these arguments:

```text
--register-url https://dash.example.com/api/endpoints/register
--register-token <one-time-token>
```

After OpenCtrl starts, the installer submits its API URL and API key to NowhereDash automatically. The registration token is never written to `/etc/openctrl` or another persistent configuration file.

Exact versions may include or omit the `v` prefix:

```bash
sudo /tmp/nowheredash-install.sh install nowhere \
  --openctrl-version v2.0.1 \
  --nowhere-version v1.7.0
```

## Install NowhereDash

```bash
sudo /tmp/nowheredash-install.sh install dash --port 4000
```

Open the displayed URL and use the web setup wizard to configure the database and administrator.

Enable Dash's built-in HTTPS with existing PEM files:

```bash
sudo /tmp/nowheredash-install.sh install dash \
  --port 443 \
  --https \
  --cert /path/fullchain.pem \
  --key /path/privkey.pem
```

Providing both `--cert` and `--key` also enables HTTPS. Use `--http` to switch an existing installation back to HTTP. In production, Dash may instead listen on HTTP behind an Nginx or Caddy TLS reverse proxy.

Release selection:

```bash
sudo /tmp/nowheredash-install.sh install dash --stable
sudo /tmp/nowheredash-install.sh install dash --beta
sudo /tmp/nowheredash-install.sh install dash --version v4.0.6
```

## Install Everything on One Host

```bash
sudo /tmp/nowheredash-install.sh install all \
  --dash-port 4000 \
  --openctrl-port 10101 \
  --openctrl-public-host node.example.com
```

Dash and OpenCtrl must use different ports. The installer completes both configuration flows and checks the ports before downloading either component.

## Manage and Update

```bash
# OpenCtrl and Nowhere
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

The installer can also be called directly:

```bash
sudo /tmp/nowheredash-install.sh update nowhere --non-interactive
sudo /tmp/nowheredash-install.sh update dash --non-interactive
sudo /tmp/nowheredash-install.sh status all
```

Updates preserve configuration and data. If the new service fails its startup or API checks, the installer attempts to restore the previous binary.

## Uninstall

Uninstalling a Nowhere node completely removes OpenCtrl, node state, the API key, configuration, and installer-generated certificates:

```bash
sudo /tmp/nowheredash-install.sh uninstall nowhere --yes
```

Uninstalling Dash preserves its data by default:

```bash
sudo /tmp/nowheredash-install.sh uninstall dash --yes
sudo /tmp/nowheredash-install.sh uninstall all --yes
```

Add `--purge` to remove Dash data as well:

```bash
sudo /tmp/nowheredash-install.sh uninstall all --purge --yes
```

## File Locations

| Content | Path |
| --- | --- |
| Dash program and data | `/opt/nowheredash` |
| Dash service | `/etc/systemd/system/nowheredash.service` |
| OpenCtrl/Nowhere binaries and state | `/opt/openctrl` |
| OpenCtrl configuration and connection details | `/etc/openctrl` |
| Nowhere Portal certificate directory | `/etc/nowhere/certs` |
| OpenCtrl service | `/etc/systemd/system/openctrl.service` |

The API key and OpenCtrl state contain sensitive information. Do not expose `/etc/openctrl` or `/opt/openctrl/bin/gob` to untrusted users.

## Run NowhereDash Manually

After extracting `nowheredash` from GitHub Releases, it can be started directly:

```bash
mkdir -p /opt/nowheredash/bin
cd /opt/nowheredash
./bin/nowheredash --port 4000
```

Common flags include `--port`, `--cert`, `--key`, `--resetpwd`, `--disable-login`, `--sse-debug-log`, and `--disable-sse-log`. The main matching environment variables are `PORT`, `TLS_CERT`, `TLS_KEY`, `DISABLE_LOGIN`, `SSE_DEBUG_LOG`, and `DISABLE_SSE_LOG`.
