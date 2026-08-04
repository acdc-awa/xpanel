#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
pkill -f 'xray_client.json' 2>/dev/null || true
sleep 0.5
nohup ./xray-bin/xray run -config xray_client.json > xray_client.log 2>&1 &
sleep 1.5
echo '=== curl via xray client (socks 10808 -> vless+reality -> 8443) ==='
curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w 'http_code=%{http_code}\n' -m 15 https://www.gstatic.com/generate_204
echo '=== xray client log tail ==='
tail -n 15 xray_client.log
echo '=== server log tail ==='
tail -n 6 xray.log