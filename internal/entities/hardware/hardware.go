// Package hardware defines the hardware inventory shared by the agent and hub.
//
// The inventory describes what is installed or attached to a system. It is
// collected rarely (at agent start and on request), never polled with stats.
// Identifier fields (serial numbers, UUIDs, MAC addresses) are only populated
// when the agent is explicitly configured to collect them.
package hardware

// Inventory is the complete hardware description of one system.
type Inventory struct {
	// Collected is the Unix time (seconds) the inventory was gathered.
	Collected int64          `json:"collected" cbor:"0,keyasint"`
	System    System         `json:"system" cbor:"1,keyasint"`
	Board     Board          `json:"board" cbor:"2,keyasint"`
	BIOS      BIOS           `json:"bios" cbor:"3,keyasint"`
	Chassis   Chassis        `json:"chassis" cbor:"4,keyasint"`
	CPUs      []CPU          `json:"cpus,omitempty" cbor:"5,keyasint,omitempty"`
	Memory    []MemoryModule `json:"memory,omitempty" cbor:"6,keyasint,omitempty"`
	PCI       []PCIDevice    `json:"pci,omitempty" cbor:"7,keyasint,omitempty"`
	USB       []USBDevice    `json:"usb,omitempty" cbor:"8,keyasint,omitempty"`
	Disks     []Disk         `json:"disks,omitempty" cbor:"9,keyasint,omitempty"`
	NICs      []NIC          `json:"nics,omitempty" cbor:"10,keyasint,omitempty"`
	Displays  []Display      `json:"displays,omitempty" cbor:"11,keyasint,omitempty"`
	Security  Security       `json:"security" cbor:"13,keyasint"`
	Batteries []Battery      `json:"batteries,omitempty" cbor:"14,keyasint,omitempty"`
	// Unavailable lists sections that could not be collected and why.
	Unavailable []Gap `json:"unavailable,omitempty" cbor:"12,keyasint,omitempty"`
}

// Gap records a section that could not be collected.
type Gap struct {
	Section string `json:"section" cbor:"0,keyasint"`
	Reason  string `json:"reason" cbor:"1,keyasint"`
}

// System is the product as a whole (SMBIOS type 1).
type System struct {
	Vendor  string `json:"vendor,omitempty" cbor:"0,keyasint,omitempty"`
	Product string `json:"product,omitempty" cbor:"1,keyasint,omitempty"`
	Version string `json:"version,omitempty" cbor:"2,keyasint,omitempty"`
	Family  string `json:"family,omitempty" cbor:"3,keyasint,omitempty"`
	SKU     string `json:"sku,omitempty" cbor:"4,keyasint,omitempty"`
	Serial  string `json:"serial,omitempty" cbor:"5,keyasint,omitempty"` // identifier
	UUID    string `json:"uuid,omitempty" cbor:"6,keyasint,omitempty"`   // identifier
}

// Board is the mainboard (SMBIOS type 2).
type Board struct {
	Vendor  string `json:"vendor,omitempty" cbor:"0,keyasint,omitempty"`
	Product string `json:"product,omitempty" cbor:"1,keyasint,omitempty"`
	Version string `json:"version,omitempty" cbor:"2,keyasint,omitempty"`
	Serial  string `json:"serial,omitempty" cbor:"3,keyasint,omitempty"` // identifier
}

// BIOS is the system firmware (SMBIOS type 0).
type BIOS struct {
	Vendor  string `json:"vendor,omitempty" cbor:"0,keyasint,omitempty"`
	Version string `json:"version,omitempty" cbor:"1,keyasint,omitempty"`
	Date    string `json:"date,omitempty" cbor:"2,keyasint,omitempty"`
}

// Chassis is the enclosure (SMBIOS type 3).
type Chassis struct {
	Vendor string `json:"vendor,omitempty" cbor:"0,keyasint,omitempty"`
	Type   string `json:"type,omitempty" cbor:"1,keyasint,omitempty"`
	Serial string `json:"serial,omitempty" cbor:"2,keyasint,omitempty"` // identifier
}

// CPU is one processor package.
type CPU struct {
	Socket    string `json:"socket,omitempty" cbor:"0,keyasint,omitempty"`
	Vendor    string `json:"vendor,omitempty" cbor:"1,keyasint,omitempty"`
	Model     string `json:"model,omitempty" cbor:"2,keyasint,omitempty"`
	Cores     int    `json:"cores,omitempty" cbor:"3,keyasint,omitempty"`
	Threads   int    `json:"threads,omitempty" cbor:"4,keyasint,omitempty"`
	Microcode string `json:"microcode,omitempty" cbor:"5,keyasint,omitempty"`
}

