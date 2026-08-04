#!/bin/bash
for u in mihomo-linux-amd64-v1.19.29.gz mihomo-linux-amd64-v1.19.0.gz mihomo-linux-amd64-v1.18.10.gz; do
  code=$(curl -sI -m 10 -o /dev/null -w '%{http_code}' "https://github.com/MetaCubeX/mihomo/releases/download/v1.19.29/$u")
  echo "$u -> $code"
done