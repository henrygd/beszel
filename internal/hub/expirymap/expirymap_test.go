//go:build testing
// +build testing

package expirymap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Not using the following methods but are useful for testing

// TESTING: Has checks if a key exists and hasn't expired
func (m *ExpiryMap[T]) Has(key string) bool {
	_, ok := m.GetOk(key)
	return ok
}

// TESTING: Get retrieves a value, returns zero value if not found or expired
func (m *ExpiryMap[T]) Get(key string) T {
	value, _ := m.GetOk(key)
	return value
}

// TESTING: Len returns the number of non-expired entries
func (m *ExpiryMap[T]) Len() int {
	count := 0
	now := time.Now()
	for _, val := range m.store.Values() {
		if val.expires.After(now) {
			count++
		}
	}
	return count
}

func TestExpiryMap_BasicOperations(t *testing.T) {
	em := New[string](time.Hour)

	// Test Set and GetOk
	em.Set("key1", "value1", time.Hour)
	value, ok := em.GetOk("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", value)

	// Test Get
	value = em.Get("key1")
	assert.Equal(t, "value1", value)

	// Test Has
	assert.True(t, em.Has("key1"))
	assert.False(t, em.Has("nonexistent"))

	// Test Remove
	em.Remove("key1")
	assert.False(t, em.Has("key1"))
}

func TestExpiryMap_Expiration(t *testing.T) {
	em := New[string](time.Hour)

	// Set a value with very short TTL
	em.Set("shortlived", "value", time.Millisecond*10)

	// Should exist immediately
	assert.True(t, em.Has("shortlived"))

	// Wait for expiration
	time.Sleep(time.Millisecond * 20)

	// Should be expired and automatically cleaned up on access
	assert.False(t, em.Has("shortlived"))
	value, ok := em.GetOk("shortlived")
	assert.False(t, ok)
	assert.Equal(t, "", value) // zero value for string
}

func TestExpiryMap_LazyCleanup(t *testing.T) {
	em := New[int](time.Hour)

	// Set multiple values with short TTL
	em.Set("key1", 1, time.Millisecond*10)
	em.Set("key2", 2, time.Millisecond*10)
	em.Set("key3", 3, time.Hour) // This one won't expire

	// Wait for expiration
	time.Sleep(time.Millisecond * 20)

	// Access expired keys should trigger lazy cleanup
	_, ok := em.GetOk("key1")
	assert.False(t, ok)

	// Non-expired key should still exist
	value, ok := em.GetOk("key3")
	assert.True(t, ok)
	assert.Equal(t, 3, value)
}

func TestExpiryMap_Len(t *testing.T) {
	em := New[string](time.Hour)

	// Initially empty
	assert.Equal(t, 0, em.Len())

	// Add some values
	em.Set("key1", "value1", time.Hour)
	em.Set("key2", "value2", time.Hour)
	em.Set("key3", "value3", time.Millisecond*10) // Will expire soon

	// Should count all initially
	assert.Equal(t, 3, em.Len())

	// Wait for one to expire
	time.Sleep(time.Millisecond * 20)

	// Len should reflect only non-expired entries
	assert.Equal(t, 2, em.Len())
}

func TestExpiryMap_CustomInterval(t *testing.T) {
	// Create with very short cleanup interval for testing
	em := New[string](time.Millisecond * 50)

	// Set a value that expires quickly
	em.Set("test", "value", time.Millisecond*10)

	// Should exist initially
	assert.True(t, em.Has("test"))

	// Wait for expiration + cleanup cycle
	time.Sleep(time.Millisecond * 100)

	// Should be cleaned up by background process
	// Note: This test might be flaky due to timing, but demonstrates the concept
	assert.False(t, em.Has("test"))
}

func TestExpiryMap_GenericTypes(t *testing.T) {
	// Test with different types
	t.Run("Int", func(t *testing.T) {
		em := New[int](time.Hour)

		em.Set("num", 42, time.Hour)
		value, ok := em.GetOk("num")
		assert.True(t, ok)
		assert.Equal(t, 42, value)
	})

	t.Run("Struct", func(t *testing.T) {
		type TestStruct struct {
			Name string
			Age  int
		}

		em := New[TestStruct](time.Hour)

		expected := TestStruct{Name: "John", Age: 30}
		em.Set("person", expected, time.Hour)

		value, ok := em.GetOk("person")
		assert.True(t, ok)
		assert.Equal(t, expected, value)
	})

	t.Run("Pointer", func(t *testing.T) {
		em := New[*string](time.Hour)

		str := "hello"
		em.Set("ptr", &str, time.Hour)

		value, ok := em.GetOk("ptr")
		assert.True(t, ok)
		require.NotNil(t, value)
		assert.Equal(t, "hello", *value)
	})
}

