#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
XR='./xray-bin/xray'
UUID3=$(cat test3_uuid.txt | cut -d= -f2)
cat > user_add.json <<EOF
{
  "inbounds": [
    {
      "tag": "vless-tcp-in",
      "port": 8443,
      "protocol": "vless",
      "settings": {
        "clients": [
          { "id": "$UUID3", "email": "test3@local", "flow": "xtls-rprx-vision", "level": 0 }
        ]
      }
    }
  ]
}
EOF
echo '=== adu ==='
$XR api adu --server=127.0.0.1:10085 user_add.json 2>&1
echo '=== pid (should stay 5210) ==='
pgrep -f 'xray run -config server.json'
echo '=== inbound users ==='
$XR api inbounduser --server=127.0.0.1:10085 -tag=vless-tcp-in 2>&1