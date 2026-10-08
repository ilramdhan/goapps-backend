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
  as 0. `OIL_RATE` is also engine-injected per period (`oil_rate.go` `resolveOilRate`).
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

(Section to be added once the CAP_PACK POY default change from branch `feat/cap-pack-poy-default` is merged.)
