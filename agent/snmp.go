package agent

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

// isIfPhysAddressOID returns true for ifPhysAddress (.1.3.6.1.2.1.2.2.1.6)
func isIfPhysAddressOID(oid string) bool {
	return strings.HasPrefix(oid, ".1.3.6.1.2.1.2.2.1.6") ||
		strings.HasPrefix(oid, "1.3.6.1.2.1.2.2.1.6")
}

// formatMAC converts a 6-byte slice to aa:bb:cc:dd:ee:ff
func formatMAC(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if len(b) == 6 { // common MAC length
		parts := make([]string, 6)
		for i := 0; i < 6; i++ {
			parts[i] = fmt.Sprintf("%02x", b[i])
		}
		return strings.Join(parts, ":")
	}
	// Fallback: hex string
	return hex.EncodeToString(b)
}

// PrettyValue returns a Go string you can print safely for a varbind.
func SNMP_PrettyValue(vb gosnmp.SnmpPDU) string {
	switch vb.Type {
	case gosnmp.OctetString:
		b, _ := vb.Value.([]byte)
		if isIfPhysAddressOID(vb.Name) {
			return formatMAC(b)
		}
		// Heuristic: printable ASCII? else hex
		printable := true
		for _, c := range b {
			if c < 0x09 || (c > 0x0d && c < 0x20) || c > 0x7e {
				printable = false
				break
			}
		}
		if printable {
			return string(b)
		}
		return hex.EncodeToString(b)

	case gosnmp.Integer:
		fallthrough
	case gosnmp.Counter32:
		fallthrough
	case gosnmp.Gauge32:
		fallthrough
	case gosnmp.TimeTicks:
		fallthrough
	case gosnmp.Uinteger32:
		fallthrough
	case gosnmp.Counter64:
		return gosnmp.ToBigInt(vb.Value).String()
		// return toBig(vb.Value).String()

	case gosnmp.IPAddress:
		// gosnmp usually gives string for IP
		if s, ok := vb.Value.(string); ok {
			return s
		}
		// some agents may return []byte
		if b, ok := vb.Value.([]byte); ok && len(b) == 4 {
			return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
		}
		return fmt.Sprintf("%v", vb.Value)

	case gosnmp.ObjectIdentifier:
		if s, ok := vb.Value.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", vb.Value)

	default:
		// Fallback for Null, NoSuchInstance, EndOfMibView, etc.
		return fmt.Sprintf("%v", vb.Value)
	}
}

type InterfacesFmt struct {
	Name        string
	ActualMTU   string
	MACAddress  string
	AdminStatus string
	OperStatus  string
	BytesIn     string
	PktsIn      string
	DiscardsIn  string
	ErrorsIn    string
	BytesOut    string
	PktsOut     string
	DiscardsOut string
	ErrorsOut   string
	Map         map[string]string
}
type SystemFmt struct {
	Model string
	Name  string
	Map   map[string]string
}

type Dictionary struct {
	Interfaces InterfacesFmt
	System     SystemFmt
}

func SNMP_OID() Dictionary {
	iface := InterfacesFmt{
		Name:        ".1.3.6.1.2.1.2.2.1.2.%v",
		ActualMTU:   ".1.3.6.1.2.1.2.2.1.4.%v",
		MACAddress:  ".1.3.6.1.2.1.2.2.1.6.%v",
		AdminStatus: ".1.3.6.1.2.1.2.2.1.7.%v",
		OperStatus:  ".1.3.6.1.2.1.2.2.1.8.%v",
		BytesIn:     ".1.3.6.1.2.1.31.1.1.1.6.%v",
		PktsIn:      ".1.3.6.1.2.1.31.1.1.1.7.%v",
		DiscardsIn:  ".1.3.6.1.2.1.2.2.1.13.%v",
		ErrorsIn:    ".1.3.6.1.2.1.2.2.1.14.%v",
		BytesOut:    ".1.3.6.1.2.1.31.1.1.1.10.%v",
		PktsOut:     ".1.3.6.1.2.1.31.1.1.1.11.%v",
		DiscardsOut: ".1.3.6.1.2.1.2.2.1.19.%v",
		ErrorsOut:   ".1.3.6.1.2.1.2.2.1.20.%v",
	}

	// SNMP_OIDHelper(OIDHelper.Interfaces.Name, 1),     // name
	// SNMP_OIDHelper(OIDHelper.Interfaces.BytesIn, 1),  // bytesRecv
	// SNMP_OIDHelper(OIDHelper.Interfaces.BytesOut, 1), // bytesSent
	// SNMP_OIDHelper(OIDHelper.Interfaces.PktsIn, 1),   // packetsRecv
	// SNMP_OIDHelper(OIDHelper.Interfaces.PktsOut, 1),  // packetsSent
	iface.Map = map[string]string{
		"name":         iface.Name,
		"actual-mtu":   iface.ActualMTU,
		"mac-address":  iface.MACAddress,
		"admin-status": iface.AdminStatus,
		"oper-status":  iface.OperStatus,
		"bytesRecv":    iface.BytesIn,  // Real Value: bytes-in
		"bytesSent":    iface.BytesOut, // Real Value: bytes-out
		"discards-in":  iface.DiscardsIn,
		"errors-in":    iface.ErrorsIn,
		"packetsRecv":  iface.PktsIn,  // Real Value: packets-in
		"packetsSent":  iface.PktsOut, // Real Value: packets-out
		"discards-out": iface.DiscardsOut,
		"errors-out":   iface.ErrorsOut,
	}

	system := SystemFmt{
		Model: ".1.3.6.1.2.1.1.1.0",
		Name:  ".1.3.6.1.2.1.1.5.0",
	}

	system.Map = map[string]string{
		"model": system.Model,
		"name":  system.Name,
	}

	return Dictionary{Interfaces: iface, System: system}
}

