package costfillassignment

// ResolvedConfig is the fully-merged, snapshot-ready config for one route level.
type ResolvedConfig struct {
	RouteLevel        int32
	FillerType        string
	FillerValue       string
	ApproverType      string
	ApproverValue     string
	ReapproveOnChange bool
	SLAFillHours      int32
	SLAApproveHours   int32
}

// HasApprover reports whether an approver was configured.
func (r ResolvedConfig) HasApprover() bool { return r.ApproverType != "" }

// Resolve performs a field-level merge: global is the base; product then request
// override only their non-nil fields. global is required (it must define filler).
func Resolve(global, product, request *Config) (ResolvedConfig, error) {
	if global == nil {
		return ResolvedConfig{}, ErrConfigNotFound
	}
	out := ResolvedConfig{RouteLevel: global.RouteLevel}
	applyConfig(&out, global)
	applyConfig(&out, product)
	applyConfig(&out, request)
	if out.FillerType == "" || out.FillerValue == "" {
		return ResolvedConfig{}, ErrConfigNotFound
	}
	return out, nil
}

func applyConfig(out *ResolvedConfig, c *Config) {
	if c == nil {
		return
	}
	if c.FillerType != nil {
		out.FillerType = *c.FillerType
	}
	if c.FillerValue != nil {
		out.FillerValue = *c.FillerValue
	}
	if c.ApproverType != nil {
		out.ApproverType = *c.ApproverType
	}
	if c.ApproverValue != nil {
		out.ApproverValue = *c.ApproverValue
	}
	if c.ReapproveOnChange != nil {
		out.ReapproveOnChange = *c.ReapproveOnChange
	}
	if c.SLAFillHours != nil {
		out.SLAFillHours = *c.SLAFillHours
	}
	if c.SLAApproveHours != nil {
		out.SLAApproveHours = *c.SLAApproveHours
	}
}
