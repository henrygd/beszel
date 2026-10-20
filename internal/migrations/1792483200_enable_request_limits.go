package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		settings := app.Settings()
		settings.RateLimits.Enabled = true
		for _, rule := range settings.RateLimits.Rules {
			if rule.Label == "*:auth" {
				return app.Save(settings)
			}
		}
		settings.RateLimits.Rules = append([]core.RateLimitRule{{
			Label:       "*:auth",
			MaxRequests: 2,
			Duration:    3,
		}}, settings.RateLimits.Rules...)
		return app.Save(settings)
	}, nil)
}
