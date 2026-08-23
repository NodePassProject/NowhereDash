#!/usr/bin/env bash

# Unified installer for Nowhere nodes (OpenCtrl + Nowhere) and NowhereDash.

set -Eeuo pipefail
umask 027

if [[ "${DEBUG:-0}" == "1" ]]; then
    set -x
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

DASH_REPO="${NOWHEREDASH_REPO:-NodePassProject/NowhereDash}"
OPENCTRL_REPO="${OPENCTRL_REPO:-NodePassProject/OpenCtrl}"
NOWHERE_REPO="${NOWHERE_REPO:-NodePassProject/Nowhere}"
INSTALLER_URL="${NOWHEREDASH_INSTALLER_URL:-https://raw.githubusercontent.com/NodePassProject/NowhereDash/main/scripts/install.sh}"

DASH_INSTALL_DIR="${NOWHEREDASH_INSTALL_DIR:-/opt/nowheredash}"
DASH_BINARY="$DASH_INSTALL_DIR/bin/nowheredash"
DASH_CONFIG="$DASH_INSTALL_DIR/config.env"
DASH_SERVICE="nowheredash"
DASH_UNIT="/etc/systemd/system/$DASH_SERVICE.service"
DASH_USER="nowhere"
DASH_CTL="/usr/local/bin/nowheredash-ctl"
DASH_USER_MARKER="$DASH_INSTALL_DIR/.installer-created-user"

NODE_INSTALL_DIR="${NOWHERE_NODE_INSTALL_DIR:-/opt/openctrl}"
NODE_BIN_DIR="$NODE_INSTALL_DIR/bin"
NODE_STATE_DIR="$NODE_BIN_DIR/gob"
OPENCTRL_BINARY="$NODE_BIN_DIR/openctrl"
NOWHERE_BINARY="$NODE_BIN_DIR/nowhere"
NODE_CONFIG_DIR="${OPENCTRL_CONFIG_DIR:-/etc/openctrl}"
NODE_ENV_FILE="$NODE_CONFIG_DIR/openctrl.env"
NODE_INSTALL_CONFIG="$NODE_CONFIG_DIR/install.conf"
NODE_API_KEY_FILE="$NODE_CONFIG_DIR/api-key"
NODE_ENDPOINT_FILE="$NODE_CONFIG_DIR/endpoint.env"
NODE_CERT_DIR="$NODE_CONFIG_DIR/certs"
NOWHERE_CERT_DIR="${NOWHERE_CERT_DIR:-/etc/nowhere/certs}"
NODE_SERVICE="openctrl"
NODE_UNIT="/etc/systemd/system/$NODE_SERVICE.service"
NODE_USER="openctrl"
NODE_CTL="/usr/local/bin/nowhere-ctl"
NODE_USER_MARKER="$NODE_INSTALL_DIR/.installer-created-user"

ACTION="menu"
TARGET=""
ASSUME_YES=0
PURGE=0
NO_FIREWALL=0
GITHUB_PROXY="${NOWHEREDASH_GITHUB_PROXY:-}"
GITHUB_PROXY_SET=0
REGISTER_URL=""
REGISTER_TOKEN=""
NODE_REGISTERED=0
WORK_DIR=""
OS_ID=""
MACHINE=""
GO_ARCH=""
DASH_ARCH=""
RUST_ARCH=""
LIBC_KIND="gnu"

DASH_PORT="4000"
DASH_CHANNEL="stable"
DASH_REQUESTED_VERSION=""
DASH_ENABLE_HTTPS="false"
DASH_CERT_PATH=""
DASH_KEY_PATH=""
DASH_PORT_SET=0
DASH_CHANNEL_SET=0
DASH_VERSION_SET=0
DASH_HTTPS_SET=0
DASH_CERT_SET=0
DASH_KEY_SET=0

NODE_LISTEN_HOST="0.0.0.0"
NODE_PUBLIC_HOST=""
NODE_PORT="10101"
NODE_PREFIX="api"
NODE_TLS="1"
NODE_CERT_PATH=""
NODE_KEY_PATH=""
OPENCTRL_REQUESTED_VERSION=""
NOWHERE_REQUESTED_VERSION=""
NODE_LISTEN_SET=0
NODE_PUBLIC_SET=0
NODE_PORT_SET=0
NODE_PREFIX_SET=0
NODE_TLS_SET=0
NODE_CERT_SET=0
NODE_KEY_SET=0
OPENCTRL_VERSION_SET=0
NOWHERE_VERSION_SET=0

RELEASE_VERSION=""
RELEASE_ASSET=""
RELEASE_BINARY=""

if [[ -n "$GITHUB_PROXY" ]]; then
    GITHUB_PROXY_SET=1
fi

log_info() {
    printf '%b[INFO]%b %s\n' "$BLUE" "$NC" "$*"
}

log_success() {
    printf '%b[SUCCESS]%b %s\n' "$GREEN" "$NC" "$*"
}

log_warning() {
    printf '%b[WARNING]%b %s\n' "$YELLOW" "$NC" "$*" >&2
}

log_error() {
    printf '%b[ERROR]%b %s\n' "$RED" "$NC" "$*" >&2
}

die() {
    log_error "$*"
    exit 1
}

cleanup() {
    if [[ -n "$WORK_DIR" && -d "$WORK_DIR" ]]; then
        rm -rf -- "$WORK_DIR"
    fi
}

on_error() {
    local code=$?
    local line="$1"
    trap - ERR
    log_error "操作在第 $line 行失败，请查看上方日志。"
    exit "$code"
}

trap cleanup EXIT
trap 'on_error "$LINENO"' ERR

show_help() {
    cat <<EOF
Nowhere / NowhereDash 统一安装脚本

用法:
  $0
  $0 install [dash|nowhere|openctrl|all] [选项]
  $0 update [dash|nowhere|openctrl|all] [选项]
  $0 uninstall [dash|nowhere|openctrl|all] [--purge] [--yes]
  $0 status [dash|nowhere|openctrl|all]
  $0 switch [dash]

兼容旧用法:
  $0 install --port 4000
  $0 uninstall

通用选项:
  --yes, --non-interactive     使用默认值，不显示配置向导
  --purge                      卸载时同时删除配置、状态和数据
  --no-firewall                不自动添加防火墙规则
  --github-proxy URL           GitHub 下载代理前缀，例如 https://ghfast.top

NowhereDash 选项:
  --port, --dash-port PORT     Dash 监听端口，默认 4000
  --beta                       安装最新预发布版本
  --stable                     安装最新正式版本
  --version VERSION            指定 Dash Release
  --https                      使用已有 PEM 证书启用 HTTPS
  --http                       禁用 Dash 内置 HTTPS
  --cert PATH                  Dash 证书路径
  --key PATH                   Dash 私钥路径

Nowhere 节点选项（自动安装 OpenCtrl）:
  --openctrl-listen HOST       OpenCtrl 监听地址，默认 0.0.0.0
  --openctrl-public-host HOST  提供给 Dash 连接的域名或 IP
  --openctrl-port PORT         OpenCtrl 端口，默认 10101
  --openctrl-prefix PATH       API 前缀，默认 api
  --openctrl-tls 0|1|2        0=HTTP，1=自签 TLS，2=已有 PEM
  --openctrl-cert PATH         OpenCtrl TLS 证书路径
  --openctrl-key PATH          OpenCtrl TLS 私钥路径
  --openctrl-version VERSION   指定 OpenCtrl Release
  --nowhere-version VERSION    指定 Nowhere Release
  --register-url URL           NowhereDash 自动注册接口
  --register-token TOKEN       NowhereDash 签发的一次性注册令牌

示例:
  $0 install nowhere
  $0 install openctrl
  $0 install dash --port 4000
  $0 install all
  $0 install nowhere --openctrl-public-host node.example.com \\
      --openctrl-tls 2 --openctrl-cert /path/fullchain.pem \\
      --openctrl-key /path/privkey.pem
  $0 install nowhere --yes \\
      --register-url https://dash.example.com/api/endpoints/register \\
      --register-token TOKEN
EOF
}

require_value() {
    [[ "$2" -ge 2 ]] || die "选项 $1 缺少参数。"
}

