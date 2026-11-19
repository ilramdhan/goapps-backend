package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

// MBLegacyFillReader loads the current column values for a proposal in ONE
// READ ONLY REPEATABLE READ transaction (implements mblegacyfill.Reader).
type MBLegacyFillReader struct {
	db *DB
}

// NewMBLegacyFillReader creates the reader.
func NewMBLegacyFillReader(db *DB) *MBLegacyFillReader { return &MBLegacyFillReader{db: db} }

type legacyFillSnapshot struct {
	values  map[string]string // entity|id|column -> value ("" for NULL)
	live    map[string]bool   // entity|id -> live row loaded
	holders map[string][]string
}

func lfKey(e mblegacyfill.Entity, id string) string { return string(e) + "|" + strings.ToLower(id) }

func (s *legacyFillSnapshot) Current(e mblegacyfill.Entity, id, column string) (string, bool) {
	if !s.live[lfKey(e, id)] {
		return "", false
	}
	return s.values[lfKey(e, id)+"|"+column], true
}

func (s *legacyFillSnapshot) Holders(column, value string) []string {
	return s.holders[column+"\x00"+strings.TrimSpace(value)]
}

type lfTable struct{ table, idCol string }

var lfTables = map[mblegacyfill.Entity]lfTable{
	mblegacyfill.EntityHead: {"mst_mb_head", "mbh_id"},
	mblegacyfill.EntitySpin: {"mst_mb_spin", "mbs_id"},
}

// Load reads all needed values for rows. Rows with non-UUID ids are treated as missing.
func (r *MBLegacyFillReader) Load(ctx context.Context, rows []mblegacyfill.Row) (snap mblegacyfill.Snapshot, err error) {
	tx, err := r.db.BeginTx(ctx, snapshotTxOptions())
	if err != nil {
		return nil, fmt.Errorf("begin legacy-fill read tx: %w", err)
	}
	defer rollbackSnapshotTx(tx)
	s := &legacyFillSnapshot{values: map[string]string{}, live: map[string]bool{}, holders: map[string][]string{}}
	for _, e := range []mblegacyfill.Entity{mblegacyfill.EntityHead, mblegacyfill.EntitySpin} {
		if err := loadLFEntity(ctx, tx, s, e, rows); err != nil {
			return nil, err
		}
	}
	if err := loadLFHolders(ctx, tx, s, rows); err != nil {
		return nil, err
	}
	return s, nil
}

func lfIDsAndColumns(e mblegacyfill.Entity, rows []mblegacyfill.Row) (ids, cols []string) {
	seenID, seenCol := map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		if row.Entity != e {
			continue
		}
		if _, err := uuid.Parse(row.ID); err == nil && !seenID[strings.ToLower(row.ID)] {
			seenID[strings.ToLower(row.ID)] = true
			ids = append(ids, row.ID)
		}
		if !seenCol[row.Spec.Column] {
			seenCol[row.Spec.Column] = true
			cols = append(cols, row.Spec.Column)
		}
	}
	return ids, cols
}

func loadLFEntity(ctx context.Context, tx *sql.Tx, s *legacyFillSnapshot, e mblegacyfill.Entity, rows []mblegacyfill.Row) (err error) {
	ids, cols := lfIDsAndColumns(e, rows)
	if len(ids) == 0 {
		return nil
	}
	t := lfTables[e]
	sel := make([]string, 0, len(cols))
	for _, c := range cols { // column names come from the static registry
		sel = append(sel, "COALESCE(CAST("+c+" AS text), '')")
	}
	//nolint:gosec // identifiers come from the static lfTables/field registry, values are bound
	q := "SELECT CAST(" + t.idCol + " AS text), " + strings.Join(sel, ", ") +
		" FROM " + t.table + " WHERE " + t.idCol + " = ANY(CAST($1 AS uuid[])) AND deleted_at IS NULL"
	rs, err := tx.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("load %s values: %w", e, err)
	}
	defer closeLFRows(rs, &err)
	for rs.Next() {
		var id string
		vals := make([]string, len(cols))
		dest := make([]any, 0, len(cols)+1)
		dest = append(dest, &id)
		for i := range vals {
			dest = append(dest, &vals[i])
		}
		if scanErr := rs.Scan(dest...); scanErr != nil {
			return fmt.Errorf("scan %s values: %w", e, scanErr)
		}
		s.live[lfKey(e, id)] = true
		for i, c := range cols {
			s.values[lfKey(e, id)+"|"+c] = vals[i]
		}
	}
	return rs.Err()
}

// loadLFHolders finds live heads already holding the target value of every
// unique-checked head row (batched per column).
func loadLFHolders(ctx context.Context, tx *sql.Tx, s *legacyFillSnapshot, rows []mblegacyfill.Row) error {
	byCol := map[string][]string{}
	for _, row := range rows {
		if !isLFUniqueRow(row) {
			continue
		}
		if v := strings.TrimSpace(row.LegacyValue); v != "" {
			byCol[row.Spec.Column] = append(byCol[row.Spec.Column], v)
		}
	}
	for col, vals := range byCol {
		if err := loadLFHolderCol(ctx, tx, s, col, vals); err != nil {
			return err
		}
	}
	return nil
}

func loadLFHolderCol(ctx context.Context, tx *sql.Tx, s *legacyFillSnapshot, col string, vals []string) (err error) {
	table, idCol := "mst_mb_head", "mbh_id"
	if strings.HasPrefix(col, "mbs_") {
		table, idCol = "mst_mb_spin", "mbs_id"
	}
	//nolint:gosec // col comes from the static registry, values are bound
	q := "SELECT CAST(" + idCol + " AS text), TRIM(CAST(" + col + " AS text)) FROM " + table +
		" WHERE deleted_at IS NULL AND TRIM(CAST(" + col + " AS text)) = ANY($1)"
	rs, err := tx.QueryContext(ctx, q, pq.Array(vals))
	if err != nil {
		return fmt.Errorf("load holders %s: %w", col, err)
	}
	defer closeLFRows(rs, &err)
	for rs.Next() {
		var id, v string
		if scanErr := rs.Scan(&id, &v); scanErr != nil {
			return fmt.Errorf("scan holders %s: %w", col, scanErr)
		}
		k := col + "\x00" + v
		s.holders[k] = append(s.holders[k], id)
	}
	if err = rs.Err(); err != nil {
		return fmt.Errorf("iterate holders %s: %w", col, err)
	}
	return nil
}

// closeLFRows closes rs, reporting the close error when nothing else failed.
func closeLFRows(rs *sql.Rows, errp *error) {
	if cerr := rs.Close(); cerr != nil && *errp == nil {
		*errp = cerr
	}
}

// isLFUniqueRow: head unique fields plus spin orion.
func isLFUniqueRow(row mblegacyfill.Row) bool {
	if row.Entity == mblegacyfill.EntitySpin {
		return row.Field == "orion"
	}
	return row.Entity == mblegacyfill.EntityHead && isLFUnique(row.Field)
}

func isLFUnique(field string) bool {
	switch field {
	case "mb_costing", "orion", "vs", "dev":
		return true
	}
	return false
}
