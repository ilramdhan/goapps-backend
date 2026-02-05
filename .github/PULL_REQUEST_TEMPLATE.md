## Description
<!-- Describe the changes briefly -->

## Type of Change
- [ ] 🐛 Bug fix (non-breaking change that fixes an issue)
- [ ] ✨ New feature (non-breaking change that adds functionality)
- [ ] 💥 Breaking change (fix or feature that changes existing API)
- [ ] ♻️ Refactor (code change without new feature or bug fix)
- [ ] 📚 Documentation update
- [ ] 🧪 Test update
- [ ] 🔧 Chore (dependencies, config, etc.)

## Service(s) Affected
- [ ] Finance Service
- [ ] IAM Service
- [ ] Shared Proto (gen/)
- [ ] Root/Common

## Changes Made
<!-- List the changes made -->
- 
- 
- 

## Related Issues
<!-- Link to related issues -->
Fixes #
Related to #

## API Changes (if applicable)
### Proto Changes
```diff
+ // Added
- // Removed
```

### Breaking Changes
<!-- Describe any breaking changes -->

## Testing Performed

### Unit Tests
- [ ] New unit tests added
- [ ] Existing unit tests pass
- [ ] Coverage maintained/improved

### Integration Tests
- [ ] New integration tests added
- [ ] Existing integration tests pass

### Manual Testing
```bash
# Commands used for testing
grpcurl -plaintext localhost:50051 ...
curl http://localhost:8080/...
```

## Lint & Build
- [ ] `golangci-lint run ./...` passes
- [ ] `go build ./...` succeeds
- [ ] `go test -race ./...` passes

## Database (if applicable)
- [ ] Migration added
- [ ] Migration tested (up and down)
- [ ] No breaking schema changes (or documented)

## Documentation
- [ ] README.md updated (if needed)
- [ ] RULES.md updated (if needed)
- [ ] Proto comments updated
- [ ] OpenAPI regenerated

## Rollback Plan
<!-- Describe how to rollback if issues occur -->

## Screenshots/Logs (if applicable)
<!-- Add screenshots or logs -->

---

### Pre-merge Checklist
- [ ] I have read and followed [RULES.md](./RULES.md)
- [ ] I have read and followed [CONTRIBUTING.md](./CONTRIBUTING.md)
- [ ] Clean Architecture principles followed
- [ ] All errors are properly handled
- [ ] Context is passed appropriately
- [ ] Structured logging is used
- [ ] No hardcoded secrets
- [ ] PR description is complete and clear
- [ ] CI checks are passing

### Reviewer Notes
<!-- Notes for reviewers -->
