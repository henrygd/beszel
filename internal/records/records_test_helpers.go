//go:build testing
// +build testing

package records

import (
	"github.com/pocketbase/pocketbase/core"
)

// TestDeleteOldSystemStats exposes deleteOldSystemStats for testing
func TestDeleteOldSystemStats(app core.App) error {
	return deleteOldSystemStats(app)
}

// TestDeleteOldAlertsHistory exposes deleteOldAlertsHistory for testing
func TestDeleteOldAlertsHistory(app core.App, countToKeep, countBeforeDeletion int) error {
	return deleteOldAlertsHistory(app, countToKeep, countBeforeDeletion)
}

// TestTwoDecimals exposes twoDecimals for testing
func TestTwoDecimals(value float64) float64 {
	return twoDecimals(value)
}
