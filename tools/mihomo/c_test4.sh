#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
echo '=== restart mihomo with DIRECT rule for local sub ==='
pkill -f 'client_c.yaml' 2>/dev/null || true
pkill -x mihomo 2>/dev/null || true
sleep 1
rm -f /tmp/sub_server.log
nohup ./mihomo -d . -f client_c.yaml > mihomo_c.log 2>&1 &
sleep 2
echo '--- first pull log ---'
cat /tmp/sub_server.log
echo '=== force refresh (PUT) ==='
curl -s -X PUT http://127.0.0.1:9090/providers/proxies/c-sub
echo
sleep 1
echo '--- sub_server.log after refresh (expect If-None-Match 304) ---'
cat /tmp/sub_server.log
echo '=== provider still cached ==='
curl -s http://127.0.0.1:9090/providers/proxies/c-sub | python3 -c "import sys,json; d=json.load(sys.stdin); print('proxies:', [p['name'] for p in d['proxies']]); print('subInfo:', d.get('subscriptionInfo'))"