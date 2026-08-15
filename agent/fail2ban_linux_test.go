//go:build linux

package agent

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFail2banDB(t *testing.T, rows []struct {
	jail, ip           string
	timeofban, bantime int64
}) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fail2ban.sqlite3")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE bans (
		ip TEXT, timeofban INTEGER, bantime INTEGER,
		bancount INTEGER, data BLOB, jail TEXT
	)`)
	require.NoError(t, err)
	for _, row := range rows {
		_, err = db.Exec(
			`INSERT INTO bans (jail, ip, timeofban, bantime) VALUES (?, ?, ?, ?)`,
			row.jail, row.ip, row.timeofban, row.bantime,
		)
		require.NoError(t, err)
	}
	return dbPath
}

func TestNewFail2banManager_AbsentFile(t *testing.T) {
	t.Setenv("FAIL2BAN_DB", "/nonexistent/path/fail2ban.sqlite3")
	assert.Nil(t, newFail2banManager())
}

func TestNewFail2banManager_PresentFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fail2ban.sqlite3")
	f, err := os.Create(dbPath)
	require.NoError(t, err)
	f.Close()
	t.Setenv("FAIL2BAN_DB", dbPath)
	mgr := newFail2banManager()
	require.NotNil(t, mgr)
	assert.Equal(t, dbPath, mgr.dbPath)
}

func TestGetBannedCounts_ActiveBans(t *testing.T) {
	now := time.Now().Unix()
	dbPath := setupFail2banDB(t, []struct {
		jail, ip           string
		timeofban, bantime int64
	}{
		{"sshd", "1.2.3.4", now - 60, 3600},    // active: expires in ~59 min
		{"sshd", "5.6.7.8", now - 60, 3600},    // active
		{"nginx", "9.10.11.12", now - 60, 3600}, // active, different jail
		{"sshd", "1.2.3.9", now - 7200, 3600},  // expired
	})
	mgr := &fail2banManager{dbPath: dbPath}
	counts := mgr.getBannedCounts()
	require.NotNil(t, counts)
	assert.Equal(t, uint32(2), counts["sshd"])
	assert.Equal(t, uint32(1), counts["nginx"])
}

func TestGetBannedCounts_PermanentBans(t *testing.T) {
	now := time.Now().Unix()
	dbPath := setupFail2banDB(t, []struct {
		jail, ip           string
		timeofban, bantime int64
	}{
		{"sshd", "1.2.3.4", now - 999999, -1}, // permanent ban (bantime < 0)
	})
	mgr := &fail2banManager{dbPath: dbPath}
	counts := mgr.getBannedCounts()
	require.NotNil(t, counts)
	assert.Equal(t, uint32(1), counts["sshd"])
}

func TestGetBannedCounts_NoBans(t *testing.T) {
	dbPath := setupFail2banDB(t, nil)
	mgr := &fail2banManager{dbPath: dbPath}
	assert.Nil(t, mgr.getBannedCounts())
}

func TestGetBannedCounts_AllExpired(t *testing.T) {
	now := time.Now().Unix()
	dbPath := setupFail2banDB(t, []struct {
		jail, ip           string
		timeofban, bantime int64
	}{
		{"sshd", "1.2.3.4", now - 7200, 3600}, // expired
	})
	mgr := &fail2banManager{dbPath: dbPath}
	assert.Nil(t, mgr.getBannedCounts())
}

func TestGetBannedCounts_DeduplicatesIPs(t *testing.T) {
	now := time.Now().Unix()
	dbPath := setupFail2banDB(t, []struct {
		jail, ip           string
		timeofban, bantime int64
	}{
		{"sshd", "1.2.3.4", now - 60, 3600},
		{"sshd", "1.2.3.4", now - 30, 3600}, // same IP, counted once
	})
	mgr := &fail2banManager{dbPath: dbPath}
	counts := mgr.getBannedCounts()
	require.NotNil(t, counts)
	assert.Equal(t, uint32(1), counts["sshd"])
}
