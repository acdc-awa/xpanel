#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== tmp client uuid (should be test3) ==='
grep -E 'id|port' xray_client_tmp.json | head -4
echo '=== server log: recent user connections ==='
grep -E 'user>>>|received request|test[123]' xray.log | tail -n 12
echo '=== stats query user>>> (does test3 still count?) ==='
./xray-bin/xray api statsquery --server=127.0.0.1:10085 -pattern 'user>>>test3' 2>&1