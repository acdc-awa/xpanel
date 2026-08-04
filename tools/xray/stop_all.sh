#!/bin/bash
echo '--- before ---'
pgrep -af 'xray run|mihomo -d' || echo 'nothing running'
pkill -f 'xray run' 2>/dev/null || true
pkill -f 'mihomo -d' 2>/dev/null || true
sleep 1.5
echo '--- after ---'
pgrep -af 'xray run|mihomo -d' || echo 'all stopped'
echo '--- listeners ---'
ss -tlnp | grep -E '8443|8444|7890|9090|10085|10808' || echo 'no listeners'