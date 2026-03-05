package agent

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

// toBig converts any numeric SNMP value to *big.Int safely.
func toBig(v interface{}) *big.Int {
	switch x := v.(type) {
	case int:
		return big.NewInt(int64(x))
	case int8:
		return big.NewInt(int64(x))
	case int16:
		return big.NewInt(int64(x))
	case int32:
		return big.NewInt(int64(x))
	case int64:
		return big.NewInt(x)
	case uint:
		return new(big.Int).SetUint64(uint64(x))
	case uint8:
		return new(big.Int).SetUint64(uint64(x))
	case uint16:
		return new(big.Int).SetUint64(uint64(x))
	case uint32:
		return new(big.Int).SetUint64(uint64(x))
	case uint64:
		return new(big.Int).SetUint64(x)
	case *big.Int:
		return x
	default:
		// gosnmp provides helper: gosnmp.ToBigInt(v)
		return gosnmp.ToBigInt(v)
	}
}

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
func PrettyValue(vb gosnmp.SnmpPDU) string {
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
		return toBig(vb.Value).String()

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

func MikrotikOID() Dictionary {
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

	// MikrotikOIDHelper(OIDHelper.Interfaces.Name, 1),     // name
	// MikrotikOIDHelper(OIDHelper.Interfaces.BytesIn, 1),  // bytesRecv
	// MikrotikOIDHelper(OIDHelper.Interfaces.BytesOut, 1), // bytesSent
	// MikrotikOIDHelper(OIDHelper.Interfaces.PktsIn, 1),   // packetsRecv
	// MikrotikOIDHelper(OIDHelper.Interfaces.PktsOut, 1),  // packetsSent
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
func MikrotikOIDHelper(tmpl string, idx int) string { return fmt.Sprintf(tmpl, idx) }

// ReverseLookup takes an OID like ".1.3.6.1.2.1.2.2.1.8.1"
// And returns e.g.: "oper-status", 1, true
func MikrotikReverseOID(dict Dictionary, oid string) string {
	for key, tmpl := range dict.Interfaces.Map {
		// get prefix before %d
		prefix := strings.Split(tmpl, "%v")[0]

		if strings.HasPrefix(oid, prefix) {
			return key
		}
	}

	return "unknown"
}

func CheckIfMikrotik() bool {
	// OIDHelper := MikrotikOID()
	// Configure GoSNMP
	IP, exists := GetEnv("MIKROTIK_IP")
	if !exists {
		return false
	}
	gosnmp.Default.Target = IP // RouterOS IP
	gosnmp.Default.Community = "public"
	gosnmp.Default.Version = gosnmp.Version2c
	gosnmp.Default.Timeout = time.Duration(2) * time.Second

	err := gosnmp.Default.Connect()
	if err != nil {
		log.Fatalf("Connect() err: %v", err)
		return false
	}
	defer gosnmp.Default.Conn.Close()
	return true
}
func CallMikrotikSNMP(oids []string) *gosnmp.SnmpPacket {
	IP, exists := GetEnv("MIKROTIK_IP")
	if !exists {
		return nil
	}
	gosnmp.Default.Target = IP // RouterOS IP
	gosnmp.Default.Community = "public"
	gosnmp.Default.Version = gosnmp.Version2c
	gosnmp.Default.Timeout = time.Duration(2) * time.Second

	err := gosnmp.Default.Connect()
	if err != nil {
		log.Fatalf("Connect() err: %v", err)
	}
	defer gosnmp.Default.Conn.Close()
	result, err := gosnmp.Default.Get(oids)
	if err != nil {
		log.Fatalf("Get() err: %v", err)
	}
	return result
}

func getMikrotikInterfaces() []int {
	Interfaces := []int{}
	gosnmp.Default.Target = "192.168.10.2" // RouterOS IP
	gosnmp.Default.Community = "public"
	gosnmp.Default.Version = gosnmp.Version2c
	gosnmp.Default.Timeout = time.Duration(2) * time.Second

	err := gosnmp.Default.Connect()
	if err != nil {
		log.Fatalf("Connect() err: %v", err)
	}
	defer gosnmp.Default.Conn.Close()
	gosnmp.Default.Walk(".1.3.6.1.2.1.2.2.1.1", func(pdu gosnmp.SnmpPDU) error {
		idx := gosnmp.ToBigInt(pdu.Value).Int64()
		Interfaces = append(Interfaces, int(idx))
		return nil
	})
	return Interfaces

}
func GetMikrotikInterfacesStats() []psutilNet.IOCountersStat {
	OIDHelper := MikrotikOID()

	// // Get System Name
	// // oids := []string{"1.3.6.1.2.1.1.5.0"}
	// Sample Input
	// {"name":"utun0","bytesSent":525428373,"bytesRecv":12412570192,"packetsSent":2225169,"packetsRecv":10483780,"errin":0,"errout":0,"dropin":0,"dropout":0,"fifoin":0,"fifoout":0}

	InterfacesResult := []psutilNet.IOCountersStat{}
	// Discover Interfaces
	Interfaces := getMikrotikInterfaces()

	for _, pos := range Interfaces {
		oids := []string{
			MikrotikOIDHelper(OIDHelper.Interfaces.Name, pos),     // name
			MikrotikOIDHelper(OIDHelper.Interfaces.BytesIn, pos),  // bytesRecv
			MikrotikOIDHelper(OIDHelper.Interfaces.BytesOut, pos), // bytesSent
			MikrotikOIDHelper(OIDHelper.Interfaces.PktsIn, pos),   // packetsRecv
			MikrotikOIDHelper(OIDHelper.Interfaces.PktsOut, pos),  // packetsSent
		}
		result := CallMikrotikSNMP(oids)
		thisInterface := psutilNet.IOCountersStat{}
		for _, vb := range result.Variables {

			key := MikrotikReverseOID(OIDHelper, vb.Name)
			val := PrettyValue(vb)
			switch key {
			case "name":
				thisInterface.Name = val
			case "bytesRecv":
				thisInterface.BytesRecv = ToUint64(val)
			case "bytesSent":
				thisInterface.BytesSent = ToUint64(val)
			case "packetsRecv":
				thisInterface.PacketsRecv = ToUint64(val)
			case "packetsSent":
				thisInterface.PacketsSent = ToUint64(val)
			}
		}
		InterfacesResult = append(InterfacesResult, thisInterface)
	}
	// for _, v := range InterfacesResult {
	// 	fmt.Println("_", v)
	// }
	return InterfacesResult

}

func ToUint64(s string) uint64 {
	// return gosnmp.ToBigInt(s).Uint64()
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0 // or handle differently
	}
	return v
}
