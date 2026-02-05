---
name: ✨ Feature Request
about: Suggest a new feature or enhancement
title: '[FEATURE] '
labels: 'type: feature, status: needs-triage'
assignees: ''
---

## Feature Description
<!-- Describe the feature clearly -->

## Service Scope
- [ ] Finance Service
- [ ] IAM Service (future)
- [ ] New Service
- [ ] Shared/Common

## Problem / Motivation
<!-- What problem does this feature solve? -->

## Proposed Solution
<!-- How do you envision the solution? -->

## API Design (if applicable)

### Proto Definition
```protobuf
// Proposed proto changes
service ExampleService {
  rpc NewMethod(NewMethodRequest) returns (NewMethodResponse);
}

message NewMethodRequest {
  string field1 = 1;
}

message NewMethodResponse {
  string result = 1;
}
```

### REST Endpoint (via gRPC-Gateway)
```
POST /api/v1/<service>/<resource>
GET  /api/v1/<service>/<resource>/{id}
```

## Database Changes (if applicable)
```sql
-- Proposed schema changes
CREATE TABLE new_table (
    id UUID PRIMARY KEY,
    field1 VARCHAR(100) NOT NULL
);
```

## Breaking Changes
<!-- Will this break existing API consumers? -->
- [ ] Yes (describe impact)
- [ ] No

## Dependencies
<!-- Does this depend on other services or features? -->

## Alternatives Considered
<!-- What other approaches did you consider? -->

## Acceptance Criteria
- [ ] Criteria 1
- [ ] Criteria 2
- [ ] Criteria 3

## Additional Context
<!-- Any other context, mockups, or examples -->

## Checklist
- [ ] I have searched existing issues for duplicates
- [ ] I have read RULES.md
- [ ] This follows Clean Architecture principles
- [ ] Proto changes are backward compatible (or clearly marked as breaking)
