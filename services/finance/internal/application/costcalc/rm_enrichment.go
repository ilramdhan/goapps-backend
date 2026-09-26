package costcalc

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// ResultDisplay carries the enrichment fields the list view already resolves
// via SQL joins (see CostResultRepository.ListResults), computed here in Go
// for the single-result path (GetCostResult / GetCostBreakdown), which reads
// straight from cst_product_cost with no such joins.
type ResultDisplay struct {
	ItemCode      string
	ItemName      string
	ShadeCode     string
	ShadeName     string
	PrimaryRMCode string
	PrimaryRMName string
	RMCount       int32
	RMDetails     []costcalcdom.RMDetailSummary
}

// productRMRefPrefix is the synthetic ref_code prefix rmRefCode (compute.go)
// uses for a PRODUCT-type RM line: "product:<product_sys_id>". Kept here as a
// single source of truth for parsing it back apart.
const productRMRefPrefix = "product:"

// resolveResultDisplay resolves the item/shade identity of productSysID plus
// the full, name-resolved RM breakdown of detail, for the single-result read
// path. It never errors on missing lookups — a name that fails to resolve
// just falls back to the raw code, matching the list view's COALESCE-to-empty
// behavior.
func (s *Service) resolveResultDisplay(ctx context.Context, productSysID int64, detail []RMCostDetail) (ResultDisplay, error) {
	disp := ResultDisplay{RMCount: safeIntToInt32Len(len(detail))}

	products, err := s.loader.LoadProducts(ctx, []int64{productSysID})
	if err != nil {
		return disp, fmt.Errorf("load product for display: %w", err)
	}
	if p := products[productSysID]; p != nil {
		disp.ShadeCode = p.ShadeCode()
		disp.ShadeName = p.ShadeName()
		disp.ItemCode = p.ErpItemCode()
	}

	names, productRefs, err := s.resolveRMNames(ctx, detail, products)
	if err != nil {
		return disp, err
	}
	if disp.ItemCode != "" {
		if in, ok := names[rmNameKey(costroute.RmTypeItem, disp.ItemCode)]; ok {
			disp.ItemName = in
		}
	}

	details := make([]costcalcdom.RMDetailSummary, 0, len(detail))
	for _, d := range detail {
		refCode := d.RefCode
		refName := names[rmNameKey(d.RMType, d.RefCode)]
		if d.RMType == costroute.RmTypeProduct {
			if sysID, ok := parseProductRMRef(d.RefCode); ok {
				if p := productRefs[sysID]; p != nil {
					refCode = p.ProductCode()
					refName = p.ProductName()
				}
			}
		}
		details = append(details, costcalcdom.RMDetailSummary{
			RouteLevel:   d.RouteLevel,
			RMType:       d.RMType,
			RefCode:      refCode,
			RefName:      refName,
			ShadeCode:    d.ShadeCode,
			UnitCost:     d.UnitCost,
			Ratio:        d.Ratio,
			Contribution: d.Contribution,
		})
	}
	sort.SliceStable(details, func(i, j int) bool { return details[i].Contribution > details[j].Contribution })
	disp.RMDetails = details
	if len(details) > 0 {
		disp.PrimaryRMCode = details[0].RefCode
		disp.PrimaryRMName = details[0].RefName
	}
	return disp, nil
}

// resolveRMNames batch-resolves display names for every distinct (rm_type,
// ref_code) pair in detail. GROUP and ITEM names come from optional loader
// capabilities (absent in test fakes → names just stay unresolved, matching
// OilGroupNameLoader's established fallback pattern); PRODUCT-type refs are
// resolved by the caller via the returned productRefs map (parsed sys IDs
// loaded through the always-present ProductLoader.LoadProducts).
func (s *Service) resolveRMNames(
	ctx context.Context, detail []RMCostDetail, alreadyLoaded map[int64]*costproductmaster.CostProductMaster,
) (map[string]string, map[int64]*costproductmaster.CostProductMaster, error) {
	groupCodes, itemCodes, productIDs := collectRMRefs(detail)

	names, err := s.resolveRMGroupAndItemNames(ctx, groupCodes, itemCodes)
	if err != nil {
		return nil, nil, err
	}
	productRefs, err := s.resolveRMProductRefs(ctx, productIDs, alreadyLoaded)
	if err != nil {
		return nil, nil, err
	}
	return names, productRefs, nil
}

