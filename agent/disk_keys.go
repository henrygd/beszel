package agent

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// fsKeysFile pins the chart keys of unnamed extra filesystems to stable
// filesystem IDs, so a disk keeps its history when the kernel gives it a
// different device name after a reboot (#294).
const fsKeysFile = "filesystems.json"

// fsKeyStore maps stable filesystem IDs to the chart key last used for them.
type fsKeyStore struct {
	path    string
	keys    map[string]string
	changed bool
}

// loadFsKeyStore reads the pinned keys from dataDir. Without a data dir the
// store still works for the current run but is never written.
func loadFsKeyStore(dataDir string) *fsKeyStore {
	s := &fsKeyStore{keys: make(map[string]string)}
	if dataDir == "" {
		return s
	}
	s.path = filepath.Join(dataDir, fsKeysFile)
	data, err := os.ReadFile(s.path)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s.keys); err != nil {
		slog.Warn("Ignoring invalid filesystem keys file", "path", s.path, "err", err)
		s.keys = make(map[string]string)
		s.changed = true
	}
	return s
}

// key returns the key pinned to id.
func (s *fsKeyStore) key(id string) (string, bool) {
	key, ok := s.keys[id]
	return key, ok
}

// pin assigns key to id. Another ID holding the same key belongs to a disk that
// is gone (present disks claim their keys first), so it releases the key.
func (s *fsKeyStore) pin(id, key string) {
	if s.keys[id] == key {
		return
	}
	for otherID, otherKey := range s.keys {
		if otherKey == key {
			slog.Info("Reassigning filesystem key", "key", key, "from", otherID, "to", id)
			delete(s.keys, otherID)
		}
	}
	s.keys[id] = key
	s.changed = true
}

// save writes the store if it changed, replacing the file atomically.
func (s *fsKeyStore) save() {
	if s.path == "" || !s.changed {
		return
	}
	data, err := json.MarshalIndent(s.keys, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		slog.Debug("Unable to save filesystem keys", "err", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		slog.Debug("Unable to save filesystem keys", "err", err)
		os.Remove(tmp)
		return
	}
	s.changed = false
}
