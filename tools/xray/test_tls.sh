#!/bin/bash
set -e
echo | timeout 8 openssl s_client -connect www.microsoft.com:443 -servername www.microsoft.com -tls1_3 2>/dev/null | grep -E 'Protocol|Cipher'
echo rc=\True
