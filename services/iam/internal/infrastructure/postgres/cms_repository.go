// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/cms"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// =============================================================================
// CMS PAGE REPOSITORY
// =============================================================================

// CMSPageRepository implements cms.PageRepository interface.
type CMSPageRepository struct {
	db *DB
}

// NewCMSPageRepository creates a new CMSPageRepository.
func NewCMSPageRepository(db *DB) *CMSPageRepository {
	return &CMSPageRepository{db: db}
}

// Create inserts a new CMS page into the database.
func (r *CMSPageRepository) Create(ctx context.Context, page *cms.Page) error {
	query := `
		INSERT INTO mst_cms_page (page_id, page_slug, page_title, page_content, meta_description, is_published, published_at, sort_order, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := r.db.ExecContext(ctx, query,
		page.ID(), page.Slug(), page.Title(), page.Content(), page.MetaDescription(),
		page.IsPublished(), page.PublishedAt(), page.SortOrder(),
		page.Audit().CreatedAt, page.Audit().CreatedBy)
	return err
}

// GetByID retrieves a CMS page by its unique identifier.
func (r *CMSPageRepository) GetByID(ctx context.Context, id uuid.UUID) (*cms.Page, error) {
	query := `
		SELECT page_id, page_slug, page_title, page_content, meta_description, is_published, published_at, sort_order,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_cms_page WHERE page_id = $1 AND deleted_at IS NULL`
	return r.scanPage(r.db.QueryRowContext(ctx, query, id))
}

// GetBySlug retrieves a CMS page by its URL slug.
func (r *CMSPageRepository) GetBySlug(ctx context.Context, slug string) (*cms.Page, error) {
	query := `
		SELECT page_id, page_slug, page_title, page_content, meta_description, is_published, published_at, sort_order,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_cms_page WHERE page_slug = $1 AND deleted_at IS NULL`
	return r.scanPage(r.db.QueryRowContext(ctx, query, slug))
}

// Update persists changes to an existing CMS page.
func (r *CMSPageRepository) Update(ctx context.Context, page *cms.Page) error {
	query := `
		UPDATE mst_cms_page SET page_title = $2, page_content = $3, meta_description = $4,
			is_published = $5, published_at = $6, sort_order = $7, updated_at = $8, updated_by = $9
		WHERE page_id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, page.ID(), page.Title(), page.Content(), page.MetaDescription(),
		page.IsPublished(), page.PublishedAt(), page.SortOrder(),
		page.Audit().UpdatedAt, page.Audit().UpdatedBy)
	return err
}

// Delete soft-deletes a CMS page.
func (r *CMSPageRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `UPDATE mst_cms_page SET deleted_at = $2, deleted_by = $3 WHERE page_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return cms.ErrPageNotFound
	}
	return nil
}

// List retrieves a paginated list of CMS pages.
func (r *CMSPageRepository) List(ctx context.Context, params cms.PageListParams) ([]*cms.Page, int64, error) {
	var whereClauses []string
	var args []interface{}
	argIdx := 1

	whereClauses = append(whereClauses, "deleted_at IS NULL")
	if params.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(LOWER(page_slug) LIKE $%d OR LOWER(page_title) LIKE $%d OR LOWER(page_content) LIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+strings.ToLower(params.Search)+"%")
		argIdx++
	}
	if params.IsPublished != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("is_published = $%d", argIdx))
		args = append(args, *params.IsPublished)
		argIdx++
	}

	whereClause := strings.Join(whereClauses, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM mst_cms_page WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "sort_order ASC"
	if params.SortBy != "" {
		order := sortASC
		if strings.ToUpper(params.SortOrder) == sortDESC {
			order = sortDESC
		}
		orderBy = fmt.Sprintf("%s %s", sanitizeCMSColumn(params.SortBy), order)
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT page_id, page_slug, page_title, page_content, meta_description, is_published, published_at, sort_order,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_cms_page WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`, whereClause, orderBy, argIdx, argIdx+1)
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in cms page list")
		}
	}()

	var pages []*cms.Page
	for rows.Next() {
		page, err := r.scanPageRows(rows)
		if err != nil {
			return nil, 0, err
		}
		pages = append(pages, page)
	}
	return pages, total, nil
}

// ExistsBySlug checks whether a page with the given slug already exists.
func (r *CMSPageRepository) ExistsBySlug(ctx context.Context, slug string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_cms_page WHERE page_slug = $1 AND deleted_at IS NULL)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, slug).Scan(&exists)
	return exists, err
}

