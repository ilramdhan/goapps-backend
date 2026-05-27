# Multi-stage Dockerfile for Finance Cost Orchestrator Service
# Build from repository ROOT: docker build -t finance-cost-orchestrator -f services/finance-cost-orchestrator/Dockerfile .
FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

# Copy gen module (needed if/when the service imports generated proto types).
COPY gen/ ./gen/

# Copy shared pkg/costcalc module (replace target in go.mod).
COPY pkg/costcalc/ ./pkg/costcalc/

# Copy this service
COPY services/finance-cost-orchestrator/ ./services/finance-cost-orchestrator/

WORKDIR /app/services/finance-cost-orchestrator

RUN go mod download

ARG VERSION=dev
ARG BUILD_TIME

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o /bin/finance-cost-orchestrator \
    ./cmd/server

# Runtime stage
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata wget

RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

COPY --from=builder /bin/finance-cost-orchestrator /app/finance-cost-orchestrator
COPY services/finance-cost-orchestrator/config.yaml /app/config.yaml

RUN chown -R appuser:appgroup /app
USER appuser

# 8092 - Prometheus metrics + /healthz
EXPOSE 8092

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8092/healthz || exit 1

LABEL maintainer="mutugading" \
    service="finance-cost-orchestrator" \
    description="Coordinator for finance cost calculation jobs"

ENTRYPOINT ["/app/finance-cost-orchestrator"]
