#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
pkill -f 'xray-bin/xray' 2>/dev/null || true
sleep 0.5
nohup ./xray-bin/xray run -config server.json > xray.log 2>&1 &
echo \$! > xray.pid
sleep 1.5
cat xray.pid
ss -tlnp | grep -E '8443|8444|10085' || true
echo '--- log tail ---'
tail -n 8 xray.log
