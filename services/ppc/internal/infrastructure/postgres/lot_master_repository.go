// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/lot"
)

// lotMasterColumns is the full projection shared by every read path, so a new
// spec column is added in exactly one place rather than four.
const lotMasterColumns = `lm_lot_no, lm_item_code, lm_shade_code,
	lm_std_weight_full, lm_std_weight_unfull, lm_notes,
	lm_created_at, lm_created_by, lm_updated_at, lm_updated_by,
	lm_source, lm_source_key, lm_synced_at,
	lm_prod_type, lm_yarn_type, lm_denier, lm_filament, lm_cross_section,
	lm_qc_grade, lm_description, lm_shade_color,
	lm_tare_box_wt, lm_tare_bobbin_wt, lm_bobbins_per_box, lm_src_bob_weight,
	lm_orion_item_code, lm_machine_no, lm_efficiency_pct,
	lm_src_status, lm_src_pak_status`

// lotSyncActor is the created_by/updated_by value stamped on sync-sourced lots.
const lotSyncActor = "sync"

// LotRepository implements lot.Repository using PostgreSQL.
type LotRepository struct {
	db *DB
}

// NewLotRepository creates a new LotRepository.
func NewLotRepository(db *DB) *LotRepository {
	return &LotRepository{db: db}
}

var _ lot.Repository = (*LotRepository)(nil)

// Create persists a new lot master.
func (r *LotRepository) Create(ctx context.Context, entity *lot.Master) error {
	query := `
		INSERT INTO lot_master (
			lm_lot_no, lm_item_code, lm_shade_code,
			lm_std_weight_full, lm_std_weight_unfull, lm_notes,
			lm_created_at, lm_created_by, lm_source
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.db.ExecContext(ctx, query,
		entity.LotNo(),
		entity.ItemCode(),
		entity.ShadeCode(),
		entity.StdWeightFull(),
		entity.StdWeightUnfull(),
		nullableNotes(entity.Notes()),
		entity.CreatedAt(),
		entity.CreatedBy(),
		lotSourceOrDefault(entity.Source()),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return lot.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create lot master: %w", err)
	}
	return nil
}

// GetByID retrieves a lot master by its lot number.
func (r *LotRepository) GetByID(ctx context.Context, lotNo string) (*lot.Master, error) {
	query := `SELECT ` + lotMasterColumns + ` FROM lot_master WHERE lm_lot_no = $1`
	return r.scanRow(r.db.QueryRowContext(ctx, query, lotNo))
}

// List retrieves lot masters with filtering and pagination.
func (r *LotRepository) List(ctx context.Context, filter lot.ListFilter) ([]*lot.Master, int64, error) {
	filter.Validate()

	base, args := buildLotListFilter(filter)
	argIdx := len(args) + 1

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count lot masters: %w", err)
	}

	// Frontend sort field -> DB column. A sort_by absent from this map falls
	// back to lm_lot_no rather than emptying the list.
	sortColumnMap := map[string]string{
		"lot_no":     "lm_lot_no",
		"item_code":  "lm_item_code",
		"shade_code": "lm_shade_code",
		"created_at": "lm_created_at",
		"source":     "lm_source",
		"prod_type":  "lm_prod_type",
		"denier":     "lm_denier",
		"synced_at":  "lm_synced_at",
	}
	orderCol := "lm_lot_no"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT ` + lotMasterColumns + ` ` + base +
		fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
			orderCol, sortDirection(filter.SortOrder), argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list lot masters: %w", err)
	}
	defer closeRows(rows)

	var result []*lot.Master
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating lot master rows: %w", err)
	}
	return result, total, nil
}

