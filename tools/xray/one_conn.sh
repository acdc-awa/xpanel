#!/bin/bash
cd /home/zhx/XrayProject/tools/xray
echo '=== conn 3997237028 full timeline ==='
grep '3997237028' xray.log
echo
echo '=== conn 4079557863 full timeline ==='
grep '4079557863' xray.log
echo
echo '=== REALITY borrow / target related lines (last 20) ==='
grep -iE 'reality|borrow|apple|target' xray.log | tail -n 20