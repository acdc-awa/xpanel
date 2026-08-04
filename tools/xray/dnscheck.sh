#!/bin/bash
echo '--- getent www.gstatic.com ---'
getent ahostsv4 www.gstatic.com || echo FAIL
echo '--- getent www.apple.com ---'
getent ahostsv4 www.apple.com || echo FAIL
echo '--- nslookup system ---'
nslookup www.gstatic.com 2>&1 | tail -n 6
echo '--- resolv.conf ---'
cat /etc/resolv.conf