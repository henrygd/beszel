package alerts

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/pocketbase/core"
)

// Notification kinds. Each kind may have its own user template.
const (
	NotificationKindStatus    = "status"
	NotificationKindSystem    = "system"
	NotificationKindContainer = "container"
	NotificationKindSmart     = "smart"
	NotificationKindSystemd   = "systemd"
	NotificationKindZfs       = "zfs"
)

var notificationKinds = []string{
	NotificationKindStatus,
	NotificationKindSystem,
	NotificationKindContainer,
	NotificationKindSmart,
	NotificationKindSystemd,
	NotificationKindZfs,
}

const (
	// limit on stored templates (in characters, not bytes)
	maxTemplateChars = 2000
	// limits on rendered output, so a template full of {logs} / {message}
	// placeholders cannot amplify a notification into hundreds of kilobytes
	maxRenderedTitleChars = 500
	maxRenderedBodyChars  = 8000
	truncatedMarker       = "…(truncated)"
)

// NotificationTemplate is a user-defined title / body pair using {placeholder}
// syntax. Empty fields keep the built-in text.
type NotificationTemplate struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// placeholderRegex matches {name} and {group.name} placeholders, optionally
// followed by an inline wording map: {state:down=offline,up=online}.
var placeholderRegex = regexp.MustCompile(`\{([a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*)(?::([^{}]*))?\}`)

// systemVarKeys are always defined (possibly empty) so a known placeholder is
// never left in the rendered text just because the value is unavailable.
var systemVarKeys = []string{
	"system.id", "system.name", "system.host", "system.port", "system.status",
	"system.hostname", "system.os", "system.kernel", "system.cpu", "system.cores",
	"system.arch", "system.agent_version", "system.uptime",
}

func isNotificationKind(kind string) bool {
	return slices.Contains(notificationKinds, kind)
}

// resolveTemplate returns the user's template for kind. ok is false when it
// defines neither a title nor a body.
func resolveTemplate(templates map[string]NotificationTemplate, kind string) (tpl NotificationTemplate, ok bool) {
	tpl = templates[kind]
	tpl.Title = strings.TrimSpace(tpl.Title)
	tpl.Body = strings.TrimSpace(tpl.Body)
	return tpl, tpl.Title != "" || tpl.Body != ""
}

// applyUserTemplates renders the user's template for data.Kind, if one exists.
// A custom body takes over link placement, so the link is no longer appended
// automatically (users add {link} where they want it).
func (am *AlertManager) applyUserTemplates(settings UserNotificationSettings, data AlertMessageData) AlertMessageData {
	if data.Kind == "" || len(settings.Templates) == 0 {
		return data
	}
	tpl, ok := resolveTemplate(settings.Templates, data.Kind)
	if !ok {
		return data
	}
	vars := am.buildTemplateVars(settings, data, tpl)
	if tpl.Title != "" {
		// titles are single line (email subject, push title)
		title := truncateRunes(renderTemplate(tpl.Title, vars), maxRenderedTitleChars)
		data.Title = strings.Join(strings.Fields(title), " ")
	}
	if tpl.Body != "" {
		data.Message = truncateRunes(renderTemplate(tpl.Body, vars), maxRenderedBodyChars)
		data.OmitLink = true
	}
	return data
}

// renderTemplate substitutes known {placeholders}. Unknown ones are kept as
// written so typos stay visible instead of silently disappearing.
func renderTemplate(tpl string, vars map[string]string) string {
	return placeholderRegex.ReplaceAllStringFunc(tpl, func(match string) string {
		parts := placeholderRegex.FindStringSubmatch(match)
		value, ok := vars[parts[1]]
		if !ok {
			return match
		}
		if parts[2] != "" {
			value = mapPlaceholderValue(value, parts[2])
		}
		return value
	})
}

// mapPlaceholderValue applies an inline "key=text,key=text" map to value, e.g.
// {state:down=offline,up=online}. Values without a matching key are unchanged.
func mapPlaceholderValue(value, mapping string) string {
	for _, pair := range strings.Split(mapping, ",") {
		if key, text, ok := strings.Cut(pair, "="); ok && strings.TrimSpace(key) == value {
			return strings.TrimSpace(text)
		}
	}
	return value
}

// truncateRunes cuts s to at most max characters, appending a marker.
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "\n" + truncatedMarker
}

