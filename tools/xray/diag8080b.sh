#!/bin/bash
echo '=== listeners on 8080 ==='
ss -tlnp | grep 8080 || echo none
echo '=== all python / openssl procs ==='
ps aux | grep -E 'python|openssl|http.server|tls8080' | grep -v grep
echo '=== try fuser ==='
fuser 8080/tcp 2>&1 || echo 'fuser: none'