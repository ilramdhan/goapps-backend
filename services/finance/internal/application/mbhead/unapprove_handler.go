// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// ErrFeatureRemovedUnApprove is returned by UnApproveHandler for every call.
//
// 🔴 USER DECISION 2026-08-26 — Un-approve was removed from the MB Recipe workflow. The
// RPC stays in the proto contract (removing it would be breaking), so the surface is
// turned off here instead. An approved recipe is reopened via Request Unlock (P10).
var ErrFeatureRemovedUnApprove = errors.New("mbhead: un-approve has been removed from the MB recipe workflow; use Request Unlock to reopen an approved recipe")

// UnApproveCommand represents the ~~APPROVED → UN_APPROVED transition~~ command.
//
// 🔴 USER DECISION 2026-08-26 — Un-approve was REMOVED as a feature. The command struct
// and the handler are kept only because UnApproveMBHead still exists in the proto
// contract (deleting an RPC is a breaking change); the handler now refuses every call.
type UnApproveCommand struct {
	MbhID       uuid.UUID
	Reason      string
	ActorUserID string
}

// UnApproveHandler handles the UnApproveMBHead command.
type UnApproveHandler struct {
	repo mbhead.Repository
}

// NewUnApproveHandler creates a new UnApproveHandler.
func NewUnApproveHandler(repo mbhead.Repository) *UnApproveHandler {
	return &UnApproveHandler{repo: repo}
}

// Handle ~~executes the un-approve MB Head transition.~~
//
// 🔴 USER DECISION 2026-08-26 — the transition was REMOVED. Handle now refuses
// immediately with ErrFeatureRemovedUnApprove and never touches the repository: it does
// not read the head, does not mutate it, and writes no transition row. A locked recipe
// is reopened through the P10 Request Unlock flow instead.
func (h *UnApproveHandler) Handle(_ context.Context, _ UnApproveCommand) (*mbhead.Entity, error) {
	return nil, ErrFeatureRemovedUnApprove
}
