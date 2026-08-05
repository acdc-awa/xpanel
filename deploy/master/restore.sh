#!/bin/bash
# Xray 面板主控数据库恢复脚本
# 用法: sudo bash restore.sh [备份文件名]     # 默认列出备份供选择
# 要求: 与 docker-compose.yml 同目录执行；备份位于 master-data 卷 backups/ 目录
set -euo pipefail

cd "$(dirname "$0")"
trap 'rc=$?; if [ $rc -ne 0 ]; then echo "恢复失败：master 可能处于停止状态，可执行 docker compose start master 恢复服务" >&2; fi; exit $rc' ERR

if ! command -v docker >/dev/null 2>&1; then
  echo "错误: 未找到 docker" >&2; exit 1
fi

if ! docker compose ps --status running 2>/dev/null | grep -q master; then
  echo "错误: master 容器未在运行（请先 docker compose up -d）" >&2; exit 1
fi

echo "==> 列出可用备份（卷 master-data:/app/data/backups）"
docker compose exec -T master sh -c 'ls -lh /app/data/backups/panel-*.db 2>/dev/null' || { echo "无备份文件"; exit 1; }

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

echo "==> 快照当前数据库（panel.db.pre-restore）"
docker compose exec -T master sh -c 'cp /app/data/panel.db /app/data/panel.db.pre-restore'

echo "==> 停止服务"
docker compose stop master

echo "==> 替换数据库（备份在容器卷内，经宿主临时文件中转：docker cp 不支持容器↔容器）"
TMPDB="restore.tmp.db"
trap 'rm -f "$TMPDB"' EXIT
docker compose cp master:/app/data/backups/$TARGET "$TMPDB"
docker compose cp "$TMPDB" master:/app/data/panel.db
rm -f "$TMPDB"

echo "==> 启动服务"
docker compose start master

echo "==> 验证"
OK=""
for i in 1 2 3 4 5; do
  if docker compose exec -T master sh -c 'wget -qO- http://127.0.0.1:18080/healthz' 2>/dev/null | grep -q '"ok"'; then
    OK="1"; break
  fi
  sleep 2
done
if [[ -z "$OK" ]]; then
  echo "healthz 未通过，请检查日志: docker compose logs master" >&2
  exit 1
fi

echo "完成。原库已保留为 /app/data/panel.db.pre-restore"
