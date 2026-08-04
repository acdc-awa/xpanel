#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
XR='./xray-bin/xray'
echo '=== generate test3 uuid ==='
UUID3=$($XR uuid)
echo "test3 uuid = $UUID3"
# 写 user_add.json
cat > user_add.json <<EOF
{
  "tag": "vless-tcp-in",
  "protocol": "vless",
  "settings": {
    "clients": [
      { "id": "$UUID3", "email": "test3@local", "flow": "xtls-rprx-vision", "level": 0 }
    ]
  }
}
EOF
echo '--- user_add.json ---'
cat user_add.json
echo '=== adu (add user) ==='
$XR api adu --server=127.0.0.1:10085 user_add.json 2>&1
echo '=== pid before/after ==='
pgrep -f 'xray run -config server.json'
echo '=== inbound users after ==='
$XR api inbounduser --server=127.0.0.1:10085 -tag=vless-tcp-in 2>&1
echo "UUID3=$UUID3" > /home/zhx/XrayProject/tools/xray/test3_uuid.txt