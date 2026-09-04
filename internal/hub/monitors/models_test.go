package monitors

import "testing"

func TestValidate_TimeoutMustBeLessThanInterval(t *testing.T) {
	m := Monitor{Name: "t", Type: TypeHTTP, Target: "https://example.com", IntervalSeconds: 60, TimeoutSeconds: 60, MaxRetries: 2}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error when timeout >= interval")
	}
}

func TestValidate_ValidMonitor(t *testing.T) {
	m := Monitor{Name: "t", Type: TypeHTTP, Target: "https://example.com", IntervalSeconds: 60, TimeoutSeconds: 10, MaxRetries: 2}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidate_UnknownType(t *testing.T) {
	m := Monitor{Name: "t", Type: "gopher", Target: "example.com", IntervalSeconds: 60, TimeoutSeconds: 10}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestValidate_IntervalBounds(t *testing.T) {
	for _, interval := range []int{0, 19, 86401} {
		m := Monitor{Name: "t", Type: TypePing, Target: "example.com", IntervalSeconds: interval, TimeoutSeconds: 10}
		if err := m.Validate(); err == nil {
			t.Fatalf("expected error for interval %d", interval)
		}
	}
}

func TestValidate_MaxRetriesBounds(t *testing.T) {
	for _, retries := range []int{-1, 11} {
		m := Monitor{Name: "t", Type: TypePing, Target: "example.com", IntervalSeconds: 60, TimeoutSeconds: 10, MaxRetries: retries}
		if err := m.Validate(); err == nil {
			t.Fatalf("expected error for max_retries %d", retries)
		}
	}
}
