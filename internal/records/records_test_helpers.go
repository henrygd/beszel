//go:build testing
// +build testing

package records

import (
	"github.com/pocketbase/pocketbase/core"
)

// DeleteOldSystemStats exposes deleteOldSystemStats for testing
func DeleteOldSystemStats(app core.App) error {
	return deleteOldSystemStats(app)
}

// DeleteOldAlertsHistory exposes deleteOldAlertsHistory for testing
func DeleteOldAlertsHistory(app core.App, countToKeep, countBeforeDeletion int) error {
	return deleteOldAlertsHistory(app, countToKeep, countBeforeDeletion)
}

// TwoDecimals exposes twoDecimals for testing
func TwoDecimals(value float64) float64 {
	return twoDecimals(value)
}
