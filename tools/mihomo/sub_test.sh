#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
echo '=== mihomo -t ==='
./mihomo -t -f client_sub.yaml 2>&1 | tail -n 2
echo '=== start mihomo ==='
pkill -f 'client_sub.yaml' 2>/dev/null || true
pkill -x mihomo 2>/dev/null || true
sleep 1
nohup ./mihomo -d . -f client_sub.yaml > mihomo_sub.log 2>&1 &
sleep 2
echo '=== providers ==='
curl -s http://127.0.0.1:9090/providers/proxies | head -c 400
echo
echo '=== provider node detail ==='
curl -s http://127.0.0.1:9090/providers/proxies/vless-sub | head -c 800
echo
echo '=== delay via provider node ==='
curl -s 'http://127.0.0.1:9090/proxies/VLESS-TCP-REALITY/delay?url=http://www.gstatic.com/generate_204&timeout=8000'
echo
echo '=== curl via mixed-port ==='
curl -x http://127.0.0.1:7890 -sS -o /dev/null -w 'code=%{http_code}\n' -m 15 https://www.gstatic.com/generate_204