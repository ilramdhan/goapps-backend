---
name: 🚀 New Service Request
about: Request a new microservice to be added
title: '[SERVICE] '
labels: 'type: feature, scope: new-service'
assignees: ''
---

## Service Information

### Basic Info
- **Service Name**: 
- **Domain/Business Area**: 
- **Priority**: [ ] Critical [ ] High [ ] Medium [ ] Low

### Purpose
<!-- Describe what this service will do -->

## Architecture

### Responsibilities
<!-- List the main responsibilities -->
1. 
2. 
3. 

### Entities
<!-- List the main domain entities -->
- Entity 1 (e.g., User, Order, Payment)
- Entity 2
- Entity 3

### Proto Definitions
```protobuf
// Proposed service definition
syntax = "proto3";

package newservice.v1;

service NewService {
  rpc CreateItem(CreateItemRequest) returns (CreateItemResponse);
  rpc GetItem(GetItemRequest) returns (GetItemResponse);
  rpc ListItems(ListItemsRequest) returns (ListItemsResponse);
}

message Item {
  string id = 1;
  string name = 2;
}
```

## API Endpoints

### gRPC Services
| Method | Description |
|--------|-------------|
| `CreateItem` | Creates a new item |
| `GetItem` | Gets item by ID |
| `ListItems` | Lists items with filtering |

### REST Endpoints (gRPC-Gateway)
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/newservice/items` | Create item |
| GET | `/api/v1/newservice/items/{id}` | Get item |
| GET | `/api/v1/newservice/items` | List items |

## Database

### Database Type
- [x] PostgreSQL
- [ ] Redis (cache only)
- [ ] Other: _______________

### Schema
```sql
-- Main tables
CREATE TABLE items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_items_name ON items(name);
```

## Dependencies

### Other Services
<!-- List services this will depend on -->
- [ ] Finance Service
- [ ] IAM Service
- [ ] None

### External Systems
<!-- List external systems -->
- [ ] Oracle Database
- [ ] Third-party API
- [ ] None

## Resource Requirements

### Estimated Load
- **Requests per second**: 
- **Data volume**: 
- **Concurrent users**: 

### Infrastructure
| Resource | Staging | Production |
|----------|---------|------------|
| CPU | 100m | 500m |
| Memory | 128Mi | 512Mi |
| Replicas | 1 | 2-5 |

## Configuration

### Environment Variables
| Variable | Description | Required |
|----------|-------------|----------|
| `DATABASE_URL` | PostgreSQL connection string | Yes |
| `REDIS_URL` | Redis connection string | No |
| `GRPC_PORT` | gRPC server port | Yes |
| `HTTP_PORT` | HTTP gateway port | Yes |

## Timeline

### Estimated Effort
- **Design**: ___ days
- **Development**: ___ days
- **Testing**: ___ days
- **Documentation**: ___ days

### Milestones
- [ ] Proto definition approved
- [ ] Domain layer complete
- [ ] Application layer complete
- [ ] Infrastructure layer complete
- [ ] Delivery layer complete
- [ ] Tests complete
- [ ] Documentation complete
- [ ] Deployed to staging
- [ ] Deployed to production

## Checklist
- [ ] Service name follows naming conventions
- [ ] Proto definitions reviewed
- [ ] Database schema reviewed
- [ ] Resource requirements estimated
- [ ] No overlap with existing services
- [ ] Clean Architecture will be followed
