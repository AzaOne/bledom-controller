# --- Stage 1: Build ---
FROM --platform=$BUILDPLATFORM golang:1.25.1-trixie AS builder

RUN apt-get update && apt-get install -y git

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
        if [ -f /app/build/bledom-controller-${TARGETOS}-${TARGETARCH} ]; then \
        echo "Not use cache by use prebuild binary"; \
        else \
        go mod download; \
        fi

COPY . .

ARG TARGETOS
ARG TARGETARCH

# Use cache mounts for the go build cache and module cache.
# This makes subsequent builds significantly faster.
# if build dir is not empty use binary from it
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    if [ -f /app/build/bledom-controller-${TARGETOS}-${TARGETARCH} ]; then \
        echo "Using pre-built binary for ${TARGETOS}/${TARGETARCH}"; \
        cp /app/build/bledom-controller-${TARGETOS}-${TARGETARCH} /app/bledom-controller; \
    elif [ -d /app/build ]; then \
        echo "Build dir exists but binary not found for ${TARGETOS}/${TARGETARCH}. Contents:"; \
        ls -l /app/build; \
        exit 1; \
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
COPY config.json .
# COPY ./schedules.json .

EXPOSE 8080

CMD ["dbus-launch", "--exit-with-session", "/app/bledom-controller"]
