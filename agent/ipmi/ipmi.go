// Package ipmi reads temperature and fan sensors from the local BMC through
// the in-band IPMI system interface (/dev/ipmi0 on Linux).
//
// The sensor data record (SDR) repository is read once and cached; each poll
// then issues one Get Sensor Reading command per temperature or fan sensor.
// Only threshold sensors with full SDRs owned by the BMC are supported, which
// covers the temperature and fan sensors exposed by common server boards.
package ipmi

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// IPMI network functions and commands used by this package.
const (
	netFnSensor  = 0x04
	netFnStorage = 0x0a

	cmdGetSensorReading = 0x2d
	cmdReserveSDRRepo   = 0x22
	cmdGetSDR           = 0x23

	bmcSlaveAddr = 0x20
)

// Completion codes with special handling.
const (
	ccOK                  = 0x00
	ccReservationCanceled = 0xc5
	ccCannotReturnBytes   = 0xca
)

// SDR record types and sensor fields.
const (
	sdrTypeFull = 0x01

	sensorTypeTemperature = 0x01
	sensorTypeFan         = 0x04

	eventTypeThreshold = 0x01

	unitDegreesC = 1
	unitDegreesF = 2
	unitKelvin   = 3
	unitRPM      = 18

	sdrHeaderLen    = 5
	sdrChunkLen     = 16
	lastSDRRecordID = 0xffff
	// maxSDRRecords bounds the repository walk in case a BMC returns a cycle.
	maxSDRRecords = 4096
)

// Kind is the kind of value a sensor reports.
type Kind uint8

const (
	KindTemperature Kind = iota + 1
	KindFan
)

// Reading is a converted sensor value: degrees Celsius for temperatures and
// RPM for fans.
type Reading struct {
	Name  string
	Value float64
}

// Readings is the result of one poll of the BMC.
type Readings struct {
	Temperatures []Reading
	Fans         []Reading
	Time         time.Time
}

// CompletionCodeError is a non-zero IPMI completion code returned by the BMC.
type CompletionCodeError struct {
	NetFn, Cmd, Code uint8
}

func (e *CompletionCodeError) Error() string {
	return fmt.Sprintf("ipmi netfn 0x%02x cmd 0x%02x: completion code 0x%02x", e.NetFn, e.Cmd, e.Code)
}

func isCompletionCode(err error, code uint8) bool {
	var ccErr *CompletionCodeError
	return errors.As(err, &ccErr) && ccErr.Code == code
}

// transport sends a request to the BMC and returns the response data after
// the completion code. A non-zero completion code is a *CompletionCodeError.
type transport interface {
	send(netFn, cmd, lun uint8, data []byte) ([]byte, error)
	close() error
}

// sensor is a temperature or fan sensor parsed from a full SDR.
type sensor struct {
	name        string
	kind        Kind
	number      uint8
	lun         uint8
	unit        uint8
	format      uint8 // analog data format: 0 unsigned, 1 one's complement, 2 two's complement
	linear      uint8
	m, b        int32
	rExp, bExp  int32
	unavailable bool // last read had no value
}

// parseFullSDR parses a full sensor record (header included). ok is false for
// records that are not BMC-owned threshold temperature or fan sensors with an
// analog reading this package can convert.
func parseFullSDR(rec []byte) (s sensor, ok bool) {
	// The ID string type/length byte sits at offset 47.
	if len(rec) < 48 || rec[3] != sdrTypeFull {
		return s, false
	}
	if rec[5] != bmcSlaveAddr || rec[13] != eventTypeThreshold {
		return s, false
	}
	units1 := rec[20]
	s.format = units1 >> 6
	if s.format == 3 || units1&0x01 != 0 { // no analog reading, or percentage
		return s, false
	}
	s.unit = rec[21]
	switch {
	case rec[12] == sensorTypeTemperature && (s.unit == unitDegreesC || s.unit == unitDegreesF || s.unit == unitKelvin):
		s.kind = KindTemperature
	case rec[12] == sensorTypeFan && s.unit == unitRPM:
		s.kind = KindFan
	default:
		return s, false
	}
	s.linear = rec[23] & 0x7f
	if s.linear > 0x0b { // non-linear sensors need per-reading factors
		return s, false
	}
	s.lun = rec[6] & 0x03
	s.number = rec[7]
	s.m = signExtend(uint32(rec[24])|uint32(rec[25]&0xc0)<<2, 10)
	s.b = signExtend(uint32(rec[26])|uint32(rec[27]&0xc0)<<2, 10)
	s.rExp = signExtend(uint32(rec[29]>>4), 4)
	s.bExp = signExtend(uint32(rec[29]&0x0f), 4)
	s.name = parseIDString(rec[47], rec[48:])
	if s.name == "" {
		s.name = "sensor_" + strconv.Itoa(int(s.number))
	}
	return s, true
}

