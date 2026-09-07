//go:build testing

package systems

import (
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
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

// closedConn stands in for a connection whose peer has gone away: opening a
// channel fails rather than succeeding, which is what NewSession does on a
// client that closeSSHConnection has already closed.
type closedConn struct{ ssh.Conn }

func (closedConn) OpenChannel(string, []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, errors.New("use of closed network connection")
}

func (closedConn) Close() error { return nil }

// TestCreateSessionDuringClose covers issue #2157: the background SMART fetch
// creates a session while the updater can be tearing the same connection down,
// so session creation must not read the client field after it is cleared.
func TestCreateSessionDuringClose(t *testing.T) {
	for range 500 {
		sys := &System{ctx: t.Context()}
		sys.client.Store(&ssh.Client{Conn: closedConn{}})

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			session, err := sys.createSessionWithTimeout(time.Second)
			assert.Nil(t, session)
			assert.Error(t, err, "a closed connection must surface an error, not a session")
		}()
		go func() {
			defer wg.Done()
			sys.closeSSHConnection()
		}()
		wg.Wait()
	}
}
