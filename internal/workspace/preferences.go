package workspace

import (
	"database/sql"
	"errors"
	"time"
)

const PreferenceMCPServerPort = "mcp.server.port"

// Preference returns a user's personalization value independently of document state.
func (s *Store) Preference(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM preferences WHERE key=?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return value, err == nil, err
}

func (s *Store) SetPreference(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO preferences(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, value, time.Now().UTC().UnixNano())
	return err
}
