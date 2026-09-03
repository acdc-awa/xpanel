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
#   --file <path>     本地 release 压缩包（xpanel-master-*.tar.gz，完全离线部署场景）
#   --mirror <url>    github.com 的替代基址/代理前缀（如 https://ghproxy.net/https://github.com）
#   --url <url>       完全自定义 release 包下载地址（优先于 version/mirror 推导；此时建议配 --digest）
#   --digest <sha>    release 包期望 sha256（提供则强制校验；缺省用 release 的 .sha256 自动校验）
#   --no-verify       跳过 sha256 校验（仅用于自建源/测试）
#   --fresh           全新覆盖（默认：检测到已有 configs/config.yaml 或 data/panel.db 时保持配置与数据）
#   --dry-run         只打印将执行的步骤，不实际执行
#
# 说明：若直连 GitHub 超时，脚本将自动切换内置镜像加速源。
# 产物：master 二进制 / web/dist / configs/config.yaml / docker-compose.yml / .env /
#       Caddyfile.reference / Dockerfile.runtime，数据挂载到当前目录 ./data（SQLite + JWT + 备份）。
#
set -euo pipefail

VERSION=""
MIRROR=""
URL=""
DIGEST=""
LOCAL_FILE=""
NO_VERIFY=0
FRESH=0
DRY_RUN=0
TARGET="$(pwd)"

GH_REPO="acdc-awa/xpanel"

usage() {
  sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) TARGET="$2"; shift 2;;
    --version) VERSION="$2"; shift 2;;
    --file) LOCAL_FILE="$2"; shift 2;;
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

# 内置 GitHub 代理候选列表（留空/直连优先，超时 6 秒自动平滑切换备选镜像）
DEFAULT_MIRRORS=(
  "https://github.com"
  "https://ghproxy.net/https://github.com"
  "https://gh-proxy.com/https://github.com"
  "https://github.moeyy.xyz/https://github.com"
)

