#!/bin/bash
pkill -f 'sub_server.py' 2>/dev/null || true
pkill -f 'client_c.yaml' 2>/dev/null || true
pkill -x mihomo 2>/dev/null || true
sleep 1
echo '=== remaining ==='
pgrep -af 'sub_server|mihomo|http.server|tls8080|xray run' || echo 'all stopped'
echo '=== listeners ==='
ss -tlnp | grep -E '8090|9090|7890|8443|8444|10085' || echo 'no listeners'