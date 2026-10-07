package custommetrics

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MetricType is the type a # TYPE line declares for a metric family.
type MetricType int

const (
	Untyped     MetricType = iota // no # TYPE, untyped or unknown; treated as a gauge
	Gauge                         // gauge
	Counter                       // counter
	Unsupported                   // histogram, summary and other multi-sample types; skipped
)

var metricTypes = map[string]MetricType{
	"gauge":          Gauge,
	"counter":        Counter,
	"untyped":        Untyped,
	"unknown":        Untyped,
	"histogram":      Unsupported,
	"gaugehistogram": Unsupported,
	"summary":        Unsupported,
	"info":           Unsupported,
	"stateset":       Unsupported,
}

// Label is one name="value" pair on a sample.
type Label struct{ Name, Value string }

// Sample is one sample line.
type Sample struct {
	Name        string  // as written, e.g. foo_total
	Labels      []Label // in the order written
	Value       float64 // may be NaN or ±Inf; the collector drops those
	TimestampMs int64   // 0 when absent
	Line        int
}

// Family is a metric family: its metadata and the samples that belong to it.
// Unsupported families carry no samples.
type Family struct {
	Name    string
	Type    MetricType
	Help    string
	Unit    string // from # UNIT, empty if none
	Samples []Sample
}

// ParseError reports a line that could not be parsed.
type ParseError struct {
	Line int
	Msg  string
}

func (e ParseError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

var helpUnescaper = strings.NewReplacer(`\\`, `\`, `\n`, "\n")

// Parse reads the subset of the Prometheus text format, and of OpenMetrics,
// that custom metrics support. A malformed line is reported and skipped, and
// the rest of the input is still read.
func Parse(data []byte) ([]Family, []ParseError) {
	p := parser{byName: make(map[string]int)}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.Trim(line, " \t\r")
		if line == "" {
			continue
		}
		if line == "# EOF" {
			break
		}
		if line[0] == '#' {
			p.metadata(i+1, line)
		} else {
			p.sample(i+1, line)
		}
	}
	return p.families, p.errors
}

type parser struct {
	families []Family
	byName   map[string]int // index in families
	errors   []ParseError
}

func (p *parser) fail(line int, format string, args ...any) {
	p.errors = append(p.errors, ParseError{Line: line, Msg: fmt.Sprintf(format, args...)})
}

// family returns the family with the given name, starting one if there is none.
func (p *parser) family(name string) *Family {
	i, ok := p.byName[name]
	if !ok {
		i = len(p.families)
		p.families = append(p.families, Family{Name: name})
		p.byName[name] = i
	}
	return &p.families[i]
}

// metadata handles # HELP, # TYPE and # UNIT lines. Any other # line is a comment.
func (p *parser) metadata(lineNo int, line string) {
	rest, ok := strings.CutPrefix(line, "# ")
	if !ok {
		return
	}
	keyword, rest := cutSpace(trimSpace(rest))
	if keyword != "HELP" && keyword != "TYPE" && keyword != "UNIT" {
		return
	}
	name, text := cutSpace(rest)
	if !validMetricName(name) {
		p.fail(lineNo, "invalid metric name %q in # %s", name, keyword)
		return
	}
	f := p.family(name)
	switch keyword {
	case "HELP":
		f.Help = strings.ToValidUTF8(helpUnescaper.Replace(text), "\uFFFD")
	case "UNIT":
		f.Unit = strings.ToValidUTF8(text, "\uFFFD")
	case "TYPE":
		typ, ok := metricTypes[text]
		if !ok {
			// Semantics unknown, so skip the family rather than chart it wrongly.
			p.fail(lineNo, "unknown type %q for %s", text, name)
			typ = Unsupported
		}
		f.Type = typ
	}
}

func (p *parser) sample(lineNo int, line string) {
	s, err := parseSample(line)
	if err != nil {
		p.fail(lineNo, "%v", err)
		return
	}
	s.Line = lineNo
	if f, keep := p.owner(s.Name); keep {
		f.Samples = append(f.Samples, s)
	}
}

// childSuffixes are what a sample name can add to its family's name: _total
// and _created for an OpenMetrics counter, the rest for the multi-sample types.
var childSuffixes = []string{"_total", "_created", "_bucket", "_count", "_sum", "_gcount", "_gsum", "_info"}

// owner returns the family a sample belongs to, and whether the sample is
// kept. Families are found by name, so metadata lines apply wherever they
// stand before the samples, including in a header block at the top of the
// file. The family with the sample's own name comes first; then a family whose
// name the sample's extends by a known suffix, if it claims the sample.
// Otherwise the sample starts a new family.
func (p *parser) owner(name string) (*Family, bool) {
	if i, ok := p.byName[name]; ok {
		_, keep := p.families[i].owns(name)
		return &p.families[i], keep
	}
	for _, suffix := range childSuffixes {
		base, ok := strings.CutSuffix(name, suffix)
		if !ok {
			continue
		}
		if i, ok := p.byName[base]; ok {
			if member, keep := p.families[i].owns(name); member {
				return &p.families[i], keep
			}
		}
	}
	return p.family(name), true
}

// owns reports whether a sample name belongs to the family, and if so whether
// the sample is kept. Children of unsupported families and counter _created
// samples belong to their family but are dropped.
func (f *Family) owns(name string) (member, keep bool) {
	if name == f.Name {
		return true, f.Type != Unsupported
	}
	suffix, ok := strings.CutPrefix(name, f.Name)
	if !ok {
		return false, false
	}
	switch f.Type {
	case Counter:
		switch suffix {
		case "_total":
			return true, true
		case "_created":
			return true, false
		}
	case Unsupported:
		switch suffix {
		case "_bucket", "_count", "_sum", "_created", "_gcount", "_gsum", "_info":
			return true, false
		}
	}
	return false, false
}

// parseSample parses name[{labels}] value [timestamp], ignoring a trailing
// OpenMetrics exemplar.
func parseSample(line string) (Sample, error) {
	var s Sample
	n := scanName(line, true)
	if n == 0 {
		return s, fmt.Errorf("expected a metric name")
	}
	s.Name, line = line[:n], trimSpace(line[n:])
	if strings.HasPrefix(line, "{") {
		var err error
		if s.Labels, line, err = parseLabels(line[1:]); err != nil {
			return s, err
		}
	}

	valueStr, line := cutSpace(line)
	if valueStr == "" {
		return s, fmt.Errorf("missing value for %s", s.Name)
	}
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return s, fmt.Errorf("invalid value %q for %s", valueStr, s.Name)
	}
	s.Value = value

	if line == "" || line[0] == '#' {
		return s, nil
	}
	tsStr, line := cutSpace(line)
	if s.TimestampMs, err = parseTimestamp(tsStr); err != nil {
		return s, fmt.Errorf("invalid timestamp %q for %s", tsStr, s.Name)
	}
	if line != "" && line[0] != '#' {
		return s, fmt.Errorf("unexpected text after timestamp for %s", s.Name)
	}
	return s, nil
}

// parseLabels parses label pairs after the opening brace and returns the text
// after the closing brace. A trailing comma is allowed.
func parseLabels(line string) ([]Label, string, error) {
	var labels []Label
	for {
		line = trimSpace(line)
		if strings.HasPrefix(line, "}") {
			return labels, trimSpace(line[1:]), nil
		}
		n := scanName(line, false)
		if n == 0 {
			return nil, "", fmt.Errorf("invalid label name")
		}
		name := line[:n]
		line = trimSpace(line[n:])
		if !strings.HasPrefix(line, "=") {
			return nil, "", fmt.Errorf("expected = after label %s", name)
		}
		line = trimSpace(line[1:])
		if !strings.HasPrefix(line, `"`) {
			return nil, "", fmt.Errorf("expected a quoted value for label %s", name)
		}
		value, rest, ok := unquote(line[1:])
		if !ok {
			return nil, "", fmt.Errorf("unterminated value for label %s", name)
		}
		for _, l := range labels {
			if l.Name == name {
				return nil, "", fmt.Errorf("duplicate label %s", name)
			}
		}
		labels = append(labels, Label{Name: name, Value: value})
		line = trimSpace(rest)
		switch {
		case strings.HasPrefix(line, ","):
			line = line[1:]
		case strings.HasPrefix(line, "}"):
		default:
			return nil, "", fmt.Errorf("expected , or } after label %s", name)
		}
	}
}

