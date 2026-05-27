package workflowtemplate

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Step is a single ordered step inside a Template.
type Step struct {
	id                      uuid.UUID
	templateID              uuid.UUID
	stepNo                  int
	stepName                string
	approverResolutionType  Resolution
	approverResolutionValue string
	slaHours                int
	allowReject             bool
	allowReassign           bool
	requirePasswordOnUnlock bool
	rejectToStepNo          int
}

// NewStep constructs a Step after validation.
func NewStep(
	stepNo int,
	stepName, resolutionType, resolutionValue string,
	slaHours int,
	allowReject, allowReassign, requirePassword bool,
	rejectToStepNo int,
) (Step, error) {
	if stepNo < 1 {
		return Step{}, ErrInvalidStep
	}
	stepName = strings.TrimSpace(stepName)
	if stepName == "" || len(stepName) > 200 {
		return Step{}, ErrInvalidStep
	}
	res, err := NewResolution(resolutionType)
	if err != nil {
		return Step{}, err
	}
	resolutionValue = strings.TrimSpace(resolutionValue)
	if resolutionValue == "" || len(resolutionValue) > 200 {
		return Step{}, ErrInvalidStep
	}
	if slaHours < 0 || rejectToStepNo < 0 {
		return Step{}, ErrInvalidStep
	}
	return Step{
		id:                      uuid.New(),
		stepNo:                  stepNo,
		stepName:                stepName,
		approverResolutionType:  res,
		approverResolutionValue: resolutionValue,
		slaHours:                slaHours,
		allowReject:             allowReject,
		allowReassign:           allowReassign,
		requirePasswordOnUnlock: requirePassword,
		rejectToStepNo:          rejectToStepNo,
	}, nil
}

// ReconstructStep rebuilds a Step from persistence.
func ReconstructStep(
	id, templateID uuid.UUID,
	stepNo int, stepName, resolutionType, resolutionValue string,
	slaHours int, allowReject, allowReassign, requirePassword bool,
	rejectToStepNo int,
) (Step, error) {
	res, err := NewResolution(resolutionType)
	if err != nil {
		return Step{}, err
	}
	return Step{
		id: id, templateID: templateID,
		stepNo: stepNo, stepName: stepName,
		approverResolutionType: res, approverResolutionValue: resolutionValue,
		slaHours:                slaHours,
		allowReject:             allowReject,
		allowReassign:           allowReassign,
		requirePasswordOnUnlock: requirePassword,
		rejectToStepNo:          rejectToStepNo,
	}, nil
}

// ID returns the identifier.
func (s Step) ID() uuid.UUID { return s.id }

// TemplateID returns the template id.
func (s Step) TemplateID() uuid.UUID { return s.templateID }

// StepNo returns the step no.
func (s Step) StepNo() int { return s.stepNo }

// StepName returns the step name.
func (s Step) StepName() string { return s.stepName }

// ApproverResolutionType returns the approver resolution type.
func (s Step) ApproverResolutionType() Resolution { return s.approverResolutionType }

// ApproverResolutionValue returns the approver resolution value.
func (s Step) ApproverResolutionValue() string { return s.approverResolutionValue }

// SLAHours returns the sla hours.
func (s Step) SLAHours() int { return s.slaHours }

// AllowReject returns the allow reject.
func (s Step) AllowReject() bool { return s.allowReject }

// AllowReassign returns the allow reassign.
func (s Step) AllowReassign() bool { return s.allowReassign }

// RequirePasswordOnUnlock returns the require password on unlock.
func (s Step) RequirePasswordOnUnlock() bool { return s.requirePasswordOnUnlock }

// RejectToStepNo returns the reject to step no.
func (s Step) RejectToStepNo() int { return s.rejectToStepNo }

// SetTemplateID is used by the repository when persisting a fresh step.
func (s *Step) SetTemplateID(id uuid.UUID) { s.templateID = id }

// Template is the aggregate root.
type Template struct {
	id          uuid.UUID
	kind        Kind
	name        Name
	version     int
	isActive    bool
	description Description
	steps       []Step
	createdAt   time.Time
	createdBy   string
	updatedAt   *time.Time
	updatedBy   string
	deletedAt   *time.Time
	deletedBy   string
}

