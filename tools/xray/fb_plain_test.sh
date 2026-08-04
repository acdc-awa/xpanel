#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== start plain http.server on 8082 ==='
pkill -f 'http.server 8082' 2>/dev/null || true
sleep 0.5
(cd /tmp && nohup python3 -m http.server 8082 --bind 127.0.0.1 > /tmp/http8082.log 2>&1 &)
sleep 1
curl -s -o /dev/null -w 'direct8082 code=%{http_code}\n' -m 5 http://127.0.0.1:8082/
echo '=== restart xray ==='
bash start_xray.sh 2>&1 | grep -E 'started'
echo '=== no-VLESS TLS to 8445 -> expect fallback 8082 -> 200 ==='
curl -sk -o /tmp/fb_plain.html -w 'via8445 code=%{http_code} size=%{size_download}\n' -m 10 https://127.0.0.1:8445/
echo '--- response head ---'
head -c 150 /tmp/fb_plain.html 2>/dev/null; echo
echo '--- http8082 log ---'
tail -n 3 /tmp/http8082.log
echo '--- xray fallback log ---'
grep -iE 'fallback' xray.log | tail -n 3