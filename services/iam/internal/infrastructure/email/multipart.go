package email

import (
	"encoding/base64"
	"fmt"
	"maps"
	"mime"
	"strings"

	"github.com/google/uuid"
)

// Attachment represents a file to attach to an email.
type Attachment struct {
	Filename    string // e.g., "rm-cost-export-202604.xlsx"
	ContentType string // e.g., "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	Data        []byte
}

// buildMessage constructs a raw RFC 2822 MIME email message.
//
// When attachments is nil or empty, it produces a simple text/html message.
// When attachments are present, it produces a multipart/mixed message with the
// HTML body as the first part followed by one attachment part per file.
func buildMessage(headers map[string]string, htmlBody string, attachments []Attachment) string {
	// Copy headers to avoid mutating the caller's map.
	h := make(map[string]string, len(headers)+2)
	maps.Copy(h, headers)

	var msg strings.Builder

	writeHeader := func(k, v string) {
		fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
	}

	if len(attachments) == 0 {
		h["MIME-Version"] = "1.0"
		h["Content-Type"] = "text/html; charset=UTF-8"
		for k, v := range h {
			writeHeader(k, v)
		}
		msg.WriteString("\r\n")
		msg.WriteString(htmlBody)
		return msg.String()
	}

	// Multipart message with unique boundary per call.
	boundary := "goapps-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]

	h["MIME-Version"] = "1.0"
	h["Content-Type"] = fmt.Sprintf(`multipart/mixed; boundary="%s"`, boundary)
	for k, v := range h {
		writeHeader(k, v)
	}
	msg.WriteString("\r\n")

	// HTML part.
	fmt.Fprintf(&msg, "--%s\r\n", boundary)
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: base64\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(wrapBase64(base64.StdEncoding.EncodeToString([]byte(htmlBody)), 76))
	msg.WriteString("\r\n")

	// Attachment parts.
	for _, att := range attachments {
		encodedName := mime.QEncoding.Encode("UTF-8", att.Filename)
		encoded := wrapBase64(base64.StdEncoding.EncodeToString(att.Data), 76)

		fmt.Fprintf(&msg, "--%s\r\n", boundary)
		fmt.Fprintf(&msg, "Content-Type: %s; name=\"%s\"\r\n", att.ContentType, encodedName)
		msg.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&msg, "Content-Disposition: attachment; filename=\"%s\"\r\n", encodedName)
		msg.WriteString("\r\n")
		msg.WriteString(encoded)
		msg.WriteString("\r\n")
	}

	fmt.Fprintf(&msg, "--%s--\r\n", boundary)
	return msg.String()
}

// wrapBase64 inserts "\r\n" every lineLen characters, as required by RFC 2045.
func wrapBase64(s string, lineLen int) string {
	var sb strings.Builder
	sb.Grow(len(s) + (len(s)/lineLen+1)*2)
	for i := 0; i < len(s); i += lineLen {
		end := min(i+lineLen, len(s))
		sb.WriteString(s[i:end])
		sb.WriteString("\r\n")
	}
	return sb.String()
}
