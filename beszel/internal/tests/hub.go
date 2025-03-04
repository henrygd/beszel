// Package tests provides helpers for testing the application.
package tests

import (
	"beszel/internal/hub"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	_ "github.com/pocketbase/pocketbase/migrations"
)

// TestHub is a wrapper hub instance used for testing.
type TestHub struct {
	core.App
	*tests.TestApp
	*hub.Hub
}

// NewTestHub creates and initializes a test application instance.
//
// It is the caller's responsibility to call app.Cleanup() when the app is no longer needed.
func NewTestHub(optTestDataDir ...string) (*TestHub, error) {
	var testDataDir string
	if len(optTestDataDir) > 0 {
		testDataDir = optTestDataDir[0]
	}

	return NewTestHubWithConfig(core.BaseAppConfig{
		DataDir:       testDataDir,
		EncryptionEnv: "pb_test_env",
	})
}

// NewTestHubWithConfig creates and initializes a test application instance
// from the provided config.
//
// If config.DataDir is not set it fallbacks to the default internal test data directory.
//
// config.DataDir is cloned for each new test application instance.
//
// It is the caller's responsibility to call app.Cleanup() when the app is no longer needed.
func NewTestHubWithConfig(config core.BaseAppConfig) (*TestHub, error) {
	testApp, err := tests.NewTestAppWithConfig(config)
	if err != nil {
		return nil, err
	}

	hub := hub.NewHub(testApp)

	t := &TestHub{
		App:     testApp,
		TestApp: testApp,
		Hub:     hub,
	}

	return t, nil
}
