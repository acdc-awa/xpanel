#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== app/dns recent (rtt) ==='
grep 'app/dns' xray.log | tail -n 15
echo
echo '=== total lines in log ==='
wc -l xray.log