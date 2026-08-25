#!/bin/bash
# Xray 面板主控数据库恢复脚本（压缩包挂载形态）
# 用法: sudo bash restore.sh [备份文件名]     # 默认列出备份供选择
# 要求: 与 docker-compose.yml 同目录执行；数据在宿主目录 ./data（挂载到容器 /app/data）
set -euo pipefail

cd "$(dirname "$0")"
trap 'rc=$?; if [ $rc -ne 0 ]; then echo "恢复失败：master 可能处于停止状态，可执行 docker compose start master 恢复服务" >&2; fi; exit $rc' ERR

if ! command -v docker >/dev/null 2>&1; then
  echo "错误: 未找到 docker" >&2; exit 1
fi

if ! docker compose ps --status running 2>/dev/null | grep -q master; then
  echo "错误: master 容器未在运行（请先 docker compose up -d）" >&2; exit 1
fi

DATA_DIR="$(pwd)/data"
echo "==> 数据目录: $DATA_DIR"

echo "==> 列出可用备份（$DATA_DIR/backups）"
ls -lh "$DATA_DIR"/backups/panel-*.db 2>/dev/null || { echo "无备份文件"; exit 1; }

TARGET="${1:-}"
if [[ -z "$TARGET" ]]; then
  if ! read -rp "输入要恢复的备份文件名: " TARGET; then
    echo "错误: 未提供备份文件名（用法: bash restore.sh <panel-YYYYMMDD-HHMMSS.db>）" >&2
    exit 1
  fi
fi
if [[ ! "$TARGET" =~ ^panel-[0-9]{8}-[0-9]{6}\.db$ ]]; then
  echo "错误: 备份文件名格式不符: $TARGET" >&2; exit 1
fi
BACKUP_FILE="$DATA_DIR/backups/$TARGET"
[[ -f "$BACKUP_FILE" ]] || { echo "错误: 备份文件不存在: $BACKUP_FILE" >&2; exit 1; }

echo "==> 停止服务（快照需在停止后进行：WAL 模式下运行中只拷主文件会不一致）"
docker compose stop master

echo "==> 快照当前数据库三件套到 ./data 根（原库另存 panel.db.pre-restore*）"
cp "$DATA_DIR/panel.db" "$DATA_DIR/panel.db.pre-restore"
cp "$DATA_DIR/panel.db-wal" "$DATA_DIR/panel.db.pre-restore-wal" 2>/dev/null || true
cp "$DATA_DIR/panel.db-shm" "$DATA_DIR/panel.db.pre-restore-shm" 2>/dev/null || true

echo "==> 替换数据库（备份在宿主 data/backups，直接拷贝）"
cp "$BACKUP_FILE" "$DATA_DIR/panel.db"

echo "==> 启动服务"
docker compose start master

echo "==> 修正数据库属主（容器内 app 用户 uid 1000）"
chown 1000:1000 "$DATA_DIR/panel.db" 2>/dev/null || true

echo "==> 验证可写性（exec 以容器 USER=app 运行，test -w 真实反映 app 可写）"
if ! docker compose exec -T master sh -c 'test -w /app/data/panel.db'; then
  echo "错误: /app/data/panel.db 对 app 不可写（data 目录属主可能非 1000）" >&2
  exit 1
fi

echo "==> 验证服务健康（healthz 为第二层；端口跟随 APP_PORT）"
OK=""
for i in 1 2 3 4 5; do
  if docker compose exec -T master sh -c 'wget -qO- http://127.0.0.1:${APP_PORT:-18080}/healthz' 2>/dev/null | grep -q '"ok"'; then
    OK="1"; break
  fi
  sleep 2
done
if [[ -z "$OK" ]]; then
  echo "healthz 未通过，请检查日志: docker compose logs master" >&2
  exit 1
fi

echo "完成。原库三件套已保留在 $DATA_DIR: ./panel.db.pre-restore*"
