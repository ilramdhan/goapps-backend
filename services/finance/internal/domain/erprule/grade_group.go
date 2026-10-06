package erprule

// Grade is the GoApps-owned view of one cost_erp_grade row (the read-only
// ERP grade replica). The only mutable attribute is the grade group
// (ceg_grade_group), and AssignGroup / ClearGroup are the only paths that
// change it: the replica sync never writes the column (design Part 1 §4.1,
// AC-11).
type Grade struct {
	code       string
	name       string
	isActive   bool
	group      *GradeGroup
	assignedBy string
}

// ReconstructGrade rebuilds a grade from persistence without validation. A
// nil group means ceg_grade_group IS NULL (the V-08 worklist).
func ReconstructGrade(code, name string, isActive bool, group *GradeGroup) *Grade {
	return &Grade{code: code, name: name, isActive: isActive, group: copyGroup(group)}
}

// AssignGroup sets the grade group. Any known group, including AX, may be
// assigned: AX grades are valued from the AX cost, not a rule. It returns
// the previous group (nil if unassigned) for the audit before/after.
func (g *Grade) AssignGroup(group GradeGroup, user string) (*GradeGroup, error) {
	parsed, err := ParseGradeGroup(string(group))
	if err != nil {
		return nil, err
	}
	if parsed != group {
		return nil, ErrInvalidGradeGroup
	}
	u, err := validateUser(user)
	if err != nil {
		return nil, err
	}
	prev := copyGroup(g.group)
	v := parsed
	g.group = &v
	g.assignedBy = u
	return prev, nil
}

// ClearGroup sets the grade group back to NULL (the grade re-enters the V-08
// worklist). It returns the previous group.
func (g *Grade) ClearGroup(user string) (*GradeGroup, error) {
	u, err := validateUser(user)
	if err != nil {
		return nil, err
	}
	prev := copyGroup(g.group)
	g.group = nil
	g.assignedBy = u
	return prev, nil
}

// Code returns ceg_grade_code.
func (g *Grade) Code() string { return g.code }

// Name returns ceg_grade_name.
func (g *Grade) Name() string { return g.name }

// IsActive returns ceg_is_active.
func (g *Grade) IsActive() bool { return g.isActive }

// Group returns the grade group, or nil when unassigned.
func (g *Grade) Group() *GradeGroup { return copyGroup(g.group) }

// HasGroup reports whether a grade group is assigned.
func (g *Grade) HasGroup() bool { return g.group != nil }

// AssignedBy returns the user of the last AssignGroup / ClearGroup in this
// instance ("" when unchanged). It is not persisted on cost_erp_grade; the
// application layer records it in the audit log.
func (g *Grade) AssignedBy() string { return g.assignedBy }

// Assignment returns the RuleSet entry for the grade and whether it has a
// group. Unassigned grades are not part of a RuleSet.
func (g *Grade) Assignment() (GradeAssignment, bool) {
	if g.group == nil {
		return GradeAssignment{}, false
	}
	return GradeAssignment{GradeCode: g.code, Group: *g.group}, true
}

func copyGroup(g *GradeGroup) *GradeGroup {
	if g == nil {
		return nil
	}
	c := *g
	return &c
}
