package workflowinstance

import (
	"time"

	"github.com/google/uuid"
)

// Status enum.
const (
	StatusInProgress = "IN_PROGRESS"
	StatusApproved   = "APPROVED"
	StatusRejected   = "REJECTED"
	StatusLocked     = "LOCKED"
	StatusUnlocked   = "UNLOCKED"
)

// Entity kinds.
const (
	EntityKindPrdRequest = "PRD_REQUEST"
	EntityKindCstProduct = "CST_PRODUCT"
	EntityKindParamFill  = "PARAM_FILL"
)

// Decision enum.
const (
	DecisionApproved   = "APPROVED"
	DecisionRejected   = "REJECTED"
	DecisionReassigned = "REASSIGNED"
	DecisionSkipped    = "SKIPPED"
)

// Step is one snapshot row inside a running instance.
type Step struct {
	id                      uuid.UUID
	instanceID              uuid.UUID
	stepNo                  int
	stepName                string
	approverResolutionType  string
	approverResolutionValue string
	slaHours                int
	allowReject             bool
	requirePasswordOnUnlock bool
	assignedAt              time.Time
	actorUserID             *uuid.UUID
	decision                string
	decidedAt               *time.Time
	comment                 string
	stuckSince              *time.Time
}

// NewPendingStep creates a fresh step row (assigned now, no decision yet).
func NewPendingStep(
	instanceID uuid.UUID,
	stepNo int,
	stepName, resType, resValue string,
	slaHours int,
	allowReject, requirePassword bool,
) Step {
	return Step{
		id:                      uuid.New(),
		instanceID:              instanceID,
		stepNo:                  stepNo,
		stepName:                stepName,
		approverResolutionType:  resType,
		approverResolutionValue: resValue,
		slaHours:                slaHours,
		allowReject:             allowReject,
		requirePasswordOnUnlock: requirePassword,
		assignedAt:              time.Now().UTC(),
	}
}

// ReconstructStep rebuilds a Step from persistence.
func ReconstructStep(
	id, instanceID uuid.UUID,
	stepNo int, stepName, resType, resValue string,
	slaHours int, allowReject, requirePassword bool,
	assignedAt time.Time,
	actorUserID *uuid.UUID,
	decision string,
	decidedAt *time.Time,
	comment string,
	stuckSince *time.Time,
) Step {
	return Step{
		id: id, instanceID: instanceID,
		stepNo: stepNo, stepName: stepName,
		approverResolutionType: resType, approverResolutionValue: resValue,
		slaHours: slaHours, allowReject: allowReject, requirePasswordOnUnlock: requirePassword,
		assignedAt:  assignedAt,
		actorUserID: actorUserID,
		decision:    decision,
		decidedAt:   decidedAt,
		comment:     comment,
		stuckSince:  stuckSince,
	}
}

// Decide marks the step decided.
func (s *Step) Decide(actor uuid.UUID, decision, comment string) {
	now := time.Now().UTC()
	s.actorUserID = &actor
	s.decision = decision
	s.decidedAt = &now
	s.comment = comment
}

// ID returns the identifier.
func (s Step) ID() uuid.UUID { return s.id }

// InstanceID returns the instance id.
func (s Step) InstanceID() uuid.UUID { return s.instanceID }

// StepNo returns the step no.
func (s Step) StepNo() int { return s.stepNo }

// StepName returns the step name.
func (s Step) StepName() string { return s.stepName }

// ApproverResolutionType returns the approver resolution type.
func (s Step) ApproverResolutionType() string { return s.approverResolutionType }

// ApproverResolutionValue returns the approver resolution value.
func (s Step) ApproverResolutionValue() string {
	return s.approverResolutionValue
}

// SLAHours returns the sla hours.
func (s Step) SLAHours() int { return s.slaHours }

// AllowReject returns the allow reject.
func (s Step) AllowReject() bool { return s.allowReject }

// RequirePasswordOnUnlock returns the require password on unlock.
func (s Step) RequirePasswordOnUnlock() bool { return s.requirePasswordOnUnlock }

// AssignedAt returns the assigned at.
func (s Step) AssignedAt() time.Time { return s.assignedAt }

// ActorUserID returns the actor user id.
func (s Step) ActorUserID() *uuid.UUID { return s.actorUserID }

// Decision returns the decision.
func (s Step) Decision() string { return s.decision }

// DecidedAt returns the decided at.
func (s Step) DecidedAt() *time.Time { return s.decidedAt }

// Comment returns the comment.
func (s Step) Comment() string { return s.comment }

// StuckSince returns the stuck since.
func (s Step) StuckSince() *time.Time { return s.stuckSince }

// Instance is the aggregate root for a running workflow.
type Instance struct {
	id              uuid.UUID
	templateID      uuid.UUID
	templateVersion int
	kind            string
	entityKind      string
	entityID        uuid.UUID
	currentStepNo   int
	status          string
	startedAt       time.Time
	startedBy       string
	completedAt     *time.Time
	steps           []Step
	totalStepsInTpl int
}