// New constructs a brand-new Template.
func New(kind, name, description string, steps []Step, createdBy string) (*Template, error) {
	k, err := NewKind(kind)
	if err != nil {
		return nil, err
	}
	n, err := NewName(name)
	if err != nil {
		return nil, err
	}
	d, err := NewDescription(description)
	if err != nil {
		return nil, err
	}
	if err := validateSteps(steps); err != nil {
		return nil, err
	}
	id := uuid.New()
	for i := range steps {
		steps[i].SetTemplateID(id)
	}
	return &Template{
		id:          id,
		kind:        k,
		name:        n,
		version:     1,
		isActive:    false,
		description: d,
		steps:       steps,
		createdAt:   time.Now().UTC(),
		createdBy:   createdBy,
	}, nil
}

// NewVersion creates a successor template (version N+1) of the given prior template.
// The new version starts inactive; callers must call Activate explicitly.
func NewVersion(prior *Template, name, description string, steps []Step, createdBy string) (*Template, error) {
	n, err := NewName(name)
	if err != nil {
		return nil, err
	}
	d, err := NewDescription(description)
	if err != nil {
		return nil, err
	}
	if err := validateSteps(steps); err != nil {
		return nil, err
	}
	id := uuid.New()
	for i := range steps {
		steps[i].SetTemplateID(id)
	}
	return &Template{
		id:          id,
		kind:        prior.kind,
		name:        n,
		version:     prior.version + 1,
		isActive:    false,
		description: d,
		steps:       steps,
		createdAt:   time.Now().UTC(),
		createdBy:   createdBy,
	}, nil
}

// Reconstruct rebuilds a Template from persistence (used by repos).
func Reconstruct(
	id uuid.UUID, kind, name string, version int, isActive bool, description string,
	steps []Step,
	createdAt time.Time, createdBy string,
	updatedAt *time.Time, updatedBy string,
	deletedAt *time.Time, deletedBy string,
) (*Template, error) {
	k, err := NewKind(kind)
	if err != nil {
		return nil, err
	}
	n, err := NewName(name)
	if err != nil {
		return nil, err
	}
	d, err := NewDescription(description)
	if err != nil {
		return nil, err
	}
	return &Template{
		id:          id,
		kind:        k,
		name:        n,
		version:     version,
		isActive:    isActive,
		description: d,
		steps:       steps,
		createdAt:   createdAt,
		createdBy:   createdBy,
		updatedAt:   updatedAt,
		updatedBy:   updatedBy,
		deletedAt:   deletedAt,
		deletedBy:   deletedBy,
	}, nil
}

// Activate marks the template active. Repository is responsible for deactivating
// sibling versions of the same kind in the same transaction.
func (t *Template) Activate(by string) {
	t.isActive = true
	now := time.Now().UTC()
	t.updatedAt = &now
	t.updatedBy = by
}

// Deactivate marks the template inactive.
func (t *Template) Deactivate(by string) {
	t.isActive = false
	now := time.Now().UTC()
	t.updatedAt = &now
	t.updatedBy = by
}

// SoftDelete marks the template deleted.
func (t *Template) SoftDelete(by string) {
	now := time.Now().UTC()
	t.deletedAt = &now
	t.deletedBy = by
	t.isActive = false
}

// ID returns the identifier.
func (t *Template) ID() uuid.UUID { return t.id }

// Kind returns the kind.
func (t *Template) Kind() Kind { return t.kind }

// Name returns the name.
func (t *Template) Name() Name { return t.name }

// Version returns the version.
func (t *Template) Version() int { return t.version }

// IsActive returns the is active.
func (t *Template) IsActive() bool { return t.isActive }

// Description returns the description.
func (t *Template) Description() Description { return t.description }

// Steps returns the steps.
func (t *Template) Steps() []Step { return t.steps }

// CreatedAt returns the created at.
func (t *Template) CreatedAt() time.Time { return t.createdAt }

// CreatedBy returns the created by.
func (t *Template) CreatedBy() string { return t.createdBy }

// UpdatedAt returns the updated at.
func (t *Template) UpdatedAt() *time.Time { return t.updatedAt }

// UpdatedBy returns the updated by.
func (t *Template) UpdatedBy() string { return t.updatedBy }

// DeletedAt returns the deleted at.
func (t *Template) DeletedAt() *time.Time { return t.deletedAt }

// DeletedBy returns the deleted by.
func (t *Template) DeletedBy() string { return t.deletedBy }

// validateSteps enforces step_no = 1..N strictly monotonic with no gaps.
func validateSteps(steps []Step) error {
	if len(steps) == 0 {
		return ErrNoSteps
	}
	for i, s := range steps {
		if s.stepNo != i+1 {
			return ErrStepOrder
		}
	}
	return nil
}
