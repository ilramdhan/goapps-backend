package lookupsync

import (
	"context"
	"fmt"
	"sort"

	"github.com/rs/zerolog/log"
)

// fillFromBoxBobbinCost lives alone in this file: TestBoxBobbinSwitchColumnsMatchSource
// parses it to keep boxBobbinCostColumns in sync with the switch below.
//
//nolint:gocyclo // flat column switch; TestBoxBobbinSwitchColumnsMatchSource parses its case labels, so it cannot be split.
func (r *Resolver) fillFromBoxBobbinCost(ctx context.Context, bbcCode, triggerParamCode string) (FillResult, error) {
	bbc, err := r.boxBobbinRepo.GetByCode(ctx, bbcCode)
	if err != nil {
		return FillResult{}, err
	}

	childParams, err := r.fillChildren(ctx, triggerParamCode)
	if err != nil {
		return FillResult{}, err
	}

	// Get latest rates (best-effort).
	var latestBobRateMkt, latestBoxRateMkt float64
	rates, rateErr := r.boxBobbinRepo.ListRates(ctx, bbc.ID())
	if rateErr != nil {
		log.Ctx(ctx).Warn().Err(rateErr).Str("bbc_code", bbcCode).Msg("ListRates failed — returning without rate fills")
	} else if len(rates) > 0 {
		sort.Slice(rates, func(i, j int) bool { return rates[i].Period() > rates[j].Period() })
		latestBobRateMkt = rates[0].BobRateMkt()
		latestBoxRateMkt = rates[0].BoxRateMkt()
	}

	nums := make(map[string]float64, len(childParams))
	for _, p := range childParams {
		col := p.LookupSourceColumn()
		switch col {
		case "no_of_bob":
			nums[p.Code().String()] = float64(bbc.NoOfBob())
		case "bbcr_bob_rate_mkt":
			// A zero/missing rate is a deliberate no-fill, not an unknown column.
			if latestBobRateMkt > 0 {
				nums[p.Code().String()] = latestBobRateMkt
			}
		case "bbcr_box_rate_mkt":
			if latestBoxRateMkt > 0 {
				nums[p.Code().String()] = latestBoxRateMkt
			}
		// Per-master Oracle columns on mst_box_bobbin_cost (registered by 000412/000421).
		// bbcr_*_val is the legacy name 000407 seeded for DELIVERY_BOB/BOX_RATE; it never
		// existed as a column, so it is aliased to the VAL master columns (same intent,
		// see 000411 column comments).
		case "bbn_reuse":
			putOpt(nums, p.Code().String(), bbc.BbnReuse())
		case "box_reuse":
			putOpt(nums, p.Code().String(), bbc.BoxReuse())
		case "box_cost":
			putOpt(nums, p.Code().String(), bbc.BoxCost())
		case "bobin_cost":
			putOpt(nums, p.Code().String(), bbc.BobinCost())
		case "box_cost_val":
			putOpt(nums, p.Code().String(), bbc.BoxCostVal())
		case "bbcr_box_rate_val":
			putOpt(nums, p.Code().String(), bbc.BoxCostVal())
		case "bobin_cost_val":
			putOpt(nums, p.Code().String(), bbc.BobinCostVal())
		case "bbcr_bob_rate_val":
			putOpt(nums, p.Code().String(), bbc.BobinCostVal())
		case "bbn_reuse_val":
			putOpt(nums, p.Code().String(), bbc.BbnReuseVal())
		case "box_reuse_val":
			putOpt(nums, p.Code().String(), bbc.BoxReuseVal())
		default:
			warnNoReader(ctx, "Box bobbin cost", col, p.Code().String(), triggerParamCode)
		}
	}

	label := fmt.Sprintf("%s — %d bob/box", bbc.Name(), bbc.NoOfBob())
	return FillResult{
		Numeric:      nums,
		Text:         map[string]string{},
		DisplayLabel: label,
		Message:      "Box bobbin cost fill values retrieved",
	}, nil
}

// putOpt stores v under code when the nullable master column has a value.
func putOpt(m map[string]float64, code string, v *float64) {
	if v != nil {
		m[code] = *v
	}
}