// New constructs a new Instance plus its first step.
func New(
	templateID uuid.UUID, templateVersion int, kind, entityKind string, entityID uuid.UUID,
	startedBy string,
	firstStepName, firstResType, firstResValue string,
	firstSLAHours int, firstAllowReject, firstRequirePassword bool,
	totalStepsInTpl int,
) (*Instance, error) {
	if !validEntityKind(entityKind) {
		return nil, ErrInvalidEntityKind
	}
	id := uuid.New()
	first := NewPendingStep(id, 1, firstStepName, firstResType, firstResValue, firstSLAHours, firstAllowReject, firstRequirePassword)
	return &Instance{
		id:              id,
		templateID:      templateID,
		templateVersion: templateVersion,
		kind:            kind,
		entityKind:      entityKind,
		entityID:        entityID,
		currentStepNo:   1,
		status:          StatusInProgress,
		startedAt:       time.Now().UTC(),
		startedBy:       startedBy,
		steps:           []Step{first},
		totalStepsInTpl: totalStepsInTpl,
	}, nil
}

// Reconstruct rebuilds an Instance from persistence.
func Reconstruct(
	id, templateID uuid.UUID, templateVersion int,
	kind, entityKind string, entityID uuid.UUID,
	currentStepNo int, status string,
	startedAt time.Time, startedBy string,
	completedAt *time.Time,
	steps []Step,
	totalStepsInTpl int,
) *Instance {
	return &Instance{
		id: id, templateID: templateID, templateVersion: templateVersion,
		kind: kind, entityKind: entityKind, entityID: entityID,
		currentStepNo: currentStepNo, status: status,
		startedAt: startedAt, startedBy: startedBy, completedAt: completedAt,
		steps: steps, totalStepsInTpl: totalStepsInTpl,
	}
}

// CurrentStep returns the step row at currentStepNo (must exist while IN_PROGRESS).
func (i *Instance) CurrentStep() (*Step, error) {
	for idx := range i.steps {
		if i.steps[idx].stepNo == i.currentStepNo && i.steps[idx].decision == "" {
			return &i.steps[idx], nil
		}
	}
	return nil, ErrCurrentStepMissing
}

// Advance records an APPROVED decision on the current step and advances state.
// If this was the last step (per template snapshot), status becomes LOCKED and
// completed_at is set. Otherwise a fresh pending step row is appended at step_no+1.
// nextStepFactory is supplied by the application layer because the template
// snapshot is needed; the factory returns the new pending step.
func (i *Instance) Advance(actor uuid.UUID, comment string, nextStepFactory func(stepNo int) (Step, error)) (*Step, error) {
	if i.status != StatusInProgress {
		return nil, ErrNotInProgress
	}
	cur, err := i.CurrentStep()
	if err != nil {
		return nil, err
	}
	cur.Decide(actor, DecisionApproved, comment)

	// Last step? Lock.
	if i.currentStepNo >= i.totalStepsInTpl {
		i.status = StatusLocked
		now := time.Now().UTC()
		i.completedAt = &now
		return nil, nil //nolint:nilnil // a locked/terminal instance legitimately yields no next step
	}

	// Append the next pending step row.
	next, err := nextStepFactory(i.currentStepNo + 1)
	if err != nil {
		return nil, err
	}
	i.currentStepNo++
	i.steps = append(i.steps, next)
	return &next, nil
}

// Reject records a REJECTED decision on the current step and sets status REJECTED.
func (i *Instance) Reject(actor uuid.UUID, comment string) error {
	if i.status != StatusInProgress {
		return ErrNotInProgress
	}
	if comment == "" {
		return ErrInvalidComment
	}
	cur, err := i.CurrentStep()
	if err != nil {
		return err
	}
	if !cur.allowReject {
		return ErrRejectNotAllowed
	}
	cur.Decide(actor, DecisionRejected, comment)
	i.status = StatusRejected
	now := time.Now().UTC()
	i.completedAt = &now
	return nil
}

// ID returns the identifier.
func (i *Instance) ID() uuid.UUID { return i.id }

// TemplateID returns the template id.
func (i *Instance) TemplateID() uuid.UUID { return i.templateID }

// TemplateVersion returns the template version.
func (i *Instance) TemplateVersion() int { return i.templateVersion }

// Kind returns the kind.
func (i *Instance) Kind() string { return i.kind }

// EntityKind returns the entity kind.
func (i *Instance) EntityKind() string { return i.entityKind }

// EntityID returns the entity id.
func (i *Instance) EntityID() uuid.UUID { return i.entityID }

// CurrentStepNo returns the current step no.
func (i *Instance) CurrentStepNo() int { return i.currentStepNo }

// Status returns the status.
func (i *Instance) Status() string { return i.status }

// StartedAt returns the started at.
func (i *Instance) StartedAt() time.Time { return i.startedAt }

// StartedBy returns the started by.
func (i *Instance) StartedBy() string { return i.startedBy }

// CompletedAt returns the completed at.
func (i *Instance) CompletedAt() *time.Time { return i.completedAt }

// Steps returns the steps.
func (i *Instance) Steps() []Step { return i.steps }

// TotalStepsInTpl returns the total steps in tpl.
func (i *Instance) TotalStepsInTpl() int { return i.totalStepsInTpl }

func validEntityKind(k string) bool {
	switch k {
	case EntityKindPrdRequest, EntityKindCstProduct, EntityKindParamFill:
		return true
	default:
		return false
	}
}
