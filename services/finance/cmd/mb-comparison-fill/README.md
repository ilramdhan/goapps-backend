# mb-comparison-fill

Writes the Finance mb-comparison Excel values (cols 63-71) into `cost_product_parameter` of the matching
ACTIVE products, inserting the missing `cost_product_applicable_param` rows (the engine only reads CPP values that have a CAPP).
Spec: `docs/superpowers/specs/2026-11-24-mb-comparison-p1-design.md` section 3. Requires migration 000616.

```
mb-comparison-fill plan   --file docs/mb-comparison-p1/dozing.csv --orion docs/mb-active-sync/oracle-results/O-01b-head-all-by-sysid.csv --out-dir out/
mb-comparison-fill apply  --plan-file out/mb-comparison-fill-plan-*.csv --actor mbcmp-<env> [--limit N] [--batch N] [--commit]
mb-comparison-fill revert --results out/mb-comparison-fill-apply-result-*.csv --actor mbcmp-<env>
```

- DB from the usual finance config/env (`config.Load`), like the other cmd tools.
- `plan` is read-only. Decisions: `APPLY`, `SKIP_SAME`, `REVIEW` (several products / duplicate Excel key), `REPORT` (not found, `#N/A`, malformed number, locked).
- `apply` is a DRY-RUN (rolled back, `*-apply-dryrun-*.csv`) unless `--commit`. Only APPLY rows run; one tx per batch, savepoint per row.
  Preflight aborts unless F_YARN_CONV_FACTOR / F_YARN_RP_DOZING / F_YARN_MB_COST have `updated_by='migration_000616'`.
  A row is skipped (FAILED) if the product got locked/inactive or a value changed since the plan.
- `revert` restores before-images only where the value still equals the after-image; CPP rows that did not exist are deleted; only CAPP rows this run inserted are deleted.
- Marker: `cpp_filled_by` / `cpp_created_by` = actor (`mbcmp-<env>`).
- Values are used as-is (no calculation). Not touched: entry_status, spin, recipe, run_ldr, relink.

CSV mapping (0 = Excel row): 3 item, 5 shade, 7 MB_SP_CODE (empty -> orion code via SYSID col 16, else unset), 8 MB_SP_DYE, 9 MB_SP_DENIER,
10 MB_SP_FILAMENT, 11 MB_SP_CC, 12 MB_SP_DOZING, 13 CONV_FACTOR_ACTUAL, 15 RP_DOZING_ACTUAL, 16 SYSID.

## spin-ldr mode (mb-comparison p2 item 2: spin LDR = Excel col 68)

Sets `mst_mb_spin` LDR to `round(sp68_min, 4)` (never `calc`), locked ACTUAL, adjustment cleared, through the domain
`SetActualLDR` (needs the build that clears the adjustment: branch `feat/mb-sp-dozing-effective-ldr` or later). Input = the
sql/06 query result `06-q1.csv` (prod and staging have different ids: export a CSV per env).
Spec: `docs/superpowers/specs/2026-11-26-mb-comparison-p2-item2-data.md`.

```
mb-comparison-fill spin-ldr plan   --file 06-q1.csv --out-dir out/
mb-comparison-fill spin-ldr apply  --plan-file out/mb-comparison-fill-spin-ldr-plan-*.csv --actor mbcmp-<env> [--limit N] [--batch N] [--commit]
mb-comparison-fill spin-ldr revert --results out/mb-comparison-fill-spin-ldr-apply-result-*.csv --actor mbcmp-<env>
```

Decisions: `APPLY_SET` (BEDA_SET_RUN_LDR, BEDA_SPIN_DIPAKAI_PRODUK_LAIN; NULL run_ldr spins become ACTUAL), `APPLY_CLEAR_ADJ`
(SAMA_ADJ_BASI, run_ldr already = target, also via SetActualLDR), `SKIP_SAME`, `SKIP_CONFLICT` (CONFLICT_EXCEL_BEDA / distinct_sp68>1),
`REPORT` (not found, live differs from the CSV snapshot, target <= 0, unknown verdict).

Run steps:
1. `plan` (read-only, re-reads live DB). Review decision counts; REPORT rows are never written. If the CSV is > 1 day old re-run sql/06 first.
2. `apply` without `--commit` = DRY-RUN (rolled back, `*-spin-ldr-apply-dryrun-*.csv`). Check every row is `WOULD_APPLY`.
3. Canary: `apply --commit --limit 1`, then `--limit 5` (re-run `plan` between runs so done rows become SKIP_SAME). Avoid CMB0000302 (19 products) in the canary.
4. Full: `apply --commit` (one tx per `--batch`, `FOR UPDATE` + live-equals-CSV-snapshot guard, savepoint per row; a changed row = FAILED `changed_since_plan`).
5. Lookup Sync PREVIEW (UI or `PreviewLookupSync`, scope master_ref = changed spins): MB_SP_DOZING items must equal sp68; review non-DOZING items before any apply.
6. Re-run sql/06: every verdict should now be SAMA.
7. Revert (if needed): `revert --results <apply-result csv(s), oldest first>`; restores the LDR group (run_ldr, type, actual, fixed, calc, adj) only where the spin still equals the after-image.
Re-running `apply --commit` after a commit is idempotent: rows already at target report `ALREADY_APPLIED` (exit 0). FAILED reasons also include `plan_tampered` (plan CSV decision/target inconsistent with sp68), `spin_superseded`, `head_deleted`.
Children of APPLY_SET parents are NOT re-cascaded by this tool (only the listed spins are written).
Result CSVs carry `before_json` / `after_json` images. Marker: `updated_by` = actor (`mbcmp-<env>`).

