//go:build testing

package email

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderAlert(t *testing.T) {
	html, err := RenderAlert(Alert{
		Title:        "CPU above threshold on web-1",
		Message:      "CPU averaged 91.00% for the previous 10 minutes.",
		Link:         "https://beszel.example.com/system/abc",
		LinkText:     "View web-1",
		SettingsLink: "https://beszel.example.com/settings/notifications",
	})
	require.NoError(t, err)

	assert.Contains(t, html, "<title>CPU above threshold on web-1</title>")
	assert.Contains(t, html, "CPU averaged 91.00% for the previous 10 minutes.")
	assert.Contains(t, html, `href="https://beszel.example.com/system/abc"`)
	assert.Contains(t, html, ">View web-1</a>")
	assert.Contains(t, html, `href="https://beszel.example.com/settings/notifications"`)

	if dir := os.Getenv("BESZEL_EMAIL_PREVIEW_DIR"); dir != "" {
		preview, _ := RenderAlert(Alert{
			Title:        "Container api is unhealthy on web-1",
			Message:      "Container api became unhealthy.\nPrevious state: healthy\n\napi logs:\n```\nERROR connection refused\nretrying in 5s\n```",
			Link:         "https://beszel.example.com/system/abc",
			LinkText:     "View web-1",
			SettingsLink: "https://beszel.example.com/settings/notifications",
		})
		require.NoError(t, os.WriteFile(dir+"/alert_email.html", []byte(preview), 0o644))
	}
}

func TestRenderAlertEscapesContent(t *testing.T) {
	html, err := RenderAlert(Alert{
		Title:   "<b>title</b>",
		Message: "```\n<script>alert(1)</script>\n```",
		Link:    "javascript:alert(1)",
	})
	require.NoError(t, err)

	assert.NotContains(t, html, "<script>")
	assert.NotContains(t, html, "<b>title</b>")
	assert.Contains(t, html, "&lt;script&gt;")
	assert.NotContains(t, html, `href="javascript:`)
	assert.Contains(t, html, ">Open Beszel</a>", "empty link text falls back")
	assert.NotContains(t, html, "Manage notifications", "no settings link when empty")
}

func TestMessageBlocks(t *testing.T) {
	assert.Equal(t, []block{
		{Lines: []string{"first line", "second line"}},
		{Lines: []string{"next paragraph"}},
		{Lines: []string{"logs:"}},
		{Code: true, Lines: []string{"line 1", "", "line 3"}},
		{Lines: []string{"(+1 more)"}},
	}, messageBlocks("first line\nsecond line\n\nnext paragraph\n\nlogs:\n```\nline 1\n\nline 3\n```\n\n(+1 more)"))

	assert.Empty(t, messageBlocks(""))
	// an unterminated fence still renders its content as code
	assert.Equal(t, []block{{Code: true, Lines: []string{"x"}}}, messageBlocks("```\nx"))
	assert.Equal(t, []block{{Lines: []string{"a", "b"}}}, messageBlocks("a\r\nb"))
}

func TestWrapPocketBaseHTML(t *testing.T) {
	html, ok := wrapPocketBaseHTML("Reset your Beszel password", "<html><head><style>x</style></head><body>\n<p>Hello,</p><a class=\"btn\" href=\"https://x/reset\">Reset password</a>\n</body></html>")
	require.True(t, ok)
	assert.Contains(t, html, "<title>Reset your Beszel password</title>")
	assert.Contains(t, html, `<p>Hello,</p><a class="btn" href="https://x/reset">Reset password</a>`)
	assert.NotContains(t, html, "<style>x</style>")

	_, ok = wrapPocketBaseHTML("subject", "<p>no body tag</p>")
	assert.False(t, ok)
}
