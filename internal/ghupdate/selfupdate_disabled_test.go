//go:build no_self_update

package ghupdate

import (
	"errors"
	"testing"
)

func TestUpdateDisabled(t *testing.T) {
	updated, err := Update(Config{ArchiveExecutable: "beszel", DataDir: t.TempDir()})
	if updated {
		t.Fatal("expected no update when self-update is disabled")
	}
	if !errors.Is(err, ErrSelfUpdateDisabled) {
		t.Fatalf("expected ErrSelfUpdateDisabled, got %v", err)
	}
}
