package ghupdate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func verifyAssetChecksum(path, digest string) error {
	algorithm, expectedHex, ok := strings.Cut(digest, ":")
	if !ok || algorithm == "" || expectedHex == "" {
		return fmt.Errorf("invalid release digest %q", digest)
	}
	if !strings.EqualFold(algorithm, "sha256") {
		return fmt.Errorf("unsupported release digest algorithm %q", algorithm)
	}

	expected, err := hex.DecodeString(expectedHex)
	if err != nil || len(expected) != sha256.Size {
		return fmt.Errorf("invalid SHA-256 release digest %q", digest)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open release for checksum verification: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to calculate release checksum: %w", err)
	}
	actual := hash.Sum(nil)
	if !bytes.Equal(actual, expected) {
		return fmt.Errorf("release checksum mismatch: expected %s, got %s", expectedHex, hex.EncodeToString(actual))
	}

	return nil
}
