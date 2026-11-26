package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWebhookServiceName(t *testing.T) {
	tests := map[string]string{
		"discord://token@id":                "Discord",
		"telegram://token@telegram?chats=1": "Telegram",
		"generic+https://example.com/hook":  "Webhook",
		"generic://example.com":             "Webhook",
		"ntfy://ntfy.sh/topic":              "ntfy",
		"teams://group@tenant/alt/group":    "Microsoft Teams",
		"not a url":                         "Webhook",
	}
	for url, want := range tests {
		assert.Equal(t, want, webhookServiceName(url), url)
	}
}

func TestUniqueChannelName(t *testing.T) {
	used := map[string]bool{}
	assert.Equal(t, "Email", uniqueChannelName(used, "Email", "a@example.com"))
	assert.Equal(t, "Discord", uniqueChannelName(used, "Discord", "a@example.com"))
	assert.Equal(t, "Discord (a@example.com)", uniqueChannelName(used, "Discord", "a@example.com"))
	assert.Equal(t, "Discord (a@example.com) 2", uniqueChannelName(used, "Discord", "a@example.com"))
	assert.Equal(t, "Email (b@example.com)", uniqueChannelName(used, "Email", "b@example.com"))
}
