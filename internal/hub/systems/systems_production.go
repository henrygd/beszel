//go:build !testing

package systems

// Background SMART fetching is enabled in production but disabled for tests (systems_test_helpers.go).
//
// The hub integration tests create/replace systems and clean up the test apps quickly.
// Background SMART fetching can outlive teardown and crash in PocketBase internals (nil DB).
func backgroundSmartFetchEnabled() bool { return true }
