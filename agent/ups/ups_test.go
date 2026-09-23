package ups

import (
	"bufio"
	"net"
	"testing"
	"time"
)

// startFakeApccupsd starts a TCP server that responds to a command with the
// given lines and returns its address. If hold is true the connection is kept
// open after the response (like real apcupsd) to exercise the read deadline.
func startFakeApccupsd(t *testing.T, lines []string, hold bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				if _, err := bufio.NewReader(c).ReadString('\n'); err != nil {
					return
				}
				for _, line := range lines {
					if _, err := c.Write([]byte(line + "\n")); err != nil {
						return
					}
				}
				if hold {
					time.Sleep(3 * time.Second)
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func TestGetStats(t *testing.T) {
	addr := startFakeApccupsd(t, []string{
		"UPSNAME ups1",
		"MODEL Back-UPS 900",
		"STATUS OL",
		"BATT_CAPACITY 100.0",
		"LOADPCT 25.0",
		"INPUTV 120.0",
		"OUTPUTV 119.5",
		"TIMELEFT 0.0",
	}, false)

	data, err := GetStats(addr)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	ups, ok := data["ups1"]
	if !ok {
		t.Fatalf("expected UPS named ups1, got %v", data)
	}
	if ups.Model != "Back-UPS 900" {
		t.Errorf("model = %q, want %q", ups.Model, "Back-UPS 900")
	}
	if ups.BatteryPct != 100 {
		t.Errorf("battery = %v, want 100", ups.BatteryPct)
	}
	if ups.LoadPct != 25 {
		t.Errorf("load = %v, want 25", ups.LoadPct)
	}
	if ups.InputV != 120 {
		t.Errorf("input = %v, want 120", ups.InputV)
	}
	if ups.OutputV != 119.5 {
		t.Errorf("output = %v, want 119.5", ups.OutputV)
	}
	if ups.OnBattery {
		t.Errorf("onBattery = true, want false for OL status")
	}
}

func TestGetStatsOnBattery(t *testing.T) {
	addr := startFakeApccupsd(t, []string{
		"UPSNAME ups1",
		"STATUS OB",
		"BATT_CAPACITY 45.0",
		"TIMELEFT 12.5",
	}, true)

	data, err := GetStats(addr)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	ups := data["ups1"]
	if !ups.OnBattery {
		t.Errorf("onBattery = false, want true for OB status")
	}
	if ups.BatteryPct != 45 {
		t.Errorf("battery = %v, want 45", ups.BatteryPct)
	}
	if ups.TimeLeft != 12.5 {
		t.Errorf("timeleft = %v, want 12.5", ups.TimeLeft)
	}
}

func TestGetStatsNameFallback(t *testing.T) {
	addr := startFakeApccupsd(t, []string{
		"MODEL Back-UPS 900",
		"STATUS OL",
		"BATT_CAPACITY 90.0",
	}, false)

	data, err := GetStats(addr)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if _, ok := data["Back-UPS 900"]; !ok {
		t.Errorf("expected UPS keyed by model, got %v", data)
	}
}

func TestGetStatsUnreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	if _, err := GetStats(addr); err == nil {
		t.Errorf("expected error for unreachable apcupsd, got nil")
	}
}