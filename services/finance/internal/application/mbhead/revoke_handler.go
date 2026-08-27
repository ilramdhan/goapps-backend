// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// ErrFeatureRemovedRevoke is returned by RevokeHandler for every call.
//
// 🔴 USER DECISION 2026-08-26 — Revoke was removed from the MB Recipe workflow. The RPC
// stays in the proto contract (removing it would be breaking), so the surface is turned
// off here instead. Any caller still wired to it gets this error, not a silent no-op.
var ErrFeatureRemovedRevoke = errors.New("mbhead: revoke has been removed from the MB recipe workflow; deactivate the recipe with the active flag instead")

// RevokeCommand represents the ~~transition to REVOKED (terminal)~~ command.
//
// 🔴 USER DECISION 2026-08-26 — Revoke was REMOVED as a feature. The command struct and
// the handler are kept only because RevokeMBHead still exists in the proto contract
// (deleting an RPC is a breaking change); the handler now refuses every call.
type RevokeCommand struct {
	MbhID       uuid.UUID
	Reason      string
	ActorUserID string
}

// RevokeHandler handles the RevokeMBHead command.
type RevokeHandler struct {
	repo mbhead.Repository
}

// NewRevokeHandler creates a new RevokeHandler.
func NewRevokeHandler(repo mbhead.Repository) *RevokeHandler {
	return &RevokeHandler{repo: repo}
}

// Handle ~~executes the revoke MB Head transition.~~
//
// 🔴 USER DECISION 2026-08-26 — the transition was REMOVED. Handle now refuses
// immediately with ErrFeatureRemovedRevoke and never touches the repository: it does
// not read the head, does not mutate it, and writes no transition row. Failing before
// any I/O keeps a stray caller from producing an audit trail for an action that no
// longer exists.
func (h *RevokeHandler) Handle(_ context.Context, _ RevokeCommand) (*mbhead.Entity, error) {
	return nil, ErrFeatureRemovedRevoke
}
