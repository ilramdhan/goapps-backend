// Package email provides SMTP email sending with HTML template rendering.
package email

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*.html.tmpl
var templateFS embed.FS

//go:embed assets
var assetFS embed.FS

// SocialLink holds a social media link rendered in the email footer.
type SocialLink struct {
	Name string // e.g., "LinkedIn", "Twitter", "Instagram"
	URL  string
}

// BaseData is embedded in every email data struct and provides branding fields
// used by the shared base template.
type BaseData struct {
	AppName        string       // e.g., "GoApps"
	AppURL         string       // e.g., "https://app.mutugading.com"
	SupportURL     string       // optional; empty omits the support link in the footer.
	Year           int          // set automatically in NewRenderer if zero.
	HeaderTagline  string       // optional tagline shown in the pre-header row (outside the card).
	HeaderTitle    string       // large bold title shown in the card hero banner.
	HeaderSubtitle string       // small text shown above HeaderTitle in the hero banner.
	CompanyName    string       // full legal name used in footer copyright.
	CompanyAddress string       // optional physical address shown in footer.
	PrivacyURL     string       // link to privacy policy; auto-derived from AppURL if empty.
	TermsURL       string       // link to terms of service; auto-derived from AppURL if empty.
	LogoURL        string       // optional logo image URL; falls back to text logo when empty.
	HeaderBgURL    string       // optional header background image URL.
	SocialLinks    []SocialLink // optional; empty omits the social row in footer.
}

// OTPData is used to render the OTP email (password reset + email verification).
type OTPData struct {
	BaseData
	RecipientEmail string
	OTPDigits      []string // pre-split via SplitOTP; includes "-" separator
	ExpiryMinutes  int
	Purpose        string // "password reset" | "email verification"
}

// SecurityData is used to render the security notification email.
type SecurityData struct {
	BaseData
	RecipientName string
	Feature       string // "Two-Factor Authentication" | "Password"
	Action        string // "enabled" | "disabled" | "changed" | "reset"
	IPAddress     string // optional; empty omits the row
	UserAgent     string // optional; empty omits the row
	OccurredAt    string // e.g., "June 11, 2026 at 14:30 WIB"
	SecureURL     string // CTA: link to account security settings
}

// CTAData holds an optional call-to-action button.
// Zero value (both fields empty) means no button is rendered.
type CTAData struct {
	Label string
	URL   string
}

// TableData is an optional inline data table for notification emails.
type TableData struct {
	Caption string // optional table title
	Headers []string
	Rows    [][]string
}

// MetaItem holds a label-value pair for transaction metadata in notification emails.
// Use it to show structured detail like "Request #", "Product", "Status".
type MetaItem struct {
	Label string
	Value string
}

// NotificationData is used to render the general notification email.
type NotificationData struct {
	BaseData
	RecipientName string
	Title         string
	MetaItems     []MetaItem // key-value transaction details shown before body paragraphs.
	Paragraphs    []string   // use SplitParagraphs() to build from a body string.
	CTA           CTAData    // zero value = no button.
	Table         *TableData // nil = no table.
	AlertLevel    string     // "" | "info" | "warning" | "error".
}

// WelcomeData is used to render the welcome email.
type WelcomeData struct {
	BaseData
	RecipientName  string
	RecipientEmail string
	LoginURL       string
}

// Renderer renders email HTML templates. It is safe for concurrent use.
type Renderer struct {
	mu    sync.RWMutex
	cache map[string]*template.Template
	base  BaseData
}

// NewRenderer creates a Renderer populated with branding data derived from config.
// The Year field is auto-set to the current year if zero.
// If LogoURL or HeaderBgURL are empty, the renderer auto-populates them from the
// embedded asset files (assets/logo.png, assets/header-bg.jpg) as base64 data URIs.
// Providing explicit URLs in BaseData overrides the embedded assets.
func NewRenderer(base BaseData) *Renderer {
	if base.Year == 0 {
		base.Year = time.Now().Year()
	}
	if base.LogoURL == "" {
		if data, err := assetFS.ReadFile("assets/logo.png"); err == nil {
			base.LogoURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
		}
	}
	appBase := strings.TrimRight(base.AppURL, "/")
	// Auto-construct URLs from AppURL so staging/production domains are picked up
	// automatically when app_url is configured. Override via explicit config fields.
	if base.HeaderBgURL == "" && appBase != "" {
		base.HeaderBgURL = appBase + "/mutugading-base.jpg"
	}
	if base.PrivacyURL == "" && appBase != "" {
		base.PrivacyURL = appBase + "/privacy"
	}
	if base.TermsURL == "" && appBase != "" {
		base.TermsURL = appBase + "/terms"
	}
	return &Renderer{
		cache: make(map[string]*template.Template),
		base:  base,
	}
}

// BaseData returns a copy of the base branding data so callers can embed it
// into typed data structs without re-specifying AppName/AppURL/Year.
func (r *Renderer) BaseData() BaseData {
	return r.base
}

// Render executes the named template (e.g. "otp", "notification") with data and
// returns the rendered HTML string. Data must embed BaseData so the base shell
// can access AppName, AppURL, SupportURL, and Year.
func (r *Renderer) Render(name string, data any) (string, error) {
	tmpl, err := r.loadTemplate(name)
	if err != nil {
		return "", fmt.Errorf("email renderer: load %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", fmt.Errorf("email renderer: execute %q: %w", name, err)
	}
	return buf.String(), nil
}

func (r *Renderer) loadTemplate(name string) (*template.Template, error) {
	r.mu.RLock()
	if tmpl, ok := r.cache[name]; ok {
		r.mu.RUnlock()
		return tmpl, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	// Double-check after acquiring write lock (another goroutine may have populated).
	if tmpl, ok := r.cache[name]; ok {
		return tmpl, nil
	}
	funcs := template.FuncMap{
		// safeURL marks a string as a trusted URL, bypassing html/template's
		// sanitization of data: URIs and other non-http schemes.
		"safeURL": func(s string) template.URL { return template.URL(s) }, //nolint:gosec // intentional URL type conversion for trusted internal email templates
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(
		templateFS,
		"templates/_base.html.tmpl",
		fmt.Sprintf("templates/%s.html.tmpl", name),
	)
	if err != nil {
		return nil, err
	}
	r.cache[name] = tmpl
	return tmpl, nil
}

// SplitOTP splits a 6-digit OTP string into a display slice with a "-" separator
// between digit groups: "840921" → ["8","4","0","-","9","2","1"].
// Non-6-digit inputs are returned as a single-element slice.
func SplitOTP(otp string) []string {
	const (
		otpLen   = 6
		otpSplit = otpLen / 2
	)
	if len(otp) != otpLen {
		return []string{otp}
	}
	return []string{
		string(otp[0]), string(otp[1]), string(otp[2]),
		"-",
		string(otp[otpSplit]), string(otp[otpSplit+1]), string(otp[otpSplit+2]),
	}
}

// SplitParagraphs splits body text on double-newline boundaries and trims whitespace.
// Single newlines within a paragraph are collapsed to a space.
// Empty segments are dropped.
func SplitParagraphs(body string) []string {
	parts := strings.Split(strings.TrimSpace(body), "\n\n")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		// Collapse single newlines to spaces for clean <p> rendering.
		trimmed = strings.ReplaceAll(trimmed, "\n", " ")
		result = append(result, trimmed)
	}
	return result
}
