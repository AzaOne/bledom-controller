#!/bin/bash

set -e

APP_NAME="bledom-controller"

OUTPUT_DIR="build"
MAIN_GO_FILE="./cmd/agent/main.go"

TARGETS=(
    "linux/amd64"
    "linux/arm64"
)

echo "Starting build for ${APP_NAME}..."

VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "0.0.0")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

# This creates a string like: -X 'main.version=v1.0' -X 'main.commit=a1b2c3d' ...
LDFLAGS_STRING="-s -w -X 'main.version=${VERSION}' -X 'main.commit=${COMMIT}' -X 'main.date=${DATE}'"

rm -rf ${OUTPUT_DIR}
mkdir -p ${OUTPUT_DIR}

for target in "${TARGETS[@]}"; do
    IFS='/' read -r GOOS GOARCH <<< "$target"

    echo "Building for ${GOOS}/${GOARCH}..."

    OUTPUT_NAME="${OUTPUT_DIR}/${APP_NAME}-${GOOS}-${GOARCH}"
    GOOS=${GOOS} GOARCH=${GOARCH} go build \
        -v \
        -ldflags="${LDFLAGS_STRING}" \
        -o ${OUTPUT_NAME} \
        ${MAIN_GO_FILE}
done

echo "Build complete. Binaries are in the '${OUTPUT_DIR}' directory."
ls -lh ${OUTPUT_DIR}