// oid formats a %d-based template with an index.
func SNMP_OIDHelper(tmpl string, idx int) string { return fmt.Sprintf(tmpl, idx) }

// ReverseLookup takes an OID like ".1.3.6.1.2.1.2.2.1.8.1"
// And returns e.g.: "oper-status", 1, true
func SNMP_ReverseOID(dict Dictionary, oid string) string {
	for key, tmpl := range dict.Interfaces.Map {
		// get prefix before %d
		prefix := strings.Split(tmpl, "%v")[0]
		if strings.HasPrefix(oid, prefix) {
			return key
		}
	}

	return "unknown"
}

func SNMP_Call(c *SNMPNetworkIO, oids []string) *gosnmp.SnmpPacket {
	result, err := c.client.Get(oids)
	if err != nil {
		slog.Error("Get() err: ", "err", err)
	}
	return result
}

func SNMP_getInterfaces(c *SNMPNetworkIO) []int {
	Interfaces := []int{}
	err := c.client.Walk(".1.3.6.1.2.1.2.2.1.1", func(pdu gosnmp.SnmpPDU) error {
		idx := gosnmp.ToBigInt(pdu.Value).Int64()
		Interfaces = append(Interfaces, int(idx))
		return nil
	})
	if err != nil {
		slog.Error("Walk() err: ", "err", err)
	}
	return Interfaces

}

func toUint64(s string) uint64 {
	// return gosnmp.ToBigInt(s).Uint64()
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0 // or handle differently
	}
	return v
}

// NetworkIOProvider abstracts network interface stats collection.
type NetworkIOProvider interface {
	IOCounters() ([]psutilNet.IOCountersStat, error)
}

// localNetworkIO reads from /proc via gopsutil (default).
type localNetworkIO struct{}

// snmpNetworkIO reads interface stats from a remote SNMP target.
type SNMPNetworkIO struct {
	client *gosnmp.GoSNMP
}

func SNMP_NetworkIO(target string) (*SNMPNetworkIO, error) {
	community, _ := GetEnv("SNMP_COMMUNITY")
	if community == "" {
		community = "public"
	}
	port, _ := GetEnv("SNMP_PORT")
	if port == "" {
		port = "161"
	}
	version, _ := GetEnv("SNMP_VERSION")
	snmpVersion := gosnmp.Version2c
	switch version {
	case "1":
		snmpVersion = gosnmp.Version1
	case "2c":
		snmpVersion = gosnmp.Version2c
	case "3":
		snmpVersion = gosnmp.Version3
	}
	thisPort, _ := strconv.ParseUint(port, 10, 16)
	client := &gosnmp.GoSNMP{
		Target:    target,
		Port:      uint16(thisPort),
		Community: community,
		Version:   snmpVersion,
		Timeout:   2 * time.Second,
	}
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("snmp connect: %w", err)
	}
	return &SNMPNetworkIO{client: client}, nil
}
func (l *localNetworkIO) IOCounters() ([]psutilNet.IOCountersStat, error) {
	return psutilNet.IOCounters(true)
}
func (c *SNMPNetworkIO) IOCounters() ([]psutilNet.IOCountersStat, error) {
	// walk IF-MIB,
	OIDHelper := SNMP_OID()

	// // Get System Name
	// // oids := []string{"1.3.6.1.2.1.1.5.0"}
	// Sample Input
	// {"name":"utun0","bytesSent":525428373,"bytesRecv":12412570192,"packetsSent":2225169,"packetsRecv":10483780,"errin":0,"errout":0,"dropin":0,"dropout":0,"fifoin":0,"fifoout":0}

	InterfacesResult := []psutilNet.IOCountersStat{}
	// Discover Interfaces
	Interfaces := SNMP_getInterfaces(c)

	for _, pos := range Interfaces {
		oids := []string{
			SNMP_OIDHelper(OIDHelper.Interfaces.Name, pos),     // name
			SNMP_OIDHelper(OIDHelper.Interfaces.BytesIn, pos),  // bytesRecv
			SNMP_OIDHelper(OIDHelper.Interfaces.BytesOut, pos), // bytesSent
			SNMP_OIDHelper(OIDHelper.Interfaces.PktsIn, pos),   // packetsRecv
			SNMP_OIDHelper(OIDHelper.Interfaces.PktsOut, pos),  // packetsSent
		}
		result := SNMP_Call(c, oids)
		thisInterface := psutilNet.IOCountersStat{}
		for _, vb := range result.Variables {

			key := SNMP_ReverseOID(OIDHelper, vb.Name)
			val := SNMP_PrettyValue(vb)
			switch key {
			case "name":
				thisInterface.Name = val
			case "bytesRecv":
				thisInterface.BytesRecv = toUint64(val)
			case "bytesSent":
				thisInterface.BytesSent = toUint64(val)
			case "packetsRecv":
				thisInterface.PacketsRecv = toUint64(val)
			case "packetsSent":
				thisInterface.PacketsSent = toUint64(val)
			}
		}
		InterfacesResult = append(InterfacesResult, thisInterface)
	}
	// for _, v := range InterfacesResult {
	// 	fmt.Println("_", v)
	// }
	return InterfacesResult, nil
}
