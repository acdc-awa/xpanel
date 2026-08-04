#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== stop old http.server, gen self-signed cert ==='
pkill -f 'http.server 8080' 2>/dev/null || true
sleep 0.3
openssl req -x509 -newkey rsa:2048 -keyout /tmp/fb_key.pem -out /tmp/fb_cert.pem -days 1 -nodes -subj '/CN=www.apple.com' 2>/dev/null
echo '=== start TLS server on 8080 (fallback target) ==='
pkill -f 's_server -accept 8080' 2>/dev/null || true
sleep 0.3
nohup openssl s_server -accept 8080 -cert /tmp/fb_cert.pem -key /tmp/fb_key.pem -www > /tmp/fb_srv.log 2>&1 &
sleep 1
echo '=== direct TLS to 8080 (control) ==='
curl -sk -o /dev/null -w 'direct8080 code=%{http_code}\n' -m 8 https://127.0.0.1:8080/
echo '=== curl TLS to 8443 (no VLESS) -> expect fallback to 8080 TLS ==='
curl -sk --resolve www.apple.com:8443:127.0.0.1 -o /tmp/fb_out.html -w 'via8443 code=%{http_code} size=%{size_download}\n' -m 10 https://www.apple.com:8443/
echo '--- response head ---'
head -c 120 /tmp/fb_out.html 2>/dev/null; echo
echo '--- s_server log (should show a connection) ---'
tail -n 5 /tmp/fb_srv.log
echo '=== control: VLESS client still works ==='
pkill -f 'xray_client_tmp.json' 2>/dev/null || true
sleep 0.3
sed 's/34903645-2a21-4d15-b8e4-57784431114d/34903645-2a21-4d15-b8e4-57784431114d/g' xray_client.json > xray_client_tmp.json
nohup ./xray-bin/xray run -config xray_client_tmp.json > xray_client_tmp.log 2>&1 &
sleep 1.2
curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w 'vless code=%{http_code}\n' -m 15 https://www.gstatic.com/generate_204