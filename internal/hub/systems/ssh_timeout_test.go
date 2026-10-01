//go:build testing

package systems

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRunWithTimeout covers the guard added for issue #2041: the per-system SSH
// data exchange must never block the updater indefinitely on a dead connection.
func TestRunWithTimeout(t *testing.T) {
	t.Run("returns the operation result when it completes before the timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			wantErr := errors.New("boom")
			onTimeoutCalled := false

			retry, err := runWithTimeout(10*time.Second, func() (bool, error) {
				return true, wantErr
			}, func() { onTimeoutCalled = true })

			assert.True(t, retry, "should return the operation's retry value")
			assert.Equal(t, wantErr, err, "should return the operation's error")
			assert.False(t, onTimeoutCalled, "onTimeout must not fire when the op completes")
		})
	})

	t.Run("times out and tears down the connection when the op blocks", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// unblock simulates a half-open connection: the op is stuck reading a
			// response that never arrives until the connection is torn down.
			unblock := make(chan struct{})
			onTimeoutCalled := false
			start := time.Now()

			retry, err := runWithTimeout(5*time.Second, func() (bool, error) {
				<-unblock
				return false, nil
			}, func() {
				onTimeoutCalled = true
				close(unblock) // tearing down the connection releases the blocked read
			})

			assert.Equal(t, 5*time.Second, time.Since(start), "should return exactly at the timeout")
			assert.True(t, retry, "a timeout should be retryable so the next tick re-dials")
			assert.Error(t, err, "a timeout must surface an error so the system is set down")
			assert.True(t, onTimeoutCalled, "onTimeout must fire so the dead connection is closed")

			synctest.Wait() // ensure the released op goroutine exits cleanly
		})
	})
}
