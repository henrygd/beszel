package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/henrygd/beszel/agent/hardware"
	"github.com/spf13/pflag"
)

// handleInventory prints the hardware inventory this agent would report, so
// operators can see exactly what is collected before enabling it.
func handleInventory(args []string) {
	flags := pflag.NewFlagSet("inventory", pflag.ExitOnError)
	identifiers := flags.Bool("identifiers", false, "Include serial numbers, UUIDs and MAC addresses")
	smbios := flags.String("smbios", "", "Path of a raw SMBIOS table (default: systemd credential, then sysfs)")
	_ = flags.Parse(args)

	inv := hardware.Collect(hardware.Options{SMBIOS: *smbios, Identifiers: *identifiers})
	out, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
