# Multi-stage Dockerfile for IAM Service
# Build from repository ROOT: docker build -t iam-service -f services/iam/Dockerfile .
# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy gen module first (needed for replace directive)
COPY gen/ ./gen/

# Copy IAM service
COPY services/iam/ ./services/iam/

WORKDIR /app/services/iam

# Download dependencies
RUN go mod download

# Build arguments
ARG VERSION=dev
ARG BUILD_TIME

# Build binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o /bin/iam-service \
    ./cmd/server/main.go

# Build seeder binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /bin/iam-seed \
    ./seeds/main.go

# Install golang-migrate (for running migrations via K8s Job)
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.1 \
    && cp $(go env GOPATH)/bin/migrate /bin/migrate

# Stage 2: Runtime
FROM alpine:3.20

WORKDIR /app

# Install ca-certificates for HTTPS and tzdata for timezone
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user with specific UID/GID
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

# Copy binaries from builder
COPY --from=builder /bin/iam-service /app/iam-service
COPY --from=builder /bin/iam-seed /app/iam-seed
COPY --from=builder /bin/migrate /app/migrate

# Copy config (can be overridden by ConfigMap in Kubernetes)
COPY services/iam/config.yaml /app/config.yaml

# Copy migration files (for running migrations via K8s Job)
COPY services/iam/migrations/postgres/ /app/migrations/

# Set ownership
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose ports
# 50052 - gRPC
# 8081 - HTTP Gateway
EXPOSE 50052 8081

# Health check using HTTP endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8081/healthz || exit 1

# Labels for metadata
LABEL maintainer="mutugading" \
    service="iam-service" \
    description="IAM microservice for authentication and authorization"

ENTRYPOINT ["/app/iam-service"]