func TestExpiryMap_ZeroValues(t *testing.T) {
	em := New[string](time.Hour)

	// Test getting non-existent key returns zero value
	value := em.Get("nonexistent")
	assert.Equal(t, "", value)

	// Test getting expired key returns zero value
	em.Set("expired", "value", time.Millisecond*10)
	time.Sleep(time.Millisecond * 20)

	value = em.Get("expired")
	assert.Equal(t, "", value)
}

func TestExpiryMap_Concurrent(t *testing.T) {
	em := New[int](time.Hour)

	// Simple concurrent access test
	done := make(chan bool, 2)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			em.Set("key", i, time.Hour)
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = em.Get("key")
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for both to complete
	<-done
	<-done

	// Should not panic and should have some value
	assert.True(t, em.Has("key"))
}

func TestExpiryMap_GetByValue(t *testing.T) {
	em := New[string](time.Hour)

	// Test getting by value when value exists
	em.Set("key1", "value1", time.Hour)
	em.Set("key2", "value2", time.Hour)
	em.Set("key3", "value1", time.Hour) // Duplicate value - should return first match

	// Test successful retrieval
	key, value, ok := em.GetByValue("value1")
	assert.True(t, ok)
	assert.Equal(t, "value1", value)
	assert.Contains(t, []string{"key1", "key3"}, key) // Should be one of the keys with this value

	// Test retrieval of unique value
	key, value, ok = em.GetByValue("value2")
	assert.True(t, ok)
	assert.Equal(t, "value2", value)
	assert.Equal(t, "key2", key)

	// Test getting non-existent value
	key, value, ok = em.GetByValue("nonexistent")
	assert.False(t, ok)
	assert.Equal(t, "", value) // zero value for string
	assert.Equal(t, "", key)   // zero value for string
}

func TestExpiryMap_GetByValue_Expiration(t *testing.T) {
	em := New[string](time.Hour)

	// Set a value with short TTL
	em.Set("shortkey", "shortvalue", time.Millisecond*10)
	em.Set("longkey", "longvalue", time.Hour)

	// Should find the short-lived value initially
	key, value, ok := em.GetByValue("shortvalue")
	assert.True(t, ok)
	assert.Equal(t, "shortvalue", value)
	assert.Equal(t, "shortkey", key)

	// Wait for expiration
	time.Sleep(time.Millisecond * 20)

	// Should not find expired value and should trigger lazy cleanup
	key, value, ok = em.GetByValue("shortvalue")
	assert.False(t, ok)
	assert.Equal(t, "", value)
	assert.Equal(t, "", key)

	// Should still find non-expired value
	key, value, ok = em.GetByValue("longvalue")
	assert.True(t, ok)
	assert.Equal(t, "longvalue", value)
	assert.Equal(t, "longkey", key)
}

func TestExpiryMap_GetByValue_GenericTypes(t *testing.T) {
	t.Run("Int", func(t *testing.T) {
		em := New[int](time.Hour)

		em.Set("num1", 42, time.Hour)
		em.Set("num2", 84, time.Hour)

		key, value, ok := em.GetByValue(42)
		assert.True(t, ok)
		assert.Equal(t, 42, value)
		assert.Equal(t, "num1", key)

		key, value, ok = em.GetByValue(99)
		assert.False(t, ok)
		assert.Equal(t, 0, value)
		assert.Equal(t, "", key)
	})

	t.Run("Struct", func(t *testing.T) {
		type TestStruct struct {
			Name string
			Age  int
		}

		em := New[TestStruct](time.Hour)

		person1 := TestStruct{Name: "John", Age: 30}
		person2 := TestStruct{Name: "Jane", Age: 25}

		em.Set("person1", person1, time.Hour)
		em.Set("person2", person2, time.Hour)

		key, value, ok := em.GetByValue(person1)
		assert.True(t, ok)
		assert.Equal(t, person1, value)
		assert.Equal(t, "person1", key)

		nonexistent := TestStruct{Name: "Bob", Age: 40}
		key, value, ok = em.GetByValue(nonexistent)
		assert.False(t, ok)
		assert.Equal(t, TestStruct{}, value)
		assert.Equal(t, "", key)
	})
}

