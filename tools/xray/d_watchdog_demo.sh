#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
pkill -f 'watchdog.sh' 2>/dev/null || true
pkill -f 'xray run -config server.json' 2>/dev/null || true
sleep 1
rm -f /tmp/watchdog.log
echo '=== start watchdog ==='
nohup bash /home/zhx/XrayProject/tools/xray/watchdog.sh > /tmp/watchdog.out 2>&1 &
sleep 3
echo '--- xray up (watchdog pulled)? ---'
pgrep -af 'xray run -config server.json' || echo 'NOT up'
ss -tlnp | grep 8443 || echo '8443 not up'
echo '=== SIGKILL xray (simulate crash) ==='
XPID=$(pgrep -f 'xray run -config server.json' | head -1)
echo "killing pid=$XPID"
kill -KILL $XPID
sleep 4
echo '--- watchdog.log ---'
cat /tmp/watchdog.log 2>/dev/null || echo 'no log'
echo '--- xray restarted? ---'
pgrep -af 'xray run -config server.json' || echo 'NOT restarted'
ss -tlnp | grep 8443 || echo '8443 not up'
echo '=== cleanup watchdog ==='
pkill -f 'watchdog.sh' 2>/dev/null || true
echo done