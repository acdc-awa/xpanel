#!/bin/bash
#
# XrayPanel 主控一键部署脚本（压缩包挂载形态）
#
# 用法（在部署目标目录内直接运行，默认解压到当前目录）：
#   curl -fsSL https://raw.githubusercontent.com/acdc-awa/xpanel/master/deploy/master/install.sh | bash
#   或下载到本地后： bash install.sh
#
# 选项：
#   --dir <path>      安装目录（默认当前目录；不存在则创建）
#   --version <v>     钉版本安装（如 v0.1.0；缺省自动取最新 release）
#   --mirror <url>    github.com 的替代基址/代理前缀（如 https://ghproxy.net/https://github.com）
#   --url <url>       完全自定义 release 包下载地址（优先于 version/mirror 推导；此时建议配 --digest）
#   --digest <sha>    release 包期望 sha256（提供则强制校验；缺省用 release 的 .sha256 自动校验）
#   --no-verify       跳过 sha256 校验（仅用于自建源/测试）
#   --fresh           全新覆盖（默认：检测到已有 configs/config.yaml 或 data/panel.db 时保持配置与数据）
#   --dry-run         只打印将执行的步骤，不实际执行
#
# 产物：master 二进制 / web/dist / configs/config.yaml / docker-compose.yml / .env /
#       Caddyfile.reference / Dockerfile.runtime，数据挂载到当前目录 ./data（SQLite + JWT + 备份）。
#
set -euo pipefail

VERSION=""
MIRROR=""
URL=""
DIGEST=""
NO_VERIFY=0
FRESH=0
DRY_RUN=0
TARGET="$(pwd)"

GH_REPO="acdc-awa/xpanel"

usage() {
  sed -n '2,21p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) TARGET="$2"; shift 2;;
    --version) VERSION="$2"; shift 2;;
    --mirror) MIRROR="$2"; shift 2;;
    --url) URL="$2"; shift 2;;
    --digest) DIGEST="$2"; shift 2;;
    --no-verify) NO_VERIFY=1; shift;;
    --fresh) FRESH=1; shift;;
    --dry-run) DRY_RUN=1; shift;;
    -h|--help) usage;;
    *) echo "未知参数: $1"; usage;;
  esac
done

run() {
  echo "==> $*"
  [[ $DRY_RUN -eq 1 ]] && return 0
  "$@"
}

for tool in curl tar sha256sum; do
  command -v "$tool" >/dev/null 2>&1 || { echo "缺少必需工具: $tool"; exit 1; }
done

# 架构探测（发布流水线提供 linux/amd64 与 linux/arm64）
ARCH=""
case "$(uname -m)" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "不支持的架构: $(uname -m)（发布仅提供 linux/amd64 与 linux/arm64）"; exit 1 ;;
esac

# 解压目标目录（默认当前目录）
mkdir -p "$TARGET"
cd "$TARGET"

# 已安装检测：默认保留现有配置与数据（升级/重复运行安全）
if [[ $FRESH -eq 0 && ( -f configs/config.yaml || -f data/panel.db ) ]]; then
  echo "==> 检测到已有安装（configs/config.yaml 或 data/panel.db 存在），默认只更新 master+web/dist，保留配置与数据"
  echo "    （如需全新覆盖请加 --fresh）"
fi

# ---------- 1. 解析 release 包下载地址 ----------
TARBALL="xpanel-master-${VERSION:-latest}-linux-${ARCH}.tar.gz"
GH_BASE="${MIRROR:-https://github.com}"
GH_BASE="${GH_BASE%/}"

if [[ -z "$URL" ]]; then
  if [[ -n "$VERSION" ]]; then
    URL="${GH_BASE}/${GH_REPO}/releases/download/${VERSION}/xpanel-master-${VERSION}-linux-${ARCH}.tar.gz"
  else
    # 自动取最新 release：先用 GitHub API 拿资产名（镜像通常只代理下载，不走 API）
    echo "==> 查询最新 release"
    API_URL="https://api.github.com/repos/${GH_REPO}/releases/latest"
    RELEASE_JSON=""
    if command -v python3 >/dev/null 2>&1; then
      RELEASE_JSON="$(curl -fsSL "$API_URL" 2>/dev/null || true)"
      ASSET="$(echo "$RELEASE_JSON" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    arch = '$ARCH'
    for a in d.get('assets', []):
        n = a['name']
        if n.startswith('xpanel-master-') and n.endswith('-linux-' + arch + '.tar.gz'):
            print(a['browser_download_url']); break
except Exception:
    pass
" 2>/dev/null || true)"
    fi
    if [[ -z "$ASSET" ]]; then
      echo "无法自动获取最新 release（GitHub API 不可达或未安装 python3），请显式指定 --version 或 --url"
      exit 1
    fi
    [[ -n "$MIRROR" ]] && ASSET="${ASSET/https:\/\/github.com/$GH_BASE}"
    URL="$ASSET"
  fi
