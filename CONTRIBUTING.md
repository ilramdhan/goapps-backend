# Contributing to goapps-backend

Thank you for your interest in contributing to `goapps-backend`! This document contains guidelines for contributing to the backend microservices repository.

---

## 📋 Table of Contents

1. [Getting Started](#getting-started)
2. [Development Environment](#development-environment)
3. [Contribution Workflow](#contribution-workflow)
4. [Pull Request Guidelines](#pull-request-guidelines)
5. [Code Review Process](#code-review-process)
6. [Testing Requirements](#testing-requirements)
7. [Documentation Standards](#documentation-standards)
8. [Commit Message Conventions](#commit-message-conventions)
9. [Adding a New Service](#adding-a-new-service)
10. [Getting Help](#getting-help)

---

## Getting Started

### Prerequisites

Before contributing, make sure you have:

1. **Go 1.24+** - [Download](https://go.dev/dl/)
2. **Docker & Docker Compose** - For local development
3. **Buf CLI** - Protocol buffer management
4. **golangci-lint v2.3.0** - Code linting
5. **golang-migrate** - Database migrations
6. **grpcurl** - gRPC testing
7. **VSCode** - Recommended editor with Go extension

### Install Tools

```bash
# Install Go (if not installed)
# See: https://go.dev/doc/install

# Install Buf CLI
go install github.com/bufbuild/buf/cmd/buf@latest

# Install golangci-lint
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.3.0

# Install golang-migrate
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Install grpcurl
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

### Clone Repository

```bash
# Clone with SSH
git clone git@github.com:mutugading/goapps-backend.git
cd goapps-backend

# Or with HTTPS
git clone https://github.com/mutugading/goapps-backend.git
cd goapps-backend
```

### VSCode Extensions

Recommended extensions for Go development:

```json
{
  "recommendations": [
    "golang.go",
    "zxh404.vscode-proto3",
    "ms-azuretools.vscode-docker",
    "streetsidesoftware.code-spell-checker"
  ]
}
```

---

## Development Environment

### Start Local Infrastructure

```bash
cd services/finance

# Start PostgreSQL and Redis
docker compose -f deployments/docker-compose.yaml up -d postgres redis

# Verify services are running
docker compose -f deployments/docker-compose.yaml ps
```

### Run Migrations

```bash
export DATABASE_URL="postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable"
make finance-migrate
```

### Run Service Locally

```bash
# From repository root
make finance-run

# Or from service directory
cd services/finance
go run cmd/server/main.go
```

### Verify Service

```bash
# Check health
curl http://localhost:8080/healthz

# List gRPC services
grpcurl -plaintext localhost:50051 list

# Test gRPC method
grpcurl -plaintext -d '{"code": "KG", "name": "Kilogram"}' \
  localhost:50051 finance.v1.UOMService/CreateUOM
```

---

## Contribution Workflow

### 1. Create Issue (Recommended)

For major changes, create an issue first using available templates:

| Template | Usage |
|----------|-------|
| [🐛 Bug Report](.github/ISSUE_TEMPLATE/bug_report.md) | Report bugs |
| [✨ Feature Request](.github/ISSUE_TEMPLATE/feature_request.md) | Request features |
| [🚀 New Service](.github/ISSUE_TEMPLATE/new_service.md) | Request new microservice |

### 2. Create Feature Branch

```bash
# Update main branch
git checkout main
git pull origin main

# Create feature branch
git checkout -b <type>/<service>/<description>

# Examples:
git checkout -b feat/finance/add-uom-export
git checkout -b fix/finance/validate-uom-code
git checkout -b refactor/finance/simplify-repository
```

### 3. Make Changes

Follow these steps while developing:

```bash
# 1. Write code following RULES.md

# 2. Run tests frequently
go test -v -race -short ./...

# 3. Run linter
golangci-lint run ./...

# 4. Fix lint issues
golangci-lint run --fix ./...
```

### 4. Commit Changes

```bash
# Stage changes
git add .

# Commit with conventional message
git commit -m "feat(uom): add bulk import from Excel"

# Push branch
git push origin <branch-name>
```

### 5. Create Pull Request

Create PR via GitHub UI or CLI:

```bash
gh pr create --title "feat(uom): add bulk import from Excel" \
  --body "## Description
  Add ability to import UOMs from Excel file.
  
  ## Changes
  - Add ImportUOMs RPC method
  - Add Excel parsing logic
  - Add validation for import data
  
  ## Testing
  - [x] Unit tests added
  - [x] Integration tests added"
```

---

## Pull Request Guidelines

### PR Template

This repository uses an automatic [Pull Request Template](.github/PULL_REQUEST_TEMPLATE.md).

### PR Requirements

| Requirement | Description |
|-------------|-------------|
| **CI Passing** | All checks must be green (lint, test, build) |
| **Review Approval** | Minimum 1 approval from maintainer |
| **No Conflicts** | Branch must be up-to-date with main |
| **Tests Added** | New code must have tests |
| **Docs Updated** | Documentation updated if needed |

### Labels

| Label | Description |
|-------|-------------|
| `type: feature` | New feature |
| `type: bug` | Bug fix |
| `type: refactor` | Code refactoring |
| `type: docs` | Documentation |
| `service: finance` | Finance service |
| `priority: critical` | Very urgent |
| `breaking-change` | Contains breaking changes |

---

## Code Review Process

### Review Checklist

**For Reviewers:**

#### Code Quality
- [ ] Follows Clean Architecture principles
- [ ] No circular imports
- [ ] Proper error handling
- [ ] Structured logging used
- [ ] Context passed appropriately

#### Testing
- [ ] Unit tests added/updated
- [ ] Test coverage adequate
- [ ] Edge cases covered
- [ ] Mock dependencies properly

#### Performance
- [ ] No N+1 queries
- [ ] Resources properly closed
- [ ] Appropriate caching
- [ ] Context timeouts used

#### Security
- [ ] Input validation present
- [ ] No hardcoded secrets
- [ ] SQL injection prevented
- [ ] Sensitive data not logged

### Review SLA

| PR Type | SLA | Reviewers |
|---------|-----|-----------|
| Hotfix | 2 hours | Any available |
| Bug fix | 24 hours | 1 maintainer |
| Feature | 48 hours | 1-2 maintainers |
| Large refactor | 1 week | 2+ maintainers |

### Providing Feedback

```markdown
# ✅ Good - Constructive with example
"Consider extracting this validation logic into a separate function 
for reusability. Example:
```go
func validateUOMCode(code string) error {
    // validation logic
}
```"

# ❌ Not helpful
"This is wrong."
```

---

## Testing Requirements

### Test Types

| Type | Location | Command |
|------|----------|---------|
| Unit | `internal/*/` | `go test -short ./internal/...` |
| Integration | `internal/infrastructure/` | `go test ./internal/infrastructure/...` |
| E2E | `tests/e2e/` | `go test ./tests/e2e/...` |

### Running Tests

```bash
# Unit tests only
go test -v -race -short ./...

# All tests
go test -v -race ./...

# With coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Specific package
go test -v ./internal/domain/uom/...
```

### Coverage Requirements

| Layer | Minimum | Target |
|-------|---------|--------|
| Domain | 90% | 95% |
| Application | 80% | 90% |
| Infrastructure | 70% | 80% |
| Delivery | 60% | 70% |

---

## Documentation Standards

### When to Update Documentation

- ✅ Adding new API endpoints
- ✅ Changing existing behavior
- ✅ Adding new configuration options
- ✅ Breaking changes
- ✅ New dependencies

### Code Comments

```go
// ✅ Good - Explains WHY
// UseCache is disabled in development to ensure fresh data during testing
UseCache: cfg.App.Env != "development",

// ❌ Bad - States the obvious
// Set UseCache to true
UseCache: true,
```

### API Documentation

Document proto files with comments:

```protobuf
// UOMService manages Units of Measure.
service UOMService {
  // CreateUOM creates a new unit of measure.
  // Returns ALREADY_EXISTS if a UOM with the same code exists.
  rpc CreateUOM(CreateUOMRequest) returns (CreateUOMResponse);
}
```

---

## Commit Message Conventions

### Format

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

### Types

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `style` | Formatting, no code change |
| `refactor` | Code change without new feature or bug fix |
| `perf` | Performance improvement |
| `test` | Adding or updating tests |
| `chore` | Build, deps, config changes |
| `ci` | CI configuration changes |

### Examples

```bash
# Feature
feat(uom): add bulk import from Excel

- Add ImportUOMs RPC method
- Add Excel parsing with excelize
- Add validation for import data

Closes #123

# Bug fix
fix(uom): handle duplicate code error correctly

The duplicate code error was not being properly mapped to
gRPC ALREADY_EXISTS status code.

Fixes #456

# Breaking change
feat(uom)!: change ID type from string to UUID

BREAKING CHANGE: UOM IDs are now UUIDs instead of strings.
Clients need to update their code to handle UUID format.
```

---

## Adding a New Service

### Step 1: Create Directory Structure

```bash
SERVICE_NAME="newservice"
mkdir -p services/${SERVICE_NAME}/{cmd/server,internal/{domain,application,infrastructure,delivery},pkg,migrations/postgres,tests/{e2e,loadtest},docs,deployments/kubernetes}
```

### Step 2: Create go.mod

```bash
cd services/${SERVICE_NAME}
go mod init github.com/mutugading/goapps-backend/services/${SERVICE_NAME}
```

### Step 3: Add Proto Definitions

Add proto files to `goapps-shared-proto`:

```protobuf
// newservice/v1/service.proto
syntax = "proto3";

package newservice.v1;

service NewService {
  rpc GetItem(GetItemRequest) returns (GetItemResponse);
}
```

### Step 4: Generate Code

```bash
cd ../goapps-shared-proto
./scripts/gen-go.sh
```

### Step 5: Implement Layers

Follow the structure from existing services (e.g., finance).

### Step 6: Add CI Workflow

Create `.github/workflows/${SERVICE_NAME}.yml` based on `finance-service.yml`.

### Step 7: Add to Infrastructure

Update `goapps-infra` with Kubernetes manifests and ArgoCD application.

---

## Getting Help

### Channels

| Channel | Purpose | Response Time |
|---------|---------|---------------|
| GitHub Issues | Bug reports, features | 24-48 hours |
| GitHub Discussions | Questions, ideas | 48-72 hours |
| Slack #goapps-backend | Quick questions | Real-time |

### Before Asking

1. ✅ Search existing issues
2. ✅ Read documentation (README, RULES)
3. ✅ Check Go documentation
4. ✅ Try debugging yourself first

### How to Ask

```markdown
### Environment
- Go version: 1.24.x
- OS: Ubuntu 24.04
- Service: finance

### What I'm trying to do
Clear description of the goal.

### What I've tried
1. Step 1
2. Step 2
3. Step 3

### What happened
Error message or unexpected behavior.

### Expected behavior
What should have happened.

### Code/Logs
\`\`\`go
// Relevant code
\`\`\`

\`\`\`
Relevant logs
\`\`\`
```

---

## Code of Conduct

### Our Standards

- 🤝 Be respectful and inclusive
- 💡 Give constructive feedback
- 📝 Document your changes
- 🔒 Never commit secrets
- ✅ Test before pushing
- 🙋 Ask if unsure

---

## Maintainers

| Name | Role | GitHub |
|------|------|--------|
| TBD | Lead Maintainer | @username |
| TBD | Maintainer | @username |

---

Thank you for contributing to `goapps-backend`! 🚀
