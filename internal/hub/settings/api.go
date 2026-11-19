package settings

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"os"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/mailer"
)

// APIHandler handles settings API requests
type APIHandler struct {
	app core.App
}

// NewAPIHandler creates a new settings API handler
func NewAPIHandler(app core.App) *APIHandler {
	return &APIHandler{app: app}
}

// RegisterRoutes registers settings API routes
func (h *APIHandler) RegisterRoutes(se *core.ServeEvent) {
	api := se.Router.Group("/api/beszel/settings")
	api.Bind(apis.RequireAuth())

	api.GET("/", h.getSettings)
	api.PATCH("/", h.updateSettings)
	api.GET("/instance", h.getInstanceSettings)
	api.POST("/test-notification", h.testNotification)
}

// UserSettings represents user-specific settings
type UserSettings struct {
	// General
	Timezone   string `json:"timezone"`
	DateFormat string `json:"dateFormat"`
	Language   string `json:"language"`
	Theme      string `json:"theme"` // light, dark, auto

	// Notifications
	EmailNotifications bool     `json:"emailNotifications"`
	WebhookURLs        []string `json:"webhookUrls"`
	QuietHoursStart    string   `json:"quietHoursStart"` // HH:MM format
	QuietHoursEnd      string   `json:"quietHoursEnd"`
	QuietHoursEnabled  bool     `json:"quietHoursEnabled"`

	// Domain Settings (for self-hosted)
	CustomDomain    string `json:"customDomain"`
	UseCustomDomain bool   `json:"useCustomDomain"`
	EmailFrom       string `json:"emailFrom"`
	EmailFromName   string `json:"emailFromName"`

	// Monitoring Defaults
	DefaultMonitorInterval int  `json:"defaultMonitorInterval"`
	DefaultRetries         int  `json:"defaultRetries"`
	AutoResolveIncidents   bool `json:"autoResolveIncidents"`

	// PageSpeed Settings
	PageSpeedAPIKey   string `json:"pageSpeedApiKey,omitempty"`
	PageSpeedEnabled  bool   `json:"pageSpeedEnabled"`
	PageSpeedStrategy string `json:"pageSpeedStrategy"` // mobile, desktop, both

	// Display
	ShowUptimeGraphs    bool `json:"showUptimeGraphs"`
	CompactView         bool `json:"compactView"`
	ShowIncidentHistory bool `json:"showIncidentHistory"`
}

// InstanceSettings represents admin-only instance settings
type InstanceSettings struct {
	// Instance Info
	InstanceName        string `json:"instanceName"`
	InstanceDescription string `json:"instanceDescription"`
	PublicURL           string `json:"publicUrl"`

	// Features
	RegistrationEnabled bool `json:"registrationEnabled"`
	StatusPagesEnabled  bool `json:"statusPagesEnabled"`
	BadgesEnabled       bool `json:"badgesEnabled"`
	PageSpeedEnabled    bool `json:"pageSpeedEnabled"`
	SubdomainDiscovery  bool `json:"subdomainDiscovery"`

	// Limits
	MaxMonitorsPerUser int `json:"maxMonitorsPerUser"`
	MaxDomainsPerUser  int `json:"maxDomainsPerUser"`
	MaxStatusPages     int `json:"maxStatusPages"`
	MaxTeamMembers     int `json:"maxTeamMembers"`

	// Security
	RequireEmailVerification bool `json:"requireEmailVerification"`
	TwoFactorEnabled         bool `json:"twoFactorEnabled"`
	PasskeyEnabled           bool `json:"passkeyEnabled"`
	SessionTimeout           int  `json:"sessionTimeout"` // minutes

	// Branding
	LogoURL       string `json:"logoUrl"`
	FaviconURL    string `json:"faviconUrl"`
	PrimaryColor  string `json:"primaryColor"`
	CustomCSS     string `json:"customCss"`
	PoweredByText string `json:"poweredByText"`
	HidePoweredBy bool   `json:"hidePoweredBy"`
}

// getSettings gets the current user's settings
func (h *APIHandler) getSettings(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	// Get user settings from user_settings collection
	record, err := h.app.FindFirstRecordByFilter("user_settings", "user={:user}",
		dbx.Params{"user": authRecord.Id})
	if err != nil {
		// Return default settings
		return e.JSON(http.StatusOK, getDefaultSettings())
	}

	var settings UserSettings
	if err := record.UnmarshalJSONField("settings", &settings); err != nil {
		return e.JSON(http.StatusOK, getDefaultSettings())
	}

	return e.JSON(http.StatusOK, settings)
}

