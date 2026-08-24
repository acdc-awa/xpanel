#!/bin/bash
# 构建 master 主控 release 压缩包（压缩包挂载形态专用）。
#
# 产物包含：master 二进制 + web/dist 前端产物 + configs/config.example.yaml + README，
# 解压到宿主目录后改 config.yaml、chown data 目录即可 docker compose up -d。
# 用法：
#   bash scripts/build-release.sh [version]
#     version 缺省用 git describe --tags --always
#   bash scripts/build-release.sh --no-web   只打二进制（跳过前端构建，web 用已有 dist）
# 输出：dist/xpanel-master-<version>-linux-amd64.tar.gz 及其 .sha256
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-$(git -C "$ROOT" describe --tags --always 2>/dev/null || echo dev)}"
NO_WEB=0
[[ "${1:-}" == "--no-web" ]] && { VERSION="$(git -C "$ROOT" describe --tags --always 2>/dev/null || echo dev)"; NO_WEB=1; }

GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
OUT="$ROOT/dist"
STAGE="$OUT/stage"
TARBALL="$OUT/xpanel-master-${VERSION}-${GOOS}-${GOARCH}.tar.gz"

echo "==> 清理旧产物"
rm -rf "$STAGE"
mkdir -p "$STAGE" "$OUT"

if [[ "$NO_WEB" -eq 0 ]]; then
  echo "==> 构建前端 (web/dist)"
  ( cd "$ROOT/web" && npm ci --no-audit --no-fund && npm run build )
fi
[[ -d "$ROOT/web/dist" ]] || { echo "错误: 缺少 $ROOT/web/dist（先 npm run build，或用 --no-web 跳过）"; exit 1; }

echo "==> 构建 master 二进制 (version=$VERSION, $GOOS/$GOARCH)"
( cd "$ROOT" && CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -o "$OUT/master" ./cmd/master )

echo "==> 组装 release 目录"
mkdir -p "$STAGE/master" "$STAGE/web" "$STAGE/configs"
cp "$OUT/master" "$STAGE/master/master"
cp -r "$ROOT/web/dist" "$STAGE/web/dist"
cp "$ROOT/configs/config.example.yaml" "$STAGE/configs/config.yaml"
cat > "$STAGE/README.txt" <<'EOF'
XrayPanel 主控 release 包（压缩包挂载形态）
===========================================
1. 解压本包到宿主目录，如 /opt/xray-panel
2. 编辑 configs/config.yaml（至少填 public_url；JWT/管理员留空=首次启动自动生成）
3. chown data 目录给容器内 app 用户（uid 1000）：
     sudo chown -R 1000:1000 data   # 或 sudo chown -R 1000:1000 /opt/xray-panel/data
4. 构建/获取固定运行时镜像（一次性）：
     docker build -f Dockerfile.runtime -t xpanel-master-runtime:latest /opt/xray-panel
   （Dockerfile.runtime 在仓库 deploy/master 或根目录；已构建过可跳过）
5. 启动：
     cd /opt/xray-panel && docker compose up -d
6. 升级：下载新 release 包 → 覆盖 master/master 与 web/dist → docker compose restart master
7. 反代：四个端口默认绑 127.0.0.1，需自备 Caddy/Nginx 按路径分流（见 deploy/master/Caddyfile 参考模板）

数据目录 data/（SQLite + JWT + 备份）务必定期备份。
EOF
cp "$ROOT/deploy/master/docker-compose.yml" "$STAGE/docker-compose.yml"
cp "$ROOT/deploy/master/.env.example" "$STAGE/.env.example"
cp "$ROOT/deploy/master/Caddyfile" "$STAGE/Caddyfile.reference"

echo "==> 打包 $TARBALL"
( cd "$OUT" && tar -czf "$TARBALL" -C "$STAGE" . )
( cd "$OUT" && sha256sum "$TARBALL" > "$TARBALL.sha256" )

echo "完成:"
echo "  $TARBALL"
echo "  $TARBALL.sha256"
rm -rf "$STAGE" "$OUT/master"
