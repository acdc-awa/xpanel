#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== start python TLS server on 8080 ==='
pkill -f 'tls8080.py' 2>/dev/null || true
pkill -f 's_server' 2>/dev/null || true
sleep 0.5
nohup python3 /home/zhx/XrayProject/tools/xray/tls8080.py > /tmp/tls8080.log 2>&1 &
sleep 1
echo '--- direct TLS to 8080 (control) ---'
curl -sk -o /dev/null -w 'direct8080 code=%{http_code}\n' -m 8 https://127.0.0.1:8080/
echo '--- curl TLS to 8443 (no VLESS) -> expect fallback to 8080 ---'
curl -sk --resolve www.apple.com:8443:127.0.0.1 -o /tmp/fb_out.html -w 'via8443 code=%{http_code} size=%{size_download}\n' -m 10 https://www.apple.com:8443/
echo '--- response head ---'
head -c 150 /tmp/fb_out.html 2>/dev/null; echo
echo '--- tls8080 log ---'
cat /tmp/tls8080.log
echo '=== control: VLESS client still works ==='
pkill -f 'xray_client_tmp.json' 2>/dev/null || true
sleep 0.3
sed 's/34903645-2a21-4d15-b8e4-57784431114d/34903645-2a21-4d15-b8e4-57784431114d/g' xray_client.json > xray_client_tmp.json
nohup ./xray-bin/xray run -config xray_client_tmp.json > xray_client_tmp.log 2>&1 &
sleep 1.2
curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w 'vless code=%{http_code}\n' -m 15 https://www.gstatic.com/generate_204