func TestExpiryMap_RemoveValue(t *testing.T) {
	em := New[string](time.Hour)

	// Test removing existing value
	em.Set("key1", "value1", time.Hour)
	em.Set("key2", "value2", time.Hour)
	em.Set("key3", "value1", time.Hour) // Duplicate value

	// Remove by value should remove one instance
	removedValue, ok := em.RemovebyValue("value1")
	assert.True(t, ok)
	assert.Equal(t, "value1", removedValue)

	// Should still have the other instance or value2
	assert.True(t, em.Has("key2")) // value2 should still exist

	// Check if one of the duplicate values was removed
	// At least one key with "value1" should be gone
	key1Exists := em.Has("key1")
	key3Exists := em.Has("key3")
	assert.False(t, key1Exists && key3Exists) // Both shouldn't exist
	assert.True(t, key1Exists || key3Exists)  // At least one should be gone

	// Test removing non-existent value
	removedValue, ok = em.RemovebyValue("nonexistent")
	assert.False(t, ok)
	assert.Equal(t, "", removedValue) // zero value for string
}

func TestExpiryMap_RemoveValue_GenericTypes(t *testing.T) {
	t.Run("Int", func(t *testing.T) {
		em := New[int](time.Hour)

		em.Set("num1", 42, time.Hour)
		em.Set("num2", 84, time.Hour)

		// Remove existing value
		removedValue, ok := em.RemovebyValue(42)
		assert.True(t, ok)
		assert.Equal(t, 42, removedValue)
		assert.False(t, em.Has("num1"))
		assert.True(t, em.Has("num2"))

		// Remove non-existent value
		removedValue, ok = em.RemovebyValue(99)
		assert.False(t, ok)
		assert.Equal(t, 0, removedValue)
	})

	t.Run("Struct", func(t *testing.T) {
		type TestStruct struct {
			Name string
			Age  int
		}

		em := New[TestStruct](time.Hour)

		person1 := TestStruct{Name: "John", Age: 30}
		person2 := TestStruct{Name: "Jane", Age: 25}

		em.Set("person1", person1, time.Hour)
		em.Set("person2", person2, time.Hour)

		// Remove existing struct
		removedValue, ok := em.RemovebyValue(person1)
		assert.True(t, ok)
		assert.Equal(t, person1, removedValue)
		assert.False(t, em.Has("person1"))
		assert.True(t, em.Has("person2"))

		// Remove non-existent struct
		nonexistent := TestStruct{Name: "Bob", Age: 40}
		removedValue, ok = em.RemovebyValue(nonexistent)
		assert.False(t, ok)
		assert.Equal(t, TestStruct{}, removedValue)
	})
}

func TestExpiryMap_RemoveValue_WithExpiration(t *testing.T) {
	em := New[string](time.Hour)

	// Set values with different TTLs
	em.Set("key1", "value1", time.Millisecond*10) // Will expire
	em.Set("key2", "value2", time.Hour)           // Won't expire
	em.Set("key3", "value1", time.Hour)           // Won't expire, duplicate value

	// Wait for first value to expire
	time.Sleep(time.Millisecond * 20)

	// Try to remove the expired value - should remove one of the "value1" entries
	removedValue, ok := em.RemovebyValue("value1")
	assert.True(t, ok)
	assert.Equal(t, "value1", removedValue)

	// Should still have key2 (different value)
	assert.True(t, em.Has("key2"))

	// Should have removed one of the "value1" entries (either key1 or key3)
	// But we can't predict which one due to map iteration order
	key1Exists := em.Has("key1")
	key3Exists := em.Has("key3")

	// Exactly one of key1 or key3 should be gone
	assert.False(t, key1Exists && key3Exists) // Both shouldn't exist
	assert.True(t, key1Exists || key3Exists)  // At least one should still exist
}

func TestExpiryMap_ValueOperations_Integration(t *testing.T) {
	em := New[string](time.Hour)

	// Test integration of GetByValue and RemoveValue
	em.Set("key1", "shared", time.Hour)
	em.Set("key2", "unique", time.Hour)
	em.Set("key3", "shared", time.Hour)

	// Find shared value
	key, value, ok := em.GetByValue("shared")
	assert.True(t, ok)
	assert.Equal(t, "shared", value)
	assert.Contains(t, []string{"key1", "key3"}, key)

	// Remove shared value
	removedValue, ok := em.RemovebyValue("shared")
	assert.True(t, ok)
	assert.Equal(t, "shared", removedValue)

	// Should still be able to find the other shared value
	key, value, ok = em.GetByValue("shared")
	assert.True(t, ok)
	assert.Equal(t, "shared", value)
	assert.Contains(t, []string{"key1", "key3"}, key)

	// Remove the other shared value
	removedValue, ok = em.RemovebyValue("shared")
	assert.True(t, ok)
	assert.Equal(t, "shared", removedValue)

	// Should not find shared value anymore
	key, value, ok = em.GetByValue("shared")
	assert.False(t, ok)
	assert.Equal(t, "", value)
	assert.Equal(t, "", key)

	// Unique value should still exist
	key, value, ok = em.GetByValue("unique")
	assert.True(t, ok)
	assert.Equal(t, "unique", value)
	assert.Equal(t, "key2", key)
}
