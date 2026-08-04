#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
echo '=== curl with If-None-Match (expect 304) ==='
curl -s -D - -o /dev/null -H 'If-None-Match: "v1-abc123"' -m 5 http://127.0.0.1:8090/sub
echo
echo '=== mihomo_c.log tail (full) ==='
tail -n 20 mihomo_c.log
echo '=== PUT refresh again, then check sub_server.log ==='
curl -s -X PUT http://127.0.0.1:9090/providers/proxies/c-sub
echo
sleep 1
echo '--- sub_server.log ---'
cat /tmp/sub_server.log