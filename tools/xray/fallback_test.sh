#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== start local http server :8080 (fallback target) ==='
pkill -f 'http.server 8080' 2>/dev/null || true
sleep 0.3
cd /tmp && nohup python3 -m http.server 8080 --bind 127.0.0.1 > /tmp/http8080.log 2>&1 &
sleep 1
echo '--- direct 8080 check ---'
curl -s -o /dev/null -w 'code=%{http_code}\n' http://127.0.0.1:8080/
echo '=== restart xray (with fallbacks) ==='
bash start_xray.sh 2>&1 | grep -E 'started'
echo '=== test: no-VLESS TLS request to 8443 (SNI www.apple.com) -> expect fallback to 8080 ==='
curl -sk --resolve www.apple.com:8443:127.0.0.1 -o /tmp/fallback.html -w 'code=%{http_code} size=%{size_download}\n' -m 10 https://www.apple.com:8443/
echo '--- fallback content head ---'
head -c 200 /tmp/fallback.html
echo
echo '=== control: VLESS client still works ==='
pkill -f 'xray_client_tmp.json' 2>/dev/null || true
sleep 0.3
sed 's/34903645-2a21-4d15-b8e4-57784431114d/34903645-2a21-4d15-b8e4-57784431114d/g' xray_client.json > xray_client_tmp.json
nohup ./xray-bin/xray run -config xray_client_tmp.json > xray_client_tmp.log 2>&1 &
sleep 1.2
curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w 'vless code=%{http_code}\n' -m 15 https://www.gstatic.com/generate_204
echo '=== xray log tail (fallback related) ==='
tail -n 8 xray.log