# --- Stage 1: Build ---
FROM --platform=$BUILDPLATFORM golang:1.25.1-bookworm AS builder

RUN apt-get update && apt-get install -y gcc libdbus-1-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

# Use cache mounts for the go build cache and module cache.
# This makes subsequent builds significantly faster.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w" \
    -o /app/bledom-controller \
    ./cmd/agent/main.go

# --- Stage 2: Final Image ---
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    bluez \
    dbus \
    dbus-x11 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/bledom-controller .

COPY ./static ./static
COPY ./patterns ./patterns
COPY config.json .
# COPY ./schedules.json .

EXPOSE 8080

CMD ["dbus-launch", "--exit-with-session", "/app/bledom-controller"]
