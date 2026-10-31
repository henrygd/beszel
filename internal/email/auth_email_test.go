//go:build testing

package email_test

import (
	"os"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/mails"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthEmailsUseBeszelLayout(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	require.NoError(t, mails.SendRecordPasswordReset(hub, user))

	message := hub.TestMailer.LastMessage()
	assert.Contains(t, message.HTML, `class="card content"`, "should use the Beszel layout")
	assert.Contains(t, message.HTML, "<title>"+message.Subject+"</title>")
	assert.Contains(t, message.HTML, "/_/#/auth/confirm-password-reset/", "should keep the template content")
	assert.NotContains(t, message.HTML, "Source Sans Pro", "should drop the PocketBase layout")

	if dir := os.Getenv("BESZEL_EMAIL_PREVIEW_DIR"); dir != "" {
		require.NoError(t, os.WriteFile(dir+"/password_reset_email.html", []byte(message.HTML), 0o644))
	}
}