parse_args() {
    if [[ $# -eq 0 ]]; then
        ACTION="menu"
        return
    fi

    case "$1" in
        install|update|uninstall|status|switch)
            ACTION="$1"
            shift
            ;;
        dash)
            ACTION="install"
            TARGET="dash"
            shift
            ;;
        nowhere|node|openctrl)
            ACTION="install"
            TARGET="nowhere"
            shift
            ;;
        all)
            ACTION="install"
            TARGET="all"
            shift
            ;;
        --help|-h|help)
            ACTION="help"
            shift
            ;;
        *)
            ACTION="install"
            ;;
    esac

    while [[ $# -gt 0 ]]; do
        case "$1" in
            dash)
                TARGET="dash"
                shift
                ;;
            nowhere|node|openctrl)
                TARGET="nowhere"
                shift
                ;;
            all)
                TARGET="all"
                shift
                ;;
            --yes|--non-interactive|-y)
                ASSUME_YES=1
                shift
                ;;
            --purge)
                PURGE=1
                shift
                ;;
            --no-firewall)
                NO_FIREWALL=1
                shift
                ;;
            --github-proxy)
                require_value "$1" "$#"
                GITHUB_PROXY="$2"
                GITHUB_PROXY_SET=1
                shift 2
                ;;
            --port|--dash-port)
                require_value "$1" "$#"
                DASH_PORT="$2"
                DASH_PORT_SET=1
                shift 2
                ;;
            --beta)
                DASH_CHANNEL="beta"
                DASH_CHANNEL_SET=1
                shift
                ;;
            --stable)
                DASH_CHANNEL="stable"
                DASH_CHANNEL_SET=1
                shift
                ;;
            --version|--dash-version)
                require_value "$1" "$#"
                DASH_REQUESTED_VERSION="$2"
                DASH_VERSION_SET=1
                shift 2
                ;;
            --https)
                DASH_ENABLE_HTTPS="true"
                DASH_HTTPS_SET=1
                shift
                ;;
            --http)
                DASH_ENABLE_HTTPS="false"
                DASH_HTTPS_SET=1
                shift
                ;;
            --cert|--dash-cert)
                require_value "$1" "$#"
                DASH_CERT_PATH="$2"
                DASH_CERT_SET=1
                shift 2
                ;;
            --key|--dash-key)
                require_value "$1" "$#"
                DASH_KEY_PATH="$2"
                DASH_KEY_SET=1
                shift 2
                ;;
            --openctrl-listen)
                require_value "$1" "$#"
                NODE_LISTEN_HOST="$2"
                NODE_LISTEN_SET=1
                shift 2
                ;;
            --openctrl-public-host)
                require_value "$1" "$#"
                NODE_PUBLIC_HOST="$2"
                NODE_PUBLIC_SET=1
                shift 2
                ;;
            --openctrl-port)
                require_value "$1" "$#"
                NODE_PORT="$2"
                NODE_PORT_SET=1
                shift 2
                ;;
            --openctrl-prefix)
                require_value "$1" "$#"
                NODE_PREFIX="$2"
                NODE_PREFIX_SET=1
                shift 2
                ;;
            --openctrl-tls)
                require_value "$1" "$#"
                NODE_TLS="$2"
                NODE_TLS_SET=1
                shift 2
                ;;
            --openctrl-cert)
                require_value "$1" "$#"
                NODE_CERT_PATH="$2"
                NODE_CERT_SET=1
                shift 2
                ;;
            --openctrl-key)
                require_value "$1" "$#"
                NODE_KEY_PATH="$2"
                NODE_KEY_SET=1
                shift 2
                ;;
            --openctrl-version)
                require_value "$1" "$#"
                OPENCTRL_REQUESTED_VERSION="$2"
                OPENCTRL_VERSION_SET=1
                shift 2
                ;;
            --nowhere-version)
                require_value "$1" "$#"
                NOWHERE_REQUESTED_VERSION="$2"
                NOWHERE_VERSION_SET=1
                shift 2
                ;;
            --register-url)
                require_value "$1" "$#"
                REGISTER_URL="$2"
                shift 2
                ;;
            --register-token)
                require_value "$1" "$#"
                REGISTER_TOKEN="$2"
                shift 2
                ;;
            --help|-h)
                ACTION="help"
                shift
                ;;
            *)
                die "未知参数: $1"
                ;;
        esac
    done

    if [[ -z "$TARGET" ]]; then
        TARGET="dash"
    fi
    if [[ "$DASH_CERT_SET" -eq 1 || "$DASH_KEY_SET" -eq 1 ]]; then
        if [[ "$DASH_HTTPS_SET" -eq 0 ]]; then
            DASH_ENABLE_HTTPS="true"
            DASH_HTTPS_SET=1
        elif [[ "$DASH_ENABLE_HTTPS" != "true" ]]; then
            die "--http 不能与 Dash 证书参数同时使用。"
        fi
    fi
    if [[ -n "$REGISTER_URL" || -n "$REGISTER_TOKEN" ]]; then
        [[ -n "$REGISTER_URL" && -n "$REGISTER_TOKEN" ]] ||
            die "--register-url 与 --register-token 必须同时使用。"
        [[ "$TARGET" == "nowhere" || "$TARGET" == "all" ]] ||
            die "自动注册参数仅适用于 Nowhere 节点安装。"
        [[ "$ACTION" == "install" ]] ||
            die "自动注册参数仅适用于首次安装。"
    fi
}

read_answer() {
    local prompt="$1"
    local answer=""
    if [[ -r /dev/tty ]]; then
        IFS= read -r -p "$prompt" answer </dev/tty || return 1
    else
        IFS= read -r -p "$prompt" answer || return 1
    fi
    printf '%s' "$answer"
}

prompt_value() {
    local prompt="$1"
    local default="$2"
    local answer
    if [[ -n "$default" ]]; then
        answer=$(read_answer "$prompt [$default]: ") || die "无法读取交互输入，请使用 --non-interactive。"
        printf '%s' "${answer:-$default}"
    else
        answer=$(read_answer "$prompt: ") || die "无法读取交互输入，请使用 --non-interactive。"
        printf '%s' "$answer"
    fi
}

confirm() {
    local prompt="$1"
    local answer
    if [[ "$ASSUME_YES" -eq 1 ]]; then
        return 0
    fi
    answer=$(read_answer "$prompt [Y/n]: ") || return 1
    [[ -z "$answer" || "$answer" =~ ^[Yy]([Ee][Ss])?$ ]]
}

interactive_menu() {
    local choice
    cat <<'EOF'

==========================================
 Nowhere / NowhereDash 安装管理
==========================================
 1) 安装或更新 Nowhere 节点（OpenCtrl + Nowhere）
 2) 安装或更新 NowhereDash
 3) 同时安装节点和 Dash
 4) 查看服务状态
 5) 卸载组件
 0) 退出
EOF
    choice=$(read_answer "请选择: ") || exit 0
    case "$choice" in
        1) ACTION="install"; TARGET="nowhere" ;;
        2) ACTION="install"; TARGET="dash" ;;
        3) ACTION="install"; TARGET="all" ;;
        4) ACTION="status"; TARGET="all" ;;
        5)
            ACTION="uninstall"
            choice=$(read_answer "卸载 [1=Nowhere 节点, 2=Dash, 3=全部]: ") || exit 0
            case "$choice" in
                1) TARGET="nowhere" ;;
                2) TARGET="dash" ;;
                3) TARGET="all" ;;
                *) die "无效选择。" ;;
            esac
            ;;
        0) exit 0 ;;
        *) die "无效选择。" ;;
    esac
}