func (r *CMSPageRepository) scanPage(row *sql.Row) (*cms.Page, error) {
	var id uuid.UUID
	var slug, title, createdBy string
	var content, metaDescription sql.NullString
	var isPublished bool
	var publishedAt sql.NullTime
	var sortOrder int
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := row.Scan(&id, &slug, &title, &content, &metaDescription, &isPublished, &publishedAt, &sortOrder,
		&createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		if err == sql.ErrNoRows {
			return nil, cms.ErrPageNotFound
		}
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}
	return cms.ReconstructPage(id, slug, title, nullStringValue(content), nullStringValue(metaDescription),
		isPublished, nullTimeToPtr(publishedAt), sortOrder, audit), nil
}

func (r *CMSPageRepository) scanPageRows(rows *sql.Rows) (*cms.Page, error) {
	var id uuid.UUID
	var slug, title, createdBy string
	var content, metaDescription sql.NullString
	var isPublished bool
	var publishedAt sql.NullTime
	var sortOrder int
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := rows.Scan(&id, &slug, &title, &content, &metaDescription, &isPublished, &publishedAt, &sortOrder,
		&createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}
	return cms.ReconstructPage(id, slug, title, nullStringValue(content), nullStringValue(metaDescription),
		isPublished, nullTimeToPtr(publishedAt), sortOrder, audit), nil
}

// =============================================================================
// CMS SECTION REPOSITORY
// =============================================================================

// CMSSectionRepository implements cms.SectionRepository interface.
type CMSSectionRepository struct {
	db *DB
}

// NewCMSSectionRepository creates a new CMSSectionRepository.
func NewCMSSectionRepository(db *DB) *CMSSectionRepository {
	return &CMSSectionRepository{db: db}
}

// Create inserts a new CMS section into the database.
func (r *CMSSectionRepository) Create(ctx context.Context, section *cms.Section) error {
	query := `
		INSERT INTO mst_cms_section (section_id, section_type, section_key, title, subtitle, content,
			icon_name, image_url, button_text, button_url, sort_order, is_published, metadata, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`
	_, err := r.db.ExecContext(ctx, query,
		section.ID(), string(section.Type()), section.Key(), section.Title(), section.Subtitle(), section.Content(),
		section.IconName(), section.ImageURL(), section.ButtonText(), section.ButtonURL(),
		section.SortOrder(), section.IsPublished(), section.Metadata(),
		section.Audit().CreatedAt, section.Audit().CreatedBy)
	return err
}

// GetByID retrieves a CMS section by its unique identifier.
func (r *CMSSectionRepository) GetByID(ctx context.Context, id uuid.UUID) (*cms.Section, error) {
	query := `
		SELECT section_id, section_type, section_key, title, subtitle, content,
			icon_name, image_url, button_text, button_url, sort_order, is_published, metadata,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_cms_section WHERE section_id = $1 AND deleted_at IS NULL`
	return r.scanSection(r.db.QueryRowContext(ctx, query, id))
}

// Update persists changes to an existing CMS section.
func (r *CMSSectionRepository) Update(ctx context.Context, section *cms.Section) error {
	query := `
		UPDATE mst_cms_section SET section_type = $2, title = $3, subtitle = $4, content = $5,
			icon_name = $6, image_url = $7, button_text = $8, button_url = $9,
			sort_order = $10, is_published = $11, metadata = $12, updated_at = $13, updated_by = $14
		WHERE section_id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, section.ID(), string(section.Type()), section.Title(), section.Subtitle(), section.Content(),
		section.IconName(), section.ImageURL(), section.ButtonText(), section.ButtonURL(),
		section.SortOrder(), section.IsPublished(), section.Metadata(),
		section.Audit().UpdatedAt, section.Audit().UpdatedBy)
	return err
}

// Delete soft-deletes a CMS section.
func (r *CMSSectionRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `UPDATE mst_cms_section SET deleted_at = $2, deleted_by = $3 WHERE section_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return cms.ErrSectionNotFound
	}
	return nil
}

