#!/bin/bash
pkill -f 'http.server 8082' 2>/dev/null || true
pkill -f 'tls8080.py' 2>/dev/null || true
pkill -f 'xray_client_tmp.json' 2>/dev/null || true
pkill -f 'xray_client.json' 2>/dev/null || true
pkill -f 'client_sub.yaml' 2>/dev/null || true
pkill -x mihomo 2>/dev/null || true
sleep 1
echo '=== remaining relevant procs ==='
pgrep -af 'xray run|mihomo|http.server|tls8080' || echo 'none (xray server may remain)'
echo '=== listeners ==='
ss -tlnp | grep -E '8443|8444|10085|7890|9090|8081|8082' || echo none