// resolveRMGroupAndItemNames resolves GROUP/ITEM display names via the
// loader's optional capabilities. A loader without a given capability (test
// fakes) simply yields no names for that kind — matching
// OilGroupNameLoader's established fallback pattern.
func (s *Service) resolveRMGroupAndItemNames(ctx context.Context, groupCodes, itemCodes []string) (map[string]string, error) {
	names := map[string]string{}
	if gl, ok := s.loader.(OilGroupNameLoader); ok && len(groupCodes) > 0 {
		groupNames, err := gl.LoadRMGroupNames(ctx, groupCodes)
		if err != nil {
			return nil, fmt.Errorf("load RM group names: %w", err)
		}
		for code, name := range groupNames {
			names[rmNameKey(costroute.RmTypeGroup, code)] = name
		}
	}
	if il, ok := s.loader.(RMItemNameLoader); ok && len(itemCodes) > 0 {
		itemNames, err := il.LoadItemNames(ctx, itemCodes)
		if err != nil {
			return nil, fmt.Errorf("load RM item names: %w", err)
		}
		for code, name := range itemNames {
			names[rmNameKey(costroute.RmTypeItem, code)] = name
		}
	}
	return names, nil
}

// resolveRMProductRefs loads whichever PRODUCT-type ref product IDs are not
// already in alreadyLoaded (e.g. the RM line's own stage product, loaded once
// by the caller) and returns the union.
func (s *Service) resolveRMProductRefs(
	ctx context.Context, productIDs []int64, alreadyLoaded map[int64]*costproductmaster.CostProductMaster,
) (map[int64]*costproductmaster.CostProductMaster, error) {
	missing := make([]int64, 0, len(productIDs))
	for _, id := range productIDs {
		if _, ok := alreadyLoaded[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return alreadyLoaded, nil
	}
	loaded, err := s.loader.LoadProducts(ctx, missing)
	if err != nil {
		return nil, fmt.Errorf("load RM product refs: %w", err)
	}
	merged := make(map[int64]*costproductmaster.CostProductMaster, len(alreadyLoaded)+len(loaded))
	for k, v := range alreadyLoaded {
		merged[k] = v
	}
	for k, v := range loaded {
		merged[k] = v
	}
	return merged, nil
}

// collectRMRefs partitions detail's distinct ref_code values by rm_type.
func collectRMRefs(detail []RMCostDetail) (groupCodes, itemCodes []string, productIDs []int64) {
	seenGroup, seenItem, seenProduct := map[string]bool{}, map[string]bool{}, map[int64]bool{}
	for _, d := range detail {
		switch d.RMType {
		case costroute.RmTypeGroup:
			if d.RefCode != "" && !seenGroup[d.RefCode] {
				seenGroup[d.RefCode] = true
				groupCodes = append(groupCodes, d.RefCode)
			}
		case costroute.RmTypeItem:
			if d.RefCode != "" && !seenItem[d.RefCode] {
				seenItem[d.RefCode] = true
				itemCodes = append(itemCodes, d.RefCode)
			}
		case costroute.RmTypeProduct:
			if sysID, ok := parseProductRMRef(d.RefCode); ok && !seenProduct[sysID] {
				seenProduct[sysID] = true
				productIDs = append(productIDs, sysID)
			}
		}
	}
	return groupCodes, itemCodes, productIDs
}

// parseProductRMRef extracts the product_sys_id out of a PRODUCT-type
// ref_code, which rmRefCode (compute.go) writes as "product:<sys_id>" rather
// than a real product code.
func parseProductRMRef(refCode string) (int64, bool) {
	rest, ok := strings.CutPrefix(refCode, productRMRefPrefix)
	if !ok {
		return 0, false
	}
	sysID, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return sysID, true
}

// rmNameKey namespaces a name lookup by rm_type so a GROUP code and an ITEM
// code that happen to collide never resolve to each other's name.
func rmNameKey(rmType, refCode string) string {
	return rmType + "|" + refCode
}

// safeIntToInt32Len bounds-checks a slice length before an int32 cast
// (gosec G115) — cpc_rm_cost_detail arrays never approach MaxInt32 entries in
// practice, but the helper keeps the cast honest regardless.
func safeIntToInt32Len(n int) int32 {
	const maxInt32 = 1<<31 - 1
	if n > maxInt32 {
		return maxInt32
	}
	return int32(n) //nolint:gosec // bounds checked above
}
