#!/bin/bash
#
# Xray 节点 Agent 一键安装脚本（主控-节点-用户三层中的「被控」）
#
# 用法：
#   bash install-agent.sh --master <ws://host/api/v1/node/ws> --node-id <ID> --secret <SECRET> [选项]
#
# 选项：
#   --master <url>      主控节点 ws 地址（生产用 wss://，由主控 Caddy 终止 TLS）
#   --node-id <id>      主控「服务器」页新增节点时生成的 node_id
#   --secret <secret>   同上，节点密钥（仅显示一次）
#   --agent-url <url>   agent 二进制下载地址（默认取 --master 的 /api/v1/download/agent）
#   --agent-file <path> 本地 agent 二进制（离线/scp 场景，优先于 --agent-url）
#   --xray-version <v>  xray 版本（默认 v26.6.27，与验证环境一致）
#   --dry-run           只打印将执行的步骤，不实际执行
#
set -euo pipefail

MASTER=""
NODE_ID=""
SECRET=""
AGENT_URL=""
AGENT_FILE=""
XRAY_VERSION="v26.6.27"
DRY_RUN=0

usage() {
  sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --master) MASTER="$2"; shift 2;;
    --node-id) NODE_ID="$2"; shift 2;;
    --secret) SECRET="$2"; shift 2;;
    --agent-url) AGENT_URL="$2"; shift 2;;
    --agent-file) AGENT_FILE="$2"; shift 2;;
    --xray-version) XRAY_VERSION="$2"; shift 2;;
    --dry-run) DRY_RUN=1; shift;;
    -h|--help) usage;;
    *) echo "未知参数: $1"; usage;;
  esac
done

[[ -z "$MASTER" || -z "$NODE_ID" || -z "$SECRET" ]] && { echo "缺少 --master / --node-id / --secret"; usage; }
# agent 下载地址默认取主控 origin 的 /api/v1/download/agent
if [[ -z "$AGENT_URL" ]]; then
  AGENT_URL="${MASTER%%/api/*}/api/v1/download/agent"
fi

echo "==> 参数"
echo "    master   : $MASTER"
echo "    node_id  : $NODE_ID"
echo "    agent    : ${AGENT_FILE:-$AGENT_URL}"
echo "    xray     : $XRAY_VERSION"
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

# ---------- 1. 安装 agent 二进制 ----------
mkdir_p /usr/local/bin
if [[ -n "$AGENT_FILE" ]]; then
  run install -m 0755 "$AGENT_FILE" /usr/local/bin/xray-agent
else
  run curl -fsSL -o /tmp/xray-agent "$AGENT_URL"
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
  if command -v sha256sum >/dev/null 2>&1 && [[ -f "${ZIP}.dgst" ]]; then
    EXPECT=$(awk '/Xray-linux-64.zip/ {print $1}' "${ZIP}.dgst" | head -1)
    ACTUAL=$(sha256sum "$ZIP" | awk '{print $1}')
    if [[ -n "$EXPECT" && "$EXPECT" != "$ACTUAL" ]]; then
      echo "校验失败: 期望 $EXPECT 实际 $ACTUAL"; exit 1
    fi
    echo "    sha256 校验通过"
  fi
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
fi

# ---------- 4. systemd 单元 ----------
echo "==> 写入 systemd 单元"
if [[ $DRY_RUN -eq 0 ]]; then
  cat > /etc/systemd/system/xray-agent.service <<'UNIT'
[Unit]
Description=Xray Agent (Xray 节点托管)
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