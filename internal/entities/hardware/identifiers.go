package hardware

// StripIdentifiers removes every field that identifies a specific unit: serial
// numbers, UUIDs and MAC addresses. The agent applies it unless identifiers are
// enabled; the hub applies it again for users who are not admins.
func (inv *Inventory) StripIdentifiers() {
	inv.System.Serial, inv.System.UUID = "", ""
	inv.Board.Serial, inv.Chassis.Serial = "", ""
	for i := range inv.Memory {
		inv.Memory[i].Serial = ""
	}
	for i := range inv.USB {
		inv.USB[i].Serial = ""
	}
	for i := range inv.Disks {
		inv.Disks[i].Serial = ""
	}
	for i := range inv.NICs {
		inv.NICs[i].MAC = ""
	}
	for i := range inv.Displays {
		inv.Displays[i].Serial = ""
	}
	for i := range inv.Batteries {
		inv.Batteries[i].Serial = ""
	}
}
