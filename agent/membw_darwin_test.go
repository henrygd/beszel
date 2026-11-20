//go:build darwin && testing

package agent

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock only the native boundary: exercise the real constructor's ownership
// transfers without needing IOReport channels on the test host.
func TestIOReportBandwidthSetupReleasesOwnedReferences(t *testing.T) {
	oldString, oldCString := cfStringCreateWithCString, cfStringGetCString
	oldDictionary, oldCount, oldValue := cfDictionaryGetValue, cfArrayGetCount, cfArrayGetValueAtIndex
	oldRelease, oldChannels := cfRelease, ioReportCopyChannelsInGroup
	oldSubscribe, oldSamples, oldName := ioReportCreateSubscription, ioReportCreateSamples, ioReportChannelGetChannelName
	t.Cleanup(func() {
		cfStringCreateWithCString, cfStringGetCString = oldString, oldCString
		cfDictionaryGetValue, cfArrayGetCount, cfArrayGetValueAtIndex = oldDictionary, oldCount, oldValue
		cfRelease, ioReportCopyChannelsInGroup = oldRelease, oldChannels
		ioReportCreateSubscription, ioReportCreateSamples, ioReportChannelGetChannelName = oldSubscribe, oldSamples, oldName
	})
	for _, stage := range []string{"channels", "subscription", "subscribed", "sample", "histograms", "ready"} {
		t.Run(stage, func(t *testing.T) {
			owned := map[uintptr]int{}
			acquire := func(ref uintptr) uintptr { owned[ref]++; return ref }
			cfRelease = func(ref uintptr) {
				require.NotZero(t, ref, "CFRelease(NULL) is invalid")
				owned[ref]--
				assert.Zero(t, owned[ref], "release unowned or already released reference %d", ref)
			}
			cfStringCreateWithCString = func(_ uintptr, s string, _ uint32) uintptr {
				return acquire(map[string]uintptr{"PMP": 1, "DCS BW": 2, "IOReportChannels": 4}[s])
			}
			ioReportCopyChannelsInGroup = func(_, _ uintptr, _, _, _ uint64) uintptr {
				if stage == "channels" {
					return 0
				}
				return acquire(3)
			}
			ioReportCreateSubscription = func(_, _ uintptr, subscribed *uintptr, _ uint64, _ uintptr) uintptr {
				if stage != "subscribed" {
					*subscribed = acquire(6)
				}
				if stage == "subscription" {
					return 0
				}
				return acquire(5)
			}
			ioReportCreateSamples = func(_, _, _ uintptr) uintptr {
				if stage == "sample" {
					return 0
				}
				return acquire(7)
			}
			cfDictionaryGetValue = func(_, _ uintptr) uintptr { return 8 }
			cfArrayGetCount = func(_ uintptr) int64 {
				if stage == "histograms" {
					return 1
				}
				return 2
			}
			cfArrayGetValueAtIndex = func(_ uintptr, i int64) uintptr { return uintptr(9 + i) }
			ioReportChannelGetChannelName = func(ch uintptr) uintptr { return ch }
			cfStringGetCString = func(s uintptr, buf *byte, size int64, _ uint32) bool {
				name := map[uintptr]string{9: "AMCC RD", 10: "AMCC WR"}[s]
				copy(unsafe.Slice(buf, int(size)), name+"\x00")
				return true
			}
			r, err := newIOReportBandwidth()
			if stage == "ready" {
				require.NoError(t, err)
				for _, ref := range []uintptr{4, 5, 6, 7} {
					assert.Equal(t, 1, owned[ref], "live reference %d released too soon", ref)
				}
				r.close()
				r.close()
			} else {
				require.Error(t, err)
				assert.Nil(t, r)
			}
			for ref, count := range owned {
				assert.Zero(t, count, "leaked reference %d", ref)
			}
		})
	}
}
