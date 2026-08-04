#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== last 40 relevant log lines with timestamps ==='
grep -E 'REALITY: processed|freedom: dialing|blocked target|from tcp:|tunneling request|splice|invalid connection' xray.log | tail -n 40