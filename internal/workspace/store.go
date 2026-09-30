// Package workspace stores recoverable document sessions and workspace metadata.
package workspace

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const RecentTabID = "recent"

type Store struct{ db *sql.DB }

type Tab struct {
	ID         string
	Kind       string
	Title      string
	DocumentID string
	SourcePath string
	Position   int
}

type Recent struct {
	Path     string
	Kind     string
	OpenedAt time.Time
}

type Document struct {
	ID        string
	Format    string
	Payload   []byte
	ViewState []byte
	Dirty     bool
	UpdatedAt time.Time
}

type Snapshot struct {
	ActiveTabID string
	Tabs        []Tab
	Recents     []Recent
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("workspace database path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create workspace database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open workspace database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA foreign_keys = ON`); err == nil {
		_, err = db.Exec(`PRAGMA busy_timeout = 5000`)
	}
	if err == nil {
		err = migrate(db)
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize workspace database: %w", err)
	}
	return &Store{db: db}, nil
}

// DefaultDatabasePath returns the per-user location used for durable workspace state.
func DefaultDatabasePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(dir, "Glance Office", "workspace.db"), nil
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > 3 {
		return fmt.Errorf("workspace database version %d is newer than supported version 3", version)
	}
	if version == 0 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		for _, q := range []string{
			`CREATE TABLE documents (id TEXT PRIMARY KEY, format TEXT NOT NULL, payload BLOB NOT NULL, dirty INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
			`CREATE TABLE tabs (id TEXT PRIMARY KEY, kind TEXT NOT NULL, title TEXT NOT NULL, document_id TEXT NOT NULL DEFAULT '', source_path TEXT NOT NULL DEFAULT '', position INTEGER NOT NULL UNIQUE)`,
			`CREATE TABLE recents (path TEXT PRIMARY KEY, kind TEXT NOT NULL, opened_at INTEGER NOT NULL)`,
			`CREATE TABLE workspace_settings (singleton INTEGER PRIMARY KEY CHECK(singleton = 1), active_tab_id TEXT NOT NULL)`,
			`INSERT INTO workspace_settings(singleton, active_tab_id) VALUES (1, 'recent')`,
			`PRAGMA user_version = 1`,
		} {
			if _, err := tx.Exec(q); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		version = 1
	}
	if version == 1 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`ALTER TABLE documents ADD COLUMN view_state BLOB NOT NULL DEFAULT X''`); err != nil {
			return err
		}
		if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		version = 2
	}
	if version == 2 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`CREATE TABLE favorites (path TEXT PRIMARY KEY)`); err != nil {
			return err
		}
		if _, err := tx.Exec(`PRAGMA user_version = 3`); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Load() (Snapshot, error) {
	out := Snapshot{}
	if err := s.db.QueryRow(`SELECT active_tab_id FROM workspace_settings WHERE singleton=1`).Scan(&out.ActiveTabID); err != nil {
		return out, err
	}
	rows, err := s.db.Query(`SELECT id, kind, title, document_id, source_path, position FROM tabs ORDER BY position, id`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var t Tab
		if err := rows.Scan(&t.ID, &t.Kind, &t.Title, &t.DocumentID, &t.SourcePath, &t.Position); err != nil {
			rows.Close()
			return out, err
		}
		out.Tabs = append(out.Tabs, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	if err := rows.Close(); err != nil {
		return out, err
	}
	rows, err = s.db.Query(`SELECT path, kind, opened_at FROM recents ORDER BY opened_at DESC`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r Recent
		var opened int64
		if err := rows.Scan(&r.Path, &r.Kind, &opened); err != nil {
			return out, err
		}
		r.OpenedAt = time.Unix(0, opened).UTC()
		out.Recents = append(out.Recents, r)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceWorkspace(w Snapshot) error {
	activeExists := w.ActiveTabID == "" || w.ActiveTabID == RecentTabID
	seen := make(map[string]bool, len(w.Tabs))
	for _, tab := range w.Tabs {
		if tab.ID == w.ActiveTabID {
			activeExists = true
		}
		if seen[tab.ID] {
			return fmt.Errorf("duplicate workspace tab id %q", tab.ID)
		}
		seen[tab.ID] = true
	}
	if !activeExists {
		return fmt.Errorf("active workspace tab %q does not exist", w.ActiveTabID)
	}
	active := w.ActiveTabID
	if active == "" {
		active = RecentTabID
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM tabs`); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM recents`); err != nil {
		return err
	}
	for _, tab := range w.Tabs {
		if _, err = tx.Exec(`INSERT INTO tabs(id,kind,title,document_id,source_path,position) VALUES(?,?,?,?,?,?)`, tab.ID, tab.Kind, tab.Title, tab.DocumentID, tab.SourcePath, tab.Position); err != nil {
			return err
		}
	}
	for _, recent := range w.Recents {
		if _, err = tx.Exec(`INSERT INTO recents(path,kind,opened_at) VALUES(?,?,?)`, recent.Path, recent.Kind, recent.OpenedAt.UTC().UnixNano()); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`UPDATE workspace_settings SET active_tab_id=? WHERE singleton=1`, active); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveDocument(doc Document) error {
	if doc.ID == "" || doc.Format == "" {
		return errors.New("document id and format are required")
	}
	dirty := 0
	if doc.Dirty {
		dirty = 1
	}
	if doc.Payload == nil {
		doc.Payload = []byte{}
	}
	if doc.ViewState == nil {
		doc.ViewState = []byte{}
	}
	_, err := s.db.Exec(`INSERT INTO documents(id,format,payload,dirty,updated_at,view_state) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET format=excluded.format,payload=excluded.payload,dirty=excluded.dirty,updated_at=excluded.updated_at,view_state=excluded.view_state`, doc.ID, doc.Format, doc.Payload, dirty, doc.UpdatedAt.UTC().UnixNano(), doc.ViewState)
	return err
}

// SaveRecents replaces the recent list in one transaction. It is intended for
// background folder scans that discover many supported files at once.
func (s *Store) SaveRecents(recents []Recent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM recents`); err != nil {
		return err
	}
	for _, recent := range recents {
		if recent.Path == "" {
			continue
		}
		openedAt := recent.OpenedAt
		if openedAt.IsZero() {
			openedAt = time.Now().UTC()
		}
		if _, err := tx.Exec(`INSERT INTO recents(path,kind,opened_at) VALUES(?,?,?)`, recent.Path, recent.Kind, openedAt.UTC().UnixNano()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LoadDocument(id string) (Document, bool, error) {
	var doc Document
	var dirty int
	var updated int64
	err := s.db.QueryRow(`SELECT id,format,payload,dirty,updated_at,view_state FROM documents WHERE id=?`, id).Scan(&doc.ID, &doc.Format, &doc.Payload, &dirty, &updated, &doc.ViewState)
	if errors.Is(err, sql.ErrNoRows) {
		return doc, false, nil
	}
	if err != nil {
		return doc, false, err
	}
	doc.Dirty = dirty != 0
	doc.UpdatedAt = time.Unix(0, updated).UTC()
	return doc, true, nil
}

func (s *Store) DeleteDocument(id string) error {
	_, err := s.db.Exec(`DELETE FROM documents WHERE id=?`, id)
	return err
}

func (s *Store) SaveViewState(id string, state []byte) error {
	if state == nil {
		state = []byte{}
	}
	result, err := s.db.Exec(`UPDATE documents SET view_state=? WHERE id=?`, state, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("workspace document %q does not exist", id)
	}
	return nil
}

func (s *Store) DeleteRecent(path string) error {
	_, err := s.db.Exec(`DELETE FROM recents WHERE path=?`, path)
	return err
}

func (s *Store) LoadFavorites() ([]string, error) {
	rows, err := s.db.Query(`SELECT path FROM favorites ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var favorites []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		favorites = append(favorites, path)
	}
	return favorites, rows.Err()
}

func (s *Store) SaveFavorite(path string, favorite bool) error {
	if path == "" {
		return errors.New("favorite path is required")
	}
	if favorite {
		_, err := s.db.Exec(`INSERT INTO favorites(path) VALUES (?) ON CONFLICT(path) DO NOTHING`, path)
		return err
	}
	_, err := s.db.Exec(`DELETE FROM favorites WHERE path=?`, path)
	return err
}
