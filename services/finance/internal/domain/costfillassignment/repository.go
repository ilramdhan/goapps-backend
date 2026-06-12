package costfillassignment

import "context"

// ConfigRepository persists the three config tiers and resolves them per level.
type ConfigRepository interface {
	// UpsertGlobal inserts/updates the active global config for a route level.
	UpsertGlobal(ctx context.Context, c *Config, actor string) error
	// DeleteGlobal soft/hard-deletes the active global config for a level.
	DeleteGlobal(ctx context.Context, routeLevel int32) error
	// ListGlobal returns all active global configs ordered by level.
	ListGlobal(ctx context.Context) ([]*Config, error)
	// GetGlobal returns the active global config for a level, or ErrConfigNotFound.
	GetGlobal(ctx context.Context, routeLevel int32) (*Config, error)
	// UpsertProduct inserts/updates a per-product override row.
	UpsertProduct(ctx context.Context, c *Config, actor string) error
	// GetProduct returns the product override for (product, level) or nil.
	GetProduct(ctx context.Context, productSysID int64, routeLevel int32) (*Config, error)
	// UpsertRequest inserts/updates a per-request override row.
	UpsertRequest(ctx context.Context, c *Config, actor string) error
	// GetRequest returns the request override for (request, level) or nil.
	GetRequest(ctx context.Context, requestID int64, routeLevel int32) (*Config, error)
}

// TaskRepository persists fill tasks + approvals and exposes tracking queries.
type TaskRepository interface {
	// BulkInsert creates all tasks for a request in one transaction.
	BulkInsert(ctx context.Context, tasks []*Task) error
	// GetByID loads a single task.
	GetByID(ctx context.Context, taskID int64) (*Task, error)
	// GetByRequestLevel loads the task for (request, level).
	GetByRequestLevel(ctx context.Context, requestID int64, routeLevel int32) (*Task, error)
	// ListByRequest returns all tasks for a request ordered by route level desc.
	ListByRequest(ctx context.Context, requestID int64) ([]*Task, error)
	// ListForUser returns tasks whose filler/approver resolves to userID/depts.
	ListForUser(ctx context.Context, userID string, deptCodes []string) ([]*Task, error)
	// Claim atomically claims an ACTIVE task; returns false if already claimed.
	Claim(ctx context.Context, taskID int64, userID string) (bool, error)
	// Save persists status + counters of an existing task.
	Save(ctx context.Context, t *Task) error
	// IncrementFilled bumps filled_params for (request, level) and returns the row.
	IncrementFilled(ctx context.Context, requestID int64, routeLevel int32, delta int32) (*Task, error)
	// CountNonApproved returns how many tasks for a request are not APPROVED.
	CountNonApproved(ctx context.Context, requestID int64) (int, error)
	// CountNonApprovedBelow returns how many tasks for a request with route level
	// strictly less than maxLevel are not APPROVED. Used by CompletionGate to
	// decide when all "regular" levels are done before creating the L100-102 chain.
	CountNonApprovedBelow(ctx context.Context, requestID int64, maxLevel int32) (int, error)
	// ActivateTask sets a task's status to ACTIVE and stamps activated_at to now.
	// Used by CompletionGate to activate L101 after L100 approves, etc.
	ActivateTask(ctx context.Context, taskID int64) error
	// MarkNotified stamps cft_last_notified_at = now for a task.
	MarkNotified(ctx context.Context, taskID int64) error
	// ListOverdue returns unfinished tasks past SLA whose last notify is stale.
	ListOverdue(ctx context.Context, reminderGapHours int) ([]*Task, error)
	// ListPendingFill returns ACTIVE/FILLING tasks whose last notify is stale (not yet submitted).
	ListPendingFill(ctx context.Context, reminderGapHours int) ([]*Task, error)
	// ListPendingApproval returns APPROVAL_PENDING tasks whose last notify is stale.
	ListPendingApproval(ctx context.Context, reminderGapHours int) ([]*Task, error)
	// AddApproval records an approval/rejection event.
	AddApproval(ctx context.Context, a *Approval) error
	// ListApprovals returns a task's approval history newest-first.
	ListApprovals(ctx context.Context, taskID int64) ([]*Approval, error)
}
