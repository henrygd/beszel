package agent

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

func TestAmdRc2IdentifySize(t *testing.T) {
	if got := unsafe.Sizeof(amdRc2Identify{}); got != 256 {
		t.Fatalf("sizeof(amdRc2Identify) = %d, want 256 to match AMD_RC2_IDENTIFY", got)
	}
}

func TestAmdRaidIndex(t *testing.T) {
	for name, want := range map[string]int{"amdraid0": 0, "amdraid12": 12} {
		if got, ok := amdRaidIndex(name); !ok || got != want {
			t.Errorf("amdRaidIndex(%q) = %d, %v", name, got, ok)
		}
	}
	for _, name := range []string{"/dev/sda", "amdraid", "amdraid-1", "amdraidx"} {
		if _, ok := amdRaidIndex(name); ok {
			t.Errorf("amdRaidIndex(%q) should fail", name)
		}
	}
}

func TestParseAmdRc2Nvme(t *testing.T) {
	buf := make([]byte, 512)
	binary.LittleEndian.PutUint16(buf[1:], 273+41) // 41 C
	buf[3], buf[4], buf[5] = 100, 10, 3
	binary.LittleEndian.PutUint64(buf[128:], 1234) // power on hours
	binary.LittleEndian.PutUint64(buf[160:], 2)    // media errors

	temp, passed, attrs := parseAmdRc2Nvme(buf)
	if temp != 41 || !passed {
		t.Fatalf("temp=%d passed=%v", temp, passed)
	}
	got := map[string]uint64{}
	for _, a := range attrs {
		got[a.Name] = a.RawValue
	}
	if got["PowerOnHours"] != 1234 || got["MediaErrors"] != 2 || got["PercentageUsed"] != 3 {
		t.Fatalf("unexpected attrs: %v", got)
	}

	buf[0] = 0x04 // reliability degraded
	if _, passed, _ = parseAmdRc2Nvme(buf); passed {
		t.Fatal("critical warning should fail health")
	}
}

func TestParseAmdRc2Ata(t *testing.T) {
	data := make([]byte, 512)
	thr := make([]byte, 512)
	entry := func(i int, id, value uint8, raw uint64) {
		off := 2 + i*12
		data[off], data[off+3], data[off+4] = id, value, value
		for b := range 6 {
			data[off+5+b] = byte(raw >> (8 * b))
		}
	}
	entry(0, 5, 100, 0)
	entry(1, 9, 99, 0x0102030405)
	entry(2, 194, 64, 36)
	thr[2], thr[3] = 5, 10

	temp, passed, attrs := parseAmdRc2Ata(data, thr)
	if temp != 36 || !passed || len(attrs) != 3 {
		t.Fatalf("temp=%d passed=%v attrs=%d", temp, passed, len(attrs))
	}
	if attrs[1].Name != "Power_On_Hours" || attrs[1].RawValue != 0x0102030405 {
		t.Fatalf("bad attr: %+v", attrs[1])
	}
	if attrs[0].Threshold != 10 {
		t.Fatalf("threshold not applied: %+v", attrs[0])
	}

	data[2+3] = 10 // value drops to threshold
	if _, passed, attrs = parseAmdRc2Ata(data, thr); passed || attrs[0].WhenFailed != "now" {
		t.Fatal("attribute at threshold should fail health")
	}
}