// buildTemplateVars assembles every placeholder value available for data.
// The systems / system_details records are only read when the template
// actually references a {system.*} placeholder.
func (am *AlertManager) buildTemplateVars(settings UserNotificationSettings, data AlertMessageData, tpl NotificationTemplate) map[string]string {
	loc, zoneName := loadNotificationLocation(am.hub, settings.Timezone)
	now := time.Now().In(loc)
	if zoneName == "" {
		zoneName, _ = now.Zone()
	}
	clockLayout := "15:04"
	if settings.HourFormat == "12h" {
		clockLayout = "3:04 PM"
	}
	vars := map[string]string{
		"title":     data.Title,
		"message":   data.Message,
		"link":      data.Link,
		"link_text": data.LinkText,
		"state":     data.State,
		"time":      now.Format("2006-01-02 " + clockLayout),
		"date":      now.Format("2006-01-02"),
		"clock":     now.Format(clockLayout),
		"time_iso":  now.Format(time.RFC3339),
		"timezone":  zoneName,
	}
	for _, key := range systemVarKeys {
		vars[key] = ""
	}
	for key, value := range data.Vars {
		vars[key] = value
	}
	if strings.Contains(tpl.Title, "{system.") || strings.Contains(tpl.Body, "{system.") {
		am.addSystemVars(vars, data.SystemID)
	}
	return vars
}

// addSystemVars fills the system.* placeholders from the systems and
// system_details records. Only values that are actually available overwrite
// what is already in vars, so callers may pre-seed fallbacks.
func (am *AlertManager) addSystemVars(vars map[string]string, systemID string) {
	if systemID == "" {
		return
	}
	sys, err := am.hub.FindRecordById("systems", systemID)
	if err != nil {
		return
	}
	set := func(key, value string) {
		if value != "" {
			vars[key] = value
		}
	}
	set("system.id", sys.Id)
	set("system.name", sys.GetString("name"))
	set("system.host", sys.GetString("host"))
	set("system.port", sys.GetString("port"))
	set("system.status", sys.GetString("status"))
	var info system.Info
	if err := sys.UnmarshalJSONField("info", &info); err == nil {
		set("system.agent_version", info.AgentVersion)
		set("system.hostname", info.Hostname)
		if info.Uptime > 0 {
			set("system.uptime", formatUptime(info.Uptime))
		}
	}
	details, err := am.hub.FindRecordById("system_details", systemID)
	if err != nil {
		return
	}
	set("system.hostname", details.GetString("hostname"))
	set("system.os", details.GetString("os_name"))
	set("system.kernel", details.GetString("kernel"))
	set("system.cpu", details.GetString("cpu"))
	if cores := details.GetInt("cores"); cores > 0 {
		set("system.cores", strconv.Itoa(cores))
	}
	set("system.arch", details.GetString("arch"))
}

// formatUptime renders seconds as "3d 4h 12m".
func formatUptime(seconds uint64) string {
	days := seconds / 86400
	hours := seconds % 86400 / 3600
	minutes := seconds % 3600 / 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// loadNotificationLocation resolves the user's timezone. When unset or
// invalid it falls back to the hub's local time (TZ environment variable) and
// returns an empty name so the caller can use the zone abbreviation instead.
func loadNotificationLocation(app core.App, name string) (*time.Location, string) {
	if name == "" {
		return time.Local, ""
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		app.Logger().Debug("Invalid notification timezone, using hub time", "timezone", name, "err", err)
		return time.Local, ""
	}
	return loc, name
}

// validateNotificationSettings enforces size limits and a loadable timezone.
func validateNotificationSettings(settings UserNotificationSettings) error {
	for kind, tpl := range settings.Templates {
		if !isNotificationKind(kind) {
			return fmt.Errorf("unknown notification template kind: %s", kind)
		}
		if utf8.RuneCountInString(tpl.Title) > maxTemplateChars || utf8.RuneCountInString(tpl.Body) > maxTemplateChars {
			return fmt.Errorf("notification template %q exceeds %d characters", kind, maxTemplateChars)
		}
	}
	if settings.Timezone != "" {
		if _, err := time.LoadLocation(settings.Timezone); err != nil {
			return fmt.Errorf("invalid timezone: %s", settings.Timezone)
		}
	}
	return nil
}

// validateUserSettingsRequest rejects user_settings writes whose notification
// template fields are invalid, so bad data never reaches the alert path.
func validateUserSettingsRequest(e *core.RecordRequestEvent) error {
	raw := strings.TrimSpace(e.Record.GetString("settings"))
	if raw == "" || raw == "null" {
		return e.Next()
	}
	var settings struct {
		Templates map[string]NotificationTemplate `json:"notificationTemplates"`
		Timezone  string                          `json:"notificationTimezone"`
	}
	if err := e.Record.UnmarshalJSONField("settings", &settings); err != nil {
		return e.BadRequestError("Invalid notification template settings", err)
	}
	err := validateNotificationSettings(UserNotificationSettings{
		Templates: settings.Templates,
		Timezone:  settings.Timezone,
	})
	if err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	return e.Next()
}
