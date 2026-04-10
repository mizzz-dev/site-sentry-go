package db

import (
	"fmt"
	"os/exec"
)

func Migrate(dbPath string) error {
	schema := `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS monitors (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  url TEXT NOT NULL,
  interval_seconds INTEGER NOT NULL,
  timeout_seconds INTEGER NOT NULL,
  is_enabled INTEGER NOT NULL DEFAULT 1,
  last_status TEXT,
  last_status_code INTEGER,
  last_response_time_ms INTEGER,
  last_checked_at TEXT,
  consecutive_failures INTEGER NOT NULL DEFAULT 0,
  last_error_message TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS check_results (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  monitor_id INTEGER NOT NULL,
  status TEXT NOT NULL,
  status_code INTEGER,
  response_time_ms INTEGER NOT NULL,
  error_message TEXT,
  checked_at TEXT NOT NULL,
  FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_check_results_monitor_checked_at ON check_results(monitor_id, checked_at DESC);
`
	cmd := exec.Command("sqlite3", dbPath, schema)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sqlite migrate failed: %w (%s)", err, string(out))
	}
	return nil
}
