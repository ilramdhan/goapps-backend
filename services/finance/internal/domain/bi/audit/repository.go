package audit

import "context"

// Repository persists and retrieves BI config-change audit entries.
type Repository interface {
	// Record appends a single audit entry. The store assigns AuditID + ChangedAt.
	Record(ctx context.Context, entry Entry) error
	// List returns audit entries newest-first, paginated, with the total count.
	// An empty entityType returns entries for all entity types.
	List(ctx context.Context, entityType string, page, pageSize int) ([]Entry, int, error)
}
