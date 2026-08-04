#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== cleanup all xray + watchdog ==='
pkill -f 'watchdog.sh' 2>/dev/null || true
pkill -f 'xray run' 2>/dev/null || true
pkill -f 'xray-bin/xray' 2>/dev/null || true
sleep 1.5
pgrep -af 'xray run' || echo 'clean'
ss -tlnp | grep -E '8443|8444' || echo 'ports free'
rm -f /tmp/watchdog.log
echo '=== start watchdog ==='
nohup bash /home/zhx/XrayProject/tools/xray/watchdog.sh > /tmp/watchdog.out 2>&1 &
sleep 3
echo '--- xray up (watchdog pulled)? ---'
pgrep -af 'server.json' || echo 'NOT up'
echo '--- watchdog.log ---'
cat /tmp/watchdog.log 2>/dev/null || echo 'no log yet'
echo '=== SIGKILL xray (simulate crash) ==='
XPID=$(pgrep -f 'server.json' | head -1)
echo "killing pid=$XPID"
kill -KILL $XPID
sleep 4
echo '--- watchdog.log after crash ---'
cat /tmp/watchdog.log
echo '--- xray restarted? ---'
pgrep -af 'server.json' || echo 'NOT restarted'
ss -tlnp | grep 8443 || echo '8443 not up'
echo '=== cleanup ==='
pkill -f 'watchdog.sh' 2>/dev/null || true
sleep 0.5
pgrep -f 'server.json' > /dev/null && echo 'xray still running (watchdog off)' || echo 'no xray'