validate_port() {
    [[ "$1" =~ ^[0-9]+$ ]] && (( 10#$1 >= 1 && 10#$1 <= 65535 ))
}

validate_host() {
    local host="${1#[}"
    host="${host%]}"
    [[ -n "$host" && "$host" =~ ^[A-Za-z0-9._:-]+$ ]]
}

validate_prefix() {
    [[ "$1" =~ ^[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*$ ]]
}

validate_version() {
    [[ "$1" =~ ^v?[0-9]+(\.[0-9]+){1,3}([.-][A-Za-z0-9._-]+)?$ ]]
}

validate_http_url() {
    [[ "$1" =~ ^https?://[^[:space:]]+$ ]] &&
        [[ "$1" != *$'\n'* && "$1" != *$'\r'* ]]
}

validate_github_proxy() {
    [[ -z "$1" ]] && return 0
    validate_http_url "$1" || return 1
    [[ "$1" != *'"'* && "$1" != *"'"* && "$1" != *'`'* && "$1" != *'\'* ]]
}

github_url() {
    local url="$1"
    if [[ -z "$GITHUB_PROXY" ]]; then
        printf '%s' "$url"
    else
        printf '%s/%s' "${GITHUB_PROXY%/}" "$url"
    fi
}

format_url_host() {
    local host="$1"
    if [[ "$host" == \[*\] ]]; then
        printf '%s' "$host"
    elif [[ "$host" == *:* ]]; then
        printf '[%s]' "$host"
    else
        printf '%s' "$host"
    fi
}

urlencode() {
    local LC_ALL=C
    local value="$1"
    local output=""
    local char encoded
    local i
    for ((i = 0; i < ${#value}; i++)); do
        char="${value:i:1}"
        case "$char" in
            [A-Za-z0-9._~-])
                output+="$char"
                ;;
            *)
                printf -v encoded '%%%02X' "'$char"
                output+="$encoded"
                ;;
        esac
    done
    printf '%s' "$output"
}

base64_string() {
    printf '%s' "$1" | base64 | tr -d '\r\n'
}

build_import_uri() {
    local api_url="$1"
    local api_key="$2"
    printf 'np://master?url=%s&key=%s' "$(base64_string "$api_url")" "$(base64_string "$api_key")"
}

json_escape() {
    local value="$1"
    value=${value//\\/\\\\}
    value=${value//\"/\\\"}
    value=${value//$'\n'/\\n}
    value=${value//$'\r'/\\r}
    value=${value//$'\t'/\\t}
    printf '%s' "$value"
}

config_get() {
    local file="$1"
    local key="$2"
    [[ -f "$file" ]] || return 0
    awk -v wanted="$key" '
        index($0, wanted "=") == 1 {
            sub(/^[^=]*=/, "")
            sub(/\r$/, "")
            print
            exit
        }
    ' "$file"
}

detect_public_host() {
    local detected=""
    detected=$(curl -4fsS --connect-timeout 3 --max-time 5 https://api.ipify.org 2>/dev/null || true)
    if ! validate_host "$detected"; then
        detected=$(hostname -I 2>/dev/null | awk '{print $1}' || true)
    fi
    if validate_host "$detected"; then
        printf '%s' "$detected"
    else
        printf '127.0.0.1'
    fi
}

require_root() {
    [[ "$(id -u)" -eq 0 ]] || die "此操作需要 root 权限，请使用 sudo。"
}

detect_system() {
    [[ "$(uname -s)" == "Linux" ]] || die "仅支持 Linux。"
    OS_ID=$(awk -F= '$1 == "ID" {gsub(/"/, "", $2); print $2; exit}' /etc/os-release 2>/dev/null || true)
    OS_ID="${OS_ID:-unknown}"
    MACHINE=$(uname -m)

    case "$MACHINE" in
        x86_64|amd64)
            GO_ARCH="amd64"
            DASH_ARCH="x86_64"
            RUST_ARCH="x86_64"
            ;;
        aarch64|arm64)
            GO_ARCH="arm64"
            DASH_ARCH="arm64"
            RUST_ARCH="aarch64"
            ;;
        armv7l)
            GO_ARCH="arm"
            DASH_ARCH="armv7hf"
            RUST_ARCH=""
            ;;
        armv6l)
            GO_ARCH="arm"
            DASH_ARCH="armv6hf"
            RUST_ARCH=""
            ;;
        *)
            die "不支持的架构: $MACHINE"
            ;;
    esac

    LIBC_KIND="gnu"
    if command -v ldd >/dev/null 2>&1 && ldd --version 2>&1 | grep -qi musl; then
        LIBC_KIND="musl"
    elif compgen -G '/lib/ld-musl-*.so.1' >/dev/null 2>&1; then
        LIBC_KIND="musl"
    fi

    command -v systemctl >/dev/null 2>&1 || die "未找到 systemctl，本脚本需要 systemd。"
    [[ -d /run/systemd/system ]] || die "systemd 当前未运行。"
    log_info "系统: $OS_ID, 架构: $MACHINE, libc: $LIBC_KIND"
}

install_dependencies() {
    local missing=()
    local command
    for command in curl tar awk sed grep find install base64 sha256sum tr; do
        command -v "$command" >/dev/null 2>&1 || missing+=("$command")
    done
    if [[ ${#missing[@]} -eq 0 ]]; then
        return
    fi

    log_info "安装基础依赖: ${missing[*]}"
    if command -v apt-get >/dev/null 2>&1; then
        apt-get update
        DEBIAN_FRONTEND=noninteractive apt-get install -y curl ca-certificates tar coreutils findutils gawk
    elif command -v dnf >/dev/null 2>&1; then
        dnf install -y curl ca-certificates tar coreutils findutils gawk
    elif command -v yum >/dev/null 2>&1; then
        yum install -y curl ca-certificates tar coreutils findutils gawk
    elif command -v pacman >/dev/null 2>&1; then
        pacman -Sy --noconfirm curl ca-certificates tar coreutils findutils gawk
    elif command -v apk >/dev/null 2>&1; then
        apk add --no-cache bash curl ca-certificates tar coreutils findutils gawk shadow
    else
        die "无法自动安装依赖，请先安装 curl、tar、coreutils、findutils 和 awk。"
    fi
}

make_work_dir() {
    if [[ -z "$WORK_DIR" ]]; then
        WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/nowheredash-install.XXXXXX")
    fi
}

github_get() {
    local url="$1"
    local output="$2"
    local args=(--fail --silent --show-error --location --retry 3 --connect-timeout 10)
    if [[ -n "${GITHUB_TOKEN:-}" ]]; then
        args+=(-H "Authorization: Bearer $GITHUB_TOKEN")
    fi
    args+=(-H "Accept: application/vnd.github+json")
    curl "${args[@]}" --output "$output" "$(github_url "$url")"
}

download_file() {
    local url="$1"
    local output="$2"
    curl --fail --silent --show-error --location --retry 3 --connect-timeout 10 \
        --output "$output" "$(github_url "$url")"
}

json_first_string() {
    local key="$1"
    local file="$2"
    sed -nE "s/.*\"$key\":[[:space:]]*\"([^\"]+)\".*/\1/p" "$file" | head -n 1
}

latest_prerelease_tag() {
    awk '
        /"tag_name"[[:space:]]*:/ {
            line = $0
            sub(/^.*"tag_name":[[:space:]]*"/, "", line)
            sub(/".*$/, "", line)
            tag = line
        }
        /"prerelease"[[:space:]]*:[[:space:]]*true/ && tag != "" {
            print tag
            exit
        }
    ' "$1"
}

fetch_release_json() {
    local repo="$1"
    local requested="$2"
    local channel="$3"
    local output="$4"
    local endpoint tag list_file

    if [[ -n "$requested" ]]; then
        validate_version "$requested" || die "无效版本号: $requested"
        [[ "$requested" == v* ]] || requested="v$requested"
        endpoint="https://api.github.com/repos/$repo/releases/tags/$requested"
    elif [[ "$channel" == "beta" ]]; then
        list_file="$WORK_DIR/releases-$RANDOM.json"
        if ! github_get "https://api.github.com/repos/$repo/releases?per_page=30" "$list_file"; then
            die "无法获取 $repo 的 Release 列表。"
        fi
        tag=$(latest_prerelease_tag "$list_file")
        [[ -n "$tag" ]] || die "$repo 当前没有可用的预发布版本。"
        endpoint="https://api.github.com/repos/$repo/releases/tags/$tag"
    else
        endpoint="https://api.github.com/repos/$repo/releases/latest"
    fi

    if ! github_get "$endpoint" "$output"; then
        die "无法获取 $repo Release，请检查仓库、版本和网络。"
    fi
    RELEASE_VERSION=$(json_first_string tag_name "$output")
    [[ -n "$RELEASE_VERSION" ]] || die "无法解析 $repo Release 版本。"
}

release_asset_url() {
    local file="$1"
    local pattern="$2"
    local url name lower
    while IFS= read -r url; do
        name="${url##*/}"
        lower=$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')
        if [[ "$lower" =~ $pattern ]]; then
            printf '%s' "$url"
            return 0
        fi
    done < <(sed -nE 's/.*"browser_download_url":[[:space:]]*"([^"]+)".*/\1/p' "$file")
    return 1
}

release_asset_digest() {
    local file="$1"
    local asset="$2"
    awk -v wanted="$asset" '
        /"name":[[:space:]]*"/ {
            line = $0
            sub(/^.*"name":[[:space:]]*"/, "", line)
            sub(/".*$/, "", line)
            active = (line == wanted)
        }
        active && /"digest":[[:space:]]*"sha256:/ {
            line = $0
            sub(/^.*"digest":[[:space:]]*"sha256:/, "", line)
            sub(/".*$/, "", line)
            print line
            exit
        }
    ' "$file"
}

release_checksum_url() {
    local file="$1"
    local url name lower
    while IFS= read -r url; do
        name="${url##*/}"
        lower=$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')
        if [[ "$lower" =~ (checksums|checksum|sha256).*\.txt$ ]]; then
            printf '%s' "$url"
            return 0
        fi
    done < <(sed -nE 's/.*"browser_download_url":[[:space:]]*"([^"]+)".*/\1/p' "$file")
    return 1
}

sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "$1" | awk '{print $NF}'
    else
        return 1
    fi
}

verify_sha256() {
    local file="$1"
    local expected="$2"
    local actual
    actual=$(sha256_file "$file") || {
        log_warning "系统没有 SHA-256 工具，跳过完整性校验。"
        return 0
    }
    [[ "$actual" == "$expected" ]] || die "SHA-256 校验失败: $(basename "$file")"
    log_success "SHA-256 校验通过: $(basename "$file")"
}

download_release_binary() {
    local label="$1"
    local repo="$2"
    local requested="$3"
    local channel="$4"
    local asset_pattern="$5"
    local binary_name="$6"
    local output_binary="$7"
    local release_json archive listing extract_dir url asset digest checksum_url checksum_file expected binary

    make_work_dir
    release_json="$WORK_DIR/$label-release.json"
    archive="$WORK_DIR/$label.tar.gz"
    listing="$WORK_DIR/$label-contents.txt"
    extract_dir="$WORK_DIR/$label-extract"

    fetch_release_json "$repo" "$requested" "$channel" "$release_json"
    url=$(release_asset_url "$release_json" "$asset_pattern") ||
        die "在 $repo $RELEASE_VERSION 中找不到当前架构资产。"
    asset="${url##*/}"

    log_info "下载 $repo $RELEASE_VERSION: $asset"
    download_file "$url" "$archive"
    [[ -s "$archive" ]] || die "下载文件为空: $asset"

    digest=$(release_asset_digest "$release_json" "$asset")
    if [[ -n "$digest" ]]; then
        verify_sha256 "$archive" "$digest"
    else
        checksum_url=$(release_checksum_url "$release_json" || true)
        if [[ -n "$checksum_url" ]]; then
            checksum_file="$WORK_DIR/$label-checksums.txt"
            download_file "$checksum_url" "$checksum_file"
            expected=$(awk -v wanted="$asset" '
                {
                    name = $NF
                    sub(/^\*/, "", name)
                    if (name == wanted) {
                        print $1
                        exit
                    }
                }
            ' "$checksum_file")
            if [[ -n "$expected" ]]; then
                verify_sha256 "$archive" "$expected"
            else
                log_warning "$asset 未出现在 Release checksum 文件中。"
            fi
        else
            log_warning "$repo $RELEASE_VERSION 没有提供可用的 SHA-256 digest。"
        fi
    fi

    if ! tar -tzf "$archive" >"$listing"; then
        die "无法读取 Release 压缩包: $asset"
    fi
    if grep -Eq '(^/|(^|/)\.\.(/|$))' "$listing"; then
        die "Release 压缩包包含不安全路径: $asset"
    fi
    mkdir -p "$extract_dir"
    tar -xzf "$archive" -C "$extract_dir"
    binary=$(find "$extract_dir" -type f -name "$binary_name" -print -quit)
    [[ -n "$binary" ]] || die "压缩包中未找到 $binary_name。"
    install -m 0755 "$binary" "$output_binary"

    RELEASE_ASSET="$asset"
    RELEASE_BINARY="$output_binary"
}

system_group_exists() {
    local group="$1"
    if command -v getent >/dev/null 2>&1; then
        getent group "$group" >/dev/null 2>&1
    else
        awk -F: -v wanted="$group" '$1 == wanted { found = 1 } END { exit !found }' /etc/group
    fi
}

ensure_system_group() {
    local group="$1"
    system_group_exists "$group" && return 0
    if command -v groupadd >/dev/null 2>&1; then
        groupadd --system "$group"
    elif command -v addgroup >/dev/null 2>&1; then
        addgroup -S "$group"
    else
        die "无法创建系统组 $group。"
    fi
}

ensure_system_user() {
    local user="$1"
    local home="$2"
    local shell="/bin/false"
    [[ -x /usr/sbin/nologin ]] && shell="/usr/sbin/nologin"
    ensure_system_group "$user"
    if id "$user" >/dev/null 2>&1; then
        return
    fi
    if command -v useradd >/dev/null 2>&1; then
        useradd --system --gid "$user" --home-dir "$home" --no-create-home --shell "$shell" "$user"
    elif command -v adduser >/dev/null 2>&1; then
        adduser -S -H -G "$user" -h "$home" -s "$shell" "$user"
    else
        die "无法创建系统用户 $user。"
    fi
}

install_binary_atomic() {
    local source="$1"
    local destination="$2"
    local owner="$3"
    local group="$4"
    mkdir -p "$(dirname "$destination")"
    if [[ -f "$destination" ]]; then
        cp -p "$destination" "$destination.previous"
    fi
    install -m 0755 -o "$owner" -g "$group" "$source" "$destination.new"
    mv -f "$destination.new" "$destination"
}

rollback_binary() {
    local destination="$1"
    if [[ -f "$destination.previous" ]]; then
        log_warning "恢复上一版本: $destination"
        mv -f "$destination.previous" "$destination"
    fi
}

wait_service_active() {
    local service="$1"
    local attempts="${2:-20}"
    local i
    for ((i = 0; i < attempts; i++)); do
        if systemctl is-active --quiet "$service"; then
            return 0
        fi
        sleep 1
    done
    return 1
}

open_firewall_port() {
    local port="$1"
    [[ "$NO_FIREWALL" -eq 0 ]] || return 0
    if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q 'Status: active'; then
        ufw allow "$port/tcp" >/dev/null
        log_success "已添加 UFW TCP/$port 规则。"
    elif command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld; then
        firewall-cmd --permanent --add-port="$port/tcp" >/dev/null
        firewall-cmd --reload >/dev/null
        log_success "已添加 firewalld TCP/$port 规则。"
    else
        log_warning "未自动修改防火墙；请确认 TCP/$port 可访问。"
    fi
}

remove_firewall_port() {
    local port="$1"
    validate_port "$port" || return 0
    if command -v ufw >/dev/null 2>&1; then
        ufw delete allow "$port/tcp" >/dev/null 2>&1 || true
    fi
    if command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld; then
        firewall-cmd --permanent --remove-port="$port/tcp" >/dev/null 2>&1 || true
        firewall-cmd --reload >/dev/null 2>&1 || true
    fi
}

load_dash_config() {
    local value https_value=""
    [[ -f "$DASH_CONFIG" ]] || return 0
    if [[ "$DASH_PORT_SET" -eq 0 ]]; then
        value=$(config_get "$DASH_CONFIG" PORT)
        [[ -n "$value" ]] && DASH_PORT="$value"
    fi
    if [[ "$DASH_CHANNEL_SET" -eq 0 ]]; then
        value=$(config_get "$DASH_CONFIG" VERSION_TYPE)
        [[ "$value" == "stable" || "$value" == "beta" ]] && DASH_CHANNEL="$value"
    fi
    if [[ "$DASH_HTTPS_SET" -eq 0 ]]; then
        https_value=$(config_get "$DASH_CONFIG" ENABLE_HTTPS)
        case "$https_value" in
            true) DASH_ENABLE_HTTPS="true" ;;
            false) DASH_ENABLE_HTTPS="false" ;;
        esac
    fi
    if [[ "$DASH_CERT_SET" -eq 0 ]]; then
        value=$(config_get "$DASH_CONFIG" TLS_CERT)
        [[ -z "$value" ]] && value=$(config_get "$DASH_CONFIG" CERT_PATH)
        [[ -n "$value" ]] && DASH_CERT_PATH="$value"
    fi
    if [[ "$DASH_KEY_SET" -eq 0 ]]; then
        value=$(config_get "$DASH_CONFIG" TLS_KEY)
        [[ -z "$value" ]] && value=$(config_get "$DASH_CONFIG" KEY_PATH)
        [[ -n "$value" ]] && DASH_KEY_PATH="$value"
    fi
    if [[ "$GITHUB_PROXY_SET" -eq 0 ]]; then
        value=$(config_get "$DASH_CONFIG" GITHUB_PROXY)
        [[ -n "$value" ]] && GITHUB_PROXY="$value"
    fi
    if [[ "$DASH_HTTPS_SET" -eq 0 && -z "$https_value" && -n "$DASH_CERT_PATH" && -n "$DASH_KEY_PATH" ]]; then
        DASH_ENABLE_HTTPS="true"
    fi
    return 0
}

configure_dash_interactive() {
    local choice https_default="N"
    choice=$(prompt_value "Dash 版本 [1=正式版, 2=Beta]" "$([[ "$DASH_CHANNEL" == "beta" ]] && printf 2 || printf 1)")
    [[ "$choice" == "2" ]] && DASH_CHANNEL="beta" || DASH_CHANNEL="stable"
    DASH_PORT=$(prompt_value "Dash 监听端口" "$DASH_PORT")

    [[ "$DASH_ENABLE_HTTPS" == "true" ]] && https_default="Y"
    choice=$(prompt_value "Dash 是否直接启用 HTTPS [y/N]" "$https_default")
    if [[ "$choice" =~ ^[Yy] ]]; then
        DASH_ENABLE_HTTPS="true"
        DASH_CERT_PATH=$(prompt_value "Dash 证书路径" "$DASH_CERT_PATH")
        DASH_KEY_PATH=$(prompt_value "Dash 私钥路径" "$DASH_KEY_PATH")
    else
        DASH_ENABLE_HTTPS="false"
    fi

    printf '\nDash 配置:\n'
    printf '  Channel: %s\n  Port: %s\n  HTTPS: %s\n' "$DASH_CHANNEL" "$DASH_PORT" "$DASH_ENABLE_HTTPS"
    confirm "应用以上 Dash 配置吗？" || exit 0
}

validate_dash_config() {
    validate_port "$DASH_PORT" || die "无效 Dash 端口: $DASH_PORT"
    [[ "$DASH_CHANNEL" == "stable" || "$DASH_CHANNEL" == "beta" ]] || die "无效 Dash channel。"
    validate_github_proxy "$GITHUB_PROXY" || die "无效 GitHub 代理地址: $GITHUB_PROXY"
    if [[ -n "$DASH_REQUESTED_VERSION" ]]; then
        validate_version "$DASH_REQUESTED_VERSION" || die "无效 Dash 版本: $DASH_REQUESTED_VERSION"
    fi
    if [[ "$DASH_ENABLE_HTTPS" == "true" ]]; then
        [[ "$DASH_CERT_PATH" != *$'\n'* && "$DASH_CERT_PATH" != *$'\r'* ]] || die "Dash 证书路径包含非法字符。"
        [[ "$DASH_KEY_PATH" != *$'\n'* && "$DASH_KEY_PATH" != *$'\r'* ]] || die "Dash 私钥路径包含非法字符。"
        [[ -f "$DASH_CERT_PATH" ]] || die "Dash 证书不存在: $DASH_CERT_PATH"
        [[ -f "$DASH_KEY_PATH" ]] || die "Dash 私钥不存在: $DASH_KEY_PATH"
    fi
}

setup_dash_directories() {
    local user_existed=0
    id "$DASH_USER" >/dev/null 2>&1 && user_existed=1
    ensure_system_user "$DASH_USER" "$DASH_INSTALL_DIR"
    mkdir -p "$DASH_INSTALL_DIR/bin" "$DASH_INSTALL_DIR/db" "$DASH_INSTALL_DIR/logs" \
        "$DASH_INSTALL_DIR/backups" "$DASH_INSTALL_DIR/certs"
    chown root:root "$DASH_INSTALL_DIR" "$DASH_INSTALL_DIR/bin"
    chmod 0755 "$DASH_INSTALL_DIR" "$DASH_INSTALL_DIR/bin"
    chown -R "$DASH_USER:$DASH_USER" "$DASH_INSTALL_DIR/db" "$DASH_INSTALL_DIR/logs" \
        "$DASH_INSTALL_DIR/backups"
    chmod 0750 "$DASH_INSTALL_DIR/db" "$DASH_INSTALL_DIR/logs" "$DASH_INSTALL_DIR/backups"
    chown root:"$DASH_USER" "$DASH_INSTALL_DIR/certs"
    chmod 0750 "$DASH_INSTALL_DIR/certs"
    [[ ! -L "$DASH_INSTALL_DIR/.env" ]] || die "拒绝使用符号链接作为 Dash .env。"
    if [[ ! -e "$DASH_INSTALL_DIR/.env" ]]; then
        install -m 0600 -o "$DASH_USER" -g "$DASH_USER" /dev/null "$DASH_INSTALL_DIR/.env"
    else
        chown "$DASH_USER:$DASH_USER" "$DASH_INSTALL_DIR/.env"
        chmod 0600 "$DASH_INSTALL_DIR/.env"
    fi
    if [[ "$user_existed" -eq 0 ]]; then
        install -m 0600 -o root -g root /dev/null "$DASH_USER_MARKER"
    fi
}

install_dash_certificates() {
    if [[ "$DASH_ENABLE_HTTPS" != "true" ]]; then
        return
    fi
    if [[ "$DASH_CERT_PATH" != "$DASH_INSTALL_DIR/certs/server.crt" ]]; then
        install -m 0644 -o root -g "$DASH_USER" "$DASH_CERT_PATH" "$DASH_INSTALL_DIR/certs/server.crt"
    fi
    if [[ "$DASH_KEY_PATH" != "$DASH_INSTALL_DIR/certs/server.key" ]]; then
        install -m 0640 -o root -g "$DASH_USER" "$DASH_KEY_PATH" "$DASH_INSTALL_DIR/certs/server.key"
    fi
    DASH_CERT_PATH="$DASH_INSTALL_DIR/certs/server.crt"
    DASH_KEY_PATH="$DASH_INSTALL_DIR/certs/server.key"
}

write_dash_config() {
    local version="$1"
    local temp="$WORK_DIR/dash-config.env"
    cat >"$temp" <<EOF
# Managed by NowhereDash installer
VERSION=$version
VERSION_TYPE=$DASH_CHANNEL
PORT=$DASH_PORT
DB_PATH=$DASH_INSTALL_DIR/db/database.db
ENABLE_HTTPS=$DASH_ENABLE_HTTPS
TLS_CERT=$([[ "$DASH_ENABLE_HTTPS" == "true" ]] && printf '%s' "$DASH_CERT_PATH")
TLS_KEY=$([[ "$DASH_ENABLE_HTTPS" == "true" ]] && printf '%s' "$DASH_KEY_PATH")
GITHUB_PROXY=$GITHUB_PROXY
EOF
    install -m 0640 -o root -g "$DASH_USER" "$temp" "$DASH_CONFIG"
}

write_dash_service() {
    cat >"$DASH_UNIT" <<EOF
[Unit]
Description=NowhereDash
Documentation=https://github.com/$DASH_REPO
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$DASH_USER
Group=$DASH_USER
WorkingDirectory=$DASH_INSTALL_DIR
Environment=HOME=$DASH_INSTALL_DIR
EnvironmentFile=$DASH_CONFIG
ExecStart="$DASH_BINARY"
Restart=on-failure
RestartSec=3
TimeoutStopSec=30
LimitNOFILE=65536
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
UMask=0027
ReadWritePaths=$DASH_INSTALL_DIR/.env $DASH_INSTALL_DIR/db $DASH_INSTALL_DIR/logs $DASH_INSTALL_DIR/backups
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF
    chmod 0644 "$DASH_UNIT"
}

write_dash_ctl() {
    local proxy_quoted
    printf -v proxy_quoted '%q' "$GITHUB_PROXY"
    cat >"$DASH_CTL" <<EOF
#!/usr/bin/env bash
set -euo pipefail
SERVICE="$DASH_SERVICE"
BINARY="$DASH_BINARY"
INSTALL_DIR="$DASH_INSTALL_DIR"
CONFIG="$DASH_CONFIG"
INSTALLER_URL="$INSTALLER_URL"
GITHUB_PROXY=$proxy_quoted

need_root() {
    [[ "\$(id -u)" -eq 0 ]] || { echo "请使用 sudo 运行。" >&2; exit 1; }
}

github_url() {
    local url="\$1"
    if [[ -z "\$GITHUB_PROXY" ]]; then
        printf '%s' "\$url"
    else
        printf '%s/%s' "\${GITHUB_PROXY%/}" "\$url"
    fi
}

run_installer() {
    need_root
    local temp
    temp=\$(mktemp /tmp/nowheredash-installer.XXXXXX)
    trap 'rm -f "\$temp"' EXIT
    curl -fsSL -o "\$temp" "\$(github_url "\$INSTALLER_URL")"
    NOWHEREDASH_GITHUB_PROXY="\$GITHUB_PROXY" bash "\$temp" "\$@"
}

case "\${1:-}" in
    start|stop|restart)
        need_root
        systemctl "\$1" "\$SERVICE"
        ;;
    status)
        systemctl status "\$SERVICE" --no-pager
        ;;
    logs)
        journalctl -u "\$SERVICE" -f -n 100
        ;;
    reset-password)
        need_root
        systemctl stop "\$SERVICE"
        trap 'systemctl start "\$SERVICE"' EXIT
        if command -v runuser >/dev/null 2>&1; then
            (cd "\$INSTALL_DIR" && runuser -u "$DASH_USER" -- "\$BINARY" --resetpwd)
        else
            (cd "\$INSTALL_DIR" && su -s /bin/sh "$DASH_USER" -c "\"\$BINARY\" --resetpwd")
        fi
        systemctl start "\$SERVICE"
        trap - EXIT
        ;;
    config)
        need_root
        cat "\$CONFIG"
        ;;
    update)
        shift
        run_installer update dash --non-interactive "\$@"
        ;;
    switch-version)
        shift
        run_installer switch dash "\$@"
        ;;
    uninstall)
        shift
        run_installer uninstall dash "\$@"
        ;;
    *)
        echo "用法: nowheredash-ctl {start|stop|restart|status|logs|reset-password|config|update|switch-version|uninstall}"
        exit 1
        ;;
esac
EOF
    chmod 0755 "$DASH_CTL"
}

probe_dash() {
    local scheme="http"
    local i
    [[ "$DASH_ENABLE_HTTPS" == "true" ]] && scheme="https"
    for ((i = 0; i < 20; i++)); do
        if curl --noproxy '*' -kfsS --max-time 3 "$scheme://127.0.0.1:$DASH_PORT/" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    return 1
}

show_dash_result() {
    local scheme="http"
    local host
    [[ "$DASH_ENABLE_HTTPS" == "true" ]] && scheme="https"
    host=$(detect_public_host)
    host=$(format_url_host "$host")
    cat <<EOF

==========================================
NowhereDash 安装完成
访问地址: $scheme://$host:$DASH_PORT
初始化: 首次访问后按页面向导创建数据库和管理员
管理命令: nowheredash-ctl status|logs|update
数据目录: $DASH_INSTALL_DIR
==========================================
EOF
}

prepare_dash_config() {
    load_dash_config
    if [[ "$ACTION" == "install" && "$ASSUME_YES" -eq 0 ]]; then
        configure_dash_interactive
    fi
    validate_dash_config
}

install_dash() {
    local config_prepared="${1:-0}"
    local staged="$WORK_DIR/nowheredash"
    local resolved
    local had_old=0

    if [[ "$config_prepared" -eq 0 ]]; then
        prepare_dash_config
    fi
    validate_dash_config

    download_release_binary dash "$DASH_REPO" "$DASH_REQUESTED_VERSION" "$DASH_CHANNEL" \
        "^nowheredash_.*linux_$DASH_ARCH\\.tar\\.gz$" nowheredash "$staged"
    resolved="$RELEASE_VERSION"

    setup_dash_directories
    install_dash_certificates
    write_dash_config "$resolved"
    write_dash_service
    write_dash_ctl

    [[ -f "$DASH_BINARY" ]] && had_old=1
    if systemctl is-active --quiet "$DASH_SERVICE"; then
        systemctl stop "$DASH_SERVICE"
    fi
    install_binary_atomic "$staged" "$DASH_BINARY" root root
    ln -sfn "$DASH_BINARY" /usr/local/bin/nowheredash

    systemctl daemon-reload
    systemctl enable "$DASH_SERVICE" >/dev/null
    if ! systemctl restart "$DASH_SERVICE" || ! wait_service_active "$DASH_SERVICE" || ! probe_dash; then
        journalctl -u "$DASH_SERVICE" -n 80 --no-pager >&2 || true
        if [[ "$had_old" -eq 1 ]]; then
            rollback_binary "$DASH_BINARY"
            systemctl restart "$DASH_SERVICE" || true
        fi
        die "NowhereDash 启动检查失败。"
    fi

    open_firewall_port "$DASH_PORT"
    log_success "NowhereDash $resolved 已启动。"
    show_dash_result
}

load_node_config() {
    local value
    [[ -f "$NODE_INSTALL_CONFIG" ]] || return 0
    if [[ "$NODE_LISTEN_SET" -eq 0 ]]; then
        value=$(config_get "$NODE_INSTALL_CONFIG" LISTEN_HOST)
        [[ -n "$value" ]] && NODE_LISTEN_HOST="$value"
    fi
    if [[ "$NODE_PUBLIC_SET" -eq 0 ]]; then
        value=$(config_get "$NODE_INSTALL_CONFIG" PUBLIC_HOST)
        [[ -n "$value" ]] && NODE_PUBLIC_HOST="$value"
    fi
    if [[ "$NODE_PORT_SET" -eq 0 ]]; then
        value=$(config_get "$NODE_INSTALL_CONFIG" PORT)
        [[ -n "$value" ]] && NODE_PORT="$value"
    fi
    if [[ "$NODE_PREFIX_SET" -eq 0 ]]; then
        value=$(config_get "$NODE_INSTALL_CONFIG" PREFIX)
        [[ -n "$value" ]] && NODE_PREFIX="$value"
    fi
    if [[ "$NODE_TLS_SET" -eq 0 ]]; then
        value=$(config_get "$NODE_INSTALL_CONFIG" TLS)
        [[ -n "$value" ]] && NODE_TLS="$value"
    fi
    if [[ "$NODE_CERT_SET" -eq 0 ]]; then
        value=$(config_get "$NODE_INSTALL_CONFIG" CERT_PATH)
        [[ -n "$value" ]] && NODE_CERT_PATH="$value"
    fi
    if [[ "$NODE_KEY_SET" -eq 0 ]]; then
        value=$(config_get "$NODE_INSTALL_CONFIG" KEY_PATH)
        [[ -n "$value" ]] && NODE_KEY_PATH="$value"
    fi
    if [[ "$GITHUB_PROXY_SET" -eq 0 ]]; then
        value=$(config_get "$NODE_INSTALL_CONFIG" GITHUB_PROXY)
        [[ -n "$value" ]] && GITHUB_PROXY="$value"
    fi
    return 0
}

configure_node_interactive() {
    local detected
    detected=$(detect_public_host)
    [[ -n "$NODE_PUBLIC_HOST" ]] || NODE_PUBLIC_HOST="$detected"
    NODE_PUBLIC_HOST=$(prompt_value "Dash 连接此节点时使用的域名或 IP" "$NODE_PUBLIC_HOST")
    NODE_LISTEN_HOST=$(prompt_value "OpenCtrl 监听地址" "$NODE_LISTEN_HOST")
    NODE_PORT=$(prompt_value "OpenCtrl 监听端口" "$NODE_PORT")
    NODE_PREFIX=$(prompt_value "OpenCtrl API 前缀" "$NODE_PREFIX")
    NODE_TLS=$(prompt_value "OpenCtrl TLS [0=HTTP, 1=自签, 2=已有 PEM]" "$NODE_TLS")
    if [[ "$NODE_TLS" == "2" ]]; then
        NODE_CERT_PATH=$(prompt_value "OpenCtrl 证书路径" "$NODE_CERT_PATH")
        NODE_KEY_PATH=$(prompt_value "OpenCtrl 私钥路径" "$NODE_KEY_PATH")
    fi

    printf '\nNowhere 节点配置:\n'
    printf '  Public host: %s\n  Listen: %s:%s\n  API prefix: /%s/v2\n  TLS: %s\n' \
        "$NODE_PUBLIC_HOST" "$NODE_LISTEN_HOST" "$NODE_PORT" "$NODE_PREFIX" "$NODE_TLS"
    confirm "安装 OpenCtrl 与 Nowhere 吗？" || exit 0
}

validate_node_config() {
    [[ -n "$RUST_ARCH" ]] || die "Nowhere 官方 Release 仅支持 x86_64 和 aarch64。"
    validate_host "$NODE_LISTEN_HOST" || die "无效 OpenCtrl 监听地址: $NODE_LISTEN_HOST"
    if [[ -z "$NODE_PUBLIC_HOST" ]]; then
        NODE_PUBLIC_HOST=$(detect_public_host)
    fi
    validate_host "$NODE_PUBLIC_HOST" || die "无效 OpenCtrl 公网地址: $NODE_PUBLIC_HOST"
    validate_port "$NODE_PORT" || die "无效 OpenCtrl 端口: $NODE_PORT"
    validate_prefix "$NODE_PREFIX" || die "无效 OpenCtrl API 前缀: $NODE_PREFIX"
    validate_github_proxy "$GITHUB_PROXY" || die "无效 GitHub 代理地址: $GITHUB_PROXY"
    [[ "$NODE_TLS" =~ ^[012]$ ]] || die "OpenCtrl TLS 必须是 0、1 或 2。"
    if [[ -n "$REGISTER_URL" ]]; then
        validate_http_url "$REGISTER_URL" || die "无效 NowhereDash 注册地址: $REGISTER_URL"
        [[ "$REGISTER_URL" != *$'\n'* && "$REGISTER_URL" != *$'\r'* ]] ||
            die "NowhereDash 注册地址包含非法字符。"
        [[ "$REGISTER_TOKEN" =~ ^[A-Za-z0-9_-]{32,256}$ ]] ||
            die "无效 NowhereDash 注册令牌。"
    fi
    if [[ -n "$OPENCTRL_REQUESTED_VERSION" ]]; then
        validate_version "$OPENCTRL_REQUESTED_VERSION" || die "无效 OpenCtrl 版本。"
    fi
    if [[ -n "$NOWHERE_REQUESTED_VERSION" ]]; then
        validate_version "$NOWHERE_REQUESTED_VERSION" || die "无效 Nowhere 版本。"
    fi
    if [[ "$NODE_TLS" == "2" ]]; then
        [[ "$NODE_CERT_PATH" != *$'\n'* && "$NODE_CERT_PATH" != *$'\r'* ]] || die "OpenCtrl 证书路径包含非法字符。"
        [[ "$NODE_KEY_PATH" != *$'\n'* && "$NODE_KEY_PATH" != *$'\r'* ]] || die "OpenCtrl 私钥路径包含非法字符。"
        [[ -f "$NODE_CERT_PATH" ]] || die "OpenCtrl 证书不存在: $NODE_CERT_PATH"
        [[ -f "$NODE_KEY_PATH" ]] || die "OpenCtrl 私钥不存在: $NODE_KEY_PATH"
    fi
}

setup_node_directories() {
    local user_existed=0
    id "$NODE_USER" >/dev/null 2>&1 && user_existed=1
    ensure_system_user "$NODE_USER" "$NODE_INSTALL_DIR"
    mkdir -p "$NODE_BIN_DIR" "$NODE_STATE_DIR" "$NODE_CONFIG_DIR" "$NODE_CERT_DIR" "$NOWHERE_CERT_DIR"
    chown root:root "$NODE_INSTALL_DIR" "$NODE_BIN_DIR"
    chmod 0755 "$NODE_INSTALL_DIR" "$NODE_BIN_DIR"
    chown -R "$NODE_USER:$NODE_USER" "$NODE_STATE_DIR"
    chmod 0700 "$NODE_STATE_DIR"
    chown root:"$NODE_USER" "$NODE_CONFIG_DIR" "$NODE_CERT_DIR" "$NOWHERE_CERT_DIR"
    chmod 0750 "$NODE_CONFIG_DIR" "$NODE_CERT_DIR" "$NOWHERE_CERT_DIR"
    if [[ "$user_existed" -eq 0 ]]; then
        install -m 0600 -o root -g root /dev/null "$NODE_USER_MARKER"
    fi
}

install_node_certificates() {
    if [[ "$NODE_TLS" != "2" ]]; then
        return
    fi
    if [[ "$NODE_CERT_PATH" != "$NODE_CERT_DIR/fullchain.pem" ]]; then
        install -m 0644 -o root -g "$NODE_USER" "$NODE_CERT_PATH" "$NODE_CERT_DIR/fullchain.pem"
    fi
    if [[ "$NODE_KEY_PATH" != "$NODE_CERT_DIR/privkey.pem" ]]; then
        install -m 0640 -o root -g "$NODE_USER" "$NODE_KEY_PATH" "$NODE_CERT_DIR/privkey.pem"
    fi
    NODE_CERT_PATH="$NODE_CERT_DIR/fullchain.pem"
    NODE_KEY_PATH="$NODE_CERT_DIR/privkey.pem"
}

write_node_config() {
    local openctrl_version="$1"
    local nowhere_version="$2"
    local listen_host master_url runtime_param cert_param=""
    local env_temp="$WORK_DIR/openctrl.env"
    local install_temp="$WORK_DIR/openctrl-install.conf"

    listen_host=$(format_url_host "$NODE_LISTEN_HOST")
    runtime_param=$(urlencode "$NOWHERE_BINARY")
    master_url="master://$listen_host:$NODE_PORT/$NODE_PREFIX?tls=$NODE_TLS&bin=$runtime_param"
    if [[ "$NODE_TLS" == "2" ]]; then
        cert_param="&crt=$(urlencode "$NODE_CERT_PATH")&key=$(urlencode "$NODE_KEY_PATH")"
        master_url+="$cert_param"
    fi

    printf 'OPENCTRL_URL=%s\n' "$master_url" >"$env_temp"
    install -m 0640 -o root -g "$NODE_USER" "$env_temp" "$NODE_ENV_FILE"

    cat >"$install_temp" <<EOF
# Managed by NowhereDash installer
LISTEN_HOST=$NODE_LISTEN_HOST
PUBLIC_HOST=$NODE_PUBLIC_HOST
PORT=$NODE_PORT
PREFIX=$NODE_PREFIX
TLS=$NODE_TLS
CERT_PATH=$([[ "$NODE_TLS" == "2" ]] && printf '%s' "$NODE_CERT_PATH")
KEY_PATH=$([[ "$NODE_TLS" == "2" ]] && printf '%s' "$NODE_KEY_PATH")
OPENCTRL_VERSION=$openctrl_version
NOWHERE_VERSION=$nowhere_version
GITHUB_PROXY=$GITHUB_PROXY
EOF
    install -m 0600 -o root -g root "$install_temp" "$NODE_INSTALL_CONFIG"
}

write_node_service() {
    cat >"$NODE_UNIT" <<EOF
[Unit]
Description=OpenCtrl master for Nowhere
Documentation=https://github.com/$OPENCTRL_REPO https://github.com/$NOWHERE_REPO
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$NODE_USER
Group=$NODE_USER
WorkingDirectory=$NODE_INSTALL_DIR
Environment=HOME=$NODE_INSTALL_DIR
EnvironmentFile=$NODE_ENV_FILE
ExecStart="$OPENCTRL_BINARY" "\${OPENCTRL_URL}"
Restart=on-failure
RestartSec=3
TimeoutStopSec=15
LimitNOFILE=1048576
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=read-only
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictRealtime=true
LockPersonality=true
ReadWritePaths=$NODE_STATE_DIR
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF
    chmod 0644 "$NODE_UNIT"
}

write_node_ctl() {
    local proxy_quoted
    printf -v proxy_quoted '%q' "$GITHUB_PROXY"
    cat >"$NODE_CTL" <<EOF
#!/usr/bin/env bash
set -euo pipefail
SERVICE="$NODE_SERVICE"
NOWHERE="$NOWHERE_BINARY"
ENDPOINT_FILE="$NODE_ENDPOINT_FILE"
INSTALLER_URL="$INSTALLER_URL"
GITHUB_PROXY=$proxy_quoted

need_root() {
    [[ "\$(id -u)" -eq 0 ]] || { echo "请使用 sudo 运行。" >&2; exit 1; }
}

github_url() {
    local url="\$1"
    if [[ -z "\$GITHUB_PROXY" ]]; then
        printf '%s' "\$url"
    else
        printf '%s/%s' "\${GITHUB_PROXY%/}" "\$url"
    fi
}

run_installer() {
    need_root
    local temp
    temp=\$(mktemp /tmp/nowhere-installer.XXXXXX)
    trap 'rm -f "\$temp"' EXIT
    curl -fsSL -o "\$temp" "\$(github_url "\$INSTALLER_URL")"
    NOWHEREDASH_GITHUB_PROXY="\$GITHUB_PROXY" bash "\$temp" "\$@"
}

show_info() {
    need_root
    local api_url api_key uri
    [[ -r "\$ENDPOINT_FILE" ]] || {
        echo "连接信息不存在，请重新运行节点安装或更新。" >&2
        exit 1
    }
    api_url=\$(sed -n 's/^OPENCTRL_API_URL=//p' "\$ENDPOINT_FILE" | head -n 1)
    api_key=\$(sed -n 's/^OPENCTRL_API_KEY=//p' "\$ENDPOINT_FILE" | head -n 1)
    uri=\$(sed -n 's/^NOWHEREDASH_IMPORT_URI=//p' "\$ENDPOINT_FILE" | head -n 1)
    [[ -n "\$api_url" && -n "\$api_key" && -n "\$uri" ]] || {
        echo "连接信息不存在，请重新运行节点安装或更新。" >&2
        exit 1
    }
    printf 'API URL: %s\nAPI KEY: %s\nURI: %s\n' "\$api_url" "\$api_key" "\$uri"
    if command -v qrencode >/dev/null 2>&1; then
        qrencode -t ANSIUTF8 "\$uri" || true
    fi
}

case "\${1:-}" in
    start|stop|restart)
        need_root
        systemctl "\$1" "\$SERVICE"
        ;;
    status)
        systemctl status "\$SERVICE" --no-pager
        ;;
    logs)
        journalctl -u "\$SERVICE" -f -n 100
        ;;
    tui)
        need_root
        if command -v runuser >/dev/null 2>&1; then
            runuser -u "$NODE_USER" -- "\$NOWHERE" tui
        else
            su -s /bin/sh "$NODE_USER" -c "\"\$NOWHERE\" tui"
        fi
        ;;
    info)
        show_info
        ;;
    update)
        shift
        run_installer update nowhere --non-interactive "\$@"
        ;;
    uninstall)
        shift
        run_installer uninstall nowhere "\$@"
        ;;
    *)
        echo "用法: nowhere-ctl {start|stop|restart|status|logs|tui|info|update|uninstall}"
        exit 1
        ;;
