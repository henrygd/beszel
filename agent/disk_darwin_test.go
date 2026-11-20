//go:build darwin

package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apfsPlist builds a minimal `diskutil info -plist` document for an APFS volume.
func apfsPlist(container string, stores ...string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>APFSContainerReference</key><string>` + container + `</string>
<key>APFSPhysicalStores</key><array>`)
	for _, s := range stores {
		b.WriteString(`<dict><key>APFSPhysicalStore</key><string>` + s + `</string></dict>`)
	}
	b.WriteString(`</array><key>FilesystemType</key><string>apfs</string></dict></plist>`)
	return []byte(b.String())
}

func TestParseApfsPhysicalDisk(t *testing.T) {
	tests := []struct {
		name string
		out  []byte
		want string
	}{
		{"apfs volume", apfsPlist("disk6", "disk5s2"), "disk5"},
		{"store is whole disk", apfsPlist("disk6", "disk5"), "disk5"},
		{"multi-store container uses first store", apfsPlist("disk13", "disk12s2", "disk3s2"), "disk12"},
		{"no physical stores", apfsPlist("disk6"), ""},
		{"malformed store name", apfsPlist("disk6", "bogus"), ""},
		{"hfs volume", []byte(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>FilesystemType</key><string>hfs</string></dict></plist>`), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseApfsPhysicalDisk(tt.out)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("invalid plist", func(t *testing.T) {
		got, err := parseApfsPhysicalDisk([]byte("not a plist"))
		assert.Error(t, err)
		assert.Empty(t, got)
	})
}
