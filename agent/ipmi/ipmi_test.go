//go:build testing

package ipmi

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sdrSpec describes a full sensor record for tests.
type sdrSpec struct {
	id         uint16
	owner      uint8
	lun        uint8
	number     uint8
	sensorType uint8
	eventType  uint8
	units1     uint8
	unit       uint8
	linear     uint8
	m, b       int
	rExp, bExp int
	name       string
	idType     uint8 // defaults to 8-bit ASCII
}

func tempSDR(id uint16, number uint8, name string) sdrSpec {
	return sdrSpec{id: id, number: number, sensorType: sensorTypeTemperature, unit: unitDegreesC, m: 1, name: name}
}

func fanSDR(id uint16, number uint8, name string) sdrSpec {
	return sdrSpec{id: id, number: number, sensorType: sensorTypeFan, unit: unitRPM, m: 100, name: name}
}

func (s sdrSpec) bytes() []byte {
	owner := s.owner
	if owner == 0 {
		owner = bmcSlaveAddr
	}
	eventType := s.eventType
	if eventType == 0 {
		eventType = eventTypeThreshold
	}
	idType := s.idType
	if idType == 0 {
		idType = 3
	}
	rec := make([]byte, 48+len(s.name))
	rec[0], rec[1] = byte(s.id), byte(s.id>>8)
	rec[2] = 0x51
	rec[3] = sdrTypeFull
	rec[4] = byte(len(rec) - sdrHeaderLen)
	rec[5] = owner
	rec[6] = s.lun
	rec[7] = s.number
	rec[12] = s.sensorType
	rec[13] = eventType
	rec[20] = s.units1
	rec[21] = s.unit
	rec[23] = s.linear
	m, b := uint16(s.m)&0x3ff, uint16(s.b)&0x3ff
	rec[24], rec[25] = byte(m), byte(m>>8)<<6
	rec[26], rec[27] = byte(b), byte(b>>8)<<6
	rec[29] = byte(s.rExp&0x0f)<<4 | byte(s.bExp&0x0f)
	rec[47] = idType<<6 | byte(len(s.name))
	copy(rec[48:], s.name)
	return rec
}

type sensorValue struct {
	raw, flags uint8
}

// fakeBMC implements transport over an in-memory SDR repository.
type fakeBMC struct {
	mu        sync.Mutex
	records   [][]byte
	readings  map[uint8]sensorValue
	maxRead   int // if > 0, whole-record reads fail with 0xca and chunks are capped
	cancelAt  int // cancel the reservation on this Get SDR call (1-based), 0 = never
	failWith  error
	delay     time.Duration
	sdrCalls  int
	readCalls atomic.Int32
	lastLUN   uint8
}

func newFakeBMC(specs ...sdrSpec) *fakeBMC {
	f := &fakeBMC{readings: map[uint8]sensorValue{}}
	for _, s := range specs {
		f.records = append(f.records, s.bytes())
	}
	return f
}

func (f *fakeBMC) close() error { return nil }

func (f *fakeBMC) send(netFn, cmd, lun uint8, data []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.failWith != nil {
		return nil, f.failWith
	}
	cc := func(code uint8) error { return &CompletionCodeError{NetFn: netFn, Cmd: cmd, Code: code} }
	switch {
	case netFn == netFnStorage && cmd == cmdReserveSDRRepo:
		return []byte{0x01, 0x00}, nil
	case netFn == netFnStorage && cmd == cmdGetSDR:
		f.sdrCalls++
		if f.sdrCalls == f.cancelAt {
			return nil, cc(ccReservationCanceled)
		}
		id := uint16(data[2]) | uint16(data[3])<<8
		offset, n := int(data[4]), int(data[5])
		idx := 0
		if id != 0 {
			idx = -1
			for i, rec := range f.records {
				if uint16(rec[0])|uint16(rec[1])<<8 == id {
					idx = i
				}
			}
			if idx < 0 {
				return nil, cc(0xcb)
			}
		}
		rec := f.records[idx]
		next := uint16(lastSDRRecordID)
		if idx+1 < len(f.records) {
			next = uint16(f.records[idx+1][0]) | uint16(f.records[idx+1][1])<<8
		}
		if f.maxRead > 0 && n > f.maxRead {
			return nil, cc(ccCannotReturnBytes)
		}
		end := min(offset+n, len(rec))
		return append([]byte{byte(next), byte(next >> 8)}, rec[offset:end]...), nil
	case netFn == netFnSensor && cmd == cmdGetSensorReading:
		f.readCalls.Add(1)
		f.lastLUN = lun
		v, ok := f.readings[data[0]]
		if !ok {
			return nil, cc(0xcb)
		}
		return []byte{v.raw, v.flags, 0x00}, nil
	}
	return nil, cc(0xc1)
}