esac
EOF
    chmod 0755 "$NODE_CTL"
}

extract_node_api_key() {
    local key=""
    local i
    for ((i = 0; i < 20; i++)); do
        key=$(journalctl -u "$NODE_SERVICE" -n 200 --no-pager 2>/dev/null |
            sed -nE 's/.*Master\.run: API key (created|loaded): ([0-9a-f]{32}).*/\2/p' |
            tail -n 1)
        if [[ "$key" =~ ^[0-9a-f]{32}$ ]]; then
            printf '%s' "$key"
            return 0
        fi
        sleep 1
    done
    return 1
}

node_scheme() {
    [[ "$NODE_TLS" == "0" ]] && printf http || printf https
}

node_probe_host() {
    local host="${NODE_LISTEN_HOST#[}"
    host="${host%]}"
    case "$host" in
        ""|0.0.0.0) printf '127.0.0.1' ;;
        ::) printf '[::1]' ;;
        *) format_url_host "$host" ;;
    esac
}

probe_node() {
    local key="$1"
    local scheme host url i
    scheme=$(node_scheme)
    host=$(node_probe_host)
    url="$scheme://$host:$NODE_PORT/$NODE_PREFIX/v2/info"
    for ((i = 0; i < 20; i++)); do
        if curl --noproxy '*' -kfsS --max-time 3 \
            -H "X-API-Key: $key" "$url" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    return 1
}

