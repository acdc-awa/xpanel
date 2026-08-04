#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
XR='./xray-bin/xray'
echo '=== 0. api lsi ==='
$XR api lsi --server=127.0.0.1:10085 2>&1
echo
echo '=== 1. baseline stats (user>>>) ==='
$XR api statsquery --server=127.0.0.1:10085 -pattern 'user>>>' 2>&1
echo
echo '=== 2. start xray client (socks 10808 -> test1) ==='
pkill -f 'xray_client.json' 2>/dev/null || true
sleep 0.5
nohup $XR run -config xray_client.json > xray_client.log 2>&1 &
sleep 1.5
echo '--- curl via socks (test1) ---'
curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w 'code=%{http_code} t=%{time_total}s\n' -m 15 https://www.gstatic.com/generate_204
echo
echo '=== 3. stats after (user>>>) ==='
$XR api statsquery --server=127.0.0.1:10085 -pattern 'user>>>' 2>&1
echo
echo '=== 4. inbound users ==='
$XR api inbounduser --server=127.0.0.1:10085 -tag=vless-tcp-in 2>&1
echo
echo '=== 5. server pid ==='
pgrep -f 'xray run -config server.json'