// buildLotListFilter assembles the WHERE clause and arguments shared by the
// count and page queries.
func buildLotListFilter(filter lot.ListFilter) (string, []interface{}) {
	base := `FROM lot_master WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(
			` AND (lm_lot_no ILIKE $%d OR lm_item_code ILIKE $%d OR lm_shade_code ILIKE $%d`+
				` OR lm_orion_item_code ILIKE $%d OR lm_description ILIKE $%d)`,
			argIdx, argIdx, argIdx, argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.ItemCode != "" {
		base += fmt.Sprintf(` AND lm_item_code = $%d`, argIdx)
		args = append(args, filter.ItemCode)
		argIdx++
	}
	if filter.ShadeCode != "" {
		base += fmt.Sprintf(` AND lm_shade_code = $%d`, argIdx)
		args = append(args, filter.ShadeCode)
		argIdx++
	}
	if filter.Source != "" {
		base += fmt.Sprintf(` AND lm_source = $%d`, argIdx)
		args = append(args, filter.Source)
		argIdx++
	}
	if filter.ProdType != "" {
		base += fmt.Sprintf(` AND lm_prod_type = $%d`, argIdx)
		args = append(args, filter.ProdType)
	}
	return base, args
}

// Update persists changes to an existing lot master, including its spec.
func (r *LotRepository) Update(ctx context.Context, entity *lot.Master) error {
	query := `
		UPDATE lot_master
		SET lm_item_code = $2, lm_shade_code = $3,
			lm_std_weight_full = $4, lm_std_weight_unfull = $5, lm_notes = $6,
			lm_updated_at = $7, lm_updated_by = $8,
			lm_prod_type = $9, lm_yarn_type = $10, lm_denier = $11,
			lm_filament = $12, lm_cross_section = $13, lm_qc_grade = $14,
			lm_description = $15, lm_shade_color = $16,
			lm_tare_box_wt = $17, lm_tare_bobbin_wt = $18,
			lm_bobbins_per_box = $19, lm_src_bob_weight = $20,
			lm_orion_item_code = $21, lm_machine_no = $22, lm_efficiency_pct = $23
		WHERE lm_lot_no = $1
	`
	spec := entity.Spec()
	res, err := r.db.ExecContext(ctx, query,
		entity.LotNo(),
		entity.ItemCode(),
		entity.ShadeCode(),
		entity.StdWeightFull(),
		entity.StdWeightUnfull(),
		nullableNotes(entity.Notes()),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
		nullableText(spec.ProdType),
		nullableText(spec.YarnType),
		nullableText(spec.Denier),
		int32PtrArg(spec.Filament),
		nullableText(spec.CrossSection),
		nullableText(spec.QCGrade),
		nullableText(spec.Description),
		nullableText(spec.ShadeColor),
		floatPtrArg(spec.TareBoxWeight),
		floatPtrArg(spec.TareBobbinWeight),
		int32PtrArg(spec.BobbinsPerBox),
		floatPtrArg(spec.SourceBobWeight),
		nullableText(spec.OrionItemCode),
		nullableText(spec.MachineNo),
		int32PtrArg(spec.EfficiencyPct),
	)
	if err != nil {
		return fmt.Errorf("failed to update lot master: %w", err)
	}
	return checkAffected(res, lot.ErrNotFound)
}

// Delete removes a lot master by its lot number.
func (r *LotRepository) Delete(ctx context.Context, lotNo string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM lot_master WHERE lm_lot_no = $1`, lotNo)
	if err != nil {
		return fmt.Errorf("failed to delete lot master: %w", err)
	}
	return checkAffected(res, lot.ErrNotFound)
}

// lotUpsertColumnCount is the number of bound parameters per lot in the batched
// sourced upsert: 8 identity/provenance values plus the 17 flattened by specArgs.
const lotUpsertColumnCount = 25

// lotUpsertChunkSize caps how many lots ride in one statement. PostgreSQL allows
// at most 65535 bound parameters per statement, so at 25 parameters per lot the
// hard ceiling is 2621; 1000 leaves headroom and keeps each round-trip short.
const lotUpsertChunkSize = 1000

