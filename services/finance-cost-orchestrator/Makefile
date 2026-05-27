# Finance Cost Orchestrator Service Makefile

.PHONY: help run dev build test vet lint fmt tidy docker-build docker-up docker-down

SERVICE_NAME := finance-cost-orchestrator
BINARY := bin/$(SERVICE_NAME)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
DOCKER_REGISTRY ?= ghcr.io/ilramdhan
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(SERVICE_NAME)

ifneq (,$(wildcard .env))
    include .env
    export
endif
ifneq (,$(wildcard .env.local))
    include .env.local
    export
endif

help:
	@echo "Finance Cost Orchestrator Makefile"
	@echo ""
	@echo "  make run            - Run service locally"
	@echo "  make dev            - Run with hot reload (requires air)"
	@echo "  make build          - Build binary"
	@echo "  make test           - Run all tests with -race"
	@echo "  make vet            - go vet"
	@echo "  make lint           - golangci-lint"
	@echo "  make fmt            - go fmt + goimports"
	@echo "  make tidy           - go mod tidy"
	@echo "  make docker-build   - Build Docker image"

run:
	go run ./cmd/server

dev:
	air

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(BINARY) ./cmd/server
	@echo "Built $(BINARY)"

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...
	@which goimports >/dev/null 2>&1 && goimports -w . || echo "goimports not installed"

tidy:
	go mod tidy

docker-build:
	docker build -t $(DOCKER_IMAGE):$(VERSION) -f Dockerfile ../..

docker-up:
	docker compose -f ../../docker-compose.yaml up -d

docker-down:
	docker compose -f ../../docker-compose.yaml down
