#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
echo '=== force refresh provider (PUT) ==='
curl -s -X PUT http://127.0.0.1:9090/providers/proxies/c-sub
echo
sleep 1
echo '=== sub_server.log after refresh (expect If-None-Match -> 304) ==='
cat /tmp/sub_server.log
echo '=== provider nodes still present (cache used) ==='
curl -s http://127.0.0.1:9090/providers/proxies/c-sub | python3 -c "import sys,json; d=json.load(sys.stdin); print('proxies:', [p['name'] for p in d['proxies']]); print('subInfo:', d.get('subscriptionInfo'))"
echo '=== mihomo log (etag/cache related) ==='
grep -iE 'etag|304|cache|provider' mihomo_c.log | tail -n 8