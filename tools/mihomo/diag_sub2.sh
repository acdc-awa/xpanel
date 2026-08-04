#!/bin/bash
echo '=== sub_server process ==='
pgrep -af 'sub_server.py' || echo 'NOT RUNNING'
echo '=== port 8090 ==='
ss -tlnp | grep 8090 || echo 'no listener'
echo '=== curl direct ==='
curl -s -o /dev/null -w 'code=%{http_code}\n' -m 5 http://127.0.0.1:8090/sub
echo '=== mihomo log tail ==='
tail -n 12 /home/zhx/XrayProject/tools/mihomo/mihomo_c.log
echo '=== mihomo log grep provider/8090 ==='
grep -E '8090|provider|PROXY' /home/zhx/XrayProject/tools/mihomo/mihomo_c.log | tail -n 8