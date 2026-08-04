#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== baseline xray log lines ==='
wc -l < xray.log
( curl -sk --resolve www.apple.com:8443:127.0.0.1 -o /tmp/fb_out2.html -w 'curl code=%{http_code}\n' -m 8 https://www.apple.com:8443/ > /tmp/curl_fb.out 2>&1 & )
sleep 2
echo '=== xray outbound conns during request (apple 443 / 8081) ==='
ss -tn | grep -E '(:443|:8081)' | head -10 || echo none
echo '=== new xray log lines during request ==='
tail -n +$(($(wc -l < xray.log)-0)) xray.log | tail -n 30 | grep -vE 'Xtls' 
echo '=== curl result ==='
cat /tmp/curl_fb.out