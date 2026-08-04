#!/bin/bash
echo '--- xray ---'
pgrep -af 'xray run' || echo none
echo '--- mihomo ---'
pgrep -af 'mihomo -d' || echo none
echo '--- listeners ---'
ss -tlnp | grep -E '8443|8444|7890|9090|10085'