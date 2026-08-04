#!/bin/bash
BIN=/home/zhx/XrayProject/tools/xray/xray-bin/xray
CONFIG=/home/zhx/XrayProject/tools/xray/server.json
while true; do
  if ! pgrep -f 'server.json' > /dev/null 2>&1; then
    echo "$(date '+%F %T') xray down, restarting" >> /tmp/watchdog.log
    cd /home/zhx/XrayProject/tools/xray
    nohup "$BIN" run -config "$CONFIG" > xray_wd.log 2>&1 &
  fi
  sleep 2
done