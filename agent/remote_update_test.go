//go:build testing

package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setRemoteUpdateDirs(t *testing.T) (unitDir, triggerDir string) {
	t.Helper()
	unitDir, triggerDir = t.TempDir(), t.TempDir()
	origUnit, origTrigger := remoteUpdateUnitDir, remoteUpdateTriggerDir
	remoteUpdateUnitDir, remoteUpdateTriggerDir = unitDir, triggerDir
	t.Cleanup(func() {
		remoteUpdateUnitDir, remoteUpdateTriggerDir = origUnit, origTrigger
	})
	return unitDir, triggerDir
}

func TestRequestRemoteUpdate(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("remote update is linux only")
	}
	unitDir, triggerDir := setRemoteUpdateDirs(t)
	trigger := filepath.Join(triggerDir, remoteUpdateTriggerName)

	// not enabled: no path unit, no trigger written
	assert.False(t, remoteUpdateEnabled())
	err := requestRemoteUpdate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote-update enable")
	assert.NoFileExists(t, trigger)

	// enabled: trigger file written
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, remoteUpdateUnitName+".path"), []byte(remoteUpdatePathContent()), 0o644))
	assert.True(t, remoteUpdateEnabled())
	require.NoError(t, requestRemoteUpdate())
	assert.FileExists(t, trigger)
}

func TestRequestAgentUpdateHandler(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("remote update is linux only")
	}
	unitDir, _ := setRemoteUpdateDirs(t)
	var sent any
	hctx := &HandlerContext{
		Agent: &Agent{},
		SendResponse: func(data any, _ *uint32) error {
			sent = data
			return nil
		},
	}
	handler := &RequestAgentUpdateHandler{}

	require.Error(t, handler.Handle(hctx))
	assert.Nil(t, sent)

	require.NoError(t, os.WriteFile(filepath.Join(unitDir, remoteUpdateUnitName+".path"), nil, 0o644))
	require.NoError(t, handler.Handle(hctx))
	assert.Equal(t, true, sent)
}

func TestRemoteUpdateUnitContent(t *testing.T) {
	_, triggerDir := setRemoteUpdateDirs(t)
	trigger := filepath.Join(triggerDir, remoteUpdateTriggerName)

	service := remoteUpdateServiceContent("/opt/beszel-agent/beszel-agent", false)
	assert.Contains(t, service, "ExecStartPre=/bin/rm -f "+trigger+"\n")
	assert.Contains(t, service, "ExecStart=/opt/beszel-agent/beszel-agent update\n")
	assert.Contains(t, service, "Type=oneshot")

	service = remoteUpdateServiceContent("/opt/beszel-agent/beszel-agent", true)
	assert.Contains(t, service, "ExecStart=/opt/beszel-agent/beszel-agent update --china-mirrors\n")

	path := remoteUpdatePathContent()
	assert.Contains(t, path, "PathExists="+trigger+"\n")
	assert.Contains(t, path, "Unit="+remoteUpdateUnitName+".service\n")
	assert.Contains(t, path, "WantedBy=multi-user.target")
}

func TestRemoteUpdateCommandValidation(t *testing.T) {
	setRemoteUpdateDirs(t)
	assert.NoError(t, RemoteUpdateCommand("status", false))
	assert.NoError(t, RemoteUpdateCommand("", false))
	assert.ErrorContains(t, RemoteUpdateCommand("bogus", false), "unknown remote-update action")
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		assert.ErrorContains(t, RemoteUpdateCommand("enable", false), "must be run as root")
	}
}
