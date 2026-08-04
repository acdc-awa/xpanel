#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
XR='./xray-bin/xray'
U1=34903645-2a21-4d15-b8e4-57784431114d
pkill -f 'xray_client_tmp.json' 2>/dev/null || true
sleep 0.3
sed "s/34903645-2a21-4d15-b8e4-57784431114d/$U1/g" xray_client.json > xray_client_tmp.json
nohup $XR run -config xray_client_tmp.json > xray_client_tmp.log 2>&1 &
sleep 1.2
echo '=== Step6: stats before download ==='
$XR api statsquery --server=127.0.0.1:10085 -pattern 'user>>>test1@local' 2>&1
echo '=== download 5MB via socks ==='
curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w 'dl code=%{http_code} t=%{time_total}s size=%{size_download}\n' -m 30 'https://speed.cloudflare.com/__down?bytes=5000000'
echo '=== stats immediately after ==='
$XR api statsquery --server=127.0.0.1:10085 -pattern 'user>>>test1@local' 2>&1
sleep 3
echo '=== stats after 3s ==='
$XR api statsquery --server=127.0.0.1:10085 -pattern 'user>>>test1@local' 2>&1
echo '=== Step7: online stats (background 20MB download) ==='
( curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -m 60 'https://speed.cloudflare.com/__down?bytes=20000000' > /dev/null 2>&1 & )
sleep 3
echo '--- statsonline test1 ---'
$XR api statsonline --server=127.0.0.1:10085 -email=test1@local 2>&1
echo '--- statsgetallonlineusers ---'
$XR api statsgetallonlineusers --server=127.0.0.1:10085 2>&1