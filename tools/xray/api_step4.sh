#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
XR='./xray-bin/xray'
UUID3=$(cat test3_uuid.txt | cut -d= -f2)
echo '=== rmu (remove test3) ==='
$XR api rmu --server=127.0.0.1:10085 -tag=vless-tcp-in test3@local 2>&1
echo '=== pid (should stay 5210) ==='
pgrep -f 'xray run -config server.json'
echo '=== inbound users ==='
$XR api inbounduser --server=127.0.0.1:10085 -tag=vless-tcp-in 2>&1 | grep -E 'email|id' 
echo '=== test3 connection (expect FAIL) ==='
pkill -f 'xray_client_tmp.json' 2>/dev/null || true
sleep 0.3
sed "s/34903645-2a21-4d15-b8e4-57784431114d/$UUID3/g" xray_client.json > xray_client_tmp.json
nohup ./xray-bin/xray run -config xray_client_tmp.json > xray_client_tmp.log 2>&1 &
sleep 1.2
curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w 'test3 code=%{http_code}\n' -m 12 https://www.gstatic.com/generate_204 || echo 'test3 connection refused as expected'
echo '=== test1 connection (expect OK) ==='
pkill -f 'xray_client_tmp.json' 2>/dev/null || true
sleep 0.3
sed "s/34903645-2a21-4d15-b8e4-57784431114d/34903645-2a21-4d15-b8e4-57784431114d/g" xray_client.json > xray_client_tmp.json
nohup ./xray-bin/xray run -config xray_client_tmp.json > xray_client_tmp.log 2>&1 &
sleep 1.2
curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w 'test1 code=%{http_code}\n' -m 12 https://www.gstatic.com/generate_204