# 带镜像故障切换与连接超时的下载函数：$1=目标文件 $2=源URL
download_with_fallback() {
  local out="$1" url="$2"
  if [[ "$url" != https://github.com/* ]]; then
    run curl -fsSL -o "$out" "$url"
    return $?
  fi

  local path="${url#https://github.com/}"
  local candidates=()

  # 若用户指定了 --mirror，优先置顶尝试
  if [[ -n "${MIRROR:-}" ]]; then
    local m="${MIRROR%/}"
    if [[ "$m" == *github.com ]]; then
      candidates+=("$m")
    else
      candidates+=("${m}/https://github.com")
    fi
  fi

  for m in "${DEFAULT_MIRRORS[@]}"; do
    candidates+=("$m")
  done

  local tried=()
  for c in "${candidates[@]}"; do
    c="${c%/}"
    local target_url="${c}/${path}"
    if [[ " ${tried[*]:-} " =~ " ${target_url} " ]]; then
      continue
    fi
    tried+=("$target_url")

    echo "==> 尝试下载: $target_url"
    if [[ $DRY_RUN -eq 1 ]]; then
      return 0
    fi
    if curl -fSL --connect-timeout 6 -o "$out" "$target_url" 2>/dev/null; then
      echo "    下载成功"
      return 0
    else
      echo "    连接超时或失败，尝试下一个候选镜像..."
    fi
  done

  echo "错误: 所有源均下载失败 ($url)"
  return 1
}

# ---------- 1. 解析 release 包下载地址 ----------
if [[ -n "$LOCAL_FILE" ]]; then
  echo "==> 使用本地 release 压缩包: $LOCAL_FILE"
  if [[ ! -f "$LOCAL_FILE" ]]; then
    echo "本地 release 文件不存在: $LOCAL_FILE"; exit 1
  fi
  TARBALL="$(basename "$LOCAL_FILE")"
  URL="file://$LOCAL_FILE"
  SHA_URL=""
elif [[ -z "$URL" ]]; then
  if [[ -n "$VERSION" ]]; then
    URL="https://github.com/${GH_REPO}/releases/download/${VERSION}/xpanel-master-${VERSION}-linux-${ARCH}.tar.gz"
  else
    echo "==> 查询最新 release"
    LATEST_TAG=""
    # 策略 1：通过候选镜像探测 /releases/latest 的 HTTP 302 重定向 Location
    candidate_bases=()
    [[ -n "${MIRROR:-}" ]] && candidate_bases+=("${MIRROR%/}")
    candidate_bases+=("https://github.com" "https://ghproxy.net/https://github.com" "https://gh-proxy.com/https://github.com")
    for b in "${candidate_bases[@]}"; do
      b="${b%/}"
      loc="$(curl -sI --connect-timeout 5 "${b}/${GH_REPO}/releases/latest" 2>/dev/null | grep -i '^Location:' | tr -d '\r\n' | awk '{print $2}')"
      if [[ -n "$loc" && "$loc" == *"/tag/"* ]]; then
        LATEST_TAG="${loc##*/}"
        break
      fi
    done

    # 策略 2：若 302 失败，回退到 GitHub API 查 releases/latest
    if [[ -z "$LATEST_TAG" && $(command -v python3) ]]; then
      API_URL="https://api.github.com/repos/${GH_REPO}/releases/latest"
      RELEASE_JSON="$(curl -fsSL --connect-timeout 5 "$API_URL" 2>/dev/null || true)"
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
      if [[ -n "$ASSET" ]]; then
        URL="$ASSET"
      fi
    fi

    if [[ -n "$LATEST_TAG" ]]; then
      URL="https://github.com/${GH_REPO}/releases/download/${LATEST_TAG}/xpanel-master-${LATEST_TAG}-linux-${ARCH}.tar.gz"
    fi

    if [[ -z "$URL" ]]; then
      echo "无法自动获取最新 release（GitHub API 与镜像跳转均不可达），请显式指定 --version、--url 或 --file"
      exit 1
    fi
  fi
  TARBALL="$(basename "$URL")"
  SHA_URL="${URL%.tar.gz}.tar.gz.sha256"
else
  TARBALL="$(basename "$URL")"
  SHA_URL="${URL%.tar.gz}.tar.gz.sha256"
fi

echo "==> 参数"
echo "    目录     : $TARGET"
echo "    下载地址 : ${LOCAL_FILE:-$URL}"
echo "    校验     : $([ $NO_VERIFY -eq 1 ] && echo '跳过（--no-verify）' || echo 'release .sha256 强制校验')"
[[ $DRY_RUN -eq 1 ]] && echo "    [DRY-RUN] 仅打印步骤"

# ---------- 2. 下载 + 校验 + 解压 ----------
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

if [[ -n "$LOCAL_FILE" ]]; then
  echo "==> 载入本地 release 包"
  run cp "$LOCAL_FILE" "${STAGE}/${TARBALL}"
  if [[ $NO_VERIFY -eq 0 ]]; then
    if [[ -n "$DIGEST" ]]; then
      EXPECT="$DIGEST"
      echo "    （使用显式 --digest 校验）"
    elif [[ -f "${LOCAL_FILE}.sha256" ]]; then
      EXPECT="$(awk '{print $1}' "${LOCAL_FILE}.sha256" | head -1)"
      echo "    （使用本地 .sha256 文件校验）"
    fi
    if [[ $DRY_RUN -eq 0 && -n "${EXPECT:-}" ]]; then
      ACTUAL="$(sha256sum "${STAGE}/${TARBALL}" | awk '{print $1}')"
      if [[ "$ACTUAL" != "$EXPECT" ]]; then
        echo "release 校验失败: 期望 $EXPECT 实际 $ACTUAL（拒绝安装）"; exit 1
      fi
      echo "    sha256 校验通过"
    fi
  fi
else
  echo "==> 获取 release 包"
  download_with_fallback "${STAGE}/${TARBALL}" "$URL"
  if [[ $NO_VERIFY -eq 0 ]]; then
    if [[ -n "$DIGEST" ]]; then
      EXPECT="$DIGEST"
      echo "    （使用显式 --digest 校验）"
    else
      echo "==> 获取校验和"
      download_with_fallback "${STAGE}/${TARBALL}.sha256" "$SHA_URL"
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
fi

echo "==> 解压到临时目录"
if [[ $DRY_RUN -eq 0 ]]; then
  run tar -xzf "${STAGE}/${TARBALL}" -C "$STAGE"
fi

echo "==> 安装到 $TARGET"
# 二进制/前端/编排模板始终覆盖（升级语义）；config.yaml 保留已有（幂等，不覆盖用户配置）
run cp "$STAGE/master/master" ./master
run chmod 0755 ./master
run rm -rf ./web && run mkdir -p ./web && run cp -r "${STAGE}/web/dist" ./web/
run cp "$STAGE/docker-compose.yml" ./docker-compose.yml
run cp "$STAGE/.env.example" ./.env.example
run cp "$STAGE/Caddyfile.reference" ./Caddyfile.reference 2>/dev/null || true
run cp "$STAGE/Dockerfile.runtime" ./Dockerfile.runtime 2>/dev/null || true
# 运行时镜像构建文件（compose 带 build 块，缺失即自动构建）
run mkdir -p ./deploy/master
run cp "$STAGE/deploy/master/entrypoint.sh" ./deploy/master/entrypoint.sh
run cp "$STAGE/.dockerignore" ./.dockerignore 2>/dev/null || true
mkdir -p configs
if [[ ! -f configs/config.yaml ]]; then
  run cp "$STAGE/configs/config.yaml" configs/config.yaml
  echo "    （首次安装）已生成 configs/config.yaml（应用配置唯一入口），请按需编辑 public_url / ws_public_url 等"
else
  echo "    configs/config.yaml 已存在，保留（如需重置请手动删除后重跑）"
fi

# ---------- 3. .env 与数据目录 ----------
if [[ ! -f .env ]]; then
  run cp ./.env.example ./.env
  echo "    （首次安装）已从 .env.example 生成 .env（仅编排参数：BIND_ADDR/宿主端口映射），应用配置在 configs/config.yaml"
else
  echo "    .env 已存在，保留"
fi
run mkdir -p data

# ---------- 3.5 迁移：.env 的 JWT_SECRET 固化进 config.yaml（2026-08-30 起环境变量退役） ----------
# 老部署曾在 .env 配过 JWT_SECRET 而 config.yaml 尚未写入 jwt.secret 时，把该值写入 yaml：
# TOTP 加密 key 由 jwt.secret 派生（services/otp.go），若不固化，删环境变量后派生 key 漂移，
# 已启用 2FA 用户的密文将无法解密（登录失败）。
if [[ -f .env && -f configs/config.yaml ]] && ! grep -qE '^[[:space:]]*secret:' configs/config.yaml; then
  ENV_JWT="$(sed -n 's/^JWT_SECRET=//p' .env | head -1 | tr -d ' \r"'"'"'')"
  if [[ -n "$ENV_JWT" && "$ENV_JWT" != \#* ]]; then
    if grep -qE '^jwt:' configs/config.yaml; then
      awk -v s="$ENV_JWT" '/^jwt:/{print; print "  secret: " s; next} {print}' configs/config.yaml > configs/config.yaml.tmp && mv configs/config.yaml.tmp configs/config.yaml
    else
      printf '\njwt:\n  secret: %s\n' "$ENV_JWT" >> configs/config.yaml
    fi
    echo "    （迁移）已把 .env 的 JWT_SECRET 固化进 configs/config.yaml；JWT_SECRET 从环境变量退役（config.yaml 成为唯一入口）"
  fi
fi

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
  1. 编辑 configs/config.yaml（应用配置唯一入口：至少填 public_url；JWT/管理员留空=首次启动自动生成）。
     .env 仅在需要改宿主端口映射/BIND_ADDR 时编辑（纯编排参数，不进进程）
  2. 启动（首次自动构建固定运行时镜像，无需手动 docker build；仅装运行时依赖不含业务代码）：
       docker compose up -d
  3. 查看初始管理员密码：
       docker compose logs master
  4. 反代：三个端口默认绑 127.0.0.1，需自备 Caddy/Nginx 按路径分流（见 Caddyfile.reference）

升级：重新运行本脚本即覆盖 master+web/dist、保留配置与数据。
EOF