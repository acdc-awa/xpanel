#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== REALITY / fallback / invalid conn logs ==='
grep -iE 'reality|fallback|invalid connection' xray.log | tail -n 12
echo '=== connections around 14:18:20 (fallback test time) ==='
grep -E '14:18:(1[5-9]|2[0-5])' xray.log | grep -vE 'Xtls' | tail -n 15