const scanning = 0x40 // sensor scanning enabled, reading available

func TestConvert(t *testing.T) {
	tests := []struct {
		name string
		spec sdrSpec
		raw  uint8
		want float64
	}{
		{"linear celsius", tempSDR(1, 1, "t"), 45, 45},
		{"fan multiplier", fanSDR(1, 1, "f"), 116, 11600},
		{"twos complement", sdrSpec{sensorType: sensorTypeTemperature, unit: unitDegreesC, units1: 2 << 6, m: 1}, 0xf6, -10},
		{"ones complement", sdrSpec{sensorType: sensorTypeTemperature, unit: unitDegreesC, units1: 1 << 6, m: 1}, 0xfe, -1},
		{"result exponent", sdrSpec{sensorType: sensorTypeTemperature, unit: unitDegreesC, m: 5, rExp: -1}, 100, 50},
		{"offset with exponent", sdrSpec{sensorType: sensorTypeTemperature, unit: unitDegreesC, m: 1, b: 5, bExp: 1}, 10, 60},
		{"negative M", sdrSpec{sensorType: sensorTypeTemperature, unit: unitDegreesC, m: -1, b: 100}, 30, 70},
		{"negative B", sdrSpec{sensorType: sensorTypeTemperature, unit: unitDegreesC, m: 1, b: -20}, 50, 30},
		{"fahrenheit", sdrSpec{sensorType: sensorTypeTemperature, unit: unitDegreesF, m: 1}, 212, 100},
		{"kelvin", sdrSpec{sensorType: sensorTypeTemperature, unit: unitKelvin, m: 1, b: 273}, 30, 29.85},
		{"inverse linearization", sdrSpec{sensorType: sensorTypeFan, unit: unitRPM, m: 1, linear: 7, rExp: -6}, 100, 10000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, ok := parseFullSDR(tt.spec.bytes())
			require.True(t, ok)
			got, ok := s.convert(tt.raw)
			require.True(t, ok)
			assert.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

func TestLinearizeRejectsInvalid(t *testing.T) {
	_, ok := linearize(7, 0) // 1/0
	assert.False(t, ok)
	_, ok = linearize(1, -1) // ln(-1)
	assert.False(t, ok)
	_, ok = linearize(0x70, 1) // non-linear
	assert.False(t, ok)
}

func TestParseFullSDR(t *testing.T) {
	s, ok := parseFullSDR(sdrSpec{number: 7, lun: 2, sensorType: sensorTypeTemperature, unit: unitDegreesC, m: 1, name: " Board Temp \x00"}.bytes())
	require.True(t, ok)
	assert.Equal(t, "Board Temp", s.name)
	assert.Equal(t, uint8(7), s.number)
	assert.Equal(t, uint8(2), s.lun)
	assert.Equal(t, KindTemperature, s.kind)

	s, ok = parseFullSDR(sdrSpec{number: 9, sensorType: sensorTypeFan, unit: unitRPM, m: 1, name: "Fan\xe9"}.bytes())
	require.True(t, ok)
	assert.Equal(t, "Fané", s.name, "8-bit ID strings are Latin-1")
	assert.Equal(t, KindFan, s.kind)

	s, ok = parseFullSDR(sdrSpec{number: 0x2a, sensorType: sensorTypeTemperature, unit: unitDegreesC, m: 1, name: "XY", idType: 2}.bytes())
	require.True(t, ok)
	assert.Equal(t, "sensor_42", s.name, "unsupported ID string encodings get a generated name")

	rejected := map[string]sdrSpec{
		"voltage":       {sensorType: 0x02, unit: 4, m: 1, name: "v"},
		"fan percent":   {sensorType: sensorTypeFan, unit: unitRPM, units1: 0x01, m: 1, name: "f"},
		"fan duty unit": {sensorType: sensorTypeFan, unit: 0, m: 1, name: "f"},
		"no analog":     {sensorType: sensorTypeTemperature, unit: unitDegreesC, units1: 3 << 6, m: 1, name: "t"},
		"discrete":      {sensorType: sensorTypeTemperature, unit: unitDegreesC, eventType: 0x6f, m: 1, name: "t"},
		"other owner":   {owner: 0x2c, sensorType: sensorTypeTemperature, unit: unitDegreesC, m: 1, name: "t"},
		"non-linear":    {sensorType: sensorTypeTemperature, unit: unitDegreesC, linear: 0x70, m: 1, name: "t"},
	}
	for name, spec := range rejected {
		_, ok := parseFullSDR(spec.bytes())
		assert.False(t, ok, name)
	}

	compact := tempSDR(1, 1, "t").bytes()
	compact[3] = 0x02
	_, ok = parseFullSDR(compact)
	assert.False(t, ok, "compact records have no conversion factors")
	_, ok = parseFullSDR(compact[:20])
	assert.False(t, ok, "truncated record")
}

func TestReadSDRs(t *testing.T) {
	specs := []sdrSpec{
		tempSDR(0x10, 1, "CPU Temp"),
		{id: 0x11, number: 2, sensorType: 0x02, unit: 4, m: 1, name: "12V"},
		fanSDR(0x12, 3, "Fan 1"),
		tempSDR(0x13, 4, "CPU Temp"),
	}
	tests := []struct {
		name     string
		maxRead  int
		cancelAt int
	}{
		{name: "whole records"},
		{name: "chunked records", maxRead: sdrChunkLen},
		{name: "reservation canceled", maxRead: sdrChunkLen, cancelAt: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bmc := newFakeBMC(specs...)
			bmc.maxRead, bmc.cancelAt = tt.maxRead, tt.cancelAt
			sensors, err := readSDRs(bmc)
			require.NoError(t, err)
			names := make([]string, 0, len(sensors))
			for _, s := range sensors {
				names = append(names, s.name)
			}
			assert.Equal(t, []string{"CPU Temp", "Fan 1", "CPU Temp_4"}, names)
		})
	}
}

func TestReadSDRsError(t *testing.T) {
	bmc := newFakeBMC(tempSDR(1, 1, "t"))
	bmc.failWith = errors.New("device gone")
	_, err := readSDRs(bmc)
	assert.ErrorContains(t, err, "device gone")
}

func TestCollectorGet(t *testing.T) {
	bmc := newFakeBMC(
		tempSDR(1, 1, "Inlet Temp"),
		tempSDR(2, 2, "Empty Slot Temp"),
		tempSDR(3, 3, "Disabled Temp"),
		tempSDR(4, 4, "Absent Temp"),
		fanSDR(5, 5, "Fan 1"),
		sdrSpec{id: 6, number: 6, lun: 1, sensorType: sensorTypeTemperature, unit: unitDegreesC, m: 1, name: "LUN1 Temp"},
	)
	bmc.readings[1] = sensorValue{raw: 24, flags: scanning}
	bmc.readings[2] = sensorValue{raw: 0, flags: scanning | 0x20} // reading unavailable
	bmc.readings[3] = sensorValue{raw: 40, flags: 0x00}           // scanning disabled
	bmc.readings[5] = sensorValue{raw: 72, flags: scanning}
	bmc.readings[6] = sensorValue{raw: 31, flags: scanning}

	c := newCollector(bmc)
	r := c.Get(time.Second)
	assert.Equal(t, []Reading{{"Inlet Temp", 24}, {"LUN1 Temp", 31}}, r.Temperatures)
	assert.Equal(t, []Reading{{"Fan 1", 7200}}, r.Fans)
	assert.False(t, r.Time.IsZero())
	assert.Equal(t, uint8(1), bmc.lastLUN, "readings are requested from the sensor's owner LUN")

	// Within the minimum interval readings come from the cache.
	calls := bmc.readCalls.Load()
	assert.Equal(t, r, c.Get(time.Second))
	assert.Equal(t, calls, bmc.readCalls.Load())

	// Once stale, the BMC is polled again without rereading the SDR repository.
	sdrCalls := bmc.sdrCalls
	c.mu.Lock()
	c.latest.Time = time.Now().Add(-c.minInterval)
	c.lastAttempt = time.Time{}
	c.mu.Unlock()
	bmc.mu.Lock()
	bmc.readings[1] = sensorValue{raw: 27, flags: scanning}
	bmc.mu.Unlock()
	r = c.Get(time.Second)
	assert.Equal(t, []Reading{{"Inlet Temp", 27}, {"LUN1 Temp", 31}}, r.Temperatures)
	assert.Equal(t, sdrCalls, bmc.sdrCalls)
}

func TestCollectorGetSlowBMC(t *testing.T) {
	bmc := newFakeBMC(tempSDR(1, 1, "Inlet Temp"))
	bmc.readings[1] = sensorValue{raw: 24, flags: scanning}
	bmc.delay = 20 * time.Millisecond

	c := newCollector(bmc)
	assert.Empty(t, c.Get(time.Millisecond).Temperatures, "a slow first poll returns nothing instead of blocking")
	assert.Eventually(t, func() bool {
		return len(c.Get(0).Temperatures) == 1
	}, time.Second, 10*time.Millisecond)
}

func TestCollectorGetError(t *testing.T) {
	bmc := newFakeBMC(tempSDR(1, 1, "Inlet Temp"))
	bmc.failWith = errors.New("device gone")
	c := newCollector(bmc)
	assert.Equal(t, Readings{}, c.Get(time.Second))

	// A failing BMC is not retried until the minimum interval has passed.
	bmc.mu.Lock()
	bmc.failWith = nil
	bmc.readings[1] = sensorValue{raw: 24, flags: scanning}
	bmc.mu.Unlock()
	assert.Equal(t, Readings{}, c.Get(time.Second))

	c.mu.Lock()
	c.lastAttempt = time.Time{}
	c.mu.Unlock()
	assert.Equal(t, []Reading{{"Inlet Temp", 24}}, c.Get(time.Second).Temperatures)
}

func TestCollectorDropsOldReadings(t *testing.T) {
	bmc := newFakeBMC(tempSDR(1, 1, "Inlet Temp"))
	bmc.readings[1] = sensorValue{raw: 24, flags: scanning}
	c := newCollector(bmc)
	require.Len(t, c.Get(time.Second).Temperatures, 1)

	// Readings older than maxReadingAge are not reported if a refresh fails.
	c.mu.Lock()
	c.latest.Time = time.Now().Add(-maxReadingAge(c.minInterval) - time.Second)
	c.lastAttempt = time.Time{}
	c.mu.Unlock()
	bmc.mu.Lock()
	bmc.failWith = errors.New("device gone")
	bmc.mu.Unlock()
	assert.Equal(t, Readings{}, c.Get(time.Second))
}

func TestSignExtend(t *testing.T) {
	assert.Equal(t, int32(-1), signExtend(0x3ff, 10))
	assert.Equal(t, int32(511), signExtend(0x1ff, 10))
	assert.Equal(t, int32(-8), signExtend(0x8, 4))
	assert.Equal(t, int32(7), signExtend(0x7, 4))
}
