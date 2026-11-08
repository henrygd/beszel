package agent

import (
	"errors"

	"github.com/henrygd/beszel/agent/hardware"
	"github.com/henrygd/beszel/agent/utils"
)

// hardwareInventoryOptions returns the collection options when the operator set
// HARDWARE_INVENTORY=true, or nil (the default: no inventory is reported).
//
//	HARDWARE_INVENTORY_IDENTIFIERS=true  also report serial numbers, UUIDs and MACs
//	HARDWARE_INVENTORY_SMBIOS=<path>     read the SMBIOS table from this file, e.g. a
//	                                     read-only bind mount in a container, where
//	                                     /sys/firmware is hidden
func hardwareInventoryOptions() *hardware.Options {
	if enabled, _ := utils.GetEnv("HARDWARE_INVENTORY"); enabled != "true" {
		return nil
	}
	identifiers, _ := utils.GetEnv("HARDWARE_INVENTORY_IDENTIFIERS")
	smbios, _ := utils.GetEnv("HARDWARE_INVENTORY_SMBIOS")
	return &hardware.Options{Identifiers: identifiers == "true", SMBIOS: smbios}
}

// GetHardwareInventoryHandler collects the hardware inventory on request. The
// request carries no parameters: what is read is fixed by the agent's own
// configuration, never by the hub.
type GetHardwareInventoryHandler struct{}

func (h *GetHardwareInventoryHandler) Handle(hctx *HandlerContext) error {
	if hctx.Agent.hardwareInventory == nil {
		return errors.New("hardware inventory is disabled on this agent (set HARDWARE_INVENTORY=true)")
	}
	return hctx.SendResponse(hardware.Collect(*hctx.Agent.hardwareInventory), hctx.RequestID)
}
