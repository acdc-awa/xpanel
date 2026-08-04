#!/bin/bash
set -e
cd /home/zhx/XrayProject/tools/mihomo
if [ ! -f mihomo ]; then
  wget -q https://github.com/MetaCubeX/mihomo/releases/download/v1.19.29/mihomo-linux-amd64-v1.19.29.gz
  gzip -dc mihomo-linux-amd64-v1.19.29.gz > mihomo
  chmod +x mihomo
fi
./mihomo -v