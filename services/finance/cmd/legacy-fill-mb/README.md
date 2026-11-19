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

go run ./cmd/legacy-fill-mb --help
```

`--batch N` = N SYSID groups per batch (a SYSID's head and spin rows stay together); head-bearing
groups run first. `--limit N` caps APPLY rows (whole groups). Apply is idempotent: re-running yields
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

Exit code is non-zero when any row (or revert item) FAILED, or the run was interrupted; all files
are still written.
