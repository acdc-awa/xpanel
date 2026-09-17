#!/bin/bash
#
# 计费桶边界存量修复脚本（traffic_cycle_start 未落在整点）
#
# 背景：流量明细落库时统一归一到整点（services/traffic.go 的 trafficPeriodBucket），
#       而计费/配额口径按 `period_start >= user.traffic_cycle_start` 过滤。
#       周期起点若落在小时中间（购买/续费/重置都是 time.Now()，必然落在中间），
#       该小时的整桶明细因桶起点早于周期起点而被整段排除——最坏情况（新用户首小时
#       就用完流量）表现为「仪表盘有流量、用户管理处计费 0」。
#
# 本脚本做的事：把存量用户的周期起点回退到所在整点，使该小时桶重新纳入计费范围。
# 代价：周期起点前那一小段（整点到购买/重置时刻）的流量会被一并计入本周期，
#       方向偏多算、有界（<1 小时）——这是脚本唯一能做的回收，整桶无法再拆分。
#       代码侧修复（切换周期时对该整点桶的计费字节清零）是精确的，本脚本不适用那种做法：
#       切换时刻已是过去，切换后的字节已合并进同一行，无法再区分。
#
# 用法（与 docker-compose.yml 同目录执行，同 restore.sh）：
#   sudo bash repair-billing-bucket.sh              # 默认只报告，不写库（dry-run）
#   sudo bash repair-billing-bucket.sh --apply      # 执行修复（先自动备份，再事务内更新）
#   sudo bash repair-billing-bucket.sh --db <path>  # 显式指定库文件
#
# 建议顺序：先部署代码侧修复并重启，再跑本脚本清理存量，否则此后的重置/购买仍会写入非整点值。
# 幂等：跑第二遍时已无匹配行，不做任何修改。
#
set -euo pipefail

cd "$(dirname "$0")"

APPLY=0
DB=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply) APPLY=1; shift ;;
    --db) DB="${2:?--db 需要库文件路径}"; shift 2 ;;
    -h|--help) sed -n '2,26p' "$0"; exit 0 ;;
    *) echo "未知参数: $1（-h 查看用法）" >&2; exit 1 ;;
  esac
done