// List retrieves a paginated list of CMS sections.
func (r *CMSSectionRepository) List(ctx context.Context, params cms.SectionListParams) ([]*cms.Section, int64, error) {
	var whereClauses []string
	var args []interface{}
	argIdx := 1

	whereClauses = append(whereClauses, "deleted_at IS NULL")
	if params.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(LOWER(section_key) LIKE $%d OR LOWER(title) LIKE $%d OR LOWER(subtitle) LIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+strings.ToLower(params.Search)+"%")
		argIdx++
	}
	if params.SectionType != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("section_type = $%d", argIdx))
		args = append(args, string(*params.SectionType))
		argIdx++
	}
	if params.IsPublished != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("is_published = $%d", argIdx))
		args = append(args, *params.IsPublished)
		argIdx++
	}

	whereClause := strings.Join(whereClauses, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM mst_cms_section WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "sort_order ASC"
	if params.SortBy != "" {
		order := sortASC
		if strings.ToUpper(params.SortOrder) == sortDESC {
			order = sortDESC
		}
		orderBy = fmt.Sprintf("%s %s", sanitizeCMSColumn(params.SortBy), order)
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT section_id, section_type, section_key, title, subtitle, content,
			icon_name, image_url, button_text, button_url, sort_order, is_published, metadata,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_cms_section WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`, whereClause, orderBy, argIdx, argIdx+1)
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in cms section list")
		}
	}()

	var sections []*cms.Section
	for rows.Next() {
		section, err := r.scanSectionRows(rows)
		if err != nil {
			return nil, 0, err
		}
		sections = append(sections, section)
	}
	return sections, total, nil
}

// ExistsByKey checks whether a section with the given key already exists.
func (r *CMSSectionRepository) ExistsByKey(ctx context.Context, key string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_cms_section WHERE section_key = $1 AND deleted_at IS NULL)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, key).Scan(&exists)
	return exists, err
}

// ListPublished retrieves all published sections ordered by sort_order.
func (r *CMSSectionRepository) ListPublished(ctx context.Context) ([]*cms.Section, error) {
	query := `
		SELECT section_id, section_type, section_key, title, subtitle, content,
			icon_name, image_url, button_text, button_url, sort_order, is_published, metadata,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_cms_section WHERE is_published = true AND deleted_at IS NULL ORDER BY sort_order ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in cms section list published")
		}
	}()

	var sections []*cms.Section
	for rows.Next() {
		section, err := r.scanSectionRows(rows)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}
	return sections, nil
}

func (r *CMSSectionRepository) scanSection(row *sql.Row) (*cms.Section, error) {
	var id uuid.UUID
	var sectionType, sectionKey, title, createdBy string
	var subtitle, content, iconName, imageURL, buttonText, buttonURL sql.NullString
	var sortOrder int
	var isPublished bool
	var metadata sql.NullString
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := row.Scan(&id, &sectionType, &sectionKey, &title, &subtitle, &content,
		&iconName, &imageURL, &buttonText, &buttonURL, &sortOrder, &isPublished, &metadata,
		&createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		if err == sql.ErrNoRows {
			return nil, cms.ErrSectionNotFound
		}
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt, CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt), UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt), DeletedBy: nullStringToPtr(deletedBy),
	}
	return cms.ReconstructSection(id, cms.SectionType(sectionType), sectionKey, title,
		nullStringValue(subtitle), nullStringValue(content),
		nullStringValue(iconName), nullStringValue(imageURL),
		nullStringValue(buttonText), nullStringValue(buttonURL),
		sortOrder, isPublished, nullStringValue(metadata), audit), nil
}

func (r *CMSSectionRepository) scanSectionRows(rows *sql.Rows) (*cms.Section, error) {
	var id uuid.UUID
	var sectionType, sectionKey, title, createdBy string
	var subtitle, content, iconName, imageURL, buttonText, buttonURL sql.NullString
	var sortOrder int
	var isPublished bool
	var metadata sql.NullString
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := rows.Scan(&id, &sectionType, &sectionKey, &title, &subtitle, &content,
		&iconName, &imageURL, &buttonText, &buttonURL, &sortOrder, &isPublished, &metadata,
		&createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt, CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt), UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt), DeletedBy: nullStringToPtr(deletedBy),
	}
	return cms.ReconstructSection(id, cms.SectionType(sectionType), sectionKey, title,
		nullStringValue(subtitle), nullStringValue(content),
		nullStringValue(iconName), nullStringValue(imageURL),
		nullStringValue(buttonText), nullStringValue(buttonURL),
		sortOrder, isPublished, nullStringValue(metadata), audit), nil
}

// =============================================================================
// CMS SETTING REPOSITORY
// =============================================================================

// CMSSettingRepository implements cms.SettingRepository interface.
type CMSSettingRepository struct {
	db *DB
}

