package agent

import "testing"

func TestParseHexOrDecByte(t *testing.T) {
	tests := []struct {
		in   string
		want uint8
		ok   bool
	}{
		{"0x01", 1, true},
		{"0X0b", 11, true},
		{"01", 1, true},
		{" 3 ", 3, true},
		{"", 0, false},
		{"0x", 0, false},
		{"nope", 0, false},
	}

	for _, tt := range tests {
		got, ok := parseHexOrDecByte(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("parseHexOrDecByte(%q) = (%d,%v), want (%d,%v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParseHexBytePair(t *testing.T) {
	a, b, ok := parseHexBytePair("0x01 0x02\n")
	if !ok || a != 1 || b != 2 {
		t.Fatalf("parseHexBytePair hex = (%d,%d,%v), want (1,2,true)", a, b, ok)
	}

	a, b, ok = parseHexBytePair("01 02")
	if !ok || a != 1 || b != 2 {
		t.Fatalf("parseHexBytePair dec = (%d,%d,%v), want (1,2,true)", a, b, ok)
	}

	_, _, ok = parseHexBytePair("0x01")
	if ok {
		t.Fatalf("parseHexBytePair short input ok=true, want false")
	}
}

func TestEmmcSmartStatus(t *testing.T) {
	if got := emmcSmartStatus(0x01); got != "PASSED" {
		t.Fatalf("emmcSmartStatus(0x01) = %q, want PASSED", got)
	}
	if got := emmcSmartStatus(0x02); got != "WARNING" {
		t.Fatalf("emmcSmartStatus(0x02) = %q, want WARNING", got)
	}
	if got := emmcSmartStatus(0x03); got != "FAILED" {
		t.Fatalf("emmcSmartStatus(0x03) = %q, want FAILED", got)
	}
	if got := emmcSmartStatus(0x00); got != "UNKNOWN" {
		t.Fatalf("emmcSmartStatus(0x00) = %q, want UNKNOWN", got)
	}
}

func TestIsEmmcBlockName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"mmcblk0", true},
		{"mmcblk1", true},
		{"mmcblk10", true},
		{"mmcblk0p1", false},
		{"sda", false},
		{"mmcblk", false},
		{"mmcblkA", false},
	}
	for _, c := range cases {
		if got := isEmmcBlockName(c.name); got != c.ok {
			t.Fatalf("isEmmcBlockName(%q) = %v, want %v", c.name, got, c.ok)
		}
	}
}
