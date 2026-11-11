//go:build testing

package custommetrics

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// BenchmarkCollectAtCaps measures one collection over maxFiles files of just
// under maxFileSize each, the most the per-file caps allow, for the kinds of
// content that cost the most. The per-collection budgets bound all of them.
//
//	go test -tags testing -run '^$' -bench CollectAtCaps ./agent/custommetrics/
func BenchmarkCollectAtCaps(b *testing.B) {
	all := "metrics:\n  max_series: 256\n  sources:\n    - path: {dir}/m\n"
	filtered := "metrics:\n  max_series: 256\n  sources:\n    - path: {dir}/m\n      include: [nothing_*]\n"
	for _, bc := range []struct {
		name, config string
		line         func(file, i int) string
	}{
		{"short samples", all, func(file, i int) string { return fmt.Sprintf("s%d_%d 1\n", file, i) }},
		{"filtered-out samples", filtered, func(file, i int) string { return fmt.Sprintf("s%d_%d 1\n", file, i) }},
		{"malformed lines", all, func(int, int) string { return "!\n" }},
		{"comments", all, func(int, int) string { return "# c\n" }},
	} {
		b.Run(bc.name, func(b *testing.B) {
			c := capsFixture(b, bc.config, bc.line)
			now := time.Now()
			b.ReportAllocs()
			for b.Loop() {
				now = now.Add(time.Second)
				c.Collect(1000, now)
			}
		})
	}
}

// capsFixture fills a source directory to the per-file caps, each line made by
// line, and returns a collector for it with warnings silenced.
func capsFixture(b *testing.B, config string, line func(file, i int) string) *Collector {
	b.Setenv("CONFIG", "")
	b.Setenv("BESZEL_AGENT_CONFIG", "")
	originalWarn, originalInfo := logWarn, logInfo
	logWarn, logInfo = func(string, ...any) {}, func(string, ...any) {}
	b.Cleanup(func() { logWarn, logInfo = originalWarn, originalInfo })

	dir := b.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "m"), 0o755); err != nil {
		b.Fatal(err)
	}
	for file := range maxFiles {
		var content strings.Builder
		for i := 0; ; i++ {
			l := line(file, i)
			if content.Len()+len(l) > maxFileSize {
				break
			}
			content.WriteString(l)
		}
		if err := os.WriteFile(filepath.Join(dir, "m", fmt.Sprintf("%03d.prom", file)), []byte(content.String()), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(strings.ReplaceAll(config, "{dir}", dir)), 0o644); err != nil {
		b.Fatal(err)
	}
	return NewCollector(dir)
}
