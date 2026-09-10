//go:build linux

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func processFixture(t testing.TB, n int) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOST_PROC", root)
	// Include a blocked and an unreadable process: neither contributes to a tracked state.
	states := []string{"R", "S", "I", "T", "Z", "D", ""}
	for i := range n {
		dir := filepath.Join(root, fmt.Sprint(i+1))
		require.NoError(t, os.Mkdir(dir, 0755))
		state := states[i%len(states)]
		if state != "" {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "status"), []byte("Name:\tfixture\nState:\t"+state+"\n"), 0644))
		}
	}
}

func TestGetProcessCounts(t *testing.T) {
	processFixture(t, 7)
	counts, err := getProcessCounts()
	require.NoError(t, err)
	require.Equal(t, [5]uint32{1, 1, 1, 1, 1}, counts)
}

func TestGetProcessCountsEnumerationFailure(t *testing.T) {
	t.Setenv("HOST_PROC", filepath.Join(t.TempDir(), "missing"))
	_, err := getProcessCounts()
	require.Error(t, err)
}

// Synthetic proc trees measure scan scaling; real-host cost is measured separately.
func BenchmarkProcessCountsScaling(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			processFixture(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := getProcessCounts(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
