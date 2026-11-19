# legacy-fill-mb

One-shot CLI that fills MB head/spin columns (`mst_mb_head`, `mst_mb_spin`) from the reviewed
Oracle legacy proposal CSV (`docs/mb-active-sync/oracle-update-proposal.csv`).
Design: `docs/superpowers/specs/2026-11-19-mb-legacy-fill-design.md`.

## Run order

**legacy-fill-mb runs BEFORE `sync-mb-active`** (den/fil of some spins change, and the sync
allocator matches on den/fil):

1. `legacy-fill-mb --plan` and review the plan CSV
2. `legacy-fill-mb --apply` (head+spin batches)
3. V-L verification SQL
4. rebuild `sync-mb-active --plan`, then its phases A-D

If sync-mb-active already ran in an environment, rebuild its plan afterwards (expect mostly KEEP).

Config/DB come from the environment exactly like `sync-mb-active` (`config.Load()`,
`DATABASE_*` variables; local: `make v2-env-on`). Use the `<svc>_owner`-capable connection that
sync-mb-active uses; apply bypasses the head lock by design.

## Modes

```bash
# 1. read-only plan (READ ONLY transaction)
go run ./cmd/legacy-fill-mb --plan --proposal oracle-update-proposal.csv --out-dir out/

# 2. apply the APPLY rows of the reviewed plan (edit/prune the plan CSV first if needed)
go run ./cmd/legacy-fill-mb --apply --plan-file out/legacy-fill-mb-plan-<ts>.csv \
  --actor ilham --batch 50 [--limit N] [--only-sysid 123,456] --out-dir out/

# 3. revert (restores before-image only where the column still holds the after-image)
go run ./cmd/legacy-fill-mb --revert --input out/legacy-fill-mb-result-<ts>.csv[,more.csv] \
  --actor ilham --out-dir out/

# 4. recovery after an interrupted apply (recalc + affected-products from the result CSV(s))
go run ./cmd/legacy-fill-mb --post-effects-only --input out/legacy-fill-mb-result-<ts>.csv[,more.csv] \
  --actor ilham --out-dir out/

go run ./cmd/legacy-fill-mb --help
```

### Recovery after an interrupt

On SIGINT/SIGTERM the batch loop stops, but the post-commit recalc and the output files still run
to completion (detached from the signal context), so committed batches are never left un-recalculated.
If the process was killed hard (SIGKILL, crash) before that step, re-running `--apply` will not help:
rows already committed report `already_in_target` and are not recalculated. Run `--post-effects-only`
with the result CSV(s) of the interrupted run instead; it writes fresh `recalc-results` and
`affected-products` files. If no result CSV was written, re-run `--plan` and use its ALREADY rows
to identify what was committed.

`--batch N` = N head groups per batch (a head and all its spin rows stay together); head-bearing
groups run first. `--only-sysid` selects whole head groups: every group containing at least one row
with a listed SYSID is kept (a spin keeps its head and vice versa). `--limit N` caps APPLY rows (whole groups). Apply is idempotent: re-running yields
`ALREADY`/`already_in_target`.

## Outputs (`--out-dir`, files suffixed with a timestamp)

| Mode | File | Content |
|---|---|---|
| plan | `legacy-fill-mb-plan-*.csv` | every executable row + class, decision (APPLY/CHANGED/ALREADY/REJECT), reason |
| plan | `legacy-fill-mb-plan-summary-*.csv` | counts by decision, class/decision, skipped usulan |
| plan | `legacy-fill-mb-plan-rejected-*.csv` | rows rejected at parse (class X, unknown field/entity) |
| apply | `legacy-fill-mb-result-*.csv` | per row result + before/after JSON + side effects (revert input) |
| apply | `legacy-fill-mb-errors-*.csv` | FAILED rows only |
| apply | `legacy-fill-mb-recalc-results-*.csv` | post-commit R and D child recalc per parent spin |
| apply | `legacy-fill-mb-affected-products-*.csv` | yarn products to lookup-sync by the operator (no queue here) |
| revert | `legacy-fill-mb-revert-result-*.csv` | per item REVERTED / ALREADY_REVERTED / CHANGED_SINCE_APPLY / FAILED |
| revert | `legacy-fill-mb-revert-recalc-results-*.csv` | post-commit R and D child recalc per spin whose den/fil/doz was restored |
| revert | `legacy-fill-mb-revert-affected-products-*.csv` | yarn products bound to the reverted spins/children (operator lookup sync) |

After a revert commits, the R and D children of every spin whose `mbs_denier`/`mbs_filament`/`mbs_dozing`
was restored (directly or via the head cascade reversal) are recalculated from the restored parent
values, so they return to the values scaled from the old parent. Children that were manually edited
between apply and revert are recalculated too (same behavior as a UI parent edit).

Exit code is non-zero when any row (or revert item) FAILED, or the run was interrupted; all files
are still written.

## Class D: targeted cascade

When a head class-D row is APPLIED, legacy-fill does NOT run the UI cascade
(`cascadeHeadToSpinsTx` / `cascadeHeadToCostProductTx`), because that overwrites every live
spin with the head values. Instead only the head columns actually applied in the transaction
are propagated, and only to spins that still carry the OLD head value
(`spin col IS NOT DISTINCT FROM old head value`; numeric columns compared as NUMERIC, text
TRIMmed with NULL and empty equivalent). Spins that deliberately differ stay untouched.

| head column | spin column |
|-------------|-------------|
| mbh_mb_costing | mbs_mb_costing |
| mbh_mgt_name | mbs_mgt_name |
| mbh_denier | mbs_denier |
| mbh_filament | mbs_filament |
| mbh_dozing | mbs_dozing |
| mbh_final_product | mbs_final_product |
| mbh_vs_number | mbs_vs_number |

- dev, code, run_ldr, labels: no propagation. Orion: only fills the empty root spin via
  `fillRootSpinOrionTx` (guarded).
- Shade and lusture are NOT cascaded (shade is class S and handled per row).
- Cost product: when `mbh_mb_costing` changed, `cpm_product_name` is renamed only if the
  product is `MB_RECIPE` and its name equals the old costing; its shade is never touched.
- Spin legacy rows still run after the cascade and win. Spin/product before/after images are
  stored in `side_effects_json`, so `--revert` restores them exactly.
