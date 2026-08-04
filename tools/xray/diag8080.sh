#!/bin/bash
echo '=== port 8080 ==='
ss -tlnp | grep 8080 || echo 'no listener on 8080'
echo '=== fb_srv.log ==='
head -n 25 /tmp/fb_srv.log 2>/dev/null || echo 'no log'
echo '=== processes ==='
pgrep -af 's_server|http.server' || echo none