#!/bin/bash
#
# XPanel-Node 一键安装脚本（主控-节点-用户三层中的「被控」）
#
# 用法：
#   bash install-agent.sh --master <ws://host/api/v1/node/ws> --node-id <ID> --secret <SECRET> [选项]
#
# 选项：
#   --master <url>       主控节点 ws 地址（生产用 wss://，由主控 Caddy 终止 TLS）
#   --node-id <id>       主控「服务器」页新增节点时生成的 node_id
#   --secret <secret>    同上，节点密钥（仅显示一次）
#   --agent-version <v>  钉版本安装（如 v0.1.0；缺省装最新 release）
#   --agent-mirror <url> github.com 的替代基址/代理前缀（如 https://ghproxy.net/https://github.com）
#   --agent-url <url>    完全自定义 agent 二进制下载地址（优先于 version/mirror 推导）
#   --agent-file <path>  本地 agent 二进制（离线/scp 场景，优先于一切下载）
#   --agent-digest <sha> 期望的 agent 二进制 sha256（提供则强制校验，不匹配拒绝安装）
#   --xray-version <v>   xray 版本（默认 v26.6.27，与验证环境一致）
#   --dry-run            只打印将执行的步骤，不实际执行
#
# 说明：agent 二进制默认从 GitHub Releases 下载并自动用 release 的 checksums.txt
# 做 sha256 校验（缺校验文件拒绝安装）；自定义 --agent-url 时无校验和来源，
# 建议显式提供 --agent-digest。
#
set -euo pipefail

MASTER=""
NODE_ID=""
SECRET=""
AGENT_VERSION=""
AGENT_MIRROR=""
AGENT_URL=""
AGENT_FILE=""
AGENT_DIGEST=""
XRAY_VERSION="v26.6.27"
DRY_RUN=0

GH_REPO="acdc-awa/XPanel-Node"

usage() {
  sed -n '2,27p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --master) MASTER="$2"; shift 2;;
    --node-id) NODE_ID="$2"; shift 2;;
    --secret) SECRET="$2"; shift 2;;
    --agent-version) AGENT_VERSION="$2"; shift 2;;
    --agent-mirror) AGENT_MIRROR="$2"; shift 2;;
    --agent-url) AGENT_URL="$2"; shift 2;;
    --agent-file) AGENT_FILE="$2"; shift 2;;
    --agent-digest) AGENT_DIGEST="$2"; shift 2;;
    --xray-version) XRAY_VERSION="$2"; shift 2;;
    --dry-run) DRY_RUN=1; shift;;
    -h|--help) usage;;
    *) echo "未知参数: $1"; usage;;
  esac
done

[[ -z "$MASTER" || -z "$NODE_ID" || -z "$SECRET" ]] && { echo "缺少 --master / --node-id / --secret"; usage; }

# 架构探测（发布流水线提供 linux/amd64 与 linux/arm64）
ARCH=""
case "$(uname -m)" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "不支持的架构: $(uname -m)（发布仅提供 linux/amd64 与 linux/arm64）"; exit 1 ;;
esac
ASSET="xray-agent-linux-${ARCH}"

# agent 下载地址推导：默认 GitHub Releases（latest 或钉版本），镜像前缀可注入
GH_BASE="${AGENT_MIRROR:-https://github.com}"
GH_BASE="${GH_BASE%/}"
if [[ -z "$AGENT_URL" && -z "$AGENT_FILE" ]]; then
  if [[ -n "$AGENT_VERSION" ]]; then
    CHANNEL="download/${AGENT_VERSION}"
  else
    CHANNEL="latest/download"
  fi
  AGENT_URL="${GH_BASE}/${GH_REPO}/releases/${CHANNEL}/${ASSET}"
  CHECKSUMS_URL="${GH_BASE}/${GH_REPO}/releases/${CHANNEL}/checksums.txt"
else
  CHECKSUMS_URL=""
fi

echo "==> 参数"
echo "    master   : $MASTER"
echo "    node_id  : $NODE_ID"
echo "    agent    : ${AGENT_FILE:-$AGENT_URL}"
echo "    arch     : $ARCH"
echo "    xray     : $XRAY_VERSION"
[[ -n "$AGENT_DIGEST" ]] && echo "    agent 校验: --agent-digest 强制校验"
[[ -n "$CHECKSUMS_URL" && -z "$AGENT_DIGEST" ]] && echo "    agent 校验: release checksums.txt 自动校验"
[[ $DRY_RUN -eq 1 ]] && echo "    [DRY-RUN] 仅打印步骤"

if [[ $DRY_RUN -eq 0 && $EUID -ne 0 ]]; then
  echo "请用 root 运行（sudo bash install-agent.sh ...）"
  exit 1
fi

run() {
  echo "==> $*"
  [[ $DRY_RUN -eq 1 ]] && return 0
  "$@"
}

mkdir_p() { run mkdir -p "$@"; }

# 校验下载/本地文件的 sha256：$1=文件 $2=期望摘要（不匹配即退出）
verify_sha256() {
  local file="$1" expect="$2" actual
  actual=$(sha256sum "$file" | awk '{print $1}')
  if [[ "$actual" != "$expect" ]]; then
    echo "agent 校验失败: 期望 $expect 实际 $actual"
    exit 1
  fi
  echo "    agent sha256 校验通过"
}

# ---------- 1. 安装 agent 二进制 ----------
mkdir_p /usr/local/bin
if [[ -n "$AGENT_FILE" ]]; then
  # U22：提供 --agent-digest 时强制校验（含本地文件），不匹配拒绝安装
  if [[ -n "$AGENT_DIGEST" ]]; then
    verify_sha256 "$AGENT_FILE" "$AGENT_DIGEST"
  fi
  run install -m 0755 "$AGENT_FILE" /usr/local/bin/xray-agent
