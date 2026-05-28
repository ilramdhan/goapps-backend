package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/upload"
)

// BiUploadRepository implements upload.Repository.
type BiUploadRepository struct {
	db *DB
}

// NewBiUploadRepository constructs a BiUploadRepository.
func NewBiUploadRepository(db *DB) *BiUploadRepository {
	return &BiUploadRepository{db: db}
}

var _ upload.Repository = (*BiUploadRepository)(nil)

// CreateSession inserts a new upload session header.
func (r *BiUploadRepository) CreateSession(ctx context.Context, u *upload.Upload) error {
	const q = `
INSERT INTO bi_excel_upload (
    upload_id, source_id, target_type, file_name, file_size, status,
    total_rows, valid_rows, invalid_rows, overwrite_rows, committed_rows,
    uploaded_by, uploaded_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
	if _, err := r.db.ExecContext(ctx, q,
		u.ID(), u.SourceID(), biNullableString(u.TargetType()), u.FileName(), u.FileSize(), u.Status(),
		u.TotalRows(), u.ValidRows(), u.InvalidRows(), u.OverwriteRows(), u.CommittedRows(),
		nullableUUID(u.UploadedBy()), u.UploadedAt(),
	); err != nil {
		return fmt.Errorf("insert upload session: %w", err)
	}
	return nil
}

// InsertStaging bulk-inserts staging rows in chunks of 1000.
func (r *BiUploadRepository) InsertStaging(ctx context.Context, uploadID uuid.UUID, rows []upload.StagingRow) error {
	const chunk = 1000
	for start := 0; start < len(rows); start += chunk {
		end := min(start+chunk, len(rows))
		if err := r.insertStagingChunk(ctx, uploadID, rows[start:end]); err != nil {
			return err
		}
	}
	return nil
}

const stagingColCount = 17

func (r *BiUploadRepository) insertStagingChunk(ctx context.Context, uploadID uuid.UUID, rows []upload.StagingRow) error {
	if len(rows) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(rows))
	args := make([]any, 0, len(rows)*stagingColCount)
	for i, row := range rows {
		base := i * stagingColCount
		ph := make([]string, stagingColCount)
		for j := range ph {
			ph[j] = fmt.Sprintf("$%d", base+j+1)
		}
		placeholders = append(placeholders, "("+strings.Join(ph, ",")+")")
		args = append(args,
			uploadID, row.RowNumber, row.Type,
			biNullableString(row.Group1), biNullableString(row.Group2), biNullableString(row.Group3),
			nullableInt(row.Group1Order), nullableInt(row.Group2Order), nullableInt(row.Group3Order),
			row.PeriodGrain, nullablePeriodDate(row), row.Value, row.DisplayValue,
			biNullableString(row.UOM), row.Scenario, row.ValidationStatus, biNullableString(row.ValidationMsg),
		)
	}
	q := `INSERT INTO bi_excel_staging (
    upload_id, row_number, type, group_1, group_2, group_3,
    group_1_order, group_2_order, group_3_order,
    periode_grain, periode_date, value, display_value,
    uom, scenario, validation_status, validation_msg
) VALUES ` + strings.Join(placeholders, ",")
	if _, err := r.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("insert staging chunk: %w", err)
	}
	return nil
}

// GetSession loads a session header by id.
func (r *BiUploadRepository) GetSession(ctx context.Context, uploadID uuid.UUID) (*upload.Upload, error) {
	const q = `
SELECT upload_id, source_id, COALESCE(target_type,''), file_name, file_size, status,
       COALESCE(total_rows,0), COALESCE(valid_rows,0), COALESCE(invalid_rows,0),
       COALESCE(overwrite_rows,0), COALESCE(committed_rows,0),
       uploaded_by, uploaded_at, committed_at, cancelled_at
FROM bi_excel_upload WHERE upload_id = $1`
	row := r.db.QueryRowContext(ctx, q, uploadID)
	u, err := scanSession(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, upload.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get upload session: %w", err)
	}
	return u, nil
}

// UpdateSession persists status, counts, and timestamps.
func (r *BiUploadRepository) UpdateSession(ctx context.Context, u *upload.Upload) error {
	const q = `
UPDATE bi_excel_upload SET
    status = $2, total_rows = $3, valid_rows = $4, invalid_rows = $5,
    overwrite_rows = $6, committed_rows = $7,
    committed_at = $8, cancelled_at = $9
WHERE upload_id = $1`
	res, err := r.db.ExecContext(ctx, q,
		u.ID(), u.Status(), u.TotalRows(), u.ValidRows(), u.InvalidRows(),
		u.OverwriteRows(), u.CommittedRows(),
		nullableTime(u.CommittedAt()), nullableTime(u.CancelledAt()),
	)
	if err != nil {
		return fmt.Errorf("update upload session: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update upload session rows affected: %w", err)
	}
	if affected == 0 {
		return upload.ErrNotFound
	}
	return nil
}

// ListSessions returns newest-first sessions plus the total count.
func (r *BiUploadRepository) ListSessions(ctx context.Context, page, pageSize int) ([]*upload.Upload, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bi_excel_upload`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count upload sessions: %w", err)
	}
	const q = `
SELECT upload_id, source_id, COALESCE(target_type,''), file_name, file_size, status,
       COALESCE(total_rows,0), COALESCE(valid_rows,0), COALESCE(invalid_rows,0),
       COALESCE(overwrite_rows,0), COALESCE(committed_rows,0),
       uploaded_by, uploaded_at, committed_at, cancelled_at
FROM bi_excel_upload ORDER BY uploaded_at DESC LIMIT $1 OFFSET $2`
	rows, err := r.db.QueryContext(ctx, q, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list upload sessions: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	var out []*upload.Upload
	for rows.Next() {
		u, scanErr := scanSession(rows.Scan)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan upload session: %w", scanErr)
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// MarkOverwrites flips VALID staging rows to WILL_OVERWRITE when the business key
// already exists in bi_fact_metric; returns the number flipped.
func (r *BiUploadRepository) MarkOverwrites(ctx context.Context, uploadID uuid.UUID) (int, error) {
	const q = `
UPDATE bi_excel_staging s SET validation_status = 'WILL_OVERWRITE'
FROM bi_fact_metric f
WHERE s.upload_id = $1
  AND s.validation_status = 'VALID'
  AND f.is_active
  AND f.type = s.type
  AND f.group_1 = s.group_1
  AND f.group_2 IS NOT DISTINCT FROM s.group_2
  AND f.group_3 IS NOT DISTINCT FROM s.group_3
  AND f.periode_grain = s.periode_grain
  AND f.periode_date = s.periode_date
  AND f.scenario = COALESCE(s.scenario, 'ACTUAL')
  AND f.dimension_key = ''`
	res, err := r.db.ExecContext(ctx, q, uploadID)
	if err != nil {
		return 0, fmt.Errorf("mark overwrites: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("mark overwrites rows affected: %w", err)
	}
	return int(affected), nil
}

// CommitToFact upserts VALID + WILL_OVERWRITE staging rows into bi_fact_metric.
func (r *BiUploadRepository) CommitToFact(ctx context.Context, uploadID uuid.UUID) (int, error) {
	const q = `
INSERT INTO bi_fact_metric (
    type, group_1, group_2, group_3,
    group_1_order, group_2_order, group_3_order,
    periode_grain, periode_date, periode_label,
    value, display_value, uom, scenario, source_id, dimension_key, uploaded_by, is_active
)
SELECT s.type, s.group_1, s.group_2, s.group_3,
       s.group_1_order, s.group_2_order, s.group_3_order,
       s.periode_grain, s.periode_date, COALESCE(s.periode_label, ''),
       s.value, COALESCE(s.display_value, s.value), s.uom, COALESCE(s.scenario, 'ACTUAL'),
       (SELECT source_id FROM bi_data_source WHERE source_code = 'EXCEL_UPLOAD'),
       '', u.uploaded_by, TRUE
FROM bi_excel_staging s
JOIN bi_excel_upload u ON u.upload_id = s.upload_id
WHERE s.upload_id = $1
  AND s.validation_status IN ('VALID','WILL_OVERWRITE')
ON CONFLICT (type, group_1, group_2, group_3, periode_grain, periode_date, scenario, dimension_key)
DO UPDATE SET
    value = EXCLUDED.value,
    display_value = EXCLUDED.display_value,
    uom = EXCLUDED.uom,
    source_id = EXCLUDED.source_id,
    uploaded_by = EXCLUDED.uploaded_by,
    loaded_at = NOW(),
    is_active = TRUE`
	res, err := r.db.ExecContext(ctx, q, uploadID)
	if err != nil {
		return 0, fmt.Errorf("commit staging to fact: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("commit rows affected: %w", err)
	}
	return int(affected), nil
}

// RefreshViews refreshes the BI materialized views after a commit.
func (r *BiUploadRepository) RefreshViews(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `REFRESH MATERIALIZED VIEW mv_bi_metric_g1`); err != nil {
		return fmt.Errorf("refresh mv_bi_metric_g1: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `REFRESH MATERIALIZED VIEW mv_bi_metric_g2`); err != nil {
		return fmt.Errorf("refresh mv_bi_metric_g2: %w", err)
	}
	return nil
}

// nullablePeriodDate returns nil for a zero period date so the DATE column stays NULL
// (invalid rows may have an unparsed period).
func nullablePeriodDate(row upload.StagingRow) any {
	if row.PeriodDate.IsZero() {
		return nil
	}
	return row.PeriodDate
}

// nullTimeValue returns the time value or a zero time when the column was NULL.
func nullTimeValue(t sql.NullTime) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

// parseNullableUUID parses a nullable string column into a UUID (Nil when NULL/invalid).
func parseNullableUUID(s sql.NullString) uuid.UUID {
	if !s.Valid || s.String == "" {
		return uuid.Nil
	}
	if id, err := uuid.Parse(s.String); err == nil {
		return id
	}
	return uuid.Nil
}

// scanSession scans a session row from any Scan-compatible function.
func scanSession(scan func(dest ...any) error) (*upload.Upload, error) {
	var (
		uploadID, sourceID                                         uuid.UUID
		targetType, fileName, status                               string
		fileSize, totalRows, validRows, invalidRows, overwriteRows int
		committedRows                                              int
		uploadedBy                                                 sql.NullString
		uploadedAt                                                 sql.NullTime
		committedAt, cancelledAt                                   sql.NullTime
	)
	if err := scan(
		&uploadID, &sourceID, &targetType, &fileName, &fileSize, &status,
		&totalRows, &validRows, &invalidRows, &overwriteRows, &committedRows,
		&uploadedBy, &uploadedAt, &committedAt, &cancelledAt,
	); err != nil {
		return nil, err
	}
	return upload.Hydrate(
		uploadID, sourceID, targetType, fileName, fileSize, status,
		totalRows, validRows, invalidRows, overwriteRows, committedRows,
		parseNullableUUID(uploadedBy),
		nullTimeValue(uploadedAt), nullTimeValue(committedAt), nullTimeValue(cancelledAt),
	), nil
}
