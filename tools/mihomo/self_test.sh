#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
pkill -f 'mihomo -d . -f client.yaml' 2>/dev/null || true
pkill -x mihomo 2>/dev/null || true
sleep 1
nohup ./mihomo -d . -f client.yaml > mihomo.log 2>&1 &
echo $! > mihomo.pid
sleep 2
echo '=== delay TCP+REALITY+vision ==='
curl -s 'http://127.0.0.1:9090/proxies/VLESS-TCP-REALITY-vision/delay?url=http://www.gstatic.com/generate_204&timeout=5000'
echo
echo '=== delay XHTTP+REALITY packet-up ==='
curl -s 'http://127.0.0.1:9090/proxies/VLESS-XHTTP-REALITY-packet-up/delay?url=http://www.gstatic.com/generate_204&timeout=5000'
echo
echo '=== curl via mixed-port (PROXY group -> TCP vision) ==='
curl -x http://127.0.0.1:7890 -sS -o /dev/null -w 'http_code=%{http_code}\n' -m 15 https://www.gstatic.com/generate_204
echo '=== connections ==='
curl -s http://127.0.0.1:9090/connections | head -c 400
echo
echo '=== mihomo log tail ==='
tail -n 12 mihomo.log