write_node_endpoint() {
    local key="$1"
    local scheme public_host base api_url import_uri temp key_temp
    scheme=$(node_scheme)
    public_host=$(format_url_host "$NODE_PUBLIC_HOST")
    base="$scheme://$public_host:$NODE_PORT"
    api_url="$base/$NODE_PREFIX/v2"
    import_uri=$(build_import_uri "$api_url" "$key")
    temp="$WORK_DIR/openctrl-endpoint.env"
    cat >"$temp" <<EOF
OPENCTRL_URL=$base
OPENCTRL_API_PATH=/$NODE_PREFIX/v2
OPENCTRL_API_URL=$api_url
OPENCTRL_API_KEY=$key
NOWHEREDASH_IMPORT_URI=$import_uri
EOF
    install -m 0600 -o root -g root "$temp" "$NODE_ENDPOINT_FILE"
    key_temp="$WORK_DIR/openctrl-api-key"
    printf '%s\n' "$key" >"$key_temp"
    install -m 0600 -o root -g root "$key_temp" "$NODE_API_KEY_FILE"
}

register_node_endpoint() {
    local key="$1"
    local scheme public_host api_url request_file response_file status error_message
    [[ "$ACTION" == "install" && -n "$REGISTER_URL" ]] || return 0

    scheme=$(node_scheme)
    public_host=$(format_url_host "$NODE_PUBLIC_HOST")
    api_url="$scheme://$public_host:$NODE_PORT/$NODE_PREFIX/v2"
    request_file="$WORK_DIR/endpoint-registration.json"
    response_file="$WORK_DIR/endpoint-registration-response.json"
    printf '{"token":"%s","apiUrl":"%s","apiKey":"%s","hostname":"%s"}' \
        "$(json_escape "$REGISTER_TOKEN")" \
        "$(json_escape "$api_url")" \
        "$(json_escape "$key")" \
        "$(json_escape "$NODE_PUBLIC_HOST")" >"$request_file"
    chmod 0600 "$request_file"

    if ! status=$(curl --silent --show-error --connect-timeout 10 --max-time 30 \
        --output "$response_file" --write-out '%{http_code}' \
        -H 'Content-Type: application/json' \
        --data-binary "@$request_file" "$REGISTER_URL"); then
        die "Nowhere 与 OpenCtrl 已安装，但无法连接 NowhereDash 完成自动注册。"
    fi
    if [[ "$status" != 2?? ]]; then
        error_message=$(json_first_string error "$response_file")
        [[ -n "$error_message" ]] || error_message="HTTP $status"
        die "Nowhere 与 OpenCtrl 已安装，但自动注册失败: $error_message"
    fi

    NODE_REGISTERED=1
    log_success "节点已自动注册到 NowhereDash。"
}

