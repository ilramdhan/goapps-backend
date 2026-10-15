package lookupsync

import (
	"context"
	"fmt"
)

func (r *Resolver) fillFromMachine(ctx context.Context, mcCode, triggerParamCode string) (FillResult, error) {
	mc, err := r.machineRepo.GetByCode(ctx, mcCode)
	if err != nil {
		return FillResult{}, err
	}

	childParams, err := r.fillChildren(ctx, triggerParamCode)
	if err != nil {
		return FillResult{}, err
	}

	nums := make(map[string]float64, len(childParams))
	for _, p := range childParams {
		col := p.LookupSourceColumn()
		numReader, hasNumReader := machineNumericReaders[col]
		if !hasNumReader {
			// Machine has no text readers, so an unmapped column is unfillable.
			warnNoReader(ctx, "Machine", col, p.Code().String(), triggerParamCode)
			continue
		}
		if val, hasVal := numReader(mc); hasVal {
			nums[p.Code().String()] = val
		}
	}

	label := fmt.Sprintf("%s (%s) — %d pos, %.0f m/min, %.1f%% eff",
		mc.Name(), mc.MCType(), mc.NoOfPosition(), mc.MCSpeed(), mc.MCEfficiency())
	return FillResult{
		Numeric:      nums,
		Text:         map[string]string{},
		DisplayLabel: label,
		Message:      "Machine fill values retrieved",
	}, nil
}

func (r *Resolver) fillFromIntermingling(ctx context.Context, intmCode, triggerParamCode string) (FillResult, error) {
	intm, err := r.interminglingRepo.GetByCode(ctx, intmCode)
	if err != nil {
		return FillResult{}, err
	}

	childParams, err := r.fillChildren(ctx, triggerParamCode)
	if err != nil {
		return FillResult{}, err
	}

	nums := make(map[string]float64, len(childParams))
	for _, p := range childParams {
		col := p.LookupSourceColumn()
		numReader, hasNumReader := interminglingNumericReaders[col]
		if !hasNumReader {
			// Intermingling has no text readers, so an unmapped column is unfillable.
			warnNoReader(ctx, "Intermingling", col, p.Code().String(), triggerParamCode)
			continue
		}
		if val, hasVal := numReader(intm); hasVal {
			nums[p.Code().String()] = val
		}
	}

	label := fmt.Sprintf("%s (%s) — %.4f USD/kg", intm.Name(), intm.Code(), intm.CostPerKg())
	return FillResult{
		Numeric:      nums,
		Text:         map[string]string{},
		DisplayLabel: label,
		Message:      "Intermingling fill values retrieved",
	}, nil
}

func (r *Resolver) fillFromProductGrade(ctx context.Context, pgCode, triggerParamCode string) (FillResult, error) {
	grade, err := r.productGradeRepo.GetByCode(ctx, pgCode)
	if err != nil {
		return FillResult{}, err
	}

	childParams, err := r.fillChildren(ctx, triggerParamCode)
	if err != nil {
		return FillResult{}, err
	}

	nums := make(map[string]float64, len(childParams))
	texts := make(map[string]string, len(childParams))
	for _, p := range childParams {
		col := p.LookupSourceColumn()
		numReader, hasNumReader := productGradeNumericReaders[col]
		if hasNumReader {
			if val, hasVal := numReader(grade); hasVal {
				nums[p.Code().String()] = val
			}
		}
		textReader, hasTextReader := productGradeTextReaders[col]
		if hasTextReader {
			if val, hasVal := textReader(grade); hasVal {
				texts[p.Code().String()] = val
			}
		}
		if !hasNumReader && !hasTextReader {
			warnNoReader(ctx, "Product grade", col, p.Code().String(), triggerParamCode)
		}
	}

	label := fmt.Sprintf("%s — BC %.1f%%, NonStd %.1f%%, Recovery %.1f%%",
		grade.Name(), grade.BCPerc(), grade.NonStdPerc(), grade.BCRecoveryRate())
	return FillResult{
		Numeric:      nums,
		Text:         texts,
		DisplayLabel: label,
		Message:      "Product grade fill values retrieved",
	}, nil
}

func (r *Resolver) fillFromMBHead(ctx context.Context, mbCosting, triggerParamCode string) (FillResult, error) {
	mbh, err := r.mbHeadRepo.GetByMBCosting(ctx, mbCosting)
	if err != nil {
		return FillResult{}, err
	}

	childParams, err := r.fillChildren(ctx, triggerParamCode)
	if err != nil {
		return FillResult{}, err
	}

	nums := make(map[string]float64, len(childParams))
	texts := make(map[string]string, len(childParams))
	for _, p := range childParams {
		col := p.LookupSourceColumn()
		numReader, hasNumReader := MBHeadNumericReaders[col]
		if hasNumReader {
			if val, hasVal := numReader(mbh); hasVal {
				nums[p.Code().String()] = val
			}
		}
		textReader, hasTextReader := mbHeadTextReaders[col]
		if hasTextReader {
			if val, hasVal := textReader(mbh); hasVal {
				texts[p.Code().String()] = val
			}
		}
		if !hasNumReader && !hasTextReader {
			warnNoReader(ctx, "MB Head", col, p.Code().String(), triggerParamCode)
		}
	}

	label := mbh.MBCosting()
	if d := mbh.Dozing(); d != nil {
		label = fmt.Sprintf("%s — %.2f%% dozing", mbh.MBCosting(), *d)
	}
	return FillResult{
		Numeric:      nums,
		Text:         texts,
		DisplayLabel: label,
		Message:      "MB Head fill values retrieved",
	}, nil
}
