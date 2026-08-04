#!/bin/bash
echo '=== sub_server process ==='
pgrep -af 'sub_server.py' || echo 'NOT RUNNING'
echo '=== sub_server.out ==='
cat /tmp/sub_server.out 2>/dev/null || echo 'no out'
echo '=== port 8090 ==='
ss -tlnp | grep 8090 || echo 'no listener'
echo '=== direct curl again ==='
curl -s -o /dev/null -w 'code=%{http_code}\n' -m 5 http://127.0.0.1:8090/sub || echo 'curl failed'