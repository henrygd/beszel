package lvm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"strconv"
	"strings"
)

// On-disk PV layout (lib/format_text/layout.h, lib/label/label.h).
const (
	sectorSize     = 512
	labelScanBytes = 4 * sectorSize // the label lives in one of the first 4 sectors
	labelID        = "LABELONE"
	labelType      = "LVM2 001"
	labelCrcOffset = 20 // label crc covers offset_xl to the end of the sector
	pvUUIDLen      = 32
	diskLocnSize   = 16
	mdaHeaderSize  = 512
	mdaMagic       = " LVM2 x[5A%r0N*>"
	mdaVersion     = 1
	lvmInitialCrc  = 0xf597a6cf
	rawLocnIgnored = 1
	maxMetadata    = 16 << 20 // sanity bound on one metadata copy
)

// ErrInvalidPV is returned when a device has no valid LVM2 label or metadata.
var ErrInvalidPV = errors.New("invalid LVM physical volume")

// errNoMetadata is returned for PVs created with --metadatacopies 0.
var errNoMetadata = errors.New("physical volume has no metadata area")

// VolumeGroup is a VG's extent accounting from its on-disk metadata.
type VolumeGroup struct {
	UUID    string // without dashes
	Name    string
	Seqno   int64 // metadata revision; the highest copy wins
	Size    uint64
	Alloc   uint64 // bytes allocated to LVs
	Devices []Device
}

// lvmCrc is LVM's calc_crc: reflected CRC-32 (IEEE) without the pre- and
// post-inversion, seeded with lvmInitialCrc.
func lvmCrc(initial uint32, b []byte) uint32 {
	return ^crc32.Update(^initial, crc32.IEEETable, b)
}

// readPVMetadata returns the VG metadata text of the newest copy referenced
// by the PV's first usable metadata area.
func readPVMetadata(r io.ReaderAt) (string, error) {
	head := make([]byte, labelScanBytes)
	if _, err := r.ReadAt(head, 0); err != nil {
		return "", err
	}
	mdas, err := parseLabel(head)
	if err != nil {
		return "", err
	}
	for _, mda := range mdas {
		text, err := readMetadataArea(r, mda)
		if errors.Is(err, errNoMetadata) {
			continue
		}
		return text, err
	}
	return "", errNoMetadata
}

type diskLocn struct{ offset, size uint64 }

// parseLabel finds the label sector and returns the PV's metadata areas.
func parseLabel(head []byte) ([]diskLocn, error) {
	le := binary.LittleEndian
	for sector := 0; sector*sectorSize < len(head); sector++ {
		b := head[sector*sectorSize : (sector+1)*sectorSize]
		if string(b[:8]) != labelID || le.Uint64(b[8:]) != uint64(sector) {
			continue
		}
		if lvmCrc(lvmInitialCrc, b[labelCrcOffset:]) != le.Uint32(b[16:]) {
			return nil, fmt.Errorf("%w: label checksum mismatch", ErrInvalidPV)
		}
		if string(b[24:32]) != labelType {
			return nil, fmt.Errorf("%w: unsupported label type %q", ErrInvalidPV, b[24:32])
		}
		// pv_header: uuid, device size, then zero-terminated lists of data
		// areas and metadata areas.
		pos := int(le.Uint32(b[20:])) + pvUUIDLen + 8
		var lists [2][]diskLocn
		for i := range lists {
			for {
				if pos+diskLocnSize > len(b) {
					return nil, fmt.Errorf("%w: truncated pv header", ErrInvalidPV)
				}
				locn := diskLocn{le.Uint64(b[pos:]), le.Uint64(b[pos+8:])}
				pos += diskLocnSize
				if locn.offset == 0 {
					break
				}
				lists[i] = append(lists[i], locn)
			}
		}
		return lists[1], nil
	}
	return nil, fmt.Errorf("%w: no label", ErrInvalidPV)
}

