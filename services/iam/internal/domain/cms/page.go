// Package cms provides domain logic for CMS content management.
package cms

import (
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// Validation regex for page slug.
var slugRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Page represents a full-page CMS content entity (Privacy Policy, Terms, etc.).
type Page struct {
	id              uuid.UUID
	slug            string
	title           string
	content         string
	metaDescription string
	isPublished     bool
	publishedAt     *time.Time
	sortOrder       int
	audit           shared.AuditInfo
}

// NewPage creates a new Page entity with validation.
func NewPage(slug, title, content, metaDescription string, isPublished bool, sortOrder int, createdBy string) (*Page, error) {
	if slug == "" {
		return nil, shared.ErrEmptyCode
	}
	if !slugRegex.MatchString(slug) {
		return nil, ErrInvalidSlugFormat
	}
	if title == "" {
		return nil, shared.ErrEmptyName
	}

	p := &Page{
		id:              uuid.New(),
		slug:            slug,
		title:           title,
		content:         content,
		metaDescription: metaDescription,
		isPublished:     isPublished,
		sortOrder:       sortOrder,
		audit:           shared.NewAuditInfo(createdBy),
	}

	if isPublished {
		now := time.Now()
		p.publishedAt = &now
	}

	return p, nil
}

// ReconstructPage reconstructs a Page from persistence.
func ReconstructPage(
	id uuid.UUID,
	slug, title, content, metaDescription string,
	isPublished bool,
	publishedAt *time.Time,
	sortOrder int,
	audit shared.AuditInfo,
) *Page {
	return &Page{
		id:              id,
		slug:            slug,
		title:           title,
		content:         content,
		metaDescription: metaDescription,
		isPublished:     isPublished,
		publishedAt:     publishedAt,
		sortOrder:       sortOrder,
		audit:           audit,
	}
}

// ID returns the page identifier.
func (p *Page) ID() uuid.UUID { return p.id }

// Slug returns the page URL slug.
func (p *Page) Slug() string { return p.slug }

// Title returns the page title.
func (p *Page) Title() string { return p.title }

// Content returns the page content.
func (p *Page) Content() string { return p.content }

// MetaDescription returns the SEO meta description.
func (p *Page) MetaDescription() string { return p.metaDescription }

// IsPublished returns whether the page is publicly visible.
func (p *Page) IsPublished() bool { return p.isPublished }

// PublishedAt returns when the page was published.
func (p *Page) PublishedAt() *time.Time { return p.publishedAt }

// SortOrder returns the display order.
func (p *Page) SortOrder() int { return p.sortOrder }

// Audit returns the audit information.
func (p *Page) Audit() shared.AuditInfo { return p.audit }

// IsDeleted returns whether the page has been soft-deleted.
func (p *Page) IsDeleted() bool { return p.audit.IsDeleted() }

// Update updates page fields.
func (p *Page) Update(title, content, metaDescription *string, isPublished *bool, sortOrder *int, updatedBy string) error {
	if p.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if title != nil {
		if *title == "" {
			return shared.ErrEmptyName
		}
		p.title = *title
	}
	if content != nil {
		p.content = *content
	}
	if metaDescription != nil {
		p.metaDescription = *metaDescription
	}
	if isPublished != nil {
		p.isPublished = *isPublished
		if *isPublished && p.publishedAt == nil {
			now := time.Now()
			p.publishedAt = &now
		}
	}
	if sortOrder != nil {
		p.sortOrder = *sortOrder
	}
	p.audit.Update(updatedBy)
	return nil
}

// SoftDelete marks the page as deleted.
func (p *Page) SoftDelete(deletedBy string) error {
	if p.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	p.audit.SoftDelete(deletedBy)
	return nil
}
