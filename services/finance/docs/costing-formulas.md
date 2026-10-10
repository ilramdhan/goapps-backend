# Costing formulas: mechanics, change policy, pending TODOs

Living doc. Update it whenever a costing formula, flag or policy changes.
Scope: the Finance cost-calculation engine (`internal/application/costcalc`).

## 1. How formulas work

Tables:
- `mst_formula`: `formula_code`, `formula_type` (CALCULATION, CONSTANT, CONDITIONAL, RM_LOOKUP, ...),
  `expression`, `result_param_id` (the param the formula produces), `is_active`.
- `formula_param`: edges `(formula_id, param_id, sort_order)`, unique `(formula_id, param_id)`.
  An edge means "this formula reads this param".

Engine behaviour (all in `internal/application/costcalc/`):
- `loader.go` `LoadFormulas` / `loadPerProductFormulas` (~line 761-810) loads, per product, only
  formulas whose result param is in the product's `cost_product_applicable_param` (CAPP) AND
  `f.is_active = TRUE` (~line 808). A formula whose result param is not attached to the product
  silently does not run, so new result params must be attached to products (see `000525`).
- CONSTANT formulas referenced by a loaded formula are auto-loaded WITHOUT a CAPP attach
  (`appendReferencedConstants`, `loader.go`): for every input of a loaded formula that no loaded
  formula produces and that is not attached to the product, an active CONSTANT formula producing
  that param is added (one batched query, no recursion since CONSTANTs have no inputs). CAPP still
  wins if the param is attached. Unreferenced CONSTANTs are never loaded. Only CONSTANT: other
  formula types still require the CAPP attach. Applies to the batch calc and `mbbatch` (same loader).
