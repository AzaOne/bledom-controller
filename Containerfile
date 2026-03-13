# --- Stage 1: Build ---
FROM --platform=$BUILDPLATFORM golang:1.25.1-trixie AS builder

RUN apt-get update && apt-get install -y git

WORKDIR /app

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./

# Try to copy pre-built binaries if they exist in the build directory.
# This is used by the GitHub Actions workflow to speed up image creation.
# We use a wildcard to avoid failure if the build directory is empty.
COPY build/bledom-controller-${TARGETOS}-${TARGETARCH}* /app/build/

RUN --mount=type=cache,target=/go/pkg/mod \
    if [ -f /app/build/bledom-controller-${TARGETOS}-${TARGETARCH} ]; then \
        echo "Pre-built binary found for ${TARGETOS}/${TARGETARCH}, skipping cache-heavy steps"; \
    else \
        go mod download; \
    fi

COPY . .

# Use cache mounts for the go build cache and module cache.
# This makes subsequent builds significantly faster.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    if [ -f /app/build/bledom-controller-${TARGETOS}-${TARGETARCH} ]; then \
        echo "Using pre-built binary for ${TARGETOS}/${TARGETARCH}"; \
        cp /app/build/bledom-controller-${TARGETOS}-${TARGETARCH} /app/bledom-controller; \
    else \
        echo "Building from source..."; \
        COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown") && \
        DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ') && \
        CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
        -ldflags="-s -w -X 'main.commit=${COMMIT}' -X 'main.date=${DATE}'" \
        -o /app/bledom-controller \
        ./cmd/agent/main.go; \
    fi

# --- Stage 2: Final Image ---
FROM debian:trixie-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    bluez \
    dbus \
    dbus-x11 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/bledom-controller .

COPY ./web ./web
COPY ./patterns ./patterns

EXPOSE 8080

CMD ["dbus-launch", "--exit-with-session", "/app/bledom-controller"]
