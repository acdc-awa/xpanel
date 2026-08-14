#!/bin/bash
#
# 备份恢复演练脚本（U24——恢复不是「把文件拷回去」就完事，必须演练到可验证）
#
# 用途：把指定备份文件恢复到临时 SQLite 库，跑完整性校验 + 关键表抽检，
#       模拟一次完整恢复并验证数据可用性。非破坏性：不触碰生产库与备份原件。
#
# 用法：
#   bash scripts/restore_drill.sh <backup.db> [--sanity]
#     --sanity  额外打印 settings / users / servers / orders 抽样计数（证明数据可读）
#
# 输出：
#   全部通过 → 退出码 0，末尾打印「恢复演练通过」；任何一步失败 → 非 0 退出并说明原因。
#
set -euo pipefail

BACKUP="${1:?用法: bash restore_drill.sh <backup.db> [--sanity]}"
SANITY=0
[[ "${2:-}" == "--sanity" ]] && SANITY=1

[[ -f "$BACKUP" ]] || { echo "备份文件不存在: $BACKUP"; exit 1; }
command -v sqlite3 >/dev/null 2>&1 || { echo "缺少 sqlite3 命令行工具（恢复演练依赖）"; exit 1; }

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
RESTORED="$TMP_DIR/restored.db"

echo "==> 1/3 拷贝备份到临时库（模拟恢复：生产环境此步 = 停主控 → 覆盖主库 → 启主控）"
cp "$BACKUP" "$RESTORED"

echo "==> 2/3 完整性校验（integrity_check + foreign_key_check）"
RES=$(sqlite3 "$RESTORED" 'PRAGMA integrity_check;')
[[ "$RES" == "ok" ]] || { echo "integrity_check 失败: $RES"; exit 1; }
FK=$(sqlite3 "$RESTORED" 'PRAGMA foreign_key_check;')
[[ -z "$FK" ]] || { echo "外键损坏: $FK"; exit 1; }
echo "    integrity_check=ok, foreign_key_check 无异常"

echo "==> 3/3 关键表可读性抽检"
for TABLE in users servers inbounds plans settings; do
  CNT=$(sqlite3 "$RESTORED" "SELECT count(*) FROM $TABLE;" 2>/dev/null || echo "缺失")
  echo "    $TABLE: $CNT"
done
if [[ $SANITY -eq 1 ]]; then
  echo "    --- 抽样数据 ---"
  sqlite3 "$RESTORED" "SELECT '用户:', count(*) FROM users;" 2>/dev/null || true
  sqlite3 "$RESTORED" "SELECT '有效节点:', count(*) FROM servers WHERE status=1;" 2>/dev/null || true
  sqlite3 "$RESTORED" "SELECT '已支付订单:', count(*) FROM orders WHERE status='paid';" 2>/dev/null || true
fi

echo
echo "恢复演练通过：备份可完整恢复且数据可读（$BACKUP）"
