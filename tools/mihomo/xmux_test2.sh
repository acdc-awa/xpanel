#!/bin/bash
cd /home/zhx/XrayProject/tools/mihomo
run_test() {
  local cfg=$1 label=$2
  pkill -f "$cfg" 2>/dev/null; pkill -x mihomo 2>/dev/null; sleep 1
  nohup ./mihomo -d . -f "$cfg" > "xmux2_${label}.log" 2>&1 &
  sleep 2
  echo "=== $label : 60 concurrent curls (500KB each) ==="
  for i in $(seq 1 60); do
    curl -x http://127.0.0.1:7890 -sS -o /dev/null -m 20 'http://speed.cloudflare.com/__down?bytes=500000' > /dev/null 2>&1 &
  done
  local peak=0
  for t in 1 2 3; do
    sleep 1
    local n=$(ss -tn state established '( sport = :8444 )' | wc -l)
    echo "  t=$t conns=$n"
    if [ "$n" -gt "$peak" ]; then peak=$n; fi
  done
  echo "[$label] PEAK conns to 8444 = $peak"
  sleep 8
  pkill -x mihomo 2>/dev/null || true
  sleep 1
}
run_test client_xmux_off.yaml OFF
run_test client_xmux_on.yaml ON
echo '=== done ==='