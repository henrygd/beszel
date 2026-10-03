//go:build testing

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCounterDelta(t *testing.T) {
	assert.Equal(t, uint64(5), counterDelta(15, 10))
	assert.Equal(t, uint64(0), counterDelta(10, 10))
	assert.Equal(t, uint64(0), counterDelta(5, 10), "counter reset yields zero")
}

func TestStallPercent(t *testing.T) {
	// 1.5s stalled over a 60s interval
	assert.Equal(t, 2.5, stallPercent(1_500_000, 60e6))
	assert.Equal(t, 0.0, stallPercent(0, 60e6))
	assert.Equal(t, 100.0, stallPercent(70_000_000, 60e6), "clamped to 100%")
}
