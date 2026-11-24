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
10 MB_SP_FILAMENT, 11 MB_SP_CC, 12 MB_SP_DOZING, 13 CONV_FACTOR_ACTUAL, 14 RP_CC, 15 RP_DOZING_ACTUAL, 16 SYSID.