# ---------- 定位数据库 ----------
if [[ -z "$DB" ]]; then
  DATA_DIR="$(pwd)/data"
  # 取 data 根目录下的 .db（排除 backups 子目录），多于一个时要求显式指定
  mapfile -t CANDIDATES < <(find "$DATA_DIR" -maxdepth 1 -type f -name '*.db' 2>/dev/null | sort)
  if [[ ${#CANDIDATES[@]} -eq 0 ]]; then
    echo "错误：未在 $DATA_DIR 找到 .db（可用 --db <path> 指定）" >&2; exit 1
  elif [[ ${#CANDIDATES[@]} -gt 1 ]]; then
    echo "错误：$DATA_DIR 下有多个 .db，请用 --db 指定：" >&2
    printf '  %s\n' "${CANDIDATES[@]}" >&2; exit 1
  fi
  DB="${CANDIDATES[0]}"
fi
[[ -f "$DB" ]] || { echo "错误：库文件不存在: $DB" >&2; exit 1; }

DB_DIR="$(cd "$(dirname "$DB")" && pwd)"
DB_BASE="$(basename "$DB")"
DATA_DIR="$DB_DIR"
BACKUP_DIR="$DB_DIR/backups"
mkdir -p "$BACKUP_DIR"

echo "==> 库文件: $DB_DIR/$DB_BASE"

# 一律以库所在目录为工作目录、SQL 内只用相对路径：绝对路径写进 SQL 文本时不会被
# MSYS/cygwin 之类环境翻译，且含空格或非 ASCII 的安装目录会踩到转义问题。
cd "$DB_DIR"

# ---------- sqlite3 执行通道：宿主优先，缺失则用容器（alpine 基线镜像本地已有） ----------
if command -v sqlite3 >/dev/null 2>&1; then
  # sql() 从 stdin 读 SQL
  sql() { sqlite3 -cmd ".timeout 30000" "$DB_BASE"; }
  echo "==> 执行通道: 宿主 sqlite3 ($(sqlite3 --version | cut -d' ' -f1))"
elif command -v docker >/dev/null 2>&1; then
  sql() {
    docker run --rm -i -v "$DB_DIR":/db -w /db alpine:3.20 sh -c '
      sed -i "s#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g" /etc/apk/repositories
      apk add --no-cache sqlite >/dev/null 2>&1 || { echo "容器内安装 sqlite 失败（可能需要网络）" >&2; exit 1; }
      exec sqlite3 -cmd ".timeout 30000" "$1"' _ "$DB_BASE"
  }
  echo "==> 执行通道: 容器内 sqlite3（alpine:3.20）"
else
  echo "错误：未找到 sqlite3，也未找到 docker。请在宿主安装 sqlite3（apt install sqlite3 / apk add sqlite）后重试。" >&2
  exit 1
fi

# ---------- 前置校验：确实是面板库 ----------
for t in users traffic_logs settings; do
  n="$(echo "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='$t';" | sql)"
  if [[ "$n" != "1" ]]; then
    echo "错误：$DB 不是有效的面板数据库（缺少表 $t）" >&2; exit 1
  fi
done
# 计费周期列是否存在（老库由 AutoMigrate 补出，正常都在）
has_col="$(echo "SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='traffic_cycle_start';" | sql)"
if [[ "$has_col" != "1" ]]; then
  echo "错误：users 表没有 traffic_cycle_start 列" >&2; exit 1
fi

# ---------- 报告：非整点用户及其修复前后用量 ----------
# 只处理形如 'YYYY-MM-DD HH:MM:SS[.frac]+00:00' 的规范文本（其余格式无法用文本前缀安全改写）
report_sql() {
  cat <<'SQL'
.mode column
.headers on
WITH unaligned AS (
  SELECT u.id,
         u.email,
         u.traffic_cycle_start AS old_start,
         substr(u.traffic_cycle_start, 1, 13) || ':00:00+00:00' AS new_start,
         u.plan_traffic_bytes AS quota
  FROM users u
  WHERE u.traffic_cycle_start IS NOT NULL
    AND u.traffic_cycle_start GLOB '????-??-?? ??:??:??*'
    AND substr(u.traffic_cycle_start, 15, 5) <> '00:00'
)
SELECT f.id,
       f.email,
       substr(f.old_start, 12, 8) AS old_hms,
       (SELECT COALESCE(SUM(l.billed_up + l.billed_down), 0) FROM traffic_logs l
          WHERE l.user_id = f.id AND l.period_start >= f.old_start) AS used_before,
       (SELECT COALESCE(SUM(l.billed_up + l.billed_down), 0) FROM traffic_logs l
          WHERE l.user_id = f.id AND l.period_start >= f.new_start) AS used_after,
       (SELECT COALESCE(SUM(l.billed_up + l.billed_down), 0) FROM traffic_logs l
          WHERE l.user_id = f.id AND l.period_start = f.new_start) AS recovered,
       f.quota,
       CASE WHEN f.quota > 0 AND (SELECT COALESCE(SUM(l.billed_up + l.billed_down), 0) FROM traffic_logs l
                 WHERE l.user_id = f.id AND l.period_start >= f.new_start) >= f.quota
            THEN '超额度!' ELSE '' END AS warn
FROM unaligned f
ORDER BY f.id;
SQL
}

echo
echo "==> 非整点周期起点的用户（修复前 → 修复后，单位：字节）"
REPORT="$(report_sql | sql)"
echo "$REPORT"
ROWS="$(echo "$REPORT" | grep -c '^ *[0-9]' || true)"

# 格式异常（非规范 UTC 文本）的用户单独列出，脚本不动它们
ODD="$(echo "SELECT COUNT(*) FROM users WHERE traffic_cycle_start IS NOT NULL AND traffic_cycle_start <> '' AND traffic_cycle_start NOT GLOB '????-??-?? ??:??:??*';" | sql)"
if [[ "$ODD" != "0" ]]; then
  echo
  echo "注意：另有 $ODD 个用户的 traffic_cycle_start 不是规范 UTC 文本（形如 2026-09-17 06:01:21+00:00），"
  echo "      脚本已跳过，不参与修复。请单独核对这些行："
  echo "SELECT id, email, quote(traffic_cycle_start) FROM users WHERE traffic_cycle_start NOT GLOB '????-??-?? ??:??:??*';" | sql
fi

if [[ "$ROWS" == "0" ]]; then
  echo
  echo "==> 无需修复：所有用户的计费周期起点都已落在整点。"
  exit 0
fi

if [[ "$APPLY" != "1" ]]; then
  echo
  echo "==> 以上为 dry-run 报告，未修改任何数据。"
  echo "    确认无误后执行：sudo bash $0 --apply"
  exit 0
fi

# ---------- 备份（与面板自身一致：VACUUM INTO 在线一致性快照，含 WAL 数据） ----------
TS="$(date +%Y%m%d-%H%M%S)"
BACKUP="$BACKUP_DIR/panel-$TS-prerepair.db"
echo
echo "==> 备份到 $BACKUP"
# 相对路径（cwd = 库所在目录）：绝对路径写进 SQL 文本在 MSYS/cygwin 下不会被翻译
if ! echo "VACUUM INTO 'backups/panel-$TS-prerepair.db';" | sql >/dev/null; then
  echo "错误：备份失败，已中止（未修改任何数据）" >&2; exit 1
fi
# 容器内 sqlite 以 root 写入时，交还宿主属主（面板以 uid 1000 运行）
OWNER="$(stat -c '%u:%g' "$DATA_DIR" 2>/dev/null || true)"
[[ -n "$OWNER" ]] && chown "$OWNER" "$BACKUP" 2>/dev/null || true
echo "    备份完成: $(ls -lh "$BACKUP" | awk '{print $5}')"

# ---------- 事务内更新 ----------
echo
echo "==> 应用修复"
cat <<'SQL' | sql
BEGIN IMMEDIATE;
UPDATE users
   SET traffic_cycle_start = substr(traffic_cycle_start, 1, 13) || ':00:00+00:00'
 WHERE traffic_cycle_start IS NOT NULL
   AND traffic_cycle_start GLOB '????-??-?? ??:??:??*'
   AND substr(traffic_cycle_start, 15, 5) <> '00:00';
SELECT '已修改行数: ' || changes();
COMMIT;
SQL

# ---------- 复验 ----------
echo
echo "==> 复验（应显示无需修复）"
AFTER="$(report_sql | sql)"
echo "$AFTER"
LEFT="$(echo "$AFTER" | grep -c '^ *[0-9]' || true)"
if [[ "$LEFT" != "0" ]]; then
  echo "错误：仍有 $LEFT 个用户未对齐，请检查" >&2; exit 1
fi

echo
echo "==> 修复完成。"
echo "    - 计费用量按新周期起点即时生效（管理端用户列表 / 用户主页 / 订阅已用量均直查，无需重启）。"
echo "    - 若本次修复使某用户跨过套餐额度，其节点账号会在下一轮用户同步（约 1 分钟）时被摘除。"
echo "    - 修复前快照：$BACKUP"
echo "      回退：停服后用该快照覆盖（面板 UI 上传恢复，或 bash restore.sh panel-$TS-prerepair.db）"
