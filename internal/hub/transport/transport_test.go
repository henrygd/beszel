package transport

import (
	"testing"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalSmartDataResponse(t *testing.T) {
	for _, test := range []struct {
		name     string
		complete bool
	}{
		{name: "complete response", complete: true},
		{name: "older agent defaults to incomplete", complete: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := common.AgentResponse{
				SmartData: map[string]smart.SmartData{
					"AAA": {SerialNumber: "AAA"},
				},
				SmartComplete: test.complete,
			}
			var result smart.SmartDataResponse
			require.NoError(t, UnmarshalResponse(response, common.GetSmartData, &result))
			assert.Equal(t, test.complete, result.Complete)
			assert.Equal(t, "AAA", result.Data["AAA"].SerialNumber)
		})
	}
}

func TestUnmarshalSystemdLogsResponse(t *testing.T) {
	logs := "2026-09-27T12:00:00+00:00 host nginx[1]: started"
	response := common.AgentResponse{String: &logs}

	var result string
	require.NoError(t, UnmarshalResponse(response, common.GetSystemdLogs, &result))
	assert.Equal(t, logs, result)
}
