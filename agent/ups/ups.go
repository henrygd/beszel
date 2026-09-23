// Package ups collects UPS metrics from apcupsd via its NIS (Network
// Information Service) socket.
package ups

import (
	"bufio"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

// DefaultAddr is the default apcupsd NIS socket address.
const DefaultAddr = "localhost:3551"

// GetStats queries apcupsd at addr (host:port) and returns UPS data keyed by
// UPS name. It returns an error if apcupsd is unreachable or returns no data.
func GetStats(addr string) (map[string]system.UpsData, error) {
	if addr == "" {
		addr = DefaultAddr
	}
	vars, err := query(addr)
	if err != nil {
		return nil, err
	}
	return parse(vars), nil
}

// query connects to the apcupsd NIS socket, requests all variables, and reads
// them into a map. apcupsd keeps the connection open after responding, so a
// read deadline is used to stop reading once the variables have been sent.
func query(addr string) (map[string]string, error) {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte("VAR ALL\n")); err != nil {
		return nil, err
	}

	vars := make(map[string]string)
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		vars[key] = value
	}
	// A read deadline is expected (apcupsd holds the connection open), so the
	// scanner error is only meaningful when no data was received at all.
	if len(vars) == 0 {
		return nil, errors.New("apcupsd returned no data")
	}
	return vars, nil
}

// parse converts raw apcupsd variables into UPS data. The UPS is keyed by its
// UPSNAME, falling back to MODEL, then a generic name.
func parse(vars map[string]string) map[string]system.UpsData {
	data := system.UpsData{
		Model:      vars["MODEL"],
		Status:     vars["STATUS"],
		OnBattery:  strings.Contains(vars["STATUS"], "OB"),
		BatteryPct: parseFloat(vars["BATT_CAPACITY"]),
		LoadPct:    parseFloat(vars["LOADPCT"]),
		InputV:     parseFloat(vars["INPUTV"]),
		OutputV:    parseFloat(vars["OUTPUTV"]),
		TimeLeft:   parseFloat(vars["TIMELEFT"]),
	}
	name := vars["UPSNAME"]
	if name == "" {
		name = vars["MODEL"]
	}
	if name == "" {
		name = "UPS"
	}
	return map[string]system.UpsData{name: data}
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}