fi
SHA_URL="${URL%.tar.gz}.tar.gz.sha256"

echo "==> 参数"
echo "    目录     : $TARGET"
echo "    下载地址 : $URL"
echo "    校验     : $([ $NO_VERIFY -eq 1 ] && echo '跳过（--no-verify）' || echo 'release .sha256 强制校验')"
[[ $DRY_RUN -eq 1 ]] && echo "    [DRY-RUN] 仅打印步骤"

# ---------- 2. 下载 + 校验 + 解压 ----------
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

echo "==> 下载 release 包"
run curl -fSL -o "${STAGE}/${TARBALL}" "$URL"
if [[ $NO_VERIFY -eq 0 ]]; then
  if [[ -n "$DIGEST" ]]; then
    EXPECT="$DIGEST"
    echo "    （使用显式 --digest 校验）"
  else
    echo "==> 下载校验和"
    run curl -fsSL -o "${STAGE}/${TARBALL}.sha256" "$SHA_URL"
  fi
  if [[ $DRY_RUN -eq 0 ]]; then
    if [[ -z "${EXPECT:-}" ]]; then
      EXPECT="$(awk '{print $1}' "${STAGE}/${TARBALL}.sha256" | head -1)"
      [[ -z "$EXPECT" ]] && { echo "校验和文件为空（拒绝安装）"; exit 1; }
    fi
    ACTUAL="$(sha256sum "${STAGE}/${TARBALL}" | awk '{print $1}')"
    if [[ "$ACTUAL" != "$EXPECT" ]]; then
      echo "release 校验失败: 期望 $EXPECT 实际 $ACTUAL（拒绝安装）"; exit 1
    fi
    echo "    sha256 校验通过"
  else
    echo "    [DRY-RUN] 校验: sha256(${TARBALL}) 与 release .sha256 比对"
  fi
fi

echo "==> 解压到临时目录"
run tar -xzf "${STAGE}/${TARBALL}" -C "$STAGE"

echo "==> 安装到 $TARGET"
# 二进制/前端/编排模板始终覆盖（升级语义）；config.yaml 保留已有（幂等，不覆盖用户配置）
run cp "$STAGE/master/master" ./master
run chmod 0755 ./master
run rm -rf ./web && run mkdir -p ./web && run cp -r "${STAGE}/web/dist" ./web/
run cp "$STAGE/docker-compose.yml" ./docker-compose.yml
run cp "$STAGE/.env.example" ./.env.example
run cp "$STAGE/Caddyfile.reference" ./Caddyfile.reference 2>/dev/null || true
run cp "$STAGE/Dockerfile.runtime" ./Dockerfile.runtime 2>/dev/null || true
mkdir -p configs
if [[ ! -f configs/config.yaml ]]; then
  run cp "$STAGE/configs/config.yaml" configs/config.yaml
  echo "    （首次安装）已生成 configs/config.yaml，请按需编辑 public_url"
else
  echo "    configs/config.yaml 已存在，保留（如需重置请手动删除后重跑）"
fi

# ---------- 3. .env 与数据目录 ----------
if [[ ! -f .env ]]; then
  run cp ./.env.example ./.env
  echo "    （首次安装）已从 .env.example 生成 .env，请编辑 BIND_ADDR/端口/公网地址等"
else
  echo "    .env 已存在，保留"
fi
run mkdir -p data

# 安装目录属主（容器内 app 用户 uid 1000）：整目录 chown，面板内更新需写 master/web 与根目录标记；非 root 时提示
if [[ $DRY_RUN -eq 0 ]]; then
  if [[ "$(id -u)" -eq 0 ]]; then
    chown -R 1000:1000 "$TARGET"
  else
    echo "    [提示] 已创建安装目录；容器内 app 用户（uid 1000）需整目录写入（面板内更新），请以 root 执行："
    echo "      sudo chown -R 1000:1000 $TARGET"
  fi
fi

# ---------- 4. 收尾 ----------
if [[ $DRY_RUN -eq 1 ]]; then
  echo "[DRY-RUN] 完成（未执行任何安装）"
  exit 0
fi
cat <<EOF

安装完成。目录: $TARGET

下一步：
  1. 编辑 .env（端口/公网地址等）与 configs/config.yaml（至少 public_url；JWT/管理员留空=首次启动自动生成）
  2. 构建固定运行时镜像（一次性，仅装运行时依赖不含业务代码）：
       docker build -f Dockerfile.runtime -t xpanel-master-runtime:latest .
  3. 启动：
       docker compose up -d
  4. 查看初始管理员密码：
       docker compose logs master
  5. 反代：三个端口默认绑 127.0.0.1，需自备 Caddy/Nginx 按路径分流（见 Caddyfile.reference）

升级：重新运行本脚本即覆盖 master+web/dist、保留配置与数据。
EOF