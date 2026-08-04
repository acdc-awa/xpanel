#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== SIGTERM (graceful) ==='
./xray-bin/xray run -config server.json > xray_sig.log 2>&1 &
XPID=$!
sleep 1.5
echo "pid=$XPID sending SIGTERM"
kill -TERM $XPID
wait $XPID
echo "SIGTERM exit code = $?"
echo '--- log tail ---'
tail -n 4 xray_sig.log
echo '--- ports ---'
ss -tlnp | grep -E '8443|8444|10085' || echo 'ports released'
echo
echo '=== SIGKILL (crash sim) ==='
./xray-bin/xray run -config server.json > xray_sig2.log 2>&1 &
XPID=$!
sleep 1.5
echo "pid=$XPID sending SIGKILL"
kill -KILL $XPID
wait $XPID
echo "SIGKILL exit code = $?"
echo '--- ports ---'
ss -tlnp | grep -E '8443|8444|10085' || echo 'ports released'