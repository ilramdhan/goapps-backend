package mbactivesync

import "context"

// SnapshotReader loads a consistent, read-only view of the MB master data the
// planner needs. Implemented in internal/infrastructure/postgres; all reads must
// happen inside a single read-only repeatable-read transaction.
type SnapshotReader interface {
	Load(ctx context.Context) (*Snapshot, error)
}
