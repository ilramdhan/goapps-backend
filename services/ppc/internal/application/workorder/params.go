package workorder

import (
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// resolvedToPPCParameter maps a resolved parameter to a domain Parameter whose
// PPC value carries the resolved value; the PC value mirrors PPC by default.
func resolvedToPPCParameter(woID int64, rp workorderdomain.ResolvedParam) *workorderdomain.Parameter {
	p := &workorderdomain.Parameter{
		WOID:         woID,
		ParamID:      rp.ParamID,
		ParamCode:    rp.ParamCode,
		ParamName:    rp.ParamName,
		DataType:     rp.DataType,
		DisplayGroup: rp.DisplayGroup,
		DisplayOrder: rp.DisplayOrder,
		IsDual:       rp.IsDual,
		ValuePPCNum:  rp.Num,
		ValuePPCText: rp.Text,
		ValuePPCFlag: rp.Flag,
	}
	mirrorPPCIntoPC(p)
	return p
}

// copyParamAsPPC copies an existing parameter's PPC value into a fresh param for
// a new WO (TEMPLATE duplicate). PC mirrors PPC.
func copyParamAsPPC(woID int64, src *workorderdomain.Parameter) *workorderdomain.Parameter {
	p := &workorderdomain.Parameter{
		WOID:         woID,
		ParamID:      src.ParamID,
		ParamCode:    src.ParamCode,
		ParamName:    src.ParamName,
		DataType:     src.DataType,
		DisplayGroup: src.DisplayGroup,
		DisplayOrder: src.DisplayOrder,
		IsDual:       src.IsDual,
		ValuePPCNum:  src.ValuePPCNum,
		ValuePPCText: src.ValuePPCText,
		ValuePPCFlag: src.ValuePPCFlag,
	}
	mirrorPPCIntoPC(p)
	return p
}

// applyPPCValue writes a typed PPC value into a parameter.
func applyPPCValue(p *workorderdomain.Parameter, in ParamValueInput) {
	p.ValuePPCNum = in.Num
	p.ValuePPCText = in.Text
	if in.HasFlag {
		p.ValuePPCFlag = in.Flag
	}
}

// applyPCValue writes a typed PC value into a parameter.
func applyPCValue(p *workorderdomain.Parameter, in ParamValueInput) {
	p.ValuePCNum = in.Num
	p.ValuePCText = in.Text
	if in.HasFlag {
		p.ValuePCFlag = in.Flag
	}
}

// mirrorPPCIntoPC copies the PPC value into the PC value (single params, or the
// default PC value before PC confirmation).
func mirrorPPCIntoPC(p *workorderdomain.Parameter) {
	p.ValuePCNum = p.ValuePPCNum
	p.ValuePCText = p.ValuePPCText
	p.ValuePCFlag = p.ValuePPCFlag
}

func indexParams(params []*workorderdomain.Parameter) map[string]*workorderdomain.Parameter {
	m := make(map[string]*workorderdomain.Parameter, len(params))
	for _, p := range params {
		m[p.ParamID] = p
	}
	return m
}

func indexInputs(inputs []ParamValueInput) map[string]ParamValueInput {
	m := make(map[string]ParamValueInput, len(inputs))
	for _, in := range inputs {
		m[in.ParamID] = in
	}
	return m
}
