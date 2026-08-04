#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
echo '=== restart sub server (new content/etag) ==='
pkill -f 'sub_server.py' 2>/dev/null || true
sleep 0.5
nohup python3 /home/zhx/XrayProject/tools/mihomo/sub_server.py > /tmp/sub_server.out 2>&1 &
sleep 1
echo '=== force refresh -> expect 200 (etag changed v2) ==='
curl -s -X PUT http://127.0.0.1:9090/providers/proxies/c-sub
echo
sleep 1
echo '--- sub_server.log ---'
cat /tmp/sub_server.log 2>/dev/null || echo 'no log'
echo '=== provider nodes (loaded despite session-id-placement) ==='
curl -s http://127.0.0.1:9090/providers/proxies/c-sub | python3 -c "import sys,json; d=json.load(sys.stdin); print('proxies:', [p['name'] for p in d['proxies']]); print('updatedAt:', d['updatedAt'])"
echo '=== mihomo warnings about unknown field? ==='
grep -iE 'warn|unknown|session|ignore' mihomo_c.log | tail -n 5