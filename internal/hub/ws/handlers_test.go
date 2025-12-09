//go:build testing

package ws

import (
	"testing"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/systemd"
	"github.com/stretchr/testify/assert"
)

func TestSystemdInfoHandlerSuccess(t *testing.T) {
	handler := &systemdInfoHandler{
		result: &systemd.ServiceDetails{},
	}

	// Test successful handling with valid ServiceInfo
	testDetails := systemd.ServiceDetails{
		"Id":             "nginx.service",
		"ActiveState":    "active",
		"SubState":       "running",
		"Description":    "A high performance web server",
		"ExecMainPID":    1234,
		"MemoryCurrent":  1024000,
	}

	response := common.AgentResponse{
		ServiceInfo: testDetails,
	}

	err := handler.Handle(response)
	assert.NoError(t, err)
	assert.Equal(t, testDetails, *handler.result)
}

func TestSystemdInfoHandlerError(t *testing.T) {
	handler := &systemdInfoHandler{
		result: &systemd.ServiceDetails{},
	}

	// Test error handling when ServiceInfo is nil
	response := common.AgentResponse{
		ServiceInfo: nil,
		Error:       "service not found",
	}

	err := handler.Handle(response)
	assert.Error(t, err)
	assert.Equal(t, "no systemd info in response", err.Error())
}

func TestSystemdInfoHandlerEmptyResponse(t *testing.T) {
	handler := &systemdInfoHandler{
		result: &systemd.ServiceDetails{},
	}

	// Test with completely empty response
	response := common.AgentResponse{}

	err := handler.Handle(response)
	assert.Error(t, err)
	assert.Equal(t, "no systemd info in response", err.Error())
}

func TestSystemdInfoHandlerLegacyNotSupported(t *testing.T) {
	handler := &systemdInfoHandler{
		result: &systemd.ServiceDetails{},
	}

	// Test that legacy format is not supported
	err := handler.HandleLegacy([]byte("some data"))
	assert.Error(t, err)
	assert.Equal(t, "legacy format not supported", err.Error())
}