// UpsertSourcedBatch merges MMSMERGE-sourced lots in bulk. Mirrors the machine
// sync's merge semantics: COALESCE on every source-owned column so a source NULL
// never erases a PPC correction, and the standard weight is seeded on insert
// only — once PPC has a weight the source must not move it, because the ETL
// divides by it to derive a bobbin count.
//
// Set-based on purpose. MMSMERGE carries ~66k rows; the previous row-at-a-time
// implementation issued a SELECT plus a write per row and blew the 60s RPC
// deadline after ~15.6k rows, leaving lot_master partially imported.
//
// lm_source is written on insert only (via ON CONFLICT DO UPDATE leaving it
// alone), so a PPC-minted lot that Oracle also knows about is not relabelled as
// imported. The RETURNING clause reports insert vs. update through xmax, which
// is 0 exactly when the row was newly inserted.
func (r *LotRepository) UpsertSourcedBatch(ctx context.Context, src []lot.SourcedLot) (lot.UpsertBatchResult, error) {
	var res lot.UpsertBatchResult

	batch := make([]lot.SourcedLot, 0, len(src))
	for _, s := range src {
		if s.LotNo == "" || s.ItemCode == "" {
			res.Skipped++
			continue
		}
		batch = append(batch, s)
	}

	for start := 0; start < len(batch); start += lotUpsertChunkSize {
		end := min(start+lotUpsertChunkSize, len(batch))
		chunk, err := r.upsertSourcedChunk(ctx, batch[start:end])
		if err != nil {
			return res, err
		}
		res.Add(chunk)
	}
	return res, nil
}

// upsertSourcedChunk writes one bounded slice of lots in a single statement.
func (r *LotRepository) upsertSourcedChunk(ctx context.Context, chunk []lot.SourcedLot) (lot.UpsertBatchResult, error) {
	var res lot.UpsertBatchResult
	if len(chunk) == 0 {
		return res, nil
	}

	var sb strings.Builder
	sb.WriteString(`INSERT INTO lot_master (
			lm_lot_no, lm_item_code, lm_shade_code,
			lm_std_weight_full, lm_std_weight_unfull,
			lm_created_by, lm_source, lm_source_key, lm_synced_at,
			lm_prod_type, lm_yarn_type, lm_denier, lm_filament, lm_cross_section,
			lm_qc_grade, lm_description, lm_shade_color,
			lm_tare_box_wt, lm_tare_bobbin_wt, lm_bobbins_per_box, lm_src_bob_weight,
			lm_orion_item_code, lm_machine_no, lm_efficiency_pct,
			lm_src_status, lm_src_pak_status
		) VALUES `)

	args := make([]interface{}, 0, len(chunk)*lotUpsertColumnCount)
	for i, s := range chunk {
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i * lotUpsertColumnCount
		sb.WriteString(lotUpsertPlaceholders(base))

		args = append(args,
			s.LotNo,
			s.ItemCode,
			nullableText(s.ShadeCode),
			floatPtrArg(s.StdWeightFull),
			lotSyncActor,
			lot.SourceMMSMERGE,
			nullableText(s.SourceKey),
			s.SyncedAt,
		)
		args = append(args, specArgs(s.Spec)...)
	}

	sb.WriteString(`
		ON CONFLICT (lm_lot_no) DO UPDATE SET
			lm_item_code       = COALESCE(EXCLUDED.lm_item_code, lot_master.lm_item_code),
			lm_shade_code      = COALESCE(lot_master.lm_shade_code, EXCLUDED.lm_shade_code),
			lm_source_key      = COALESCE(EXCLUDED.lm_source_key, lot_master.lm_source_key),
			lm_synced_at       = EXCLUDED.lm_synced_at,
			lm_prod_type       = COALESCE(EXCLUDED.lm_prod_type, lot_master.lm_prod_type),
			lm_yarn_type       = COALESCE(EXCLUDED.lm_yarn_type, lot_master.lm_yarn_type),
			lm_denier          = COALESCE(EXCLUDED.lm_denier, lot_master.lm_denier),
			lm_filament        = COALESCE(EXCLUDED.lm_filament, lot_master.lm_filament),
			lm_cross_section   = COALESCE(EXCLUDED.lm_cross_section, lot_master.lm_cross_section),
			lm_qc_grade        = COALESCE(EXCLUDED.lm_qc_grade, lot_master.lm_qc_grade),
			lm_description     = COALESCE(EXCLUDED.lm_description, lot_master.lm_description),
			lm_shade_color     = COALESCE(EXCLUDED.lm_shade_color, lot_master.lm_shade_color),
			lm_tare_box_wt     = COALESCE(EXCLUDED.lm_tare_box_wt, lot_master.lm_tare_box_wt),
			lm_tare_bobbin_wt  = COALESCE(EXCLUDED.lm_tare_bobbin_wt, lot_master.lm_tare_bobbin_wt),
			lm_bobbins_per_box = COALESCE(EXCLUDED.lm_bobbins_per_box, lot_master.lm_bobbins_per_box),
			lm_src_bob_weight  = COALESCE(EXCLUDED.lm_src_bob_weight, lot_master.lm_src_bob_weight),
			lm_orion_item_code = COALESCE(EXCLUDED.lm_orion_item_code, lot_master.lm_orion_item_code),
			lm_machine_no      = COALESCE(EXCLUDED.lm_machine_no, lot_master.lm_machine_no),
			lm_efficiency_pct  = COALESCE(EXCLUDED.lm_efficiency_pct, lot_master.lm_efficiency_pct),
			lm_src_status      = COALESCE(EXCLUDED.lm_src_status, lot_master.lm_src_status),
			lm_src_pak_status  = COALESCE(EXCLUDED.lm_src_pak_status, lot_master.lm_src_pak_status),
			lm_std_weight_full = COALESCE(lot_master.lm_std_weight_full, EXCLUDED.lm_std_weight_full),
			lm_updated_at      = EXCLUDED.lm_synced_at,
			lm_updated_by      = EXCLUDED.lm_created_by
		RETURNING (xmax = 0) AS inserted`)

	rows, err := r.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return res, fmt.Errorf("upsert %d sourced lots: %w", len(chunk), err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("postgres: failed to close sourced-lot upsert rows")
		}
	}()

	for rows.Next() {
		var inserted bool
		if scanErr := rows.Scan(&inserted); scanErr != nil {
			return res, fmt.Errorf("scan sourced-lot upsert outcome: %w", scanErr)
		}
		if inserted {
			res.Inserted++
		} else {
			res.Updated++
		}
	}
	if err := rows.Err(); err != nil {
		return res, fmt.Errorf("iterate sourced-lot upsert outcomes: %w", err)
	}
	return res, nil
}

