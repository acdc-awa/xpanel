#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
run_test() {
  local cfg=$1 label=$2
  pkill -f "$cfg" 2>/dev/null; pkill -x mihomo 2>/dev/null; sleep 1
  nohup ./mihomo -d . -f "$cfg" > "xmux_${label}.log" 2>&1 &
  sleep 2
  echo "=== $label : 30 concurrent curls via mixed-port ==="
  for i in $(seq 1 30); do
    curl -x http://127.0.0.1:7890 -sS -o /dev/null -m 10 'http://speed.cloudflare.com/__down?bytes=50000' > /dev/null 2>&1 &
  done
  sleep 3
  local conns=$(ss -tn state established '( sport = :8444 )' | wc -l)
  echo "[$label] established conns to 8444 = $conns"
  sleep 7
  pkill -x mihomo 2>/dev/null || true
  sleep 1
}
run_test client_xmux_off.yaml OFF
run_test client_xmux_on.yaml ON
echo '=== done ==='