package custommetrics

import (
	"slices"
	"strings"
	"unicode"
)

// unitSuffixes are the conventional name suffixes a unit is inferred from.
var unitSuffixes = []string{"watts", "volts", "amperes", "joules", "bytes", "seconds", "celsius", "hertz", "ratio", "percent"}

// sanitize collapses each run of characters other than "_" and letters,
// digits and combining marks, in any script, to "_", and trims "_" from both
// ends. Values written in other scripts stay distinct and readable, and the
// result is always valid UTF-8: invalid bytes read as U+FFFD, a symbol, and
// collapse like one.
func sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pending := false
	for _, r := range s {
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) {
			if pending {
				b.WriteByte('_')
				pending = false
			}
			b.WriteRune(r)
		} else {
			pending = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// labelValues returns sanitised label values in order of label name, skipping
// values that sanitise to nothing. Sorting by name keeps a key independent of
// the order labels are written in.
func labelValues(labels []Label) []string {
	sorted := slices.Clone(labels)
	slices.SortFunc(sorted, func(a, b Label) int { return strings.Compare(a.Name, b.Name) })
	values := make([]string, 0, len(sorted))
	for _, l := range sorted {
		if v := sanitize(l.Value); v != "" {
			values = append(values, v)
		}
	}
	return values
}

// joinKey joins non-empty parts with "_".
func joinKey(prefix, name string, values []string) string {
	parts := make([]string, 0, len(values)+2)
	if prefix != "" {
		parts = append(parts, prefix)
	}
	parts = append(parts, name)
	parts = append(parts, values...)
	return strings.Join(parts, "_")
}

// seriesKey flattens a sample into its series key: the source prefix, the
// metric name as written, then the label values. The prefix must already be
// sanitised.
//
// The key is the series' stored identity: it keys the values in custom_stats
// and the names the hub keeps in cmr, and display_names entries can name it.
// Changing how keys are built starts every affected series anew.
func seriesKey(prefix string, s *Sample) string {
	return joinKey(prefix, s.Name, labelValues(s.Labels))
}

// baseUnit returns the family's unit: its # UNIT line, capped at maxUnitLength
// runes, else a conventional name suffix (looking past a trailing _total),
// else "". The unit is copied onto every series of the family, so an uncapped
// one could inflate the metadata to megabytes.
func baseUnit(f *Family) string {
	if f.Unit != "" {
		return truncateRunes(f.Unit, maxUnitLength)
	}
	name := strings.TrimSuffix(f.Name, "_total")
	for _, unit := range unitSuffixes {
		if strings.HasSuffix(name, "_"+unit) {
			return unit
		}
	}
	return ""
}

// seriesUnit is the unit a series is charted in: the base unit, with "/s"
// appended for a counter reported as a rate.
func seriesUnit(f *Family, counters CounterMode) string {
	unit := baseUnit(f)
	if f.Type == Counter && counters == CountersRate {
		unit += "/s"
	}
	return unit
}

// baseName is the family name without _total, for derived chart titles. The
// unit word stays: a chart may mix units, so it is what tells pi_core_volts
// from pi_core_amperes.
func baseName(f *Family) string {
	return strings.TrimSuffix(f.Name, "_total")
}

// displayName looks a series up in its source's display_names, either as the
// file names it (its key without the source prefix, which survives a prefix
// change) or by its key, which is the line name shown without an entry.
func displayName(displayNames map[string]string, key string, s *Sample) (string, bool) {
	if len(displayNames) == 0 {
		return "", false
	}
	if name, ok := displayNames[seriesKey("", s)]; ok {
		return name, true
	}
	name, ok := displayNames[key]
	return name, ok
}

// seriesDisplayName resolves a series' line name: its display_names entry,
// else its key, so the name an operator sees is one display_names accepts.
func seriesDisplayName(displayNames map[string]string, key string, s *Sample) string {
	if name, ok := displayName(displayNames, key, s); ok {
		return name
	}
	return key
}

// chartTitle resolves the title of the chart a family's series are drawn in.
// Series with the same title share a chart. A source's chart setting wins;
// otherwise each metric gets its own chart, titled with the display name of
// its only series if it has one, else its humanised base name.
func chartTitle(chart string, displayNames map[string]string, prefix string, f *Family) string {
	if chart != "" {
		return chart
	}
	if len(f.Samples) == 1 {
		s := &f.Samples[0]
		if name, ok := displayName(displayNames, seriesKey(prefix, s), s); ok {
			return name
		}
	}
	return humanize(baseName(f))
}

// humanize turns a metric name into a title: underscores become spaces and the
// first letter is capitalised, so pi_power_board reads "Pi power board".
func humanize(name string) string {
	name = strings.Join(strings.FieldsFunc(name, func(r rune) bool { return r == '_' }), " ")
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
