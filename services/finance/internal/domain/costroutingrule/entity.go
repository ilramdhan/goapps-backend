// Package costroutingrule is the cost_routing_rule domain (PRD Phase A §7.1.4, CRR_).
// Admin-managed first-match-wins rules. JSON condition stays opaque to the domain;
// the rule-evaluation engine will interpret it on the request submit path (deferred).
package costroutingrule

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Sentinel errors.
var (
	// ErrNotFound when a rule is missing.
	ErrNotFound = errors.New("cost routing rule not found")
	// ErrInvalidCondition when the JSON predicate fails parse.
	ErrInvalidCondition = errors.New("invalid condition (must be JSON object)")
	// ErrInvalidAction when action_type is outside the allowed set.
	ErrInvalidAction = errors.New("invalid action_type (must be AUTO_ASSIGN | TO_TRIAGE)")
)

// Action types.
const (
	ActionAutoAssign = "AUTO_ASSIGN"
	ActionToTriage   = "TO_TRIAGE"
)

var allowedAction = map[string]struct{}{ActionAutoAssign: {}, ActionToTriage: {}}

// Rule is the aggregate.
type Rule struct {
	RuleID       int32
	Priority     int32
	Condition    string // JSON predicate tree
	ActionType   string
	ActionTarget string
	IsActive     bool
	CreatedBy    string
	CreatedAt    time.Time
}

// NewInput is the create-time input.
type NewInput struct {
	Priority     int32
	Condition    string
	ActionType   string
	ActionTarget string
	CreatedBy    string
}

// New constructs an active rule.
func New(in NewInput) (*Rule, error) {
	if _, ok := allowedAction[in.ActionType]; !ok {
		return nil, ErrInvalidAction
	}
	if err := validateCondition(in.Condition); err != nil {
		return nil, err
	}
	return &Rule{
		Priority:     in.Priority,
		Condition:    in.Condition,
		ActionType:   in.ActionType,
		ActionTarget: strings.TrimSpace(in.ActionTarget),
		IsActive:     true,
		CreatedBy:    in.CreatedBy,
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// UpdateInput is the update-time input.
type UpdateInput struct {
	Priority     int32
	Condition    string
	ActionType   string
	ActionTarget string
	IsActive     bool
}

// Update mutates editable fields.
func (r *Rule) Update(in UpdateInput) error {
	if _, ok := allowedAction[in.ActionType]; !ok {
		return ErrInvalidAction
	}
	if err := validateCondition(in.Condition); err != nil {
		return err
	}
	r.Priority = in.Priority
	r.Condition = in.Condition
	r.ActionType = in.ActionType
	r.ActionTarget = strings.TrimSpace(in.ActionTarget)
	r.IsActive = in.IsActive
	return nil
}

func validateCondition(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return ErrInvalidCondition
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return ErrInvalidCondition
	}
	return nil
}

// Filter for List.
type Filter struct {
	ActiveFilter string // "all" | "active" | "inactive" | ""
	Page         int
	PageSize     int
}

// Repository persists rules.
type Repository interface {
	Create(ctx context.Context, r *Rule) error
	GetByID(ctx context.Context, id int32) (*Rule, error)
	Update(ctx context.Context, r *Rule) error
	Delete(ctx context.Context, id int32) error
	List(ctx context.Context, f Filter) (items []*Rule, total int64, err error)
}
