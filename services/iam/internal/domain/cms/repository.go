// Package cms provides domain logic for CMS content management.
package cms

import (
	"context"

	"github.com/google/uuid"
)

// PageRepository defines the interface for CMS page persistence operations.
type PageRepository interface {
	Create(ctx context.Context, page *Page) error
	GetByID(ctx context.Context, id uuid.UUID) (*Page, error)
	GetBySlug(ctx context.Context, slug string) (*Page, error)
	Update(ctx context.Context, page *Page) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params PageListParams) ([]*Page, int64, error)
	ExistsBySlug(ctx context.Context, slug string) (bool, error)
}

// PageListParams contains parameters for listing CMS pages.
type PageListParams struct {
	Page        int
	PageSize    int
	Search      string
	IsPublished *bool
	SortBy      string
	SortOrder   string
}

// SectionRepository defines the interface for CMS section persistence operations.
type SectionRepository interface {
	Create(ctx context.Context, section *Section) error
	GetByID(ctx context.Context, id uuid.UUID) (*Section, error)
	Update(ctx context.Context, section *Section) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params SectionListParams) ([]*Section, int64, error)
	ExistsByKey(ctx context.Context, key string) (bool, error)
	ListPublished(ctx context.Context) ([]*Section, error)
}

// SectionListParams contains parameters for listing CMS sections.
type SectionListParams struct {
	Page        int
	PageSize    int
	Search      string
	SectionType *SectionType
	IsPublished *bool
	SortBy      string
	SortOrder   string
}

// SettingRepository defines the interface for CMS setting persistence operations.
type SettingRepository interface {
	GetByKey(ctx context.Context, key string) (*Setting, error)
	Update(ctx context.Context, setting *Setting) error
	List(ctx context.Context, group string) ([]*Setting, error)
	ListAll(ctx context.Context) ([]*Setting, error)
}
