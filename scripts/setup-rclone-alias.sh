#!/bin/bash

if [ $# -lt 1 ]; then
  cat <<EOF
Usage: $0 <alias-name>

This script configures an rclone alias using environment variables.

Required environment variables:
  AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY
  S3_ENDPOINT

  S3_PUBLIC_URL_PREFIX
EOF
  exit 1
fi

ALIAS_NAME="$1"

# Validate required environment variables
if [ -z "${AWS_ACCESS_KEY_ID}" ] || [ -z "${AWS_SECRET_ACCESS_KEY}" ]; then
  echo "Error: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set."
  exit 1
fi

if [ -z "${S3_ENDPOINT}" ]; then
  echo "Error: S3_ENDPOINT must be set."
  exit 1
fi

echo "Setting rclone alias '$ALIAS_NAME' with endpoint: ${S3_ENDPOINT}"
if [[ "${S3_ENDPOINT}" =~ [Cc]loudflare ]]; then
  set -x
  rclone config create "${ALIAS_NAME}" s3 \
    provider Cloudflare \
    access_key_id "${AWS_ACCESS_KEY_ID}" \
    secret_access_key "${AWS_SECRET_ACCESS_KEY}" \
    endpoint "${S3_ENDPOINT}" \
    acl private
else
  set -x
  rclone config create "${ALIAS_NAME}" s3 \
    provider AWS \
    access_key_id "${AWS_ACCESS_KEY_ID}" \
    secret_access_key "${AWS_SECRET_ACCESS_KEY}" \
    endpoint "${S3_ENDPOINT}"
fi

if [ $? -eq 0 ]; then
  echo "rclone alias '$ALIAS_NAME' set successfully."
else
  echo "Error: Failed to set rclone alias '$ALIAS_NAME'."
  exit 1
fi