show_node_result() {
    local key scheme host base api_url import_uri registration_message
    key=$(cat "$NODE_API_KEY_FILE")
    scheme=$(node_scheme)
    host=$(format_url_host "$NODE_PUBLIC_HOST")
    base="$scheme://$host:$NODE_PORT"
    api_url="$base/$NODE_PREFIX/v2"
    import_uri=$(build_import_uri "$api_url" "$key")
    if [[ "$NODE_REGISTERED" -eq 1 ]]; then
        registration_message="已自动添加到 NowhereDash，无需手动填写。"
    else
        registration_message="请在 NowhereDash 的“添加节点”中填写 URL 和 API Key。"
    fi
    cat <<EOF

==========================================
Nowhere 节点安装完成
OpenCtrl URL: $base
API path: /$NODE_PREFIX/v2
API URL: $api_url
API KEY: $key
URI: $import_uri

$registration_message
Portal 证书目录: $NOWHERE_CERT_DIR
管理命令: nowhere-ctl status|logs|tui|info|update
==========================================
EOF
    if command -v qrencode >/dev/null 2>&1; then
        qrencode -t ANSIUTF8 "$import_uri" || log_warning "二维码输出失败，URI 文本仍可直接使用。"
    fi
}

prepare_node_config() {
    load_node_config
    if [[ "$ACTION" == "install" && "$ASSUME_YES" -eq 0 ]]; then
        configure_node_interactive
    fi
    validate_node_config
}

