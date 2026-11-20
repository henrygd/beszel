package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/henrygd/beszel/internal/entities/notification"
)

type WebhookProvider struct {
	settings notification.WebhookSettings
}

func NewWebhookProvider(settings notification.WebhookSettings) *WebhookProvider {
	return &WebhookProvider{settings: settings}
}

func (p *WebhookProvider) Validate() error {
	if p.settings.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}
	return nil
}

func (p *WebhookProvider) Send(msg *notification.NotificationMessage) error {
	if err := p.Validate(); err != nil {
		return err
	}

	method := p.settings.Method
	if method == "" {
		method = "POST"
	}

	body := p.formatBody(msg)
	req, err := http.NewRequest(method, p.settings.URL, bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range p.settings.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (p *WebhookProvider) formatBody(msg *notification.NotificationMessage) string {
	if p.settings.BodyTemplate != "" {
		return p.settings.BodyTemplate
	}

	data := map[string]interface{}{
		"title":     msg.Title,
		"body":      msg.Body,
		"monitor":   msg.MonitorName,
		"url":       msg.MonitorURL,
		"status":    msg.Status,
		"timestamp": msg.Timestamp,
		"ping":      msg.Ping,
		"message":   msg.Message,
	}

	b, _ := json.Marshal(data)
	return string(b)
}
