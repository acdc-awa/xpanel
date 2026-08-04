#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
rm -f /tmp/sub_server.log
echo '=== start sub server on 8090 ==='
pkill -f 'sub_server.py' 2>/dev/null || true
sleep 0.5
nohup python3 /home/zhx/XrayProject/tools/mihomo/sub_server.py > /tmp/sub_server.out 2>&1 &
sleep 1
echo '--- direct fetch /sub (control) ---'
curl -s -D /tmp/sub_headers.txt -o /tmp/sub_body.yaml http://127.0.0.1:8090/sub
cat /tmp/sub_headers.txt
echo '--- body head ---'
head -3 /tmp/sub_body.yaml
echo '=== start mihomo ==='
pkill -f 'client_c.yaml' 2>/dev/null || true
pkill -x mihomo 2>/dev/null || true
sleep 1
nohup ./mihomo -d . -f client_c.yaml > mihomo_c.log 2>&1 &
sleep 2
echo '=== provider detail ==='
curl -s http://127.0.0.1:9090/providers/proxies/c-sub | python3 -m json.tool 2>/dev/null | head -40
echo '=== sub_server.log (UA / If-None-Match) ==='
cat /tmp/sub_server.log 2>/dev/null || echo 'no log'