// readMetadataArea reads the committed metadata text from a circular
// metadata area, following a wrap back to just after the header.
func readMetadataArea(r io.ReaderAt, mda diskLocn) (string, error) {
	le := binary.LittleEndian
	header := make([]byte, mdaHeaderSize)
	if _, err := r.ReadAt(header, int64(mda.offset)); err != nil {
		return "", err
	}
	if string(header[4:20]) != mdaMagic || le.Uint32(header[20:]) != mdaVersion {
		return "", fmt.Errorf("%w: bad metadata area header", ErrInvalidPV)
	}
	if lvmCrc(lvmInitialCrc, header[4:]) != le.Uint32(header) {
		return "", fmt.Errorf("%w: metadata area checksum mismatch", ErrInvalidPV)
	}
	start, size := le.Uint64(header[24:]), le.Uint64(header[32:])
	// raw_locn[0] is the committed copy; [1] is only set mid-commit.
	offset, length := le.Uint64(header[40:]), le.Uint64(header[48:])
	checksum, flags := le.Uint32(header[56:]), le.Uint32(header[60:])
	if length == 0 || flags&rawLocnIgnored != 0 {
		return "", errNoMetadata
	}
	if length > maxMetadata || offset >= size || offset < mdaHeaderSize || length > size-mdaHeaderSize {
		return "", fmt.Errorf("%w: metadata location out of range", ErrInvalidPV)
	}
	text := make([]byte, length)
	first := min(length, size-offset)
	if _, err := r.ReadAt(text[:first], int64(start+offset)); err != nil {
		return "", err
	}
	if first < length {
		if _, err := r.ReadAt(text[first:], int64(start+mdaHeaderSize)); err != nil {
			return "", err
		}
	}
	if lvmCrc(lvmInitialCrc, text) != checksum {
		return "", fmt.Errorf("%w: metadata checksum mismatch", ErrInvalidPV)
	}
	return strings.TrimRight(string(text), "\x00"), nil
}

// ParseVGMetadata extracts extent accounting from LVM2 text metadata.
func ParseVGMetadata(text string) (VolumeGroup, error) {
	root, err := parseConfig(text)
	if err != nil {
		return VolumeGroup{}, err
	}
	var vgSection *cfgSection
	for _, s := range root.sections {
		if _, ok := s.values["extent_size"]; ok {
			vgSection = s
			break
		}
	}
	if vgSection == nil {
		return VolumeGroup{}, errors.New("no volume group in metadata")
	}
	extentSize := uint64(vgSection.int("extent_size")) * sectorSize
	vg := VolumeGroup{
		UUID:  strings.ReplaceAll(vgSection.str("id"), "-", ""),
		Name:  vgSection.name,
		Seqno: vgSection.int("seqno"),
	}
	if extentSize == 0 || len(vg.UUID) != 32 {
		return VolumeGroup{}, errors.New("incomplete volume group metadata")
	}
	if pvs := vgSection.section("physical_volumes"); pvs != nil {
		for _, pv := range pvs.sections {
			vg.Size += uint64(pv.int("pe_count")) * extentSize
			name := pv.str("device")
			if name == "" {
				name = pv.name
			}
			state := "ONLINE"
			if pv.hasFlag("MISSING") {
				state = "MISSING"
			}
			vg.Devices = append(vg.Devices, Device{Name: name, State: state})
		}
	}
	// Only striped (and linear) segments map extents onto PVs; thin, RAID,
	// cache and pool segments reference sub-LVs that are counted themselves.
	if lvs := vgSection.section("logical_volumes"); lvs != nil {
		for _, lv := range lvs.sections {
			for _, seg := range lv.sections {
				if seg.str("type") == "striped" {
					vg.Alloc += uint64(seg.int("extent_count")) * extentSize
				}
			}
		}
	}
	vg.Alloc = min(vg.Alloc, vg.Size)
	return vg, nil
}

// cfgSection is a node of LVM's config format: name { key = value ... }.
type cfgSection struct {
	name     string
	values   map[string]any // string, int64 or []any
	sections []*cfgSection
}

func (s *cfgSection) str(key string) string {
	v, _ := s.values[key].(string)
	return v
}

func (s *cfgSection) int(key string) int64 {
	v, _ := s.values[key].(int64)
	return v
}

