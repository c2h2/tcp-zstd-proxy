#!/bin/bash
set -e

# Build for Linux amd64
echo "Building Linux/amd64..."
GOOS=linux GOARCH=amd64 go build -o build/tcp_zstd_proxy .

# Build for Linux ARM (ARMv8)
echo "Building Linux/ARM..."
GOOS=linux GOARCH=arm64 go build -o build/tcp_zstd_proxy_armv8 .

echo "Builds complete."
