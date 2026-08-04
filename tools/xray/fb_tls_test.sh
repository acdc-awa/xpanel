#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== restart xray ==='
bash start_xray.sh 2>&1 | grep -E 'started'
echo '=== direct TLS 8081 control ==='
curl -sk -o /dev/null -w 'direct8081 code=%{http_code}\n' -m 8 https://127.0.0.1:8081/
echo '=== no-VLESS TLS to 8445 -> expect fallback to 8081 (200) ==='
curl -sk -o /tmp/fb_tls.html -w 'via8445 code=%{http_code} size=%{size_download}\n' -m 10 https://127.0.0.1:8445/
echo '--- response head ---'
head -c 150 /tmp/fb_tls.html 2>/dev/null; echo
echo '--- tls8081 log (should show via8445 request) ---'
tail -n 3 /tmp/tls8081.log
echo '=== xray log: fallback starts? ==='
grep -iE 'fallback' xray.log | tail -n 5