func (s *cfgSection) section(name string) *cfgSection {
	for _, child := range s.sections {
		if child.name == name {
			return child
		}
	}
	return nil
}

// hasFlag reports whether status or flags lists contain flag.
func (s *cfgSection) hasFlag(flag string) bool {
	for _, key := range []string{"status", "flags"} {
		list, _ := s.values[key].([]any)
		for _, v := range list {
			if v == flag {
				return true
			}
		}
	}
	return false
}

// parseConfig parses LVM's config text format into a section tree.
func parseConfig(text string) (*cfgSection, error) {
	p := &cfgParser{text: text}
	root := &cfgSection{values: map[string]any{}}
	if err := p.parseBody(root, false); err != nil {
		return nil, err
	}
	return root, nil
}

type cfgParser struct {
	text string
	pos  int
}

func (p *cfgParser) errorf(format string, args ...any) error {
	line := strings.Count(p.text[:p.pos], "\n") + 1
	return fmt.Errorf("metadata line %d: %s", line, fmt.Sprintf(format, args...))
}

// skip advances past whitespace, NULs and # comments.
func (p *cfgParser) skip() {
	for p.pos < len(p.text) {
		switch c := p.text[p.pos]; {
		case c == '#':
			if i := strings.IndexByte(p.text[p.pos:], '\n'); i >= 0 {
				p.pos += i
			} else {
				p.pos = len(p.text)
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == 0:
			p.pos++
		default:
			return
		}
	}
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '-' || c == '.' || c == '+' || c == '/' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func (p *cfgParser) ident() string {
	start := p.pos
	for p.pos < len(p.text) && isIdentByte(p.text[p.pos]) {
		p.pos++
	}
	return p.text[start:p.pos]
}

func (p *cfgParser) parseBody(s *cfgSection, nested bool) error {
	for {
		p.skip()
		if p.pos >= len(p.text) {
			if nested {
				return p.errorf("unterminated section %q", s.name)
			}
			return nil
		}
		if p.text[p.pos] == '}' {
			if !nested {
				return p.errorf("unexpected '}'")
			}
			p.pos++
			return nil
		}
		name := p.ident()
		if name == "" {
			return p.errorf("unexpected %q", p.text[p.pos])
		}
		p.skip()
		if p.pos >= len(p.text) {
			return p.errorf("unexpected end after %q", name)
		}
		switch p.text[p.pos] {
		case '{':
			p.pos++
			child := &cfgSection{name: name, values: map[string]any{}}
			if err := p.parseBody(child, true); err != nil {
				return err
			}
			s.sections = append(s.sections, child)
		case '=':
			p.pos++
			v, err := p.value()
			if err != nil {
				return err
			}
			s.values[name] = v
		default:
			return p.errorf("expected '{' or '=' after %q", name)
		}
	}
}

func (p *cfgParser) value() (any, error) {
	p.skip()
	if p.pos >= len(p.text) {
		return nil, p.errorf("missing value")
	}
	switch c := p.text[p.pos]; {
	case c == '"':
		return p.quoted()
	case c == '[':
		p.pos++
		var list []any
		for {
			p.skip()
			if p.pos < len(p.text) && p.text[p.pos] == ']' {
				p.pos++
				return list, nil
			}
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			list = append(list, v)
			p.skip()
			if p.pos < len(p.text) && p.text[p.pos] == ',' {
				p.pos++
			}
		}
	default:
		token := p.ident()
		if token == "" {
			return nil, p.errorf("unexpected %q", c)
		}
		if n, err := strconv.ParseInt(token, 10, 64); err == nil {
			return n, nil
		}
		return token, nil
	}
}

func (p *cfgParser) quoted() (string, error) {
	var b strings.Builder
	for p.pos++; p.pos < len(p.text); p.pos++ {
		switch c := p.text[p.pos]; c {
		case '"':
			p.pos++
			return b.String(), nil
		case '\\':
			if p.pos+1 < len(p.text) {
				p.pos++
				b.WriteByte(p.text[p.pos])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", p.errorf("unterminated string")
}
