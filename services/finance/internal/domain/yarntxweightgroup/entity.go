// Package yarntxweightgroup provides domain logic for TX Weight groups.
package yarntxweightgroup

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Column limits mirror mst_yarn_tx_weight_group (migration 000536).
const (
	maxCodeLen        = 30
	maxNameLen        = 100
	maxDescriptionLen = 200
)

// codePattern mirrors the proto rule ^[A-Z][A-Z0-9_]*$.
var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Entity is the aggregate root for a TX Weight group: its header, the set of
// product types that share it and its grade rules.
type Entity struct {
	id           uuid.UUID
	code         string
	name         string
	description  string
	productTypes []ProductTypeRef
	rules        []Rule
	createdAt    time.Time
	createdBy    string
	updatedAt    *time.Time
	updatedBy    *string
	deletedAt    *time.Time
	deletedBy    *string
}

// Input carries the full, replaceable state of a group (create and update).
type Input struct {
	Code           string
	Name           string
	Description    string
	ProductTypeIDs []int32
	Rules          []Rule
}

type normalizedInput struct {
	code, name, description string
	productTypes            []ProductTypeRef
	rules                   []Rule
}

func normalizeInput(in Input) (normalizedInput, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" || len(code) > maxCodeLen || !codePattern.MatchString(code) {
		return normalizedInput{}, ErrInvalidCode
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return normalizedInput{}, ErrInvalidName
	}
	desc := strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(desc) > maxDescriptionLen {
		return normalizedInput{}, ErrDescriptionTooLong
	}
	types, err := normalizeProductTypeIDs(in.ProductTypeIDs)
	if err != nil {
		return normalizedInput{}, err
	}
	rules, err := normalizeRules(in.Rules)
	if err != nil {
		return normalizedInput{}, err
	}
	return normalizedInput{code: code, name: name, description: desc, productTypes: types, rules: rules}, nil
}

// New creates a new TX Weight group with validation.
func New(in Input, createdBy string) (*Entity, error) {
	n, err := normalizeInput(in)
	if err != nil {
		return nil, err
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entity{
		id: uuid.New(), code: n.code, name: n.name, description: n.description,
		productTypes: n.productTypes, rules: n.rules, createdAt: time.Now(), createdBy: createdBy,
	}, nil
}

// ReconstructParams carries persisted fields for Reconstruct.
type ReconstructParams struct {
	ID           uuid.UUID
	Code         string
	Name         string
	Description  string
	ProductTypes []ProductTypeRef
	Rules        []Rule
	CreatedAt    time.Time
	CreatedBy    string
	UpdatedAt    *time.Time
	UpdatedBy    *string
	DeletedAt    *time.Time
	DeletedBy    *string
}

// Reconstruct rebuilds an entity from persistence data (no validation).
func Reconstruct(p ReconstructParams) *Entity {
	return &Entity{
		id: p.ID, code: p.Code, name: p.Name, description: p.Description,
		productTypes: p.ProductTypes, rules: p.Rules,
		createdAt: p.CreatedAt, createdBy: p.CreatedBy, updatedAt: p.UpdatedAt, updatedBy: p.UpdatedBy,
		deletedAt: p.DeletedAt, deletedBy: p.DeletedBy,
	}
}

// ID returns the UUID primary key.
func (e *Entity) ID() uuid.UUID { return e.id }

// Code returns the group code.
func (e *Entity) Code() string { return e.code }

// Name returns the group name.
func (e *Entity) Name() string { return e.name }

// Description returns the optional description.
func (e *Entity) Description() string { return e.description }

// ProductTypes returns a copy of the mapped product types.
func (e *Entity) ProductTypes() []ProductTypeRef {
	return append([]ProductTypeRef(nil), e.productTypes...)
}

// ProductTypeIDs returns the mapped product type ids.
func (e *Entity) ProductTypeIDs() []int32 {
	out := make([]int32, len(e.productTypes))
	for i, pt := range e.productTypes {
		out[i] = pt.ID
	}
	return out
}

// Rules returns a copy of the grade rules (grade display order).
func (e *Entity) Rules() []Rule { return append([]Rule(nil), e.rules...) }

// CreatedAt returns the creation timestamp.
func (e *Entity) CreatedAt() time.Time { return e.createdAt }

// CreatedBy returns the creator.
func (e *Entity) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp.
func (e *Entity) UpdatedAt() *time.Time { return e.updatedAt }

// UpdatedBy returns the last updater.
func (e *Entity) UpdatedBy() *string { return e.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (e *Entity) DeletedAt() *time.Time { return e.deletedAt }

// DeletedBy returns who soft-deleted the group.
func (e *Entity) DeletedBy() *string { return e.deletedBy }

// IsDeleted reports whether the group is soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != nil }

// Update replaces the whole group state (header, type set and rules). A
// failed validation leaves the entity unchanged.
func (e *Entity) Update(in Input, updatedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	n, err := normalizeInput(in)
	if err != nil {
		return err
	}
	e.code, e.name, e.description = n.code, n.name, n.description
	e.productTypes, e.rules = n.productTypes, n.rules
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the group as deleted.
func (e *Entity) SoftDelete(deletedBy string) error {
	if e.IsDeleted() {
		return ErrAlreadyDeleted
	}
	now := time.Now()
	e.deletedAt = &now
	e.deletedBy = &deletedBy
	return nil
}