// parseIDString decodes an SDR ID string. Only the 8-bit ASCII/Latin-1 form
// is common in practice; other encodings fall back to a generated name.
func parseIDString(typeLen byte, data []byte) string {
	if typeLen>>6 != 3 {
		return ""
	}
	n := min(int(typeLen&0x1f), len(data))
	runes := make([]rune, 0, n)
	for _, c := range data[:n] {
		if c == 0 {
			break
		}
		runes = append(runes, rune(c)) // Latin-1 maps directly to code points
	}
	return strings.TrimSpace(string(runes))
}

func signExtend(v uint32, bits uint) int32 {
	shift := 32 - bits
	return int32(v<<shift) >> shift
}

// convert applies the SDR conversion formula y = L[(M*x + B*10^K1) * 10^K2]
// and returns the value in degrees Celsius or RPM.
func (s *sensor) convert(raw uint8) (float64, bool) {
	var x float64
	switch s.format {
	case 1:
		x = float64(signExtend(uint32(raw), 8))
		if raw&0x80 != 0 {
			x++ // one's complement
		}
	case 2:
		x = float64(int8(raw))
	default:
		x = float64(raw)
	}
	y := (float64(s.m)*x + float64(s.b)*math.Pow10(int(s.bExp))) * math.Pow10(int(s.rExp))
	y, ok := linearize(s.linear, y)
	if !ok {
		return 0, false
	}
	switch s.unit {
	case unitDegreesF:
		y = (y - 32) * 5 / 9
	case unitKelvin:
		y -= 273.15
	}
	return y, true
}

func linearize(l uint8, y float64) (float64, bool) {
	switch l {
	case 0:
	case 1:
		y = math.Log(y)
	case 2:
		y = math.Log10(y)
	case 3:
		y = math.Log2(y)
	case 4:
		y = math.Exp(y)
	case 5:
		y = math.Pow(10, y)
	case 6:
		y = math.Exp2(y)
	case 7:
		y = 1 / y
	case 8:
		y = y * y
	case 9:
		y = y * y * y
	case 10:
		y = math.Sqrt(y)
	case 11:
		y = math.Cbrt(y)
	default:
		return 0, false
	}
	return y, !math.IsNaN(y) && !math.IsInf(y, 0)
}

// readSDRs walks the SDR repository and returns its temperature and fan sensors.
func readSDRs(t transport) ([]sensor, error) {
	r := sdrReader{t: t, readLen: 0xff}
	if err := r.reserve(); err != nil {
		return nil, err
	}
	var sensors []sensor
	names := make(map[string]int)
	id := uint16(0)
	for range maxSDRRecords {
		rec, next, err := r.record(id)
		if err != nil {
			return nil, fmt.Errorf("reading SDR 0x%04x: %w", id, err)
		}
		if s, ok := parseFullSDR(rec); ok {
			// BMC names are not guaranteed unique; suffix duplicates with the sensor number.
			if names[s.name]++; names[s.name] > 1 {
				s.name += "_" + strconv.Itoa(int(s.number))
			}
			sensors = append(sensors, s)
		}
		if next == lastSDRRecordID || next == id {
			return sensors, nil
		}
		id = next
	}
	return sensors, nil
}

type sdrReader struct {
	t           transport
	reservation [2]byte
	readLen     uint8 // bytes per Get SDR request; drops to sdrChunkLen if the BMC refuses whole records
}

func (r *sdrReader) reserve() error {
	resp, err := r.t.send(netFnStorage, cmdReserveSDRRepo, 0, nil)
	if err != nil {
		return err
	}
	if len(resp) < 2 {
		return errors.New("short reserve SDR repository response")
	}
	copy(r.reservation[:], resp)
	return nil
}

// record reads one SDR, retrying when another client cancels our reservation.
func (r *sdrReader) record(id uint16) ([]byte, uint16, error) {
	for attempt := 0; ; attempt++ {
		rec, next, err := r.readRecord(id)
		if attempt < 3 && isCompletionCode(err, ccReservationCanceled) {
			if err := r.reserve(); err != nil {
				return nil, 0, err
			}
			continue
		}
		return rec, next, err
	}
}

func (r *sdrReader) readRecord(id uint16) ([]byte, uint16, error) {
	if r.readLen == 0xff {
		data, next, err := r.getSDR(id, 0, 0xff)
		if err == nil && len(data) >= sdrHeaderLen && len(data) >= sdrHeaderLen+int(data[4]) {
			return data[:sdrHeaderLen+int(data[4])], next, nil
		}
		if err != nil && !isCompletionCode(err, ccCannotReturnBytes) && !isCompletionCode(err, 0xff) {
			return nil, 0, err
		}
		r.readLen = sdrChunkLen
	}
	header, next, err := r.getSDR(id, 0, sdrHeaderLen)
	if err != nil {
		return nil, 0, err
	}
	if len(header) < sdrHeaderLen {
		return nil, 0, errors.New("short SDR header")
	}
	rec := append(make([]byte, 0, sdrHeaderLen+int(header[4])), header[:sdrHeaderLen]...)
	for len(rec) < cap(rec) {
		n := min(int(r.readLen), cap(rec)-len(rec))
		chunk, _, err := r.getSDR(id, uint8(len(rec)), uint8(n))
		if err != nil {
			return nil, 0, err
		}
		if len(chunk) == 0 {
			return nil, 0, errors.New("empty SDR chunk")
		}
		rec = append(rec, chunk[:min(len(chunk), n)]...)
	}
	return rec, next, nil
}

