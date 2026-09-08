//go:build testing

package utils

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeGoRunsFn(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	ran := false
	SafeGo("test task", func() {
		defer wg.Done()
		ran = true
	})
	wg.Wait()

	assert.True(t, ran, "expected fn to run")
}

// A panic in a detached goroutine cannot be recovered by its parent, so without
// SafeGo it takes the whole process down. Reaching the end of these subtests at
// all is the assertion.
func TestSafeGoRecoversPanic(t *testing.T) {
	t.Run("explicit panic", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		SafeGo("panicking task", func() {
			defer wg.Done()
			panic("boom")
		})
		wg.Wait()
	})

	t.Run("nil map write", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		SafeGo("nil map task", func() {
			defer wg.Done()
			var m map[string]string
			m["key"] = "value"
		})
		wg.Wait()
	})

	t.Run("nil pointer dereference", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		SafeGo("nil deref task", func() {
			defer wg.Done()
			type row struct{ name string }
			var r *row
			_ = r.name
		})
		wg.Wait()
	})
}
