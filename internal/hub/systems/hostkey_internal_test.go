//go:build testing

package systems

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	_ "github.com/henrygd/beszel/internal/migrations"
)

// fakeHubStub satisfies the hubLike interface on top of a plain PocketBase
// test app; the host key path only uses the embedded core.App.
type fakeHubStub struct {
	core.App
}

func (f *fakeHubStub) GetSSHKey(string) (ssh.Signer, error) { return nil, errors.New("unused") }
func (f *fakeHubStub) HandleSystemAlerts(*core.Record, *system.CombinedData) error {
	return nil
}
func (f *fakeHubStub) HandleNetworkMonitorAlerts(*core.Record, map[string]monitor.Result) error {
	return nil
}
func (f *fakeHubStub) HandleStatusAlerts(string, *core.Record) error     { return nil }
func (f *fakeHubStub) HandleContainerAlerts(*core.Record, *system.CombinedData, func(string) (string, error)) error {
	return nil
}
func (f *fakeHubStub) CancelPendingStatusAlerts(string)    {}
func (f *fakeHubStub) CancelPendingContainerAlerts(string) {}

// newTOFUSystem builds a system record (with the given agent version in its
// persisted info), a fingerprint record, and a *System wired to a manager
// backed by the test app — everything verifyAgentHostKey touches.
var tofuTestSeq int

func newTOFUSystem(t *testing.T, app core.App, agentVersion string) *System {
	t.Helper()
	tofuTestSeq++

	usersCol, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(usersCol)
	user.Set("email", fmt.Sprintf("tofu-%d@test.local", tofuTestSeq))
	user.Set("password", "1234567890")
	user.Set("role", "user")
	require.NoError(t, app.Save(user))

	systemsCol, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	systemRecord := core.NewRecord(systemsCol)
	systemRecord.Set("name", "tofu-test")
	systemRecord.Set("host", "127.0.0.1")
	systemRecord.Set("port", "45999")
	systemRecord.Set("users", []string{user.Id})
	require.NoError(t, app.Save(systemRecord))

	info, err := json.Marshal(&system.Info{AgentVersion: agentVersion})
	require.NoError(t, err)
	systemRecord.Set("info", info)
	require.NoError(t, app.Save(systemRecord))

	fpCol, err := app.FindCollectionByNameOrId("fingerprints")
	require.NoError(t, err)
	fpRecord := core.NewRecord(fpCol)
	fpRecord.Set("system", systemRecord.Id)
	fpRecord.Set("token", "test-token")
	fpRecord.Set("fingerprint", "abc123def4567890")
	require.NoError(t, app.Save(fpRecord))

	manager := NewSystemManager(&fakeHubStub{App: app})
	sys := manager.NewSystem(systemRecord.Id)
	sys.manager = manager
	return sys
}

func tofuKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return sshPub
}

// TestVerifyAgentHostKey pins the TOFU behavior of the hub's SSH dial path:
// a key is learned once from agents that persist one (>= MinVersionStableHostKey),
// later connections must present the same key, and legacy agents (ephemeral
// keys) are exempt from pinning entirely.
func TestVerifyAgentHostKey(t *testing.T) {
	app, err := pbTests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()

	t.Run("stable agent key is learned then enforced", func(t *testing.T) {
		sys := newTOFUSystem(t, app, "0.22.0")
		key1 := tofuKey(t)

		// First contact: accept and record.
		require.NoError(t, sys.verifyAgentHostKey(key1))
		fpRecord, err := app.FindFirstRecordByFilter("fingerprints", "system = {:system}", map[string]any{"system": sys.Id})
		require.NoError(t, err)
		require.NotEmpty(t, fpRecord.GetString("host_key"), "key must be recorded on first contact")

		// Same key: accept.
		require.NoError(t, sys.verifyAgentHostKey(key1))

		// Different key: reject — this is the MITM stop.
		err = sys.verifyAgentHostKey(tofuKey(t))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host key mismatch")
	})

	t.Run("legacy agent key is not pinned", func(t *testing.T) {
		sys := newTOFUSystem(t, app, "0.12.0")
		key := tofuKey(t)

		require.NoError(t, sys.verifyAgentHostKey(key))
		fpRecord, err := app.FindFirstRecordByFilter("fingerprints", "system = {:system}", map[string]any{"system": sys.Id})
		require.NoError(t, err)
		assert.Empty(t, fpRecord.GetString("host_key"),
			"ephemeral-key agents must not poison the record")

		// A legacy agent changing its key stays accepted (previous behavior).
		assert.NoError(t, sys.verifyAgentHostKey(tofuKey(t)))
	})

	t.Run("unknown agent version is not pinned", func(t *testing.T) {
		sys := newTOFUSystem(t, app, "")
		key := tofuKey(t)

		require.NoError(t, sys.verifyAgentHostKey(key))
		fpRecord, err := app.FindFirstRecordByFilter("fingerprints", "system = {:system}", map[string]any{"system": sys.Id})
		require.NoError(t, err)
		assert.Empty(t, fpRecord.GetString("host_key"))
	})
}