else
  run curl -fsSL -o /tmp/xray-agent "$AGENT_URL"
  if [[ -n "$AGENT_DIGEST" ]]; then
    verify_sha256 /tmp/xray-agent "$AGENT_DIGEST"
  elif [[ -n "$CHECKSUMS_URL" ]]; then
    # 默认 GitHub 源：拉 release checksums.txt 强制校验（缺文件/缺条目拒绝安装）
    run curl -fsSL -o /tmp/xray-agent.sha256 "$CHECKSUMS_URL"
    if [[ $DRY_RUN -eq 0 ]]; then
      EXPECT=$(awk -v a="$ASSET" '$2 == a {print $1}' /tmp/xray-agent.sha256 | head -1)
      if [[ -z "$EXPECT" ]]; then
        echo "checksums.txt 缺少 $ASSET 条目（拒绝安装）"; rm -f /tmp/xray-agent; exit 1
      fi
      verify_sha256 /tmp/xray-agent "$EXPECT"
      rm -f /tmp/xray-agent.sha256
    fi
  else
    echo "    [警告] 自定义 --agent-url 且未提供 --agent-digest，agent 二进制未校验（供应链加固建议提供）"
  fi
  run chmod 0755 /tmp/xray-agent
  run install -m 0755 /tmp/xray-agent /usr/local/bin/xray-agent
  run rm -f /tmp/xray-agent
fi

# ---------- 2. 安装 xray-core ----------
run mkdir_p /usr/local/share/xray
if [[ ! -x /usr/local/bin/xray ]] || ! /usr/local/bin/xray version >/dev/null 2>&1; then
  ZIP="/tmp/xray-${XRAY_VERSION}.zip"
  URL="https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/Xray-linux-64.zip"
  echo "==> 下载 xray ${XRAY_VERSION}"
  run curl -fL -o "$ZIP" "$URL"
  echo "==> 下载校验和"
  run curl -fL -o "${ZIP}.dgst" "${URL}.dgst"
  # U22：sha256 校验必检——缺工具/缺条目/不匹配一律拒绝安装（不再静默跳过）
  if ! command -v sha256sum >/dev/null 2>&1; then
    echo "缺少 sha256sum 工具，无法校验 xray 完整性（拒绝安装）"; exit 1
  fi
  EXPECT=$(awk '/Xray-linux-64.zip/ {print $1}' "${ZIP}.dgst" | head -1)
  if [[ -z "$EXPECT" ]]; then
    echo "校验和文件缺少 Xray-linux-64.zip 条目（拒绝安装）"; exit 1
  fi
  ACTUAL=$(sha256sum "$ZIP" | awk '{print $1}')
  if [[ "$EXPECT" != "$ACTUAL" ]]; then
    echo "校验失败: 期望 $EXPECT 实际 $ACTUAL"; exit 1
  fi
  echo "    sha256 校验通过"
  run unzip -o "$ZIP" -d /tmp/xray-extract >/dev/null 2>&1 || run python3 -m zipfile -e "$ZIP" /tmp/xray-extract
  run install -m 0755 /tmp/xray-extract/xray /usr/local/bin/xray
  run install -m 0644 /tmp/xray-extract/geoip.dat /usr/local/share/xray/geoip.dat
  run install -m 0644 /tmp/xray-extract/geosite.dat /usr/local/share/xray/geosite.dat
  run rm -rf /tmp/xray-extract "$ZIP" "${ZIP}.dgst"
else
  echo "==> xray 已存在，跳过下载"
fi

# ---------- 3. 写 Agent 配置 ----------
mkdir_p /etc/xray-agent /var/log/xray-agent /run/xray-agent
echo "==> 写入 /etc/xray-agent/config.yml"
if [[ $DRY_RUN -eq 0 ]]; then
  cat > /etc/xray-agent/config.yml <<EOF
master:
  url: ${MASTER}
  node_id: ${NODE_ID}
  secret: ${SECRET}
xray:
  bin: /usr/local/bin/xray
  config_path: /etc/xray-agent/config.json
  log_path: /var/log/xray-agent/xray.log
  pid_file: /run/xray-agent/xray.pid
stats:
  api_addr: 127.0.0.1:10085
  collect_interval: 30s
  report_interval: 60s
heartbeat_interval: 30s
reconnect_max: 60s
EOF
  # U22：配置文件含节点密钥，权限收紧为仅 root 可读写（默认 umask 会生成 0644）
  chmod 0600 /etc/xray-agent/config.yml
fi

# ---------- 4. systemd 单元 ----------
echo "==> 写入 systemd 单元"
if [[ $DRY_RUN -eq 0 ]]; then
  cat > /etc/systemd/system/xray-agent.service <<'UNIT'
[Unit]
Description=XPanel-Node Agent (Xray 节点托管)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/xray-agent -config /etc/xray-agent/config.yml
Restart=always
RestartSec=3
User=root
WorkingDirectory=/etc/xray-agent
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable --now xray-agent
fi

# ---------- 5. 结果 ----------
if [[ $DRY_RUN -eq 0 ]]; then
  sleep 2
  systemctl --no-pager --full status xray-agent | head -12 || true
  echo
  echo "安装完成。可在主控「服务器」页查看该节点在线状态，并「生成并下发配置」。"
else
  echo "[DRY-RUN] 完成（未执行任何安装）"
fi