install_node() {
    local config_prepared="${1:-0}"
    local oc_staged="$WORK_DIR/openctrl"
    local nw_staged="$WORK_DIR/nowhere"
    local oc_version nw_version api_key
    local had_oc=0 had_nw=0

    if [[ "$config_prepared" -eq 0 ]]; then
        prepare_node_config
    fi
    validate_node_config

    download_release_binary openctrl "$OPENCTRL_REPO" "$OPENCTRL_REQUESTED_VERSION" stable \
        "^openctrl_.*_linux_$GO_ARCH\\.tar\\.gz$" openctrl "$oc_staged"
    oc_version="$RELEASE_VERSION"
    download_release_binary nowhere "$NOWHERE_REPO" "$NOWHERE_REQUESTED_VERSION" stable \
        "^nowhere-$RUST_ARCH-unknown-linux-$LIBC_KIND\\.tar\\.gz$" nowhere "$nw_staged"
    nw_version="$RELEASE_VERSION"

    setup_node_directories
    install_node_certificates
    write_node_config "$oc_version" "$nw_version"
    write_node_service
    write_node_ctl

    [[ -f "$OPENCTRL_BINARY" ]] && had_oc=1
    [[ -f "$NOWHERE_BINARY" ]] && had_nw=1
    if systemctl is-active --quiet "$NODE_SERVICE"; then
        systemctl stop "$NODE_SERVICE"
    fi
    install_binary_atomic "$oc_staged" "$OPENCTRL_BINARY" root root
    install_binary_atomic "$nw_staged" "$NOWHERE_BINARY" root root
    ln -sfn "$NOWHERE_BINARY" /usr/local/bin/nowhere
    ln -sfn "$OPENCTRL_BINARY" /usr/local/bin/openctrl

    systemctl daemon-reload
    systemctl enable "$NODE_SERVICE" >/dev/null
    if ! systemctl restart "$NODE_SERVICE" || ! wait_service_active "$NODE_SERVICE"; then
        journalctl -u "$NODE_SERVICE" -n 100 --no-pager >&2 || true
        [[ "$had_oc" -eq 1 ]] && rollback_binary "$OPENCTRL_BINARY"
        [[ "$had_nw" -eq 1 ]] && rollback_binary "$NOWHERE_BINARY"
        systemctl restart "$NODE_SERVICE" >/dev/null 2>&1 || true
        die "OpenCtrl 启动失败。"
    fi

    api_key=$(extract_node_api_key) || {
        journalctl -u "$NODE_SERVICE" -n 100 --no-pager >&2 || true
        die "无法从 OpenCtrl 日志读取 API Key。"
    }
    if ! probe_node "$api_key"; then
        journalctl -u "$NODE_SERVICE" -n 100 --no-pager >&2 || true
        [[ "$had_oc" -eq 1 ]] && rollback_binary "$OPENCTRL_BINARY"
        [[ "$had_nw" -eq 1 ]] && rollback_binary "$NOWHERE_BINARY"
        systemctl restart "$NODE_SERVICE" >/dev/null 2>&1 || true
        die "OpenCtrl API 健康检查失败。"
    fi

    write_node_endpoint "$api_key"
    open_firewall_port "$NODE_PORT"
    log_success "OpenCtrl $oc_version 与 Nowhere $nw_version 已启动。"
    register_node_endpoint "$api_key"
    show_node_result
}

