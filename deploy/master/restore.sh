#!/bin/bash
# Xray 面板主控数据库恢复脚本
# 用法: sudo bash restore.sh [备份文件名]     # 默认列出备份供选择
# 要求: 与 docker-compose.yml 同目录执行；备份位于 master-data 卷 backups/ 目录
set -euo pipefail

cd "$(dirname "$0")"

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
  read -rp "输入要恢复的备份文件名: " TARGET
fi
if [[ ! "$TARGET" =~ ^panel-[0-9]{8}-[0-9]{6}\.db$ ]]; then
  echo "错误: 备份文件名格式不符: $TARGET" >&2; exit 1
fi

echo "==> 停止服务"
docker compose stop master

echo "==> 替换数据库（备份先另存为 panel.db.pre-restore）"
docker compose exec -T master sh -c \
  "cp /app/data/panel.db /app/data/panel.db.pre-restore && cp /app/data/backups/$TARGET /app/data/panel.db && chown app:app /app/data/panel.db"

echo "==> 启动服务"
docker compose start master

sleep 3
echo "==> 验证"
docker compose exec -T master sh -c 'wget -qO- http://127.0.0.1:18080/healthz' || \
  curl -fsS http://127.0.0.1:18080/healthz || echo "healthz 未通过，请检查日志: docker compose logs master"

echo "完成。原库已保留为 /app/data/panel.db.pre-restore"
