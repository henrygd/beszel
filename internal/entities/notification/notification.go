package notification

import "time"

// Provider types
const (
	ProviderEmail    = "email"
	ProviderWebhook  = "webhook"
	ProviderDiscord  = "discord"
	ProviderSlack    = "slack"
	ProviderTelegram = "telegram"
	ProviderGotify   = "gotify"
	ProviderPushover = "pushover"
)

// Notification represents a notification provider configuration
type Notification struct {
	ID        string                 `json:"id" db:"id"`
	Name      string                 `json:"name" db:"name"`
	Type      string                 `json:"type" db:"type"`
	IsDefault bool                   `json:"is_default" db:"is_default"`
	Settings  map[string]interface{} `json:"settings" db:"settings"`
	UserID    string                 `json:"user" db:"user"`
	Active    bool                   `json:"active" db:"active"`
	Created   time.Time              `json:"created" db:"created"`
	Updated   time.Time              `json:"updated" db:"updated"`
}

// MonitorNotification links monitors to notifications
type MonitorNotification struct {
	ID             string    `json:"id" db:"id"`
	MonitorID      string    `json:"monitor" db:"monitor"`
	NotificationID string    `json:"notification" db:"notification"`
	UserID         string    `json:"user" db:"user"`
	Created        time.Time `json:"created" db:"created"`
}

// NotificationMessage represents a message to be sent
type NotificationMessage struct {
	Title       string
	Body        string
	MonitorName string
	MonitorURL  string
	Status      string
	Timestamp   time.Time
	Ping        int
	Message     string
}

// EmailSettings for SMTP email notifications
type EmailSettings struct {
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUser     string `json:"smtp_user"`
	SMTPPassword string `json:"smtp_password"`
	FromEmail    string `json:"from_email"`
	ToEmail      string `json:"to_email"`
	UseTLS       bool   `json:"use_tls"`
}

// WebhookSettings for webhook notifications
type WebhookSettings struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	Headers      map[string]string `json:"headers"`
	BodyTemplate string            `json:"body_template"`
}

// DiscordSettings for Discord webhook notifications
type DiscordSettings struct {
	WebhookURL string `json:"webhook_url"`
	Username   string `json:"username"`
	AvatarURL  string `json:"avatar_url"`
}

// SlackSettings for Slack webhook notifications
type SlackSettings struct {
	WebhookURL string `json:"webhook_url"`
	Channel    string `json:"channel"`
	Username   string `json:"username"`
}

// TelegramSettings for Telegram bot notifications
type TelegramSettings struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// GotifySettings for Gotify notifications
type GotifySettings struct {
	ServerURL string `json:"server_url"`
	AppToken  string `json:"app_token"`
	Priority  int    `json:"priority"`
}

// PushoverSettings for Pushover notifications
type PushoverSettings struct {
	AppToken string `json:"app_token"`
	UserKey  string `json:"user_key"`
	Priority int    `json:"priority"`
	Device   string `json:"device"`
}

// Provider interface for notification implementations
type Provider interface {
	Send(message *NotificationMessage) error
	Validate() error
}

// GetSettings returns typed settings based on provider type
func (n *Notification) GetSettings() interface{} {
	switch n.Type {
	case ProviderEmail:
		var settings EmailSettings
		if m, ok := n.Settings["smtp_host"].(string); ok {
			settings.SMTPHost = m
		}
		if m, ok := n.Settings["smtp_port"].(float64); ok {
			settings.SMTPPort = int(m)
		}
		if m, ok := n.Settings["smtp_user"].(string); ok {
			settings.SMTPUser = m
		}
		if m, ok := n.Settings["smtp_password"].(string); ok {
			settings.SMTPPassword = m
		}
		if m, ok := n.Settings["from_email"].(string); ok {
			settings.FromEmail = m
		}
		if m, ok := n.Settings["to_email"].(string); ok {
			settings.ToEmail = m
		}
		if m, ok := n.Settings["use_tls"].(bool); ok {
			settings.UseTLS = m
		}
		return settings
	case ProviderWebhook:
		var settings WebhookSettings
		if m, ok := n.Settings["url"].(string); ok {
			settings.URL = m
		}
		if m, ok := n.Settings["method"].(string); ok {
			settings.Method = m
		}
		if m, ok := n.Settings["headers"].(map[string]interface{}); ok {
			settings.Headers = make(map[string]string)
			for k, v := range m {
				if s, ok := v.(string); ok {
					settings.Headers[k] = s
				}
			}
		}
		if m, ok := n.Settings["body_template"].(string); ok {
			settings.BodyTemplate = m
		}
		return settings
	case ProviderDiscord:
		var settings DiscordSettings
		if m, ok := n.Settings["webhook_url"].(string); ok {
			settings.WebhookURL = m
		}
		if m, ok := n.Settings["username"].(string); ok {
			settings.Username = m
		}
		if m, ok := n.Settings["avatar_url"].(string); ok {
			settings.AvatarURL = m
		}
		return settings
	case ProviderSlack:
		var settings SlackSettings
		if m, ok := n.Settings["webhook_url"].(string); ok {
			settings.WebhookURL = m
		}
		if m, ok := n.Settings["channel"].(string); ok {
			settings.Channel = m
		}
		if m, ok := n.Settings["username"].(string); ok {
			settings.Username = m
		}
		return settings
	case ProviderTelegram:
		var settings TelegramSettings
		if m, ok := n.Settings["bot_token"].(string); ok {
			settings.BotToken = m
		}
		if m, ok := n.Settings["chat_id"].(string); ok {
			settings.ChatID = m
		}
		return settings
	case ProviderGotify:
		var settings GotifySettings
		if m, ok := n.Settings["server_url"].(string); ok {
			settings.ServerURL = m
		}
		if m, ok := n.Settings["app_token"].(string); ok {
			settings.AppToken = m
		}
		if m, ok := n.Settings["priority"].(float64); ok {
			settings.Priority = int(m)
		}
		return settings
	case ProviderPushover:
		var settings PushoverSettings
		if m, ok := n.Settings["app_token"].(string); ok {
			settings.AppToken = m
		}
		if m, ok := n.Settings["user_key"].(string); ok {
			settings.UserKey = m
		}
		if m, ok := n.Settings["priority"].(float64); ok {
			settings.Priority = int(m)
		}
		if m, ok := n.Settings["device"].(string); ok {
			settings.Device = m
		}
		return settings
	default:
		return nil
	}
}

// NotificationEvent represents a notification sent event
type NotificationEvent struct {
	ID             string    `json:"id" db:"id"`
	NotificationID string    `json:"notification" db:"notification"`
	MonitorID      string    `json:"monitor" db:"monitor"`
	Status         string    `json:"status" db:"status"`
	Message        string    `json:"message" db:"message"`
	SentAt         time.Time `json:"sent_at" db:"sent_at"`
	Error          string    `json:"error" db:"error"`
}