// lotUpsertPlaceholders renders one VALUES tuple starting at the given
// zero-based parameter offset. lm_std_weight_unfull is a literal NULL: it is
// PPC-local and the source has no equivalent.
func lotUpsertPlaceholders(base int) string {
	var sb strings.Builder
	sb.WriteByte('(')
	for i := range lotUpsertColumnCount {
		if i > 0 {
			sb.WriteString(", ")
		}
		// The 5th column (lm_std_weight_unfull) is not bound.
		if i == 4 {
			sb.WriteString("NULL, ")
		}
		sb.WriteByte('$')
		sb.WriteString(strconv.Itoa(base + i + 1))
	}
	sb.WriteByte(')')
	return sb.String()
}

// specArgs flattens a Spec into driver arguments in the order both sourced
// statements bind them, so the insert and the merge cannot drift apart.
func specArgs(spec lot.Spec) []interface{} {
	return []interface{}{
		nullableText(spec.ProdType),
		nullableText(spec.YarnType),
		nullableText(spec.Denier),
		int32PtrArg(spec.Filament),
		nullableText(spec.CrossSection),
		nullableText(spec.QCGrade),
		nullableText(spec.Description),
		nullableText(spec.ShadeColor),
		floatPtrArg(spec.TareBoxWeight),
		floatPtrArg(spec.TareBobbinWeight),
		int32PtrArg(spec.BobbinsPerBox),
		floatPtrArg(spec.SourceBobWeight),
		nullableText(spec.OrionItemCode),
		nullableText(spec.MachineNo),
		int32PtrArg(spec.EfficiencyPct),
		nullableText(spec.SourceStatus),
		nullableText(spec.SourcePakStatus),
	}
}

// lotSourceOrDefault guards against an empty provenance reaching the
// chk_lot_master_source constraint.
func lotSourceOrDefault(source string) string {
	if source == "" {
		return lot.SourcePPC
	}
	return source
}

func (r *LotRepository) scanRow(row *sql.Row) (*lot.Master, error) {
	var dto lotMasterDTO
	err := dto.scan(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, lot.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan lot master: %w", err)
	}
	return dto.toEntity(), nil
}

