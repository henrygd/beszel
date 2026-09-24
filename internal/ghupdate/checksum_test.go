package ghupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyAssetChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "asset")
	if err := os.WriteFile(path, []byte("beszel release asset"), 0600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		digest  string
		wantErr string
	}{
		{
			name:   "valid",
			digest: "sha256:2316f86af2c3af2f0ef595ad5359cdd19b329d4829a5425460ca9ffbf92671ab",
		},
		{
			name:    "mismatch",
			digest:  "sha256:0316f86af2c3af2f0ef595ad5359cdd19b329d4829a5425460ca9ffbf92671ab",
			wantErr: "checksum mismatch",
		},
		{
			name:    "malformed",
			digest:  "sha256:not-a-checksum",
			wantErr: "invalid SHA-256",
		},
		{
			name:    "missing",
			wantErr: "invalid release digest",
		},
		{
			name:    "unsupported algorithm",
			digest:  "sha512:2316f86af2c3af2f0ef595ad5359cdd19b329d4829a5425460ca9ffbf92671ab",
			wantErr: "unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyAssetChecksum(path, tt.digest)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("expected checksum to verify, got %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
