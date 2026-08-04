#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
rm -f /tmp/sub_server.log
pkill -f 'client_c.yaml' 2>/dev/null || true
pkill -x mihomo 2>/dev/null || true
sleep 1
nohup ./mihomo -d . -f client_c.yaml > mihomo_c.log 2>&1 &
sleep 2
echo '--- first pull (sub_server.log) ---'
cat /tmp/sub_server.log 2>/dev/null || echo 'no log yet'
echo '=== force refresh (PUT) ==='
curl -s -X PUT http://127.0.0.1:9090/providers/proxies/c-sub
echo
sleep 1
echo '--- sub_server.log after refresh ---'
cat /tmp/sub_server.log 2>/dev/null
echo '=== provider still cached ==='
curl -s http://127.0.0.1:9090/providers/proxies/c-sub | python3 -c "import sys,json; d=json.load(sys.stdin); print('proxies:', [p['name'] for p in d['proxies']]); print('subInfo:', d.get('subscriptionInfo'))"
echo '=== mihomo pull errors? ==='
grep -iE 'pull error|8090' mihomo_c.log | tail -n 5