- `topoSortFormulas` (`loader.go` ~983, Kahn) orders formulas by the `formula_param` edges.
- `buildInitialScope` (`compute.go` ~334-373) zero-fills every edge param missing from the scope
  (and the formula's own result param) with `0`, silently.
  Consequence: EVERY param referenced in an expression MUST have a `formula_param` edge.
  A missing edge means wrong evaluation order and/or a silent zero, with no error.
- Expression syntax (expr-lang): arithmetic, ternary `a ? b : c` (nestable), `==`, `&&`, `||`.
  Ternary formulas use type `CALCULATION` (e.g. `F_YARN_OIL_GAIN`, `F_YARN_CAP_PACK`).
- Product-class flags are NOT `mst_parameter` rows; the engine injects them as float64 1/0 from
  `cost_product_type.cpt_oil_class`: `IS_PTY`, `IS_POY`, `IS_SUPERBA`
  (`oil_rate.go:29,31,33`; injected by `injectProductClassFlags`, `oil_rate.go:63`, called from
  `compute.go` `buildInitialScope`). They need no edge. A product with no oil class gets all three
  as 0. `IS_ACTUAL` (1 for the ACTUAL calc type, 0 for FORECAST/SELLING; `oil_rate.go`
  `injectCalcTypeFlags`, also from `buildInitialScope`) is injected the same way, per `ComputeInput.CalcType`
  (the eval cache only memoises compiled expressions, so it is safe across calc types). `OIL_RATE` is also engine-injected per period (`oil_rate.go` `resolveOilRate`).
  See also the header of migration `000524`.

## 2. Change policy

Editing a formula through the web UI is fine for a quick prod experiment, but it MUST be followed by
an additive, guarded migration so seeds/other environments catch up. Prod may be ahead of the seeds
(example: `F_YARN_WASTE_LESS_MB_OPU` was edited via web on 2026-10-07 and synced by `000560`).

Migration pattern (see `000510`, `000524`, `000560`; never edit an applied migration):
- `UPDATE mst_formula ... WHERE formula_code = X AND expression = '<exact current text>'` plus an
  `EXISTS` guard on the result param code. Idempotent, never clobbers a hand edit.
- `INSERT INTO formula_param ... WHERE NOT EXISTS (...)` for every referenced param.
- Tag rows with `updated_by = '<topic>_<migration no>'`; the down file removes edges first (while the tag
  still identifies the rewritten formula), restores the old expression, and removes anything created.

ALWAYS run the read-only check first. `docs/export-product-cost/verify-top92-93-conv-ex-mb.sql` lives in
the goapps workspace (parent dir `../docs/...` outside this repo, not in this repo). Core queries:

```sql
-- Q1: live expression of formulas by result param
SELECT f.formula_code, p.param_code AS result_param, f.expression, f.formula_type,
       f.is_active, f.updated_at, f.updated_by
FROM mst_formula f
JOIN mst_parameter p ON p.id = f.result_param_id
WHERE f.deleted_at IS NULL AND f.formula_code IN ('F_YARN_CONV_CAP','F_YARN_CONV_DEL')
ORDER BY f.formula_code;

-- Q2: edges of a formula
SELECT f.formula_code, fp.sort_order, p.param_code
FROM mst_formula f
JOIN formula_param fp ON fp.formula_id = f.id
JOIN mst_parameter p  ON p.id = fp.param_id
WHERE f.formula_code IN ('F_YARN_CONV_CAP','F_YARN_CONV_DEL') AND f.deleted_at IS NULL
ORDER BY f.formula_code, fp.sort_order;
```

Q3 in that file compares per-product term values (from `cst_product_cost.cpc_param_snapshot`) with the
legacy formula; use it after every recalc.

## 3. Rows 92 / 93: Only Conversion Cap./Del. Packing ex MB

Params `ONLY_CONV_CAP_PACK_EXCL_MB` (92) / `ONLY_CONV_DEL_PACK_EXCL_MB` (93), formulas
`F_YARN_CONV_CAP` / `F_YARN_CONV_DEL`. Legacy formula (confirmed by Finance 2026-10-07):

```
cap_pack | del_pack + waste_less_mb_doz_opu + heat_set_cost_per_kg + oil_cost + intermingling
 + spl_cost_1 + spl_cost_2 + steam_cost_cng + softener_cost + washing_cost + total_91 + oil_gain
```

| Legacy term | GoApps param | Source |
|---|---|---|
| total_91 | `TOTAL_FIXEDCOST_PER_KG` | formula `F_YARN_TOTAL_FIXED` |
| cap_pack / del_pack | `CAPTIVE_PACK_COST` / `DELIVERY_PACK_COST` | formulas `F_YARN_CAP_PACK` / `F_YARN_DEL_PACK` |
| waste_less_mb_doz_opu | `WASTE_LESS_MB_OPU` | formula `F_YARN_WASTE_LESS_MB_OPU` = `(RM_LANDED_COST * RM_NORMS) - RM_RATE`; a cost, ADDED |
| heat_set_cost_per_kg | `HEATSET_COST_PER_KG` | formula |
| oil_cost | `OIL_COST` | formula `F_YARN_OIL_COST` (000524) |
| intermingling | `INTERMINGLING` | fill-group lookup from `mst_intermingling.cost_per_kg` via `intm_cost_per_kg`, stored per product |
| spl_cost_1 | `SPECIAL_COST_1` | INPUT |
| spl_cost_2 | `SPECIAL_COST_2` | independent INPUT since `000560` (mirror formula `F_YARN_SPECIAL_COST_2` deactivated; may differ from cost 1) |
| softener_cost | `SOFTNER_COST` (spelled SOFTNER) | INPUT |
| steam_cost_cng | `STEAM_COST_CNG` | formula `F_YARN_STEAM_COST` (currently `0`, see TODO) |
| washing_cost | `WASHING_COST` | formula `F_YARN_WASHING_COST` (currently `0`, see TODO) |
| oil_gain | `OIL_GAIN` | formula `F_YARN_OIL_GAIN`; stored NEGATIVE, added |

History: `000408` seed (rows 5 terms) -> `000510` (+`OIL_GAIN`) -> `000560` (+6 terms).
Downstream consumers: `F_YARN_CAP_PRE_QL` / `F_YARN_DEL_PRE_QL` (`000532`).
Export mapping: `internal/worker/costsheet_alldata.go` (~160-162).
Test model: `internal/application/costcalc/compute_conv_ex_mb_terms_test.go`.

## 4. TODO: STEAM_COST_CNG and WASHING_COST

Current state: `F_YARN_STEAM_COST` and `F_YARN_WASHING_COST` have expression literal `0`
(`000408:60-61`, type CALCULATION). They are already wired into rows 92/93 by `000560`, so ONLY their
own formulas need to change. Do NOT touch `F_YARN_CONV_CAP` / `F_YARN_CONV_DEL`.
In the 2026-08 data both are 0 for all 17,649 products.

The legacy rule is unknown: the legacy system has per-product-type conditional logic. Ask Finance / the
legacy team for the `PKG_YARN_CALCULATION` source per product type.

Recipe:
1. Get the legacy rule per product type plus 2-3 sample products with legacy values.
2. Identify or create the input params; make sure the products have them attached in CAPP and have values.
3. Write the expression with the product flags, e.g.
   `IS_SUPERBA == 1 ? <superba rule> : (IS_PTY == 1 ? <pty rule> : 0)`.
4. Migration (pattern of `000524`): `UPDATE` guarded on `expression = '0'` AND `formula_code` AND the result
   param (`STEAM_COST_CNG` / `WASHING_COST`). Type stays `CALCULATION` (ternaries work with it, like
   `F_YARN_OIL_GAIN`). `INSERT` a `formula_param` edge for every referenced param (flags need none).
   Check no cycle: must not depend on `ONLY_CONV_*` or anything downstream. Down restores `'0'` and removes
   the edges.
5. Unit test in `internal/application/costcalc` like `compute_conv_ex_mb_terms_test.go`, including a
   migration text assertion.
6. Recalc and verify with Q3 of the verify SQL against the legacy sample values.

## 5. Rows 42 / 43: Cap-/Del-Pack cost

Params `CAPTIVE_PACK_COST` (42) / `DELIVERY_PACK_COST` (43), formulas `F_YARN_CAP_PACK` /
`F_YARN_DEL_PACK`. Both feed rows 92 / 93 (section 3) and the box-weight chain below.

**Row 42 (since `000561`, Finance requirement 2026-10-08):** POY products get a configurable
default instead of the box formula; every other product type keeps the formula.

```
F_YARN_CAP_PACK = IS_POY == 1 ? CAP_PACK_POY_DEFAULT
                : (CAPTIVE_BOX_WT > 0 ? (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE) / CAPTIVE_BOX_WT : 0)
F_YARN_CAP_PACK_POY_DEFAULT (CONSTANT) = 0.0078  -> CAP_PACK_POY_DEFAULT
```

Same pattern as `OIL_GAIN_POY_DEFAULT` (`000524` / `000525`): the value lives in its own CONSTANT
formula, never as a literal inside `F_YARN_CAP_PACK`.

| Change wanted | How |
|---|---|
| New POY value (e.g. 0.0078 -> 0.0080) | Edit expression of `F_YARN_CAP_PACK_POY_DEFAULT` in Master Formula (web). No migration needed for a value-only change, then recalc. |
| POY back to the box formula | Remove the `IS_POY == 1 ? CAP_PACK_POY_DEFAULT : ( ... )` wrapper from `F_YARN_CAP_PACK`, then follow up with a guarded migration (section 2). The `CAP_PACK_POY_DEFAULT` edge/param can stay; unused edges are harmless. |
| Other product type gets its own default | Add another CONSTANT formula + param (`CAP_PACK_<TYPE>_DEFAULT`), nest another ternary branch, add the `formula_param` edge, attach the param to those products. |

Gotchas:
- `IS_POY` is engine-injected from `cost_product_type.cpt_oil_class` (`oil_rate.go` `injectProductClassFlags`), not an `mst_parameter` row: no edge needed for it.
- A formula only runs if its result param is in the product's applicable params. `000561` attached
  `CAP_PACK_POY_DEFAULT` to existing POY products (excl. MB, marker `seed_cap_pack_poy_000561`).
  No attach is needed for new POY products: since the auto-load of referenced CONSTANT formulas
  (section 1), `F_YARN_CAP_PACK_POY_DEFAULT` loads whenever `F_YARN_CAP_PACK` loads. `000561`'s
  attach is harmless. The same applies to `OIL_GAIN_POY_DEFAULT` (`F_YARN_OIL_GAIN_POY_DEFAULT`
  used by `F_YARN_OIL_GAIN`).
- Rows 42 and 43 pack rates depend on the calc type since `000564`, see below.

### Pack rates by calc type (since `000564`)

ACTUAL uses VAL rates (master `bobin_cost_val` / `box_cost_val`); FORECAST and SELLING use MKT rates
(master `bobin_cost` / `box_cost`, "Bobbin/Box Rate MKT"). Selected by the engine flag `IS_ACTUAL`.

```
F_YARN_CAP_PACK = IS_POY == 1 ? CAP_PACK_POY_DEFAULT : (CAPTIVE_BOX_WT > 0 ?
    (IS_ACTUAL == 1 ? (CAPTIVE_NO_OF_BOB * CAP_BOB_RATE_VAL + CAP_BOX_RATE_VAL)
                    : (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE)) / CAPTIVE_BOX_WT : 0)
F_YARN_DEL_PACK = DELIVERY_BOX_WT > 0 ?
    (IS_ACTUAL == 1 ? (DELIVERY_NO_OF_BOB * DELIVERY_BOB_RATE + DELIVERY_BOX_RATE)
                    : (DELIVERY_NO_OF_BOB * DEL_BOB_RATE_MKT + DEL_BOX_RATE_MKT)) / DELIVERY_BOX_WT : 0
```

| Param | Fill group | Source column | Used for |
|---|---|---|---|
| `CAPTIVE_BOB_RATE` / `CAPTIVE_BOX_RATE` (existing) | `CAPTIVE_PACK_CODE` | `bbcr_bob_rate_mkt` / `bbcr_box_rate_mkt` | captive MKT |
| `CAP_BOB_RATE_VAL` / `CAP_BOX_RATE_VAL` (new) | `CAPTIVE_PACK_CODE` | `bobin_cost_val` / `box_cost_val` | captive ACTUAL |
| `DELIVERY_BOB_RATE` / `DELIVERY_BOX_RATE` (existing) | `DELIVERY_PACK_CODE` | `bobin_cost_val` / `box_cost_val` | delivery ACTUAL |
| `DEL_BOB_RATE_MKT` / `DEL_BOX_RATE_MKT` (new) | `DELIVERY_PACK_CODE` | `bobin_cost` / `box_cost` | delivery FORECAST/SELLING |

Asymmetry: captive MKT reads the latest-period rate history (`bbcr_*_mkt`), delivery MKT reads the
master columns directly. To change a source: Master Parameter -> edit the param -> Source Column (web,
no migration), then re-save the pack code on products (or rely on the migration backfill) and recalc.
A recalculation is required for existing results to change. A guarded migration (`000564`) attached the
new params to every product having the parent pack code and backfilled their values; attaching a pack
code in the UI auto-attaches all active fill-group children, so new products get them too.

Upstream chain (prod expressions as of 2026-10; prod is ahead of the `000408` seed text for the
box/bobbin weights, so check Q1-style SQL before writing a guard):

```
CAPTIVE_BOX_WT   = CAPTIVE_NO_OF_BOB * NET_BOB_WT
NET_BOB_WT       = (AX_WT*AX_PERC/100) + (AE_WT*AE_PERC/100) + (A9_WT*A9_PERC/100)
                 + (A_WT*A_PERC/100) + (B_WT*B_PERC/100) + (C_WT*C_PERC/100)
CAPTIVE_NO_OF_BOB = marketing_result(product,'CAPTIVE_NO_OF_BOB',period)   -- F_YARN_CAP_NO_BOB_FROM_MKT
```

Test model: `internal/application/costcalc/compute_cap_pack_poy_test.go`.

## 6. Row 73 MB Cost Marketing for SUPERBA (Superba Cost SP master)

`MB_COST_MKT` (row 73, formula `F_YARN_MB_COST`) is overridden for SUPERBA-class products only
(`cost_product_type.cpt_oil_class = 'SUPERBA'`) by migrations `000567` + `000568`:

```
F_YARN_MB_COST = IS_SUPERBA == 1 ? SUPERBA_MB_COST : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0)
```

- **Prod drift**: PROD's formula had been web-edited (admin, 2026-09-24) to
  `((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0`, so `000567` (guarded on the 000408 text
  `MB_RATE_MKT * MB_SP_DOZING / 100.0`) matched 0 rows there. `000568` rewrites both that prod text (case A) and
  000567's output in lower envs (case B, syncing them to the waste-adjusted arm), adds the
  `F_YARN_MB_COST -> WASTE_PERC` edge if missing, and backs up to `bak_000568_formula`.

- `SUPERBA_MB_COST` is an **engine-injected** scope key (like `IS_POY` / `IS_ACTUAL`): no `mst_parameter`
  row, no `formula_param` edge. Loader `LoadSuperbaCost` (`internal/application/costcalc/loader_superba.go`)
  resolves it, `applySuperbaMBCost` (`compute.go`, right after `applyOilRate`) injects it.
- Source: table `cost_superba_cost_sp` (master page `/finance/master/superba-cost-sps`). Link =
  `cost_product_master.cpm_shade_code` = `shade_code`, compared `UPPER(TRIM())`, active + not deleted only.
  Duplicate shade code → row with the largest `legacy_sys_id`. Value used = `old_value` always
  (`new_value` is informational).
- Missing row for a SUPERBA product → product **BLOCKED** with reason `MISSING_SUPERBA_COST` (never silent 0).
- Non-SUPERBA products and the MB batch path (`ComputeInput.Oil == nil`) get `SUPERBA_MB_COST = 0`, are never
  looked up or blocked, and keep the old formula arm. Test model: `compute_superba_mb_cost_test.go`.
- Row 64 `MB_SP_DYE`: for SUPERBA products the costing export and the product-master Param tab
  (`RequiredParamEntry.display_value`) show the master's colour name. Display only; the stored param is untouched.
- Seed: `000566` (644 rows from `docs/SUPERBA_COST_SP.CSV`, cp1252 → UTF-8, `source='SEED'`), generator
  `scripts/gen_superba_cost_sp_seed.py`.
- **Sync**: `SyncSuperbaCostSps` upserts by `legacy_sys_id` and overwrites MANUAL rows (source → ORACLE). The Oracle
  source is **not configured yet** (as of 2026-10-09 no `MGTDAT` table holds the CSV's Sys Id + OLD Value;
  `MGT_CDM_SHADE_SUPERBA`, `MGT_CST_MKT.MKTCST_MBCOST`, `OT_STD_COST_PRODUCTS_MGT.FG_MB` were checked and ruled
  out). Until the legacy team names the source, Sync returns "not configured" (409) and values are maintained via
  seed + manual edit.
- **Future superba formula**: implement the `SuperbaCostSource` seam in costcalc instead of the master lookup;
  `F_YARN_MB_COST` does not need to change.
- **Rollout rule**: before applying `000567` in an environment, run the coverage query below; every row returned
  will be BLOCKED after the migration.

```sql
SELECT pm.cpm_product_sys_id, pm.cpm_product_code, pm.cpm_shade_code
FROM cost_product_master pm
JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id AND pt.cpt_oil_class = 'SUPERBA'
WHERE NOT EXISTS (
  SELECT 1 FROM cost_superba_cost_sp s
  WHERE s.is_active AND s.deleted_at IS NULL
    AND UPPER(TRIM(s.shade_code)) = UPPER(TRIM(pm.cpm_shade_code)));
```

### MB source resolver (shade-driven MB_SP_CODE / MB_SP_DYE auto-fill, 000569)

The user only enters the product **shade**; the backend fills the MB source parameters.

- **Where**: `internal/domain/mbsource` (resolver + providers), `internal/application/mbsourceautofill`
  (service), hooks on product create, product update and the CPM bulk import (all best-effort: a
  failure is logged and never fails the save), plus one-time backfill migration `000569`.
- **Match**: `UPPER(TRIM(shade))` both sides, exact. Providers are ordered and batch-only; first hit
  wins, so **MB spin wins over Superba** when a shade exists in both.
- **MB spin provider**: live + active `mst_mb_spin` rows by `mbs_shade_code`. Several rows per shade
  -> one deterministic pick: `mbs_status` Spinning > Boughtout > R and D (other/NULL last), then
  newest (`GREATEST(created_at, updated_at)` desc), then `mbs_id`. The ORDER BY lives in
  `mbsource.SpinPickOrderSQL` and migration 000569 repeats the same text (a test asserts equality).
  MB_SP_CODE = ORION item code (else mb_costing, else spin id), companion `cpp_value_mb_spin_id` =
  the picked spin, children = the same columns the Param-tab fill uses (`mbspin.NumericFillReaders` /
  `TextFillReaders`, shared with `yarn_lookup_fill_handler`).
- **Superba provider** (`cost_superba_cost_sp`): MB_SP_CODE = the normalized shade, MB_SP_DYE =
  colour name; rate / dozing / other children stay empty (MB cost still comes from the
  `IS_SUPERBA` branch above).
- **Write rules**: only EMPTY cells; a product that already has any MB_SP_CODE value is skipped
  entirely; locked and MB-typed products are skipped. MB_SP_CODE + its fill-group children are
  attached (CAPP) only for products whose shade resolved. Values carry
  `cpp_filled_by = 'auto_mb_source'` (backfill: `'backfill_mb_source_000569'`) so a future
  "re-resolve" can find them. Values are frozen like any manual fill (no calc-time lookup; engine
  and `mbbatch` unchanged).
- **Fill handler**: selecting/refreshing a Superba shade in the Param tab no longer fails; when the
  MB_SPIN lookup is NotFound the handler falls back to the Superba provider and fills MB_SP_DYE.
- **Future merge of Superba into MB spin** (rows keyed by shade): delete `SuperbaProvider` and the
  handler fallback; MBSpinProvider then hits first for those shades; re-resolve products whose
  MB_SP_CODE holds a shade code (identifiable via `cpp_filled_by` and "no matching spin id") so they
  get rate/dozing/spin id; finally drop the `IS_SUPERBA` branch of `F_YARN_MB_COST` with a guarded
  (prod-text) formula migration, 000568-style. No per-product source is persisted, so no product
  rewrite is required.
