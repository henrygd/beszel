package agent

import (
	"testing"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatherStatsDoesNotAttachDetailsToCachedRequests(t *testing.T) {
	agent := &Agent{
		cache:         NewSystemDataCache(),
		systemDetails: system.Details{Hostname: "updated-host", Podman: true},
		detailsDirty:  true,
	}
	cached := &system.CombinedData{
		Info: system.Info{Hostname: "cached-host"},
	}
	agent.cache.Set(cached, defaultDataCacheTimeMs)

	response := agent.gatherStats(common.DataRequestOptions{CacheTimeMs: defaultDataCacheTimeMs})

	assert.Same(t, cached, response)
	assert.Nil(t, response.Details)
	assert.True(t, agent.detailsDirty)
	assert.Equal(t, "cached-host", response.Info.Hostname)
	assert.Nil(t, cached.Details)

	secondResponse := agent.gatherStats(common.DataRequestOptions{CacheTimeMs: defaultDataCacheTimeMs})
	assert.Same(t, cached, secondResponse)
	assert.Nil(t, secondResponse.Details)
}

func TestUpdateSystemDetailsMarksDetailsDirty(t *testing.T) {
	agent := &Agent{}

	agent.updateSystemDetails(func(details *system.Details) {
		details.Hostname = "updated-host"
		details.Podman = true
	})

	assert.True(t, agent.detailsDirty)
	assert.Equal(t, "updated-host", agent.systemDetails.Hostname)
	assert.True(t, agent.systemDetails.Podman)

	original := &system.CombinedData{}
	realTimeResponse := agent.attachSystemDetails(original, 1000, true)
	assert.Same(t, original, realTimeResponse)
	assert.Nil(t, realTimeResponse.Details)
	assert.True(t, agent.detailsDirty)

	response := agent.attachSystemDetails(original, defaultDataCacheTimeMs, false)
	require.NotNil(t, response.Details)
	assert.NotSame(t, original, response)
	assert.Equal(t, "updated-host", response.Details.Hostname)
	assert.True(t, response.Details.Podman)
	assert.False(t, agent.detailsDirty)
	assert.Nil(t, original.Details)
}