// unquote reads a label value up to its closing quote, resolving \\, \" and \n.
func unquote(s string) (value, rest string, ok bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			return b.String(), s[i+1:], true
		case '\\':
			if i+1 == len(s) {
				return "", "", false
			}
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case '\\', '"':
				b.WriteByte(s[i])
			default:
				b.WriteByte('\\')
				b.WriteByte(s[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", "", false
}

// parseTimestamp reads an integer as milliseconds (Prometheus text format) and
// a number with a decimal point or exponent as seconds (OpenMetrics). Known
// limitation: OpenMetrics also allows whole seconds with no decimal point,
// which this reads as milliseconds, so such samples count as stale. Producers
// can leave timestamps out; a file ending in # EOF could be read as
// OpenMetrics instead.
func parseTimestamp(s string) (int64, error) {
	if !strings.ContainsAny(s, ".eE") {
		return strconv.ParseInt(s, 10, 64)
	}
	seconds, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	ms := math.Round(seconds * 1000)
	if math.IsNaN(ms) || math.Abs(ms) >= math.MaxInt64 {
		return 0, fmt.Errorf("timestamp out of range")
	}
	return int64(ms), nil
}

// scanName returns the length of the metric name ([a-zA-Z_:][a-zA-Z0-9_:]*)
// or label name ([a-zA-Z_][a-zA-Z0-9_]*) at the start of s.
func scanName(s string, colon bool) int {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || (colon && c == ':') || (i > 0 && '0' <= c && c <= '9') {
			continue
		}
		return i
	}
	return len(s)
}

func validMetricName(s string) bool {
	return s != "" && scanName(s, true) == len(s)
}

func trimSpace(s string) string {
	return strings.TrimLeft(s, " \t")
}

// cutSpace splits s at its first run of spaces or tabs.
func cutSpace(s string) (before, after string) {
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], trimSpace(s[i:])
}
