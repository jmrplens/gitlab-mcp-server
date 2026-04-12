#!/usr/bin/env bash
# obfuscate-token.sh -- XOR-obfuscate a token for embedding in Go binaries.
# Usage: scripts/obfuscate-token.sh <TOKEN>
# Output: two lines: OBFUSCATED_TOKEN and OBFUSCATION_KEY (for use as ldflags)
set -euo pipefail

if [[ $# -ne 1 ]] || [[ -z "$1" ]]; then
    echo "Usage: $0 <TOKEN>" >&2
    exit 1
fi

TOKEN="$1"
LEN=${#TOKEN}

# Generate a random key of the same length as the token.
KEY_BYTES=$(head -c "$LEN" /dev/urandom | od -An -tx1 | tr -d ' \n')

# XOR the token with the key and produce hex output.
TOKEN_HEX=$(printf '%s' "$TOKEN" | od -An -tx1 | tr -d ' \n')

# Perform XOR in pure bash (portable).
CIPHER_HEX=""
for ((i = 0; i < ${#TOKEN_HEX}; i += 2)); do
    T_BYTE="0x${TOKEN_HEX:$i:2}"
    K_BYTE="0x${KEY_BYTES:$i:2}"
    XOR=$(printf '%02x' $(( T_BYTE ^ K_BYTE )))
    CIPHER_HEX="${CIPHER_HEX}${XOR}"
done

echo "OBFUSCATED_TOKEN=${CIPHER_HEX}"
echo "OBFUSCATION_KEY=${KEY_BYTES}"
