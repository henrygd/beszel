package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
)

const fingerprintFileName = "fingerprint"

// knownBadUUID is a commonly known "product_uuid" that is not unique across systems.
const knownBadUUID = "03000200-0400-0500-0006-000700080009"

// GetFingerprint returns the agent fingerprint. It first tries to read a saved
// fingerprint from the data directory. If not found (or dataDir is empty), it
// generates one from system properties. The hostname and cpuModel parameters are
// used as fallback material if host.HostID() fails. If either is empty, they
// are fetched from the system automatically.
//
// If a new fingerprint is generated and a dataDir is provided, it is saved.
func GetFingerprint(dataDir, hostname, cpuModel string) string {
	if dataDir != "" {
		if fp, err := readFingerprint(dataDir); err == nil {
			return fp
		}
	}
	fp := generateFingerprint(hostname, cpuModel)
	if dataDir != "" {
		_ = SaveFingerprint(dataDir, fp)
	}
	return fp
}

// generateFingerprint creates a fingerprint from system properties.
// It tries host.HostID() first, falling back to hostname + cpuModel.
// If hostname or cpuModel are empty, they are fetched from the system.
func generateFingerprint(hostname, cpuModel string) string {
	fingerprint, err := host.HostID()
	if err != nil || fingerprint == "" || fingerprint == knownBadUUID {
		if hostname == "" {
			hostname, _ = os.Hostname()
		}
		if cpuModel == "" {
			if info, err := cpu.Info(); err == nil && len(info) > 0 {
				cpuModel = info[0].ModelName
			}
		}
		fingerprint = hostname + cpuModel
	}

	sum := sha256.Sum256([]byte(fingerprint))
	return hex.EncodeToString(sum[:24])
}

// readFingerprint reads the saved fingerprint from the data directory.
func readFingerprint(dataDir string) (string, error) {
	fp, err := os.ReadFile(filepath.Join(dataDir, fingerprintFileName))
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(fp))
	if s == "" {
		return "", errors.New("fingerprint file is empty")
	}
	return s, nil
}

// SaveFingerprint writes the fingerprint to the data directory.
func SaveFingerprint(dataDir, fingerprint string) error {
	return os.WriteFile(filepath.Join(dataDir, fingerprintFileName), []byte(fingerprint), 0o644)
}

// DeleteFingerprint removes the saved fingerprint file from the data directory.
// Returns nil if the file does not exist (idempotent).
func DeleteFingerprint(dataDir string) error {
	err := os.Remove(filepath.Join(dataDir, fingerprintFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
