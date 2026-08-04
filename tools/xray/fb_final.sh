#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== no-VLESS TLS to 8445 (HTTP/1.1) -> expect fallback 8082 -> 200 ==='
curl --http1.1 -sk -o /tmp/fb_ok.html -w 'via8445 code=%{http_code} size=%{size_download}\n' -m 10 https://127.0.0.1:8445/
echo '--- response head ---'
head -c 150 /tmp/fb_ok.html 2>/dev/null; echo
echo '--- http8082 log ---'
tail -n 2 /tmp/http8082.log
echo '--- xray fallback log ---'
grep -iE 'fallback' xray.log | tail -n 3