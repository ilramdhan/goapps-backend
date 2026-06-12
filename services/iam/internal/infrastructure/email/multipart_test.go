package email

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMessage_NoAttachments(t *testing.T) {
	headers := map[string]string{
		"From":    "GoApps <noreply@goapps.local>",
		"To":      "user@example.com",
		"Subject": "Test email",
	}
	msg := buildMessage(headers, "<p>Hello</p>", nil)
	assert.Contains(t, msg, "Content-Type: text/html; charset=UTF-8")
	assert.Contains(t, msg, "<p>Hello</p>")
	assert.Contains(t, msg, "From: GoApps <noreply@goapps.local>")
	assert.NotContains(t, msg, "multipart/mixed")
}

func TestBuildMessage_WithAttachment(t *testing.T) {
	headers := map[string]string{
		"From":    "GoApps <noreply@goapps.local>",
		"To":      "user@example.com",
		"Subject": "Export ready",
	}
	attachments := []Attachment{
		{
			Filename:    "report.xlsx",
			ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			Data:        []byte("fake xlsx data"),
		},
	}
	msg := buildMessage(headers, "<p>See attached.</p>", attachments)
	assert.Contains(t, msg, "multipart/mixed")
	assert.Contains(t, msg, "Content-Type: text/html; charset=UTF-8")
	assert.Contains(t, msg, "report.xlsx")
	assert.Contains(t, msg, "Content-Transfer-Encoding: base64")
	assert.Contains(t, msg, "Content-Disposition: attachment")
	// HTML body is base64-encoded in multipart messages
	assert.Contains(t, msg, "PHA+U2VlIGF0dGFjaGVkLjwvcD4=")
}

func TestBuildMessage_BoundaryIsUnique(t *testing.T) {
	makeHeaders := func() map[string]string {
		return map[string]string{"From": "a@b.com", "To": "c@d.com", "Subject": "x"}
	}
	att := []Attachment{{Filename: "f.pdf", ContentType: "application/pdf", Data: []byte("data")}}
	msg1 := buildMessage(makeHeaders(), "<p>1</p>", att)
	msg2 := buildMessage(makeHeaders(), "<p>2</p>", att)
	extractBoundary := func(s string) string {
		for line := range strings.SplitSeq(s, "\r\n") {
			if strings.HasPrefix(line, "Content-Type: multipart/mixed") {
				parts := strings.Split(line, `boundary="`)
				if len(parts) > 1 {
					return strings.TrimSuffix(parts[1], `"`)
				}
			}
		}
		return ""
	}
	b1 := extractBoundary(msg1)
	b2 := extractBoundary(msg2)
	require.NotEmpty(t, b1)
	require.NotEmpty(t, b2)
	assert.NotEqual(t, b1, b2)
}

func TestWrapBase64_WrapsAt76Chars(t *testing.T) {
	input := strings.Repeat("A", 200)
	result := wrapBase64(input, 76)
	lines := strings.Split(strings.TrimRight(result, "\r\n"), "\r\n")
	for i, line := range lines {
		if i < len(lines)-1 {
			assert.LessOrEqual(t, len(line), 76, "line %d too long: %q", i, line)
		}
	}
}