func (r *sdrReader) getSDR(id uint16, offset, n uint8) ([]byte, uint16, error) {
	req := []byte{r.reservation[0], r.reservation[1], byte(id), byte(id >> 8), offset, n}
	resp, err := r.t.send(netFnStorage, cmdGetSDR, 0, req)
	if err != nil {
		return nil, 0, err
	}
	if len(resp) < 2 {
		return nil, 0, errors.New("short get SDR response")
	}
	return resp[2:], uint16(resp[0]) | uint16(resp[1])<<8, nil
}

// readSensor returns the converted value of s, or ok=false when the BMC has no
// reading for it (absent device, scanning disabled, or reading unavailable).
func readSensor(t transport, s *sensor) (value float64, ok bool, err error) {
	resp, err := t.send(netFnSensor, cmdGetSensorReading, s.lun, []byte{s.number})
	if err != nil {
		var ccErr *CompletionCodeError
		if errors.As(err, &ccErr) {
			return 0, false, nil // e.g. 0xcb sensor not present
		}
		return 0, false, err
	}
	if len(resp) < 2 {
		return 0, false, nil
	}
	flags := resp[1]
	if flags&0x20 != 0 || flags&0x40 == 0 { // reading unavailable, or scanning disabled
		return 0, false, nil
	}
	value, ok = s.convert(resp[0])
	return value, ok, nil
}

// Collector polls the BMC and caches the latest readings.
type Collector struct {
	t           transport
	minInterval time.Duration

	mu          sync.Mutex
	latest      Readings
	lastAttempt time.Time
	done        chan struct{} // closed when the in-flight refresh finishes
	inflight    atomic.Bool

	// Only touched by the goroutine holding inflight.
	sensors []sensor
	loaded  bool
	failing bool
}

// DefaultMinInterval is how long readings are reused before the BMC is polled
// again. Agents may collect several times per second in real-time mode, while
// BMCs typically refresh their own sensors every few seconds.
const DefaultMinInterval = 5 * time.Second

func newCollector(t transport) *Collector {
	return &Collector{t: t, minInterval: DefaultMinInterval}
}

// Get returns the latest readings, starting a refresh if they are older than
// the collector's minimum interval and waiting up to wait for it to finish. A
// slow BMC never blocks the caller for longer than wait; the previous readings
// are returned instead, or empty readings if they are too old to be useful.
func (c *Collector) Get(wait time.Duration) Readings {
	c.mu.Lock()
	latest := c.latest
	stale := time.Since(latest.Time) >= c.minInterval
	done := c.done
	// lastAttempt rate-limits retries when the BMC keeps failing.
	if stale && time.Since(c.lastAttempt) >= c.minInterval && c.inflight.CompareAndSwap(false, true) {
		c.lastAttempt = time.Now()
		done = make(chan struct{})
		c.done = done
		go c.refresh(done)
	}
	c.mu.Unlock()

	if done != nil && stale {
		select {
		case <-done:
			c.mu.Lock()
			latest = c.latest
			c.mu.Unlock()
		case <-time.After(wait):
		}
	}
	if time.Since(latest.Time) > maxReadingAge(c.minInterval) {
		return Readings{}
	}
	return latest
}

func maxReadingAge(minInterval time.Duration) time.Duration {
	return max(2*time.Minute, 3*minInterval)
}

func (c *Collector) refresh(done chan struct{}) {
	defer func() {
		c.inflight.Store(false)
		close(done)
	}()
	readings, err := c.poll()
	if err != nil {
		if !c.failing {
			slog.Warn("Error reading IPMI sensors", "err", err)
		}
		c.failing = true
		return
	}
	if c.failing {
		slog.Info("IPMI sensors readable again")
		c.failing = false
	}
	c.mu.Lock()
	c.latest = readings
	c.mu.Unlock()
}

// poll reads every cached sensor. It is only called from one goroutine at a
// time, guarded by inflight.
func (c *Collector) poll() (Readings, error) {
	if !c.loaded {
		sensors, err := readSDRs(c.t)
		if err != nil {
			return Readings{}, err
		}
		c.sensors, c.loaded = sensors, true
	}
	var r Readings
	for i := range c.sensors {
		s := &c.sensors[i]
		value, ok, err := readSensor(c.t, s)
		if err != nil {
			return Readings{}, err
		}
		s.unavailable = !ok
		if !ok {
			continue
		}
		switch s.kind {
		case KindTemperature:
			r.Temperatures = append(r.Temperatures, Reading{s.name, value})
		case KindFan:
			r.Fans = append(r.Fans, Reading{s.name, value})
		}
	}
	r.Time = time.Now()
	return r, nil
}

// Close releases the IPMI device.
func (c *Collector) Close() error {
	return c.t.close()
}
