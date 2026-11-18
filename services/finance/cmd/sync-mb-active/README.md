# sync-mb-active

One-shot CLI that synchronizes the MB (masterbatch) active state in `finance_db` with the Finance
"mb-active" file. Design: `docs/superpowers/specs/2026-11-17-mb-active-sync-design*.md`.

DB connection comes from the finance service config (`config.Load()`, env `DATABASE_*`, same as the service).
Exactly one mode per run.

## Modes and flags

| Mode | Flags | Writes DB |
|------|-------|-----------|
| `--plan` | `--file F` [`--out-dir D`] | no (read-only tx) |
| `--report` | `--plan-rows R --plan-heads H` [`--out F`] [`--out-dir D`] | no |
| `--apply` | `--plan-rows R --plan-heads H --phase A\|B\|C\|D --actor NAME` [`--batch N`] [`--limit N`] [`--only-sysid S1,S2`] [`--prev-results RES.csv`...] | yes |
| `--revert` | `--input RES.csv`... `--actor NAME` [`--limit N`] [`--only-sysid ...`] | yes |

- `--batch N` is required (N > 0) for phases C and D.
- `--prev-results` (repeatable) resumes duplicates: maps `ln` to `clone_spin_id` from earlier result CSVs.
- `--revert` restores the before-image of APPLIED items whose columns still hold the after-image (reverse order); SUPERSEDE items are not reverted.
- Rows with `decision` other than `APPLY` in the reviewed plan CSVs are not applied (edit `decision` during review).

## Phases

| Phase | Content |
|-------|---------|
| A | KEEP / UPDATE_LDR / UPDATE_SHADE / FG of existing spins |
| B | supersede / deactivate heads |
| C | DUPLICATE_SPIN / GENERATE_SPIN (+ supersede) |
| D | REACTIVATE (+ supersede); lists NEW_RECIPE / REPORT_* in manual-items |

Apply is idempotent: a rerun reports already-applied items as `SKIPPED` (`already_in_target`).
Order is fixed: plan, review, report, A, B, C, D.

## Output files (in `--out-dir`, `<ts>` = yyyymmdd-HHMMSS)

- `sync-mb-active-plan-rows-<ts>.csv`, `sync-mb-active-plan-heads-<ts>.csv`, plan summary (`--plan`)
- `sync-mb-active-report-<ts>.csv` + `-summary.txt` (`--report`)
- `sync-mb-active-result-<phase>-<ts>.csv`, `-errors-<phase>-<ts>.csv`, `-manual-items-<phase>-<ts>.csv` (`--apply`)
- `sync-mb-active-revert-result-<ts>.csv`, `-revert-manual-items-<ts>.csv` (`--revert`)

## CSV schemas

Source of truth: `internal/application/mbactivesync/csv.go` and `report.go`.

- **plan-rows** (`RowsHeader`): `ln, sysid, file_*` (poy, shade, mbcode, dye, doz, den, fil, cs), current DB state
  (`head_*`, `spin_*`, `alloc_*`), `action` (KEEP, UPDATE_LDR, UPDATE_SHADE, REACTIVATE, DUPLICATE_SPIN, GENERATE_SPIN, NEW_RECIPE, REPORT_*),
  `role, reason, sub_actions, phase, source_spin_id, target_*`, `item_key, decision`.
- **plan-heads** (`HeadsHeader`): `item_key, kind` (SUPERSEDE, SUPERSEDE_HEAD, DEACTIVATE_HEAD, REPORT_HEAD), `old_head_id, old_sysid, old_name, old_spin_id, old_spin_live, new_head_id, new_sysid, new_name, new_spin_ref, new_head_live, ref_products_pointer, ref_products_text, phase, reason, decision`.
- **result** (`ResultHeader`): `item_key, ln, sysid, action, phase, result` (APPLIED|PARTIAL|SKIPPED|FAILED)`, reason, spin_id, clone_spin_id, head_id, before_json, after_json, message, actor, applied_at`.
- **errors**: plan-row columns of FAILED items + `error`.
- **manual-items** (`ManualItemsHeader`): `item_key, ln, sysid, action, phase, reason, spin_id, clone_spin_id, head_id, message`.
- **report** (`ReportHeader`): `product_sys_id, product_code, product_name, type, shade, poy_upstream, cur_sp_code, cur_spin_id, sim_sp_code, sim_spin_id, cur_dozing, sim_dozing, dozing_delta, via, locked, note`.

## Examples

```bash
sync-mb-active --plan --file mb-active.csv --out-dir out
sync-mb-active --report --plan-rows out/<rows>.csv --plan-heads out/<heads>.csv --out out/report.csv
sync-mb-active --apply --phase A --limit 20 --actor mbas --plan-rows out/<rows>.csv --plan-heads out/<heads>.csv --out-dir out
sync-mb-active --apply --phase C --batch 50 --actor mbas --prev-results out/<result-C>.csv --plan-rows ... --plan-heads ... --out-dir out
sync-mb-active --revert --input out/<result-A>.csv --actor mbas --out-dir out
```

Never run `--apply`/`--revert` against a real DB before the plan was reviewed and the report approved by Finance.
