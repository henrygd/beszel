package expirymap

import (
	"reflect"
	"time"

	"github.com/pocketbase/pocketbase/tools/store"
)

type val[T any] struct {
	value   T
	expires time.Time
}

type ExpiryMap[T any] struct {
	store           *store.Store[string, *val[T]]
	cleanupInterval time.Duration
}

// New creates a new expiry map with custom cleanup interval
func New[T any](cleanupInterval time.Duration) *ExpiryMap[T] {
	m := &ExpiryMap[T]{
		store:           store.New(map[string]*val[T]{}),
		cleanupInterval: cleanupInterval,
	}
	m.startCleaner()
	return m
}

// Set stores a value with the given TTL
func (m *ExpiryMap[T]) Set(key string, value T, ttl time.Duration) {
	m.store.Set(key, &val[T]{
		value:   value,
		expires: time.Now().Add(ttl),
	})
}

// GetOk retrieves a value and checks if it exists and hasn't expired
// Performs lazy cleanup of expired entries on access
func (m *ExpiryMap[T]) GetOk(key string) (T, bool) {
	value, ok := m.store.GetOk(key)
	if !ok {
		return *new(T), false
	}

	// Check if expired and perform lazy cleanup
	if value.expires.Before(time.Now()) {
		m.store.Remove(key)
		return *new(T), false
	}

	return value.value, true
}

// GetByValue retrieves a value by value
func (m *ExpiryMap[T]) GetByValue(val T) (key string, value T, ok bool) {
	for key, v := range m.store.GetAll() {
		if reflect.DeepEqual(v.value, val) {
			// check if expired
			if v.expires.Before(time.Now()) {
				m.store.Remove(key)
				break
			}
			return key, v.value, true
		}
	}
	return "", *new(T), false
}

// Remove explicitly removes a key
func (m *ExpiryMap[T]) Remove(key string) {
	m.store.Remove(key)
}

// RemovebyValue removes a value by value
func (m *ExpiryMap[T]) RemovebyValue(value T) (T, bool) {
	for key, val := range m.store.GetAll() {
		if reflect.DeepEqual(val.value, value) {
			m.store.Remove(key)
			return val.value, true
		}
	}
	return *new(T), false
}

// startCleaner runs the background cleanup process
func (m *ExpiryMap[T]) startCleaner() {
	go func() {
		tick := time.Tick(m.cleanupInterval)
		for range tick {
			m.cleanup()
		}
	}()
}

// cleanup removes all expired entries
func (m *ExpiryMap[T]) cleanup() {
	now := time.Now()
	for key, val := range m.store.GetAll() {
		if val.expires.Before(now) {
			m.store.Remove(key)
		}
	}
}
