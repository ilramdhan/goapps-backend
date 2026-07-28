# Multi-stage Dockerfile for PPC (Production Planning & Control) Service.
# Build from repository ROOT: docker build -t ppc-service -f services/ppc/Dockerfile .
# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies.
RUN apk add --no-cache git ca-certificates tzdata

# Copy gen module first (needed for the `replace gen => ../../gen` directive).
COPY gen/ ./gen/

# Copy shared module (needed for `replace services/shared => ../shared`; used by
# the Excel export in daily-performance).
COPY services/shared/ ./services/shared/

# Copy PPC service.
COPY services/ppc/ ./services/ppc/

WORKDIR /app/services/ppc

# Download dependencies.
RUN go mod download

# Build arguments.
ARG VERSION=dev
ARG BUILD_TIME

# Build server binary with optimizations.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o /bin/ppc-service \
    ./cmd/server/main.go

# Build seeder binary (run via K8s seed Job; infra seed-job expects /app/ppc-seed).
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /bin/ppc-seed \
    ./seeds/main.go

# Install golang-migrate (for running migrations via K8s Job).
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.1 \
    && cp $(go env GOPATH)/bin/migrate /bin/migrate

# Stage 2: Runtime
FROM alpine:3.20

WORKDIR /app

# Install ca-certificates for HTTPS and tzdata for timezone.
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user with specific UID/GID.
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

# Copy binaries from builder.
COPY --from=builder /bin/ppc-service /app/ppc-service
COPY --from=builder /bin/ppc-seed /app/ppc-seed
COPY --from=builder /bin/migrate /app/migrate

# Copy config (can be overridden by ConfigMap in Kubernetes).
COPY services/ppc/config.yaml /app/config.yaml

# Copy migration files (for running migrations via K8s Job).
COPY services/ppc/migrations/postgres/ /app/migrations/

# Set ownership.
RUN chown -R appuser:appgroup /app

# Switch to non-root user.
USER appuser

# Expose ports.
# 50053 - gRPC
# 8082  - HTTP Gateway + /metrics
EXPOSE 50053 8082

# Health check using HTTP endpoint.
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8082/healthz || exit 1

# Labels for metadata.
LABEL maintainer="mutugading" \
    service="ppc-service" \
    description="Production Planning & Control microservice"

ENTRYPOINT ["/app/ppc-service"]