safe_remove_tree() {
    local path="$1"
    local normalized

    [[ "$path" == /* ]] || die "拒绝删除非绝对路径: $path"
    [[ "$path" != *'//'* && "$path" != *$'\n'* && "$path" != *$'\r'* ]] ||
        die "拒绝删除不安全路径: $path"

    normalized="$path"
    while [[ "$normalized" != "/" && "$normalized" == */ ]]; do
        normalized="${normalized%/}"
    done
    case "/${normalized#/}/" in
        *'/../'*|*'/./'*) die "拒绝删除不安全路径: $path" ;;
    esac
    case "$normalized" in
        /|/bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/srv|/sys|/tmp|/usr|/usr/local|/var)
            die "拒绝删除系统目录: $normalized"
            ;;
    esac
    rm -rf -- "$normalized"
}

uninstall_dash() {
    local port remove_user=0
    [[ -f "$DASH_USER_MARKER" ]] && remove_user=1
    port=$(config_get "$DASH_CONFIG" PORT)
    systemctl disable --now "$DASH_SERVICE" >/dev/null 2>&1 || true
    rm -f "$DASH_UNIT" "$DASH_CTL" /usr/local/bin/nowheredash
    rm -f "$DASH_BINARY" "$DASH_BINARY.new" "$DASH_BINARY.previous"
    remove_firewall_port "$port"
    if [[ "$PURGE" -eq 1 ]]; then
        safe_remove_tree "$DASH_INSTALL_DIR"
        if [[ "$remove_user" -eq 1 ]] && id "$DASH_USER" >/dev/null 2>&1; then
            userdel "$DASH_USER" >/dev/null 2>&1 || true
        fi
        log_success "NowhereDash 程序和数据已删除。"
    else
        log_success "NowhereDash 已卸载，数据保留在 $DASH_INSTALL_DIR。"
    fi
}

uninstall_node() {
    local port remove_user=0
    [[ -f "$NODE_USER_MARKER" ]] && remove_user=1
    port=$(config_get "$NODE_INSTALL_CONFIG" PORT)
    systemctl disable --now "$NODE_SERVICE" >/dev/null 2>&1 || true
    rm -f "$NODE_UNIT" "$NODE_CTL" /usr/local/bin/nowhere /usr/local/bin/openctrl
    rm -f "$OPENCTRL_BINARY" "$OPENCTRL_BINARY.new" "$OPENCTRL_BINARY.previous" \
        "$NOWHERE_BINARY" "$NOWHERE_BINARY.new" "$NOWHERE_BINARY.previous"
    remove_firewall_port "$port"
    if [[ "$PURGE" -eq 1 ]]; then
        safe_remove_tree "$NODE_INSTALL_DIR"
        safe_remove_tree "$NODE_CONFIG_DIR"
        safe_remove_tree "$NOWHERE_CERT_DIR"
        if [[ "$remove_user" -eq 1 ]] && id "$NODE_USER" >/dev/null 2>&1; then
            userdel "$NODE_USER" >/dev/null 2>&1 || true
        fi
        log_success "OpenCtrl、Nowhere、节点状态和配置已删除。"
    else
        log_success "节点程序已卸载；OpenCtrl 状态和配置已保留。"
    fi
}

show_status() {
    local service="$1"
    if systemctl cat "$service.service" >/dev/null 2>&1; then
        systemctl status "$service" --no-pager || true
    else
        log_warning "$service 未安装。"
    fi
}

switch_dash_channel() {
    load_dash_config
    if [[ "$DASH_CHANNEL" == "beta" ]]; then
        DASH_CHANNEL="stable"
    else
        DASH_CHANNEL="beta"
    fi
    DASH_CHANNEL_SET=1
    log_info "将 NowhereDash 切换到 $DASH_CHANNEL channel。"
    confirm "继续吗？" || exit 0
    ACTION="update"
    install_dash
}

dispatch_install() {
    case "$TARGET" in
        dash)
            install_dash
            ;;
        nowhere)
            install_node
            ;;
        all)
            prepare_node_config
            prepare_dash_config
            if ((10#$NODE_PORT == 10#$DASH_PORT)); then
                die "OpenCtrl 与 Dash 不能使用同一端口 $NODE_PORT。"
            fi
            install_node 1
            install_dash 1
            ;;
        *)
            die "未知安装目标: $TARGET"
            ;;
    esac
}

dispatch_uninstall() {
    local label="$TARGET"
    [[ "$PURGE" -eq 1 ]] && label="$label（含全部数据）"
    confirm "确认卸载 $label 吗？" || exit 0
    case "$TARGET" in
        dash) uninstall_dash ;;
        nowhere) uninstall_node ;;
        all)
            uninstall_dash
            uninstall_node
            ;;
        *) die "未知卸载目标: $TARGET" ;;
    esac
    systemctl daemon-reload
}

main() {
    parse_args "$@"
    if [[ "$ACTION" == "help" ]]; then
        show_help
        return
    fi
    if [[ "$ACTION" == "menu" ]]; then
        interactive_menu
    fi

    if [[ "$ACTION" == "status" ]]; then
        case "$TARGET" in
            dash) show_status "$DASH_SERVICE" ;;
            nowhere) show_status "$NODE_SERVICE" ;;
            all)
                show_status "$NODE_SERVICE"
                show_status "$DASH_SERVICE"
                ;;
        esac
        return
    fi

    require_root
    detect_system
    install_dependencies
    make_work_dir

    case "$ACTION" in
        install|update)
            dispatch_install
            ;;
        uninstall)
            dispatch_uninstall
            ;;
        switch)
            [[ "$TARGET" == "dash" ]] || die "仅 Dash 支持 stable/beta 切换。"
            switch_dash_channel
            ;;
        *)
            die "未知操作: $ACTION"
            ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
