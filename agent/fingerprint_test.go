//go:build testing
// +build testing

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFingerprint(t *testing.T) {
	t.Run("reads existing fingerprint from file", func(t *testing.T) {
		dir := t.TempDir()
		expected := "abc123def456"
		err := os.WriteFile(filepath.Join(dir, fingerprintFileName), []byte(expected), 0644)
		require.NoError(t, err)

		fp := GetFingerprint(dir, "", "")
		assert.Equal(t, expected, fp)
	})

	t.Run("trims whitespace from file", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, fingerprintFileName), []byte("  abc123  \n"), 0644)
		require.NoError(t, err)

		fp := GetFingerprint(dir, "", "")
		assert.Equal(t, "abc123", fp)
	})

	t.Run("generates fingerprint when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		fp := GetFingerprint(dir, "", "")
		assert.NotEmpty(t, fp)
	})

	t.Run("generates fingerprint when dataDir is empty", func(t *testing.T) {
		fp := GetFingerprint("", "", "")
		assert.NotEmpty(t, fp)
	})

	t.Run("generates consistent fingerprint for same inputs", func(t *testing.T) {
		fp1 := GetFingerprint("", "myhost", "mycpu")
		fp2 := GetFingerprint("", "myhost", "mycpu")
		assert.Equal(t, fp1, fp2)
	})

	t.Run("prefers saved fingerprint over generated", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, SaveFingerprint(dir, "saved-fp"))

		fp := GetFingerprint(dir, "anyhost", "anycpu")
		assert.Equal(t, "saved-fp", fp)
	})
}

func TestSaveFingerprint(t *testing.T) {
	t.Run("saves fingerprint to file", func(t *testing.T) {
		dir := t.TempDir()
		err := SaveFingerprint(dir, "abc123")
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, fingerprintFileName))
		require.NoError(t, err)
		assert.Equal(t, "abc123", string(content))
	})

	t.Run("overwrites existing fingerprint", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, SaveFingerprint(dir, "old"))
		require.NoError(t, SaveFingerprint(dir, "new"))

		content, err := os.ReadFile(filepath.Join(dir, fingerprintFileName))
		require.NoError(t, err)
		assert.Equal(t, "new", string(content))
	})
}

func TestDeleteFingerprint(t *testing.T) {
	t.Run("deletes existing fingerprint", func(t *testing.T) {
		dir := t.TempDir()
		fp := filepath.Join(dir, fingerprintFileName)
		err := os.WriteFile(fp, []byte("abc123"), 0644)
		require.NoError(t, err)

		err = DeleteFingerprint(dir)
		require.NoError(t, err)

		// Verify file is gone
		_, err = os.Stat(fp)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("no error when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		err := DeleteFingerprint(dir)
		assert.NoError(t, err)
	})
}
