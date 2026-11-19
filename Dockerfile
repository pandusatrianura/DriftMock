# syntax=docker/dockerfile:1

# ==============================================================================
# Stage 1: Build binary
# ==============================================================================
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install build dependencies and CA certs
RUN apk add --no-cache git ca-certificates tzdata

# Cache dependency layers
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build static Linux binary with debug info stripped
ARG VERSION=v1.0.0
ARG COMMIT=docker
ARG BUILD_DATE=now

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w -X github.com/pandusatrianura/driftmock/internal/cli.Version=${VERSION} -X github.com/pandusatrianura/driftmock/internal/cli.Commit=${COMMIT} -X github.com/pandusatrianura/driftmock/internal/cli.BuildDate=${BUILD_DATE}" \
    -o /build/bin/driftmock \
    ./cmd/driftmock

# ==============================================================================
# Stage 2: Minimal runtime image
# ==============================================================================
FROM alpine:3.21 AS runtime

# Install CA certificates and curl for health probes
RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -g 10001 -S driftgroup && \
    adduser -u 10001 -S driftuser -G driftgroup

WORKDIR /app

# Create default stubs directory owned by non-root user
RUN mkdir -p /app/.drift/stubs && \
    chown -R driftuser:driftgroup /app

# Copy binary from builder
COPY --from=builder --chown=driftuser:driftgroup /build/bin/driftmock /usr/local/bin/driftmock

# 12-Factor default environment variables
ENV DRIFTMOCK_PORT=8080 \
    DRIFTMOCK_STUBS_DIR=/app/.drift \
    DRIFTMOCK_LOG_FORMAT=json

# Run as non-root user
USER 10001:10001

EXPOSE 8080

# Built-in healthcheck probe using /healthz endpoint
HEALTHCHECK --interval=10s --timeout=3s --start-period=3s --retries=3 \
    CMD curl -f http://localhost:8080/healthz || exit 1

# Default execution: serve recorded mocks offline
ENTRYPOINT ["driftmock"]
CMD ["replay"]
