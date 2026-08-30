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
for arg in "$@"; do [[ "$arg" == "--no-web" ]] && NO_WEB=1; done
[[ "${1:-}" == "--no-web" ]] && VERSION="$(git -C "$ROOT" describe --tags --always 2>/dev/null || echo dev)"

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
( cd "$ROOT" && CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -ldflags "-s -w -X main.Version=${VERSION}" -o "$OUT/master" ./cmd/master )

echo "==> 组装 release 目录"
mkdir -p "$STAGE/master" "$STAGE/web" "$STAGE/configs"
cp "$OUT/master" "$STAGE/master/master"
cp -r "$ROOT/web/dist" "$STAGE/web/dist"
cp "$ROOT/configs/config.example.yaml" "$STAGE/configs/config.yaml"
cat > "$STAGE/README.txt" <<'EOF'
XrayPanel 主控 release 包（压缩包挂载形态）
===========================================
一、一键部署（推荐）：运行包内 install.sh，自动下载并解压本包、
生成 configs/config.yaml 与 .env、创建 data 目录
    1. 上传本包到你打算部署的目录（或任选目录后加 --dir 指定）：
         cd /opt/xray-panel
         bash install.sh            # 或 bash install.sh --dir /opt/xray-panel
    2. 编辑 configs/config.yaml（至少填 public_url；JWT/管理员留空=首次启动自动生成）
    3. 编辑 .env（端口/公网地址/备份等；有默认值可先不动）
    4. 安装目录属主（本机已自动创建，但 uid 1000 属主需 root 执行——面板内更新需写整目录）：
         sudo chown -R 1000:1000 /opt/xray-panel
    5. 启动（首次自动构建固定运行时镜像，无需手动 docker build）：
         docker compose up -d
    6. 查看初始管理员密码：
         docker compose logs master

二、手动安装（等价）
    1. 解压本包到宿主目录，如 /opt/xray-panel
    2. 编辑 configs/config.yaml 与 .env
    3. sudo chown -R 1000:1000 /opt/xray-panel
    4. cd /opt/xray-panel && docker compose up -d

三、升级：面板内「系统设置 → 更新」自更新（下载校验替换后容器自重启）；
    或重新运行 install.sh（覆盖 master/ 与 web/dist）→ docker compose restart master
四、反代：三个端口默认绑 127.0.0.1，需自备 Caddy/Nginx 按路径分流（见 Caddyfile.reference）

数据目录 data/（SQLite + JWT + 备份）务必定期备份。
EOF
cp "$ROOT/deploy/master/docker-compose.yml" "$STAGE/docker-compose.yml"
cp "$ROOT/deploy/master/.env.example" "$STAGE/.env.example"
cp "$ROOT/deploy/master/Caddyfile" "$STAGE/Caddyfile.reference"
cp "$ROOT/Dockerfile.runtime" "$STAGE/Dockerfile.runtime"
cp "$ROOT/deploy/master/install.sh" "$STAGE/install.sh"
# 运行时镜像构建所需（Dockerfile.runtime 的 COPY 路径与仓库结构一致，宿主目录直接 docker build）
mkdir -p "$STAGE/deploy/master"
cp "$ROOT/deploy/master/entrypoint.sh" "$STAGE/deploy/master/entrypoint.sh"
# .dockerignore：宿主目录内 docker build context 只含 Dockerfile.runtime 与 entrypoint（排除 data/业务文件）
cat > "$STAGE/.dockerignore" <<'EOF'
*
!Dockerfile.runtime
!deploy/
!deploy/master/
!deploy/master/entrypoint.sh
EOF

echo "==> 打包 $TARBALL"
( cd "$OUT" && tar -czf "$TARBALL" -C "$STAGE" . )
( cd "$OUT" && sha256sum "$TARBALL" > "$TARBALL.sha256" )

echo "完成:"
echo "  $TARBALL"
echo "  $TARBALL.sha256"
rm -rf "$STAGE" "$OUT/master"
