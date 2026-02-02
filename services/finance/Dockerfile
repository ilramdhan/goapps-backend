# Multi-stage Dockerfile for Finance Service
# Build from repository ROOT: docker build -t finance-service -f services/finance/Dockerfile .
# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy gen module first (needed for replace directive)
COPY gen/ ./gen/

# Copy finance service
COPY services/finance/ ./services/finance/

WORKDIR /app/services/finance

# Download dependencies
RUN go mod download

# Build arguments
ARG VERSION=dev
ARG BUILD_TIME

# Build binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o /bin/finance-service \
    ./cmd/server/main.go

# Stage 2: Runtime
FROM alpine:3.20

WORKDIR /app

# Install ca-certificates for HTTPS and tzdata for timezone
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user with specific UID/GID
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

# Copy binary from builder
COPY --from=builder /bin/finance-service /app/finance-service

# Copy config (can be overridden by ConfigMap in Kubernetes)
COPY services/finance/config.yaml /app/config.yaml

# Set ownership
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose ports
# 50051 - gRPC
# 8080 - HTTP Gateway
EXPOSE 50051 8080

# Health check using HTTP endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/healthz || exit 1

# Labels for metadata
LABEL maintainer="mutugading" \
    service="finance-service" \
    description="Finance microservice for UOM management"

ENTRYPOINT ["/app/finance-service"]

