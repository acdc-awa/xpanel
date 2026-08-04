#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
UUID=$1
pkill -f 'xray_client_tmp.json' 2>/dev/null || true
sleep 0.3
sed "s/34903645-2a21-4d15-b8e4-57784431114d/$UUID/g" xray_client.json > xray_client_tmp.json
nohup ./xray-bin/xray run -config xray_client_tmp.json > xray_client_tmp.log 2>&1 &
sleep 1.2
curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w "uuid=$UUID code=%{http_code} t=%{time_total}s\n" -m 15 https://www.gstatic.com/generate_204