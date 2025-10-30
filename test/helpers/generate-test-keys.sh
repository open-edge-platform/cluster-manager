#!/bin/bash
# SPDX-FileCopyrightText: (C) 2025 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

# Generate RSA key pair for testing JWT tokens
# This script generates keys dynamically to avoid committing private keys to the repository
# Uses only openssl and standard shell tools (no Python required)

set -e

KEY_DIR="${KEY_DIR:-/tmp/cluster-manager-test-keys}"
PRIVATE_KEY_FILE="${KEY_DIR}/test-private-key.pem"
PUBLIC_KEY_FILE="${KEY_DIR}/test-public-key.pem"
JWK_FILE="${KEY_DIR}/test-jwk.json"
KID_FILE="${KEY_DIR}/test-kid.txt"

# Create directory if it doesn't exist
mkdir -p "${KEY_DIR}"

# Check if keys already exist
if [ -f "${PRIVATE_KEY_FILE}" ] && [ -f "${PUBLIC_KEY_FILE}" ] && [ -f "${JWK_FILE}" ] && [ -f "${KID_FILE}" ]; then
    echo "Test keys already exist in ${KEY_DIR}"
    exit 0
fi

echo "Generating test RSA key pair..."

# Generate 2048-bit RSA private key
openssl genrsa -out "${PRIVATE_KEY_FILE}" 2048 2>/dev/null

# Extract public key
openssl rsa -in "${PRIVATE_KEY_FILE}" -pubout -out "${PUBLIC_KEY_FILE}" 2>/dev/null

echo "Generating JWK (JSON Web Key)..."

# Extract modulus (n) from the public key
# The modulus is in hex format, we need to convert it to base64url
MODULUS_HEX=$(openssl rsa -pubin -in "${PUBLIC_KEY_FILE}" -noout -modulus 2>/dev/null | sed 's/Modulus=//')

# Convert hex to binary to base64url (without padding, with URL-safe chars)
# Important: remove newlines with tr -d '\n' to create a single-line base64url string
MODULUS_BASE64URL=$(echo "${MODULUS_HEX}" | xxd -r -p | base64 | tr -d '\n' | tr '+/' '-_' | tr -d '=')

# The public exponent is typically 65537 (0x010001) which in base64url is "AQAB"
EXPONENT_BASE64URL="AQAB"

# Use a dynamic kid with timestamp to force JWKS cache refresh
TIMESTAMP=$(date +%s)
KID="test-key-${TIMESTAMP}"

# Create JWK JSON
cat > "${JWK_FILE}" << EOF
{
  "keys": [
    {
      "kty": "RSA",
      "kid": "${KID}",
      "use": "sig",
      "alg": "PS512",
      "n": "${MODULUS_BASE64URL}",
      "e": "${EXPONENT_BASE64URL}"
    }
  ]
}
EOF

# Save the kid for use by test code
echo "${KID}" > "${KID_FILE}"

echo "Test keys generated successfully:"
echo "  Private key: ${PRIVATE_KEY_FILE}"
echo "  Public key:  ${PUBLIC_KEY_FILE}"
echo "  JWK:         ${JWK_FILE}"
echo ""
echo "JWK content:"
cat "${JWK_FILE}"