## relink mode (mb-comparison p2 p4: MB_SP_CODE pointer = the spin the Excel means)

Sets only `cost_product_parameter.cpp_value_mb_spin_id` of `MB_SP_CODE` (the text stays). No other param is written, no lookup
sync, no recalc. Input = the `sql/10-parity-plan` per-env exports: `plan-E-relink.csv` (action `RELINK`), optional
`plan-D-names.csv` (`RELINK_TO_NAMED_HEAD_SPIN`, uses `goapps_value` as the current pointer), optional `plan-CONFLICT-child.csv`
(`MOVE_TO_CHILD`: `parent_spin_id`, `target_den_fil_ldr` = den/fil/ldr, `child_key`). Every other action is ignored. All rows must
carry the same `env` (mixed env = error). Spec: `docs/mb-comparison-p2/PARITY-PLAN.md`.

```
mb-comparison-fill relink plan   --e plan-E-relink.csv [--d plan-D-names.csv] [--conflict plan-CONFLICT-child.csv] --out-dir out/
mb-comparison-fill relink apply  --plan-file out/mb-comparison-fill-relink-plan-*.csv --actor mbcmp-<env> [--limit N] [--batch N] [--commit]
mb-comparison-fill relink revert --results out/mb-comparison-fill-relink-apply-result-*.csv --actor mbcmp-<env>
```

Decisions: `POINTER` (set the pointer; inactive / TIE_EQUIVALENT targets are allowed, soft-deleted ones are not), `CHILD` (reuse or create
the child spin of the parent, then point), `SKIP_SAME` (already on the target / on a matching child), `REPORT` (product not found /
inactive / locked, no MB_SP_CODE row, target or parent spin missing or deleted, pointer differs from the export, product in several
input rows). The plan snapshots the live text + pointer (`snap_text`, `snap_ptr`).

Run steps (same shape as spin-ldr):
1. `plan` (read-only, re-reads the live DB). Review decision counts; REPORT rows are never written. Run pointers (E + D) first, re-export sql/10, then the CONFLICT children.
2. `apply` without `--commit` = DRY-RUN (rolled back, `*-relink-apply-dryrun-*.csv`): all rows `WOULD_APPLY`. Dry-run never creates children.
3. Canary `apply --commit --limit 1`, then `--limit 5`, then the full run (re-run `plan` between runs so done rows become `SKIP_SAME`).
4. Apply guard (one tx per `--batch`, savepoint per row): product `FOR UPDATE` + CPP row `FOR UPDATE`, product active and unlocked, live pointer AND text equal the plan snapshot (else `FAILED changed_since_plan`), target spin not soft-deleted.
5. Children: rows are grouped by (parent, den/fil/LDR 4dp). An existing live ACTUAL child of that parent with the same den/fil/LDR is reused. Otherwise the existing `DuplicateSpin` creates it in its OWN tx, BEFORE the pointer batch (it cannot join the batch tx): den/fil/LDR from the target, no FG, `RequireLiveHead=false`, LDR locked ACTUAL. A crash between the two leaves an unreferenced child; the re-run reuses it. Result CSV: `child_spin_id`, `child_created` (true only on the first row of a group that really created it).
6. Revert: `revert --results <apply-result csv(s), oldest first>` restores the before-pointer only where the live pointer still equals the after-pointer (else `CHANGED_SINCE_APPLY`), then soft-deletes the children this run created only when no `cost_product_parameter` row points at them, they have no child spins and `mbs_duplicated_by` = actor; otherwise `KEPT_REFERENCED`.
Re-running `apply --commit` is idempotent (`ALREADY_APPLIED`). FAILED reasons also include `plan_tampered`, `target_spin_missing_or_deleted`, `child: duplicate_spin: ...`.
Apply and revert require `--actor mbcmp-<env>` where `<env>` is the plan's `env` column (rows are applied in product_sys_id order). `--batch` defaults to 100 (same as every mode); keep it. A mid-batch DB error aborts the batch and reports none of its rows as APPLIED.
Children: created with the PARENT's name (no "(copy)"). Revert considers every child id any result row recorded (also FAILED rows, since the child commits before the pointer batch) and deletes it only if unreferenced, childless and created by the actor.
Result CSV columns: `env, row, product_sys_id, decision, status, reason, before_json, after_json, child_spin_id, child_created`. Marker: `cpp_updated_by` = actor.
Caveat: a UI save of the MB_SP_CODE parameter re-resolves the pointer from the text and would undo a relink.