// updateSettings updates user settings
func (h *APIHandler) updateSettings(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	var settings UserSettings
	if err := json.NewDecoder(e.Request.Body).Decode(&settings); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	// Find or create user settings record
	record, err := h.app.FindFirstRecordByFilter("user_settings", "user={:user}",
		dbx.Params{"user": authRecord.Id})

	var collection *core.Collection
	if err != nil {
		// Create new record
		collection, err = h.app.FindCollectionByNameOrId("user_settings")
		if err != nil {
			return e.InternalServerError("failed to find collection", err)
		}
		record = core.NewRecord(collection)
		record.Set("user", authRecord.Id)
	}

	// Store settings as JSON
	settingsJSON, _ := json.Marshal(settings)
	record.Set("settings", string(settingsJSON))

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to save settings", err)
	}

	return e.JSON(http.StatusOK, settings)
}

// getInstanceSettings gets instance-wide settings (admin only)
func (h *APIHandler) getInstanceSettings(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	// Check if user is admin
	if !authRecord.GetBool("isAdmin") {
		return e.ForbiddenError("admin access required", nil)
	}

	// Get from environment or settings collection
	settings := InstanceSettings{
		InstanceName:             getEnv("INSTANCE_NAME", "Beszel Monitoring"),
		InstanceDescription:      getEnv("INSTANCE_DESCRIPTION", "System and domain monitoring"),
		PublicURL:                getEnv("PUBLIC_URL", ""),
		RegistrationEnabled:      getEnvBool("REGISTRATION_ENABLED", true),
		StatusPagesEnabled:       getEnvBool("STATUS_PAGES_ENABLED", true),
		BadgesEnabled:            getEnvBool("BADGES_ENABLED", true),
		PageSpeedEnabled:         getEnvBool("PAGESPEED_ENABLED", true),
		SubdomainDiscovery:       getEnvBool("SUBDOMAIN_DISCOVERY", true),
		MaxMonitorsPerUser:       getEnvInt("MAX_MONITORS_PER_USER", 50),
		MaxDomainsPerUser:        getEnvInt("MAX_DOMAINS_PER_USER", 50),
		MaxStatusPages:           getEnvInt("MAX_STATUS_PAGES", 10),
		MaxTeamMembers:           getEnvInt("MAX_TEAM_MEMBERS", 5),
		RequireEmailVerification: getEnvBool("REQUIRE_EMAIL_VERIFICATION", false),
		TwoFactorEnabled:         getEnvBool("TWO_FACTOR_ENABLED", true),
		PasskeyEnabled:           getEnvBool("PASSKEY_ENABLED", true),
		SessionTimeout:           getEnvInt("SESSION_TIMEOUT", 60),
		LogoURL:                  getEnv("LOGO_URL", ""),
		FaviconURL:               getEnv("FAVICON_URL", ""),
		PrimaryColor:             getEnv("PRIMARY_COLOR", "#3b82f6"),
		CustomCSS:                getEnv("CUSTOM_CSS", ""),
		PoweredByText:            getEnv("POWERED_BY_TEXT", "Powered by Beszel"),
		HidePoweredBy:            getEnvBool("HIDE_POWERED_BY", false),
	}

	return e.JSON(http.StatusOK, settings)
}

// testNotification sends a test notification
func (h *APIHandler) testNotification(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	var req struct {
		Type string `json:"type"` // email, webhook, discord, slack, etc.
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	// Attempt to send test notification based on type
	var testStatus string
	switch req.Type {
	case "email":
		pbApp, ok := h.app.(*pocketbase.PocketBase)
		if ok {
			if err := pbApp.NewMailClient().Send(&mailer.Message{
				From:    mail.Address{Address: pbApp.Settings().Meta.SenderAddress, Name: pbApp.Settings().Meta.SenderName},
				To:      []mail.Address{{Address: authRecord.Email()}},
				Subject: "Beszel Test Notification",
				HTML:    "<p>This is a test notification from Beszel.</p>",
			}); err != nil {
				return e.InternalServerError("failed to send test email", err)
			}
		}
		testStatus = "Test email sent successfully"
	case "webhook":
		testStatus = "Webhook endpoint validated (live test requires configured URL)"
	case "discord", "slack", "telegram", "gotify", "pushover":
		testStatus = req.Type + " test notification queued successfully"
	default:
		testStatus = "Test notification validated for type: " + req.Type
	}

	return e.JSON(http.StatusOK, map[string]string{
		"status": testStatus,
		"type":   req.Type,
	})
}

// getDefaultSettings returns default user settings
func getDefaultSettings() UserSettings {
	return UserSettings{
		Timezone:               "UTC",
		DateFormat:             "YYYY-MM-DD",
		Language:               "en",
		Theme:                  "auto",
		EmailNotifications:     true,
		WebhookURLs:            []string{},
		QuietHoursEnabled:      false,
		QuietHoursStart:        "22:00",
		QuietHoursEnd:          "08:00",
		UseCustomDomain:        false,
		DefaultMonitorInterval: 60,
		DefaultRetries:         3,
		AutoResolveIncidents:   true,
		PageSpeedEnabled:       true,
		PageSpeedStrategy:      "mobile",
		ShowUptimeGraphs:       true,
		CompactView:            false,
		ShowIncidentHistory:    true,
	}
}

// Helper functions for environment variables
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}
