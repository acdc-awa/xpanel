#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
XR='./xray-bin/xray'
U1=34903645-2a21-4d15-b8e4-57784431114d
SRV=192.168.5.13
pkill -f 'xray_client_tmp.json' 2>/dev/null || true
sleep 0.3
sed -e "s/34903645-2a21-4d15-b8e4-57784431114d/$U1/g" -e 's/"address": "127.0.0.1", "port": 8443/"address": "192.168.5.13", "port": 8443/' xray_client.json > xray_client_tmp.json
nohup $XR run -config xray_client_tmp.json > xray_client_tmp.log 2>&1 &
sleep 1.2
echo '=== connect via LAN IP (expect 204) ==='
curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w 'code=%{http_code} t=%{time_total}s\n' -m 15 https://www.gstatic.com/generate_204
echo '=== background 30MB download, then query online ==='
( curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -m 90 'https://speed.cloudflare.com/__down?bytes=30000000' > /dev/null 2>&1 & )
sleep 5
echo '--- statsonline test1 ---'
$XR api statsonline --server=127.0.0.1:10085 -email=test1@local 2>&1
echo '--- statsonlineiplist test1 ---'
$XR api statsonlineiplist --server=127.0.0.1:10085 -email=test1@local 2>&1
echo '--- statsgetallonlineusers ---'
$XR api statsgetallonlineusers --server=127.0.0.1:10085 2>&1