// MemoryModule is one memory slot (SMBIOS type 17); empty slots have SizeMB 0.
type MemoryModule struct {
	Locator       string `json:"locator,omitempty" cbor:"0,keyasint,omitempty"`
	Bank          string `json:"bank,omitempty" cbor:"1,keyasint,omitempty"`
	SizeMB        uint64 `json:"size_mb" cbor:"2,keyasint"`
	Type          string `json:"type,omitempty" cbor:"3,keyasint,omitempty"`
	FormFactor    string `json:"form_factor,omitempty" cbor:"4,keyasint,omitempty"`
	SpeedMTs      int    `json:"speed_mts,omitempty" cbor:"5,keyasint,omitempty"`
	ConfiguredMTs int    `json:"configured_mts,omitempty" cbor:"6,keyasint,omitempty"`
	Manufacturer  string `json:"manufacturer,omitempty" cbor:"7,keyasint,omitempty"`
	PartNumber    string `json:"part_number,omitempty" cbor:"8,keyasint,omitempty"`
	Serial        string `json:"serial,omitempty" cbor:"9,keyasint,omitempty"` // identifier
}

// PCIDevice is one PCI function. IDs are lowercase hex without prefix.
type PCIDevice struct {
	Address         string `json:"address" cbor:"0,keyasint"`
	Class           string `json:"class" cbor:"1,keyasint"`
	Vendor          string `json:"vendor" cbor:"2,keyasint"`
	Device          string `json:"device" cbor:"3,keyasint"`
	SubsystemVendor string `json:"subsystem_vendor,omitempty" cbor:"4,keyasint,omitempty"`
	SubsystemDevice string `json:"subsystem_device,omitempty" cbor:"5,keyasint,omitempty"`
	Driver          string `json:"driver,omitempty" cbor:"6,keyasint,omitempty"`
	// Names are filled in by the hub from pci.ids; agents never send them.
	VendorName string `json:"vendor_name,omitempty" cbor:"-"`
	DeviceName string `json:"device_name,omitempty" cbor:"-"`
	ClassName  string `json:"class_name,omitempty" cbor:"-"`
}

// USBDevice is one attached USB device (hubs included).
type USBDevice struct {
	Bus          int      `json:"bus" cbor:"0,keyasint"`
	Port         string   `json:"port" cbor:"1,keyasint"`
	Vendor       string   `json:"vendor" cbor:"2,keyasint"`
	Product      string   `json:"product" cbor:"3,keyasint"`
	Manufacturer string   `json:"manufacturer,omitempty" cbor:"4,keyasint,omitempty"`
	Name         string   `json:"name,omitempty" cbor:"5,keyasint,omitempty"`
	SpeedMbps    string   `json:"speed_mbps,omitempty" cbor:"6,keyasint,omitempty"`
	Kinds        []string `json:"kinds,omitempty" cbor:"7,keyasint,omitempty"`  // e.g. keyboard, mouse, storage
	Serial       string   `json:"serial,omitempty" cbor:"8,keyasint,omitempty"` // identifier
	// Names are filled in by the hub from usb.ids; agents never send them.
	VendorName  string `json:"vendor_name,omitempty" cbor:"-"`
	ProductName string `json:"product_name,omitempty" cbor:"-"`
}

// Disk is one physical block device.
type Disk struct {
	Name       string `json:"name" cbor:"0,keyasint"`
	Model      string `json:"model,omitempty" cbor:"1,keyasint,omitempty"`
	Vendor     string `json:"vendor,omitempty" cbor:"2,keyasint,omitempty"`
	SizeBytes  uint64 `json:"size_bytes" cbor:"3,keyasint"`
	Rotational bool   `json:"rotational,omitempty" cbor:"4,keyasint,omitempty"`
	Transport  string `json:"transport,omitempty" cbor:"5,keyasint,omitempty"`
	Firmware   string `json:"firmware,omitempty" cbor:"6,keyasint,omitempty"`
	Serial     string `json:"serial,omitempty" cbor:"7,keyasint,omitempty"` // identifier
}

// NIC is one physical network interface.
type NIC struct {
	Name      string `json:"name" cbor:"0,keyasint"`
	Driver    string `json:"driver,omitempty" cbor:"1,keyasint,omitempty"`
	PCI       string `json:"pci,omitempty" cbor:"2,keyasint,omitempty"`
	SpeedMbps int    `json:"speed_mbps,omitempty" cbor:"3,keyasint,omitempty"`
	MAC       string `json:"mac,omitempty" cbor:"4,keyasint,omitempty"` // identifier
}

// Display is one connected monitor, from its EDID.
type Display struct {
	Connector    string `json:"connector" cbor:"0,keyasint"`
	Manufacturer string `json:"manufacturer,omitempty" cbor:"1,keyasint,omitempty"` // PNP ID, e.g. DEL
	ProductCode  string `json:"product_code,omitempty" cbor:"2,keyasint,omitempty"`
	Name         string `json:"name,omitempty" cbor:"3,keyasint,omitempty"`
	Year         int    `json:"year,omitempty" cbor:"4,keyasint,omitempty"`
	WidthCM      int    `json:"width_cm,omitempty" cbor:"5,keyasint,omitempty"`
	HeightCM     int    `json:"height_cm,omitempty" cbor:"6,keyasint,omitempty"`
	Serial       string `json:"serial,omitempty" cbor:"7,keyasint,omitempty"` // identifier
}
