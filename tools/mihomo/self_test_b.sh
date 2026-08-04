#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
pkill -f 'client_b.yaml' 2>/dev/null || true
pkill -x mihomo 2>/dev/null || true
sleep 1
nohup ./mihomo -d . -f client_b.yaml > mihomo_b.log 2>&1 &
sleep 2
for n in XHTTP-packet-up XHTTP-stream-up XHTTP-stream-one XHTTP-auto TCP-REALITY-vision; do
  echo "=== delay $n ==="
  curl -s "http://127.0.0.1:9090/proxies/$n/delay?url=http://www.gstatic.com/generate_204&timeout=8000"
  echo
done
echo '=== log tail ==='
tail -n 6 mihomo_b.log