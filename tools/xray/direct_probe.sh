#!/bin/bash
echo '=== direct (no proxy) to gstatic x3 ==='
for i in 1 2 3; do curl -sS -o /dev/null -w "code=%{http_code} total=%{time_total}s connect=%{time_connect}s\n" -m 20 https://www.gstatic.com/generate_204; sleep 1; done
echo '=== direct to apple x2 (reality borrow target) ==='
for i in 1 2; do curl -sS -o /dev/null -w "code=%{http_code} total=%{time_total}s\n" -m 20 https://www.apple.com; sleep 1; done
echo '=== TCP RTT to fake-ip gw (198.18.0.1:443) ==='
timeout 5 bash -c 'echo > /dev/tcp/198.18.0.1/443' && echo gw-ok || echo gw-fail