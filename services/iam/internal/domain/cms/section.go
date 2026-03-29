// Package cms provides domain logic for CMS content management.
package cms

import (
	"regexp"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// SectionType represents the type of a CMS section.
type SectionType string

// Section type constants.
const (
	SectionTypeHero        SectionType = "HERO"
	SectionTypeFeature     SectionType = "FEATURE"
	SectionTypeFAQ         SectionType = "FAQ"
	SectionTypeTestimonial SectionType = "TESTIMONIAL"
	SectionTypeCTA         SectionType = "CTA"
	SectionTypeCustom      SectionType = "CUSTOM"
)

// ValidSectionTypes contains all valid section types.
var ValidSectionTypes = map[SectionType]bool{
	SectionTypeHero:        true,
	SectionTypeFeature:     true,
	SectionTypeFAQ:         true,
	SectionTypeTestimonial: true,
	SectionTypeCTA:         true,
	SectionTypeCustom:      true,
}

// Validation regex for section key.
var sectionKeyRegex = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Section represents an individual landing page section (hero, feature, FAQ, etc.).
type Section struct {
	id          uuid.UUID
	sectionType SectionType
	sectionKey  string
	title       string
	subtitle    string
	content     string
	iconName    string
	imageURL    string
	buttonText  string
	buttonURL   string
	sortOrder   int
	isPublished bool
	metadata    string
	audit       shared.AuditInfo
}

// NewSection creates a new Section entity with validation.
func NewSection(
	sectionType SectionType,
	sectionKey, title, subtitle, content string,
	iconName, imageURL, buttonText, buttonURL string,
	sortOrder int, isPublished bool, metadata string,
	createdBy string,
) (*Section, error) {
	if !ValidSectionTypes[sectionType] {
		return nil, ErrInvalidSectionType
	}
	if sectionKey == "" {
		return nil, shared.ErrEmptyCode
	}
	if !sectionKeyRegex.MatchString(sectionKey) {
		return nil, ErrInvalidSectionKey
	}
	if title == "" {
		return nil, shared.ErrEmptyName
	}

	return &Section{
		id:          uuid.New(),
		sectionType: sectionType,
		sectionKey:  sectionKey,
		title:       title,
		subtitle:    subtitle,
		content:     content,
		iconName:    iconName,
		imageURL:    imageURL,
		buttonText:  buttonText,
		buttonURL:   buttonURL,
		sortOrder:   sortOrder,
		isPublished: isPublished,
		metadata:    metadata,
		audit:       shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructSection reconstructs a Section from persistence.
func ReconstructSection(
	id uuid.UUID,
	sectionType SectionType,
	sectionKey, title, subtitle, content string,
	iconName, imageURL, buttonText, buttonURL string,
	sortOrder int, isPublished bool, metadata string,
	audit shared.AuditInfo,
) *Section {
	return &Section{
		id:          id,
		sectionType: sectionType,
		sectionKey:  sectionKey,
		title:       title,
		subtitle:    subtitle,
		content:     content,
		iconName:    iconName,
		imageURL:    imageURL,
		buttonText:  buttonText,
		buttonURL:   buttonURL,
		sortOrder:   sortOrder,
		isPublished: isPublished,
		metadata:    metadata,
		audit:       audit,
	}
}

// ID returns the section identifier.
func (s *Section) ID() uuid.UUID { return s.id }

// Type returns the section type.
func (s *Section) Type() SectionType { return s.sectionType }

// Key returns the section key.
func (s *Section) Key() string { return s.sectionKey }

// Title returns the section title.
func (s *Section) Title() string { return s.title }

// Subtitle returns the section subtitle.
func (s *Section) Subtitle() string { return s.subtitle }

// Content returns the section content.
func (s *Section) Content() string { return s.content }

// IconName returns the icon name.
func (s *Section) IconName() string { return s.iconName }

// ImageURL returns the image URL.
func (s *Section) ImageURL() string { return s.imageURL }

// ButtonText returns the button text.
func (s *Section) ButtonText() string { return s.buttonText }

// ButtonURL returns the button URL.
func (s *Section) ButtonURL() string { return s.buttonURL }

// SortOrder returns the sort order.
func (s *Section) SortOrder() int { return s.sortOrder }

// IsPublished returns whether the section is published.
func (s *Section) IsPublished() bool { return s.isPublished }

// Metadata returns the metadata JSON string.
func (s *Section) Metadata() string { return s.metadata }

// Audit returns the audit information.
func (s *Section) Audit() shared.AuditInfo { return s.audit }

// IsDeleted returns whether the section has been soft-deleted.
func (s *Section) IsDeleted() bool { return s.audit.IsDeleted() }

// Update updates section fields.
func (s *Section) Update(
	sectionType *SectionType,
	title, subtitle, content *string,
	iconName, imageURL, buttonText, buttonURL *string,
	sortOrder *int, isPublished *bool, metadata *string,
	updatedBy string,
) error {
	if s.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if sectionType != nil {
		if !ValidSectionTypes[*sectionType] {
			return ErrInvalidSectionType
		}
		s.sectionType = *sectionType
	}
	if title != nil {
		if *title == "" {
			return shared.ErrEmptyName
		}
		s.title = *title
	}
	if subtitle != nil {
		s.subtitle = *subtitle
	}
	if content != nil {
		s.content = *content
	}
	if iconName != nil {
		s.iconName = *iconName
	}
	if imageURL != nil {
		s.imageURL = *imageURL
	}
	if buttonText != nil {
		s.buttonText = *buttonText
	}
	if buttonURL != nil {
		s.buttonURL = *buttonURL
	}
	if sortOrder != nil {
		s.sortOrder = *sortOrder
	}
	if isPublished != nil {
		s.isPublished = *isPublished
	}
	if metadata != nil {
		s.metadata = *metadata
	}
	s.audit.Update(updatedBy)
	return nil
}

// SoftDelete marks the section as deleted.
func (s *Section) SoftDelete(deletedBy string) error {
	if s.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	s.audit.SoftDelete(deletedBy)
	return nil
}