// NewCMSSettingRepository creates a new CMSSettingRepository.
func NewCMSSettingRepository(db *DB) *CMSSettingRepository {
	return &CMSSettingRepository{db: db}
}

// GetByKey retrieves a CMS setting by its key.
func (r *CMSSettingRepository) GetByKey(ctx context.Context, key string) (*cms.Setting, error) {
	query := `
		SELECT setting_id, setting_key, setting_value, setting_type, setting_group, description, is_editable,
			created_at, created_by, updated_at, updated_by
		FROM mst_cms_setting WHERE setting_key = $1`
	return r.scanSetting(r.db.QueryRowContext(ctx, query, key))
}

// Update persists changes to an existing CMS setting.
func (r *CMSSettingRepository) Update(ctx context.Context, setting *cms.Setting) error {
	query := `UPDATE mst_cms_setting SET setting_value = $2, updated_at = $3, updated_by = $4 WHERE setting_key = $1`
	result, err := r.db.ExecContext(ctx, query, setting.Key(), setting.Value(),
		setting.Audit().UpdatedAt, setting.Audit().UpdatedBy)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return cms.ErrSettingNotFound
	}
	return nil
}

// List retrieves CMS settings filtered by group.
func (r *CMSSettingRepository) List(ctx context.Context, group string) ([]*cms.Setting, error) {
	query := `
		SELECT setting_id, setting_key, setting_value, setting_type, setting_group, description, is_editable,
			created_at, created_by, updated_at, updated_by
		FROM mst_cms_setting WHERE setting_group = $1 ORDER BY setting_key ASC`

	rows, err := r.db.QueryContext(ctx, query, group)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in cms setting list")
		}
	}()

	var settings []*cms.Setting
	for rows.Next() {
		setting, err := r.scanSettingRows(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	return settings, nil
}

// ListAll retrieves all CMS settings.
func (r *CMSSettingRepository) ListAll(ctx context.Context) ([]*cms.Setting, error) {
	query := `
		SELECT setting_id, setting_key, setting_value, setting_type, setting_group, description, is_editable,
			created_at, created_by, updated_at, updated_by
		FROM mst_cms_setting ORDER BY setting_group ASC, setting_key ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in cms setting list all")
		}
	}()

	var settings []*cms.Setting
	for rows.Next() {
		setting, err := r.scanSettingRows(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	return settings, nil
}

func (r *CMSSettingRepository) scanSetting(row *sql.Row) (*cms.Setting, error) {
	var id uuid.UUID
	var key, value, settingType, settingGroup, createdBy string
	var description sql.NullString
	var isEditable bool
	var createdAt time.Time
	var updatedAt sql.NullTime
	var updatedBy sql.NullString

	if err := row.Scan(&id, &key, &value, &settingType, &settingGroup, &description, &isEditable,
		&createdAt, &createdBy, &updatedAt, &updatedBy); err != nil {
		if err == sql.ErrNoRows {
			return nil, cms.ErrSettingNotFound
		}
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt, CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt), UpdatedBy: nullStringToPtr(updatedBy),
	}
	return cms.ReconstructSetting(id, key, value, cms.SettingType(settingType),
		settingGroup, nullStringValue(description), isEditable, audit), nil
}

func (r *CMSSettingRepository) scanSettingRows(rows *sql.Rows) (*cms.Setting, error) {
	var id uuid.UUID
	var key, value, settingType, settingGroup, createdBy string
	var description sql.NullString
	var isEditable bool
	var createdAt time.Time
	var updatedAt sql.NullTime
	var updatedBy sql.NullString

	if err := rows.Scan(&id, &key, &value, &settingType, &settingGroup, &description, &isEditable,
		&createdAt, &createdBy, &updatedAt, &updatedBy); err != nil {
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt, CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt), UpdatedBy: nullStringToPtr(updatedBy),
	}
	return cms.ReconstructSetting(id, key, value, cms.SettingType(settingType),
		settingGroup, nullStringValue(description), isEditable, audit), nil
}

// =============================================================================
// CMS HELPER FUNCTIONS
// =============================================================================

// sanitizeCMSColumn sanitizes column name to prevent SQL injection for CMS queries.
func sanitizeCMSColumn(column string) string {
	allowed := map[string]string{
		"slug":       "page_slug",
		"title":      "title",
		"key":        "section_key",
		"sort_order": "sort_order",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}
	if col, ok := allowed[column]; ok {
		return col
	}
	return "created_at"
}