func (r *LotRepository) scanRows(rows *sql.Rows) (*lot.Master, error) {
	var dto lotMasterDTO
	if err := dto.scan(rows.Scan); err != nil {
		return nil, fmt.Errorf("failed to scan lot master row: %w", err)
	}
	return dto.toEntity(), nil
}

type lotMasterDTO struct {
	LotNo           string
	ItemCode        string
	ShadeCode       sql.NullString
	StdWeightFull   sql.NullFloat64
	StdWeightUnfull sql.NullFloat64
	Notes           sql.NullString
	CreatedAt       time.Time
	CreatedBy       string
	UpdatedAt       sql.NullTime
	UpdatedBy       sql.NullString

	Source    sql.NullString
	SourceKey sql.NullString
	SyncedAt  sql.NullTime

	ProdType        sql.NullString
	YarnType        sql.NullString
	Denier          sql.NullString
	Filament        sql.NullInt32
	CrossSection    sql.NullString
	QCGrade         sql.NullString
	Description     sql.NullString
	ShadeColor      sql.NullString
	TareBoxWeight   sql.NullFloat64
	TareBobbinWt    sql.NullFloat64
	BobbinsPerBox   sql.NullInt32
	SourceBobWeight sql.NullFloat64
	OrionItemCode   sql.NullString
	MachineNo       sql.NullString
	EfficiencyPct   sql.NullInt32
	SourceStatus    sql.NullString
	SourcePakStatus sql.NullString
}

// scan binds the lotMasterColumns projection through either row.Scan or
// rows.Scan, keeping the single-row and multi-row paths on one column order.
func (d *lotMasterDTO) scan(dest func(...interface{}) error) error {
	return dest(
		&d.LotNo, &d.ItemCode, &d.ShadeCode,
		&d.StdWeightFull, &d.StdWeightUnfull, &d.Notes,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
		&d.Source, &d.SourceKey, &d.SyncedAt,
		&d.ProdType, &d.YarnType, &d.Denier, &d.Filament, &d.CrossSection,
		&d.QCGrade, &d.Description, &d.ShadeColor,
		&d.TareBoxWeight, &d.TareBobbinWt, &d.BobbinsPerBox, &d.SourceBobWeight,
		&d.OrionItemCode, &d.MachineNo, &d.EfficiencyPct,
		&d.SourceStatus, &d.SourcePakStatus,
	)
}

func (d *lotMasterDTO) toEntity() *lot.Master {
	entity := lot.Reconstruct(
		d.LotNo, d.ItemCode, nullString(d.ShadeCode),
		d.StdWeightFull.Float64, d.StdWeightUnfull.Float64, nullString(d.Notes),
		d.CreatedAt, d.CreatedBy,
		nullTimePtr(d.UpdatedAt), nullStringPtr(d.UpdatedBy),
	)
	return entity.WithProvenance(
		nullString(d.Source), nullString(d.SourceKey), nullTimePtr(d.SyncedAt),
		lot.Spec{
			ProdType:         nullString(d.ProdType),
			YarnType:         nullString(d.YarnType),
			Denier:           nullString(d.Denier),
			Filament:         nullInt32Ptr(d.Filament),
			CrossSection:     nullString(d.CrossSection),
			QCGrade:          nullString(d.QCGrade),
			Description:      nullString(d.Description),
			ShadeColor:       nullString(d.ShadeColor),
			TareBoxWeight:    nullFloatPtr(d.TareBoxWeight),
			TareBobbinWeight: nullFloatPtr(d.TareBobbinWt),
			BobbinsPerBox:    nullInt32Ptr(d.BobbinsPerBox),
			SourceBobWeight:  nullFloatPtr(d.SourceBobWeight),
			OrionItemCode:    nullString(d.OrionItemCode),
			MachineNo:        nullString(d.MachineNo),
			EfficiencyPct:    nullInt32Ptr(d.EfficiencyPct),
			SourceStatus:     nullString(d.SourceStatus),
			SourcePakStatus:  nullString(d.SourcePakStatus),
		},
	)
}

func nullableNotes(notes string) sql.NullString {
	if notes == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: notes, Valid: true}
}
