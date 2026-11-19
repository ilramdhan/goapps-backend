package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

const lfShadeLookup = `SELECT ces_shade_code, COALESCE(ces_shade_name, '') FROM cost_erp_shade
	WHERE UPPER(REPLACE(REPLACE(REPLACE(ces_shade_code, '-', ''), ' ', ''), E'\t', '')) = $1 ORDER BY ces_shade_id LIMIT 1`

// lfShadeMaster finds the master row by nk; ok=false when missing.
func lfShadeMaster(ctx context.Context, tx *sql.Tx, code string) (mcode, mname string, ok bool, err error) {
	err = tx.QueryRowContext(ctx, lfShadeLookup, mblegacyfill.ShadeNK(code)).Scan(&mcode, &mname)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("lookup shade master: %w", err)
	}
	return mcode, mname, true, nil
}

// lfGroupShadeRow finds the other shade row (field) of the same entity+id in the group.
func lfGroupShadeRow(g *lfGroup, r mblegacyfill.PlanRow, field string) (mblegacyfill.PlanRow, bool) {
	for _, o := range g.rows {
		if o.Row.Entity == r.Row.Entity && strings.EqualFold(o.Row.ID, r.Row.ID) && o.Row.Field == field {
			return o, true
		}
	}
	return mblegacyfill.PlanRow{}, false
}

func lfShadeCol(e mblegacyfill.Entity, field string) (mblegacyfill.FieldSpec, bool) {
	return mblegacyfill.Lookup(e, field)
}

// lfHandleS writes class-S rows against the shade master (design section 6).
func lfHandleS(ctx context.Context, tx *sql.Tx, g *lfGroup, r mblegacyfill.PlanRow, cur string) (lfOutcome, error) {
	if r.Row.Field == "shade_name" {
		return lfShadeName(ctx, tx, g, r, cur)
	}
	return lfShadeCode(ctx, tx, g, r, cur)
}

func lfShadeCode(ctx context.Context, tx *sql.Tx, g *lfGroup, r mblegacyfill.PlanRow, cur string) (lfOutcome, error) {
	legacy := mblegacyfill.Normalize(r.Row.LegacyValue)
	nameRow, hasName := lfGroupShadeRow(g, r, "shade_name")
	mcode, mname, ok, err := lfShadeMaster(ctx, tx, legacy)
	if err != nil {
		return lfOutcome{}, err
	}
	out := lfOutcome{before: map[string]string{}, after: map[string]string{}}
	if !ok {
		newName := ""
		if hasName {
			newName = mblegacyfill.Normalize(nameRow.Row.LegacyValue)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO cost_erp_shade (ces_shade_code, ces_shade_name, ces_is_active, ces_synced_at, ces_shade_source)
			VALUES ($1, NULLIF($2, ''), TRUE, NOW(), 'MANUAL')`, legacy, newName); err != nil {
			return lfOutcome{}, fmt.Errorf("insert shade master: %w", err)
		}
		mcode, mname = legacy, newName
		out.after["shade_master_created"] = legacy
	}
	spec := r.Row.Spec
	if err := lfSetShade(ctx, tx, g, r, spec, mcode, cur, &out); err != nil {
		return lfOutcome{}, err
	}
	if hasName || mname == "" {
		return out, nil // the shade_name row (if any) writes the name itself
	}
	nspec, okSpec := lfShadeCol(r.Row.Entity, "shade_name")
	if !okSpec {
		return out, nil
	}
	ncur, _, err := lfReadColumn(ctx, tx, r.Row.Entity, r.Row.ID, nspec.Column, "")
	if err != nil {
		return lfOutcome{}, err
	}
	return out, lfSetShade(ctx, tx, g, r, nspec, mname, ncur, &out)
}

func lfShadeName(ctx context.Context, tx *sql.Tx, g *lfGroup, r mblegacyfill.PlanRow, cur string) (lfOutcome, error) {
	name := mblegacyfill.Normalize(r.Row.LegacyValue)
	code := ""
	if cr, ok := lfGroupShadeRow(g, r, "shade_code"); ok {
		code = cr.Row.LegacyValue
	} else if cspec, ok := lfShadeCol(r.Row.Entity, "shade_code"); ok {
		c, _, err := lfReadColumn(ctx, tx, r.Row.Entity, r.Row.ID, cspec.Column, "")
		if err != nil {
			return lfOutcome{}, err
		}
		code = c
	}
	if mblegacyfill.Normalize(code) != "" {
		_, mname, ok, err := lfShadeMaster(ctx, tx, mblegacyfill.Normalize(code))
		if err != nil {
			return lfOutcome{}, err
		}
		if ok && mname != "" {
			name = mname
		}
	}
	out := lfOutcome{before: map[string]string{}, after: map[string]string{}}
	err := lfSetShade(ctx, tx, g, r, r.Row.Spec, name, cur, &out)
	return out, err
}

// lfSetShade writes one shade column and records before/after in out.
func lfSetShade(ctx context.Context, tx *sql.Tx, g *lfGroup, r mblegacyfill.PlanRow, spec mblegacyfill.FieldSpec, value, cur string, out *lfOutcome) error {
	if err := lfWriteColumn(ctx, tx, g, r.Row.Entity, r.Row.ID, spec, spec.Column, value); err != nil {
		return err
	}
	after, _, err := lfReadColumn(ctx, tx, r.Row.Entity, r.Row.ID, spec.Column, "")
	if err != nil {
		return err
	}
	out.before[spec.Column], out.after[spec.Column] = mblegacyfill.Normalize(cur), after
	return nil
}
