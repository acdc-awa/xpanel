#!/bin/bash
echo '=== mihomo delay x3 (TCP node) ==='
for i in 1 2 3; do curl -s "http://127.0.0.1:9090/proxies/VLESS-TCP-REALITY-vision/delay?url=http://www.gstatic.com/generate_204&timeout=10000"; echo; sleep 1; done
echo '=== curl via mihomo x3 ==='
for i in 1 2 3; do curl -x http://127.0.0.1:7890 -sS -o /dev/null -w "code=%{http_code} total=%{time_total}s connect=%{time_connect}s\n" -m 15 https://www.gstatic.com/generate_204; sleep 1; done
echo '=== curl via xray-client(socks10808) x3 ==='
for i in 1 2 3; do curl -x socks5h://127.0.0.1:10808 -sS -o /dev/null -w "code=%{http_code} total=%{time_total}s connect=%{time_connect}s\n" -m 15 https://www.gstatic.com/generate_204; sleep 1; done