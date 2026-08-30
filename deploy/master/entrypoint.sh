#!/bin/sh
#
# 容器 PID 1 包装：master 原生启动（无额外守护进程）。
#
# 回滚语义：仅当「更新待确认（/app/.update-pending 存在）且新版本启动失败（退出码非 0）」时，
# 用 /app/master.prev + /app/web.prev 回滚上一版本并重新启动；随后清除标记。
# 正常生命周期不触发回滚：更新自杀/SIGTERM 优雅退出均为 exit 0；管理员 docker stop 亦为
# SIGTERM → exit 0。新版本成功启动后由 master 自身清理 update-pending 与 .prev 备份。
set -u

APP=/app/master
CFG=/app/configs/config.yaml

start_app() {
  if [ -n "${1:-}" ]; then
    "$APP" "$@"
  else
    "$APP" -config "$CFG"
  fi
  return $?
}

start_app "$@"
code=$?
if [ "$code" -ne 0 ] && [ -f /app/.update-pending ] && [ -f /app/master.prev ]; then
  echo "[entrypoint] master 启动失败（code=$code）且存在待确认更新，回滚上一版本" >&2
  cp /app/master.prev "$APP"
  rm -f /app/master.prev
  if [ -d /app/web.prev ]; then
    rm -rf /app/web/dist
    cp -r /app/web.prev /app/web/dist
    rm -rf /app/web.prev
  fi
  rm -f /app/.update-pending
  start_app "$@"
  code=$?
fi
exit $code