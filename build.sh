#!/bin/bash

SCRIPT_NAME=$(basename "$0")
FAILURES=""
OUTPUT="dist/numParser"
LDFLAGS="-s -w"

# Clean
go clean -i -r -cache

mkdir -p dist

# macOS ARM64
GOARCH="arm64"
GOOS="darwin"
BIN_FILENAME="${OUTPUT}-${GOOS}-${GOARCH}"
echo "Building ${BIN_FILENAME}..."
CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags="${LDFLAGS}" -o "${BIN_FILENAME}" ./cmd || FAILURES="${FAILURES} ${GOOS}/${GOARCH}"

# Linux AMD64
GOARCH="amd64"
GOOS="linux"
BIN_FILENAME="${OUTPUT}-${GOOS}-${GOARCH}"
echo "Building ${BIN_FILENAME}..."
CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags="${LDFLAGS}" -o "${BIN_FILENAME}" ./cmd || FAILURES="${FAILURES} ${GOOS}/${GOARCH}"

# Check errors
if [[ -n "${FAILURES}" ]]; then
  echo ""
  echo "${SCRIPT_NAME} failed on: ${FAILURES}"
  exit 1
fi

echo "Build successful!"
