package battery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimarySelection(t *testing.T) {
	tests := []struct {
		name string
		bats []Battery
		want string
	}{
		{"largest reported capacity", []Battery{{Name: "Small", FullChargeCapacity: 20, HasFullChargeCapacity: true, System: true}, {Name: "Large", FullChargeCapacity: 80, HasFullChargeCapacity: true}}, "Large"},
		{"reported ranks over missing", []Battery{{Name: "Unknown", System: true}, {Name: "Known", FullChargeCapacity: 1, HasFullChargeCapacity: true}}, "Known"},
		{"system wins capacity tie", []Battery{{Name: "Peripheral", FullChargeCapacity: 50, HasFullChargeCapacity: true}, {Name: "System", FullChargeCapacity: 50, HasFullChargeCapacity: true, System: true}}, "System"},
		{"name resolves final tie", []Battery{{Name: "Zed"}, {Name: "Alpha"}}, "Alpha"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Primary(tt.bats)
			require.True(t, ok)
			assert.Equal(t, tt.want, got.Name)
		})
	}
	_, ok := Primary(nil)
	assert.False(t, ok)
}

func TestNormalizeBatteriesFallbackNames(t *testing.T) {
	bats := normalizeBatteries([]Battery{{}, {}, {Name: "Mouse"}, {Name: "Mouse"}})
	assert.Equal(t, []string{"Battery 1", "Battery 2", "Mouse", "Mouse (2)"}, []string{bats[0].Name, bats[1].Name, bats[2].Name, bats[3].Name})
}
