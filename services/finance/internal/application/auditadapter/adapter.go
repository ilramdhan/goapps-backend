// Package auditadapter wires the costauditlog emitter to consumers that don't
// import the audit domain directly (e.g., costproductrequest.TransitionHandler).
// Keeps the dependency direction one-way: audit consumes nothing from the consumers.
package auditadapter

import (
	"context"

	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	cpr "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
)

// CprEmitter adapts auditapp.Emitter to the cpr.AuditEmitter interface.
type CprEmitter struct{ emitter *auditapp.Emitter }

// NewCprEmitter constructs the adapter.
func NewCprEmitter(emitter *auditapp.Emitter) *CprEmitter {
	return &CprEmitter{emitter: emitter}
}

// Emit forwards a transition audit entry to the underlying emitter.
func (a *CprEmitter) Emit(ctx context.Context, entry cpr.AuditEntry) error {
	return a.emitter.Emit(ctx, auditdomain.NewInput{
		EntityType: entry.EntityType,
		EntityID:   entry.EntityID,
		Operation:  entry.Operation,
		BeforeData: entry.BeforeData,
		AfterData:  entry.AfterData,
		UserID:     entry.UserID,
	})
}
