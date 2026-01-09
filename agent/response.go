package agent

import (
	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/systemd"
)

// newAgentResponse creates an AgentResponse using legacy typed fields.
// This maintains backward compatibility with <= 0.17 hubs that expect specific fields.
func newAgentResponse(data any, requestID *uint32) common.AgentResponse {
	response := common.AgentResponse{Id: requestID}
	switch v := data.(type) {
	case *system.CombinedData:
		response.SystemData = v
	case *common.FingerprintResponse:
		response.Fingerprint = v
	case string:
		response.String = &v
	case map[string]smart.SmartData:
		response.SmartData = v
	case systemd.ServiceDetails:
		response.ServiceInfo = v
	default:
		// For unknown types, use the generic Data field
		response.Data, _ = cbor.Marshal(data)
	}
	return response
}
