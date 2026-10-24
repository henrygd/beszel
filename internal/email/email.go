// Package email renders Beszel's HTML emails.
package email

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

var (
	//go:embed layout.html
	layoutSource string
	//go:embed alert.html
	alertSource string
)

var (
	layoutTemplate  = template.Must(template.New("layout").Parse(layoutSource))
	alertTemplate   = template.Must(template.Must(layoutTemplate.Clone()).Parse(alertSource))
	contentTemplate = template.Must(template.Must(layoutTemplate.Clone()).Parse(`{{define "content"}}{{.Content}}{{end}}`))
)

// Alert is the content of an alert email.
type Alert struct {
	Title        string
	Message      string
	Link         string
	LinkText     string
	SettingsLink string
}

// block is a paragraph or a fenced code block of an alert message.
type block struct {
	Code  bool
	Lines []string
}

// RenderAlert renders the HTML body of an alert email.
func RenderAlert(a Alert) (string, error) {
	if a.LinkText == "" {
		a.LinkText = "Open Beszel"
	}
	var sb strings.Builder
	err := alertTemplate.Execute(&sb, struct {
		Alert
		Blocks []block
	}{a, messageBlocks(a.Message)})
	return sb.String(), err
}

// messageBlocks splits a message into code blocks (``` fences) and
// paragraphs (separated by blank lines).
func messageBlocks(message string) []block {
	var blocks []block
	var lines []string
	inCode := false
	flush := func() {
		if len(lines) > 0 {
			blocks = append(blocks, block{Code: inCode, Lines: lines})
		}
		lines = nil
	}
	for line := range strings.SplitSeq(strings.ReplaceAll(message, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(strings.TrimSpace(line), "```"):
			flush()
			inCode = !inCode
		case !inCode && strings.TrimSpace(line) == "":
			flush()
		default:
			lines = append(lines, line)
		}
	}
	flush()
	return blocks
}

// BindAuthEmails restyles PocketBase's auth emails (verification, password
// reset, email change, OTP and login alert) with the Beszel layout.
func BindAuthEmails(app core.App) {
	app.OnMailerRecordVerificationSend().BindFunc(restyleAuthEmail)
	app.OnMailerRecordPasswordResetSend().BindFunc(restyleAuthEmail)
	app.OnMailerRecordEmailChangeSend().BindFunc(restyleAuthEmail)
	app.OnMailerRecordOTPSend().BindFunc(restyleAuthEmail)
	app.OnMailerRecordAuthAlertSend().BindFunc(restyleAuthEmail)
}

func restyleAuthEmail(e *core.MailerRecordEvent) error {
	if html, ok := wrapPocketBaseHTML(e.Message.Subject, e.Message.HTML); ok {
		e.Message.HTML = html
	} else {
		e.App.Logger().Warn("Could not restyle auth email, sending PocketBase layout", "subject", e.Message.Subject)
	}
	return e.Next()
}

// wrapPocketBaseHTML replaces PocketBase's mail layout with the Beszel layout,
// keeping the (possibly admin-customized) template content inside <body>.
func wrapPocketBaseHTML(title, pbHTML string) (string, bool) {
	start := strings.Index(pbHTML, "<body>")
	end := strings.LastIndex(pbHTML, "</body>")
	if start == -1 || end < start {
		return "", false
	}
	// The content comes from the collection's email template, which PocketBase
	// already renders as trusted HTML with escaped placeholder values.
	content := template.HTML(strings.TrimSpace(pbHTML[start+len("<body>") : end]))
	var sb strings.Builder
	if err := contentTemplate.Execute(&sb, struct {
		Title   string
		Content template.HTML
	}{title, content}); err != nil {
		return "", false
	}
	return sb.String(), true
}
