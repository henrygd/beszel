package migrations

import (
	"slices"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// signInRateLimitRule limits password, OTP, and OAuth2 sign-in attempts per client address.
var signInRateLimitRule = core.RateLimitRule{Label: "*:auth", MaxRequests: 10, Duration: 60}

// untouchedDefaultRateLimitRules are PocketBase's default non-auth rules. They are
// frozen here rather than read from core.NewSettings() so the migration stays
// deterministic if PocketBase changes its defaults.
var untouchedDefaultRateLimitRules = []core.RateLimitRule{
	{Label: "*:auth", MaxRequests: 2, Duration: 3},
	{Label: "*:create", MaxRequests: 20, Duration: 5},
	{Label: "/api/batch", MaxRequests: 3, Duration: 1},
	{Label: "/api/", MaxRequests: 300, Duration: 10},
}

// Enables request limits for sign-in only. Unmodified PocketBase default rules are
// removed so bulk UI actions and agent connections (which may share a reverse proxy
// address) are not throttled. Rules customized by an administrator are kept.
func init() {
	m.Register(func(app core.App) error {
		settings := app.Settings()
		settings.RateLimits.Enabled = true

		rules := slices.DeleteFunc(slices.Clone(settings.RateLimits.Rules), func(r core.RateLimitRule) bool {
			return slices.Contains(untouchedDefaultRateLimitRules, r)
		})

		hasAuthRule := slices.ContainsFunc(rules, func(r core.RateLimitRule) bool {
			return r.Label == signInRateLimitRule.Label
		})
		if !hasAuthRule {
			rules = append([]core.RateLimitRule{signInRateLimitRule}, rules...)
		}

		settings.RateLimits.Rules = rules
		return app.Save(settings)
	}, nil)
}
