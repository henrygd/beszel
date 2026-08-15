//go:build linux

package agent

import (
	"database/sql"
	"log/slog"
	"os"

	"github.com/henrygd/beszel/agent/utils"
	_ "modernc.org/sqlite"
)

type fail2banManager struct {
	dbPath string
}

func newFail2banManager() *fail2banManager {
	path, _ := utils.GetEnv("FAIL2BAN_DB")
	if path == "" {
		path = "/var/lib/fail2ban/fail2ban.sqlite3"
	}
	if _, err := os.Stat(path); err != nil {
		slog.Debug("Fail2ban DB not found", "path", path)
		return nil
	}
	slog.Info("Fail2ban", "db", path)
	return &fail2banManager{dbPath: path}
}

func (m *fail2banManager) getBannedCounts() map[string]uint32 {
	db, err := sql.Open("sqlite", "file:"+m.dbPath+"?mode=ro")
	if err != nil {
		slog.Debug("Fail2ban DB open error", "err", err)
		return nil
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT jail, COUNT(DISTINCT ip) FROM bans
		 WHERE timeofban + bantime > unixepoch() OR bantime < 0
		 GROUP BY jail`,
	)
	if err != nil {
		slog.Debug("Fail2ban query error", "err", err)
		return nil
	}
	defer rows.Close()

	counts := make(map[string]uint32)
	for rows.Next() {
		var jail string
		var count uint32
		if err := rows.Scan(&jail, &count); err != nil {
			slog.Debug("Fail2ban scan error", "err", err)
			return nil
		}
		counts[jail] = count
	}
	if err := rows.Err(); err != nil {
		slog.Debug("Fail2ban rows error", "err", err)
		return nil
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}
