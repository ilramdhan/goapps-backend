# Multi-stage Dockerfile for Finance Cost Worker Service
# Build from repository ROOT: docker build -t finance-cost-worker -f services/finance-cost-worker/Dockerfile .
FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY gen/ ./gen/
COPY pkg/costcalc/ ./pkg/costcalc/
COPY services/finance-cost-worker/ ./services/finance-cost-worker/

WORKDIR /app/services/finance-cost-worker

RUN go mod download

ARG VERSION=dev
ARG BUILD_TIME

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o /bin/finance-cost-worker \
    ./cmd/server

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata wget

RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

COPY --from=builder /bin/finance-cost-worker /app/finance-cost-worker
COPY services/finance-cost-worker/config.yaml /app/config.yaml

RUN chown -R appuser:appgroup /app
USER appuser

# 8093 - Prometheus metrics + /healthz
EXPOSE 8093

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8093/healthz || exit 1

LABEL maintainer="mutugading" \
    service="finance-cost-worker" \
    description="Worker that consumes calc chunks for finance cost calculation"

ENTRYPOINT ["/app/finance-cost-worker"]
