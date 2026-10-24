// Package workspace stores recoverable document sessions and workspace metadata.
package workspace

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const RecentTabID = "recent"

type Store struct {
	db                *sql.DB
	path              string
	sessionsDir       string
	tempSessionsDir   bool
	wordVersionPolicy wordVersionPolicy
}

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
	ID          string
	Format      string
	Payload     []byte
	PayloadSize int64
	ViewState   []byte
	Dirty       bool
	UpdatedAt   time.Time
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
	filePath := ""
	if path != ":memory:" && !strings.HasPrefix(strings.ToLower(path), "file:") {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve workspace database path: %w", err)
		}
		filePath = absPath
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
	sessionsDir := ""
	tempSessionsDir := false
	if filePath != "" {
		sessionsDir = filepath.Join(filepath.Dir(filePath), "sessions")
	} else {
		tempDir, err := os.MkdirTemp("", "glance-sessions-*")
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("create temporary sessions directory: %w", err)
		}
		sessionsDir = tempDir
		tempSessionsDir = true
	}
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		if tempSessionsDir {
			_ = os.RemoveAll(sessionsDir)
		}
		db.Close()
		return nil, fmt.Errorf("create sessions directory: %w", err)
	}
	store := &Store{
		db:                db,
		path:              filePath,
		sessionsDir:       sessionsDir,
		tempSessionsDir:   tempSessionsDir,
		wordVersionPolicy: defaultWordVersionPolicy(),
	}
	_ = store.CleanupOrphanedSessions()
	return store, nil
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
	if version > 8 {
		return fmt.Errorf("workspace database version %d is newer than supported version 8", version)
	}
	if version == 0 {
		_, _ = db.Exec(`PRAGMA auto_vacuum = INCREMENTAL`)
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
		version = 3
	}
	if version == 3 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, query := range []string{
			`CREATE TABLE word_versions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				path_key TEXT NOT NULL,
				display_path TEXT NOT NULL,
				source TEXT NOT NULL,
				created_at INTEGER NOT NULL,
				sha256 TEXT NOT NULL,
				size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
				payload BLOB NOT NULL,
				UNIQUE(path_key, sha256)
			)`,
			`CREATE INDEX word_versions_path_created ON word_versions(path_key, created_at DESC, id DESC)`,
			`PRAGMA user_version = 4`,
		} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		version = 4
	}
	if version == 4 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, query := range []string{
			`CREATE TABLE agent_conversations (
				id TEXT PRIMARY KEY,
				document_id TEXT NOT NULL,
				title TEXT NOT NULL DEFAULT '',
				messages BLOB NOT NULL DEFAULT X'',
				reasoning BLOB NOT NULL DEFAULT X'',
				created_at INTEGER NOT NULL,
				updated_at INTEGER NOT NULL
			)`,
			`CREATE INDEX agent_conversations_document_updated ON agent_conversations(document_id, updated_at DESC, id DESC)`,
			`PRAGMA user_version = 5`,
		} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		version = 5
	}
	if version == 5 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, query := range []string{`CREATE TABLE preferences (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL)`, `PRAGMA user_version = 6`} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		version = 6
	}
	if version == 6 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, query := range []string{
			`CREATE TABLE home_folders (
				id TEXT PRIMARY KEY NOT NULL,
				parent_id TEXT REFERENCES home_folders(id) ON DELETE CASCADE,
				name TEXT NOT NULL CHECK(length(trim(name)) > 0),
				sort_order INTEGER NOT NULL DEFAULT 0,
				source_path TEXT,
				CHECK (source_path IS NULL OR (parent_id IS NULL AND length(source_path) > 0))
			)`,
			`CREATE UNIQUE INDEX home_folders_sibling_name ON home_folders(IFNULL(parent_id, ''), name COLLATE NOCASE)`,
			`CREATE UNIQUE INDEX home_folders_source_path ON home_folders(source_path) WHERE source_path IS NOT NULL`,
			`CREATE INDEX home_folders_parent_sort ON home_folders(parent_id, sort_order, id)`,
			`CREATE TABLE home_document_folders (
				path TEXT PRIMARY KEY NOT NULL CHECK(path <> ''),
				folder_id TEXT NOT NULL REFERENCES home_folders(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX home_document_folders_folder_path ON home_document_folders(folder_id, path)`,
			`PRAGMA user_version = 7`,
		} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		version = 7
	}

	if version == 7 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS proofreading_projects(id TEXT PRIMARY KEY, metadata BLOB NOT NULL, payload BLOB, updated_at INTEGER NOT NULL)`); err != nil {
			return err
		}
		if _, err = tx.Exec(`PRAGMA user_version = 8`); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) Close() error {
	if s.tempSessionsDir && s.sessionsDir != "" {
		_ = os.RemoveAll(s.sessionsDir)
	}
	return s.db.Close()
}

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

func (s *Store) payloadPath(id string) string {
	if s == nil || s.sessionsDir == "" || id == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(id))
	hashHex := hex.EncodeToString(hash[:8])
	var safe strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			safe.WriteRune(r)
		}
	}
	base := safe.String()
	if len(base) > 48 {
		base = base[:48]
	}
	if base == "" {
		base = "doc"
	}
	return filepath.Join(s.sessionsDir, fmt.Sprintf("%s-%s.dat", base, hashHex))
}

func (s *Store) writePayloadFile(id string, data []byte) error {
	target := s.payloadPath(id)
	if target == "" {
		return errors.New("cannot determine document payload path")
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create sessions directory: %w", err)
	}
	tmpFile, err := os.CreateTemp(dir, ".tmp-session-*")
	if err != nil {
		return fmt.Errorf("create temporary session file: %w", err)
	}
	tmpPath := tmpFile.Name()
	var committed bool
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if len(data) > 0 {
		if _, err := tmpFile.Write(data); err != nil {
			_ = tmpFile.Close()
			return fmt.Errorf("write session file: %w", err)
		}
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync session file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close session file: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("commit session file: %w", err)
	}
	committed = true
	return nil
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
	if err := s.writePayloadFile(doc.ID, doc.Payload); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO documents(id,format,payload,dirty,updated_at,view_state) VALUES(?,?,X'',?,?,?) ON CONFLICT(id) DO UPDATE SET format=excluded.format,payload=X'',dirty=excluded.dirty,updated_at=excluded.updated_at,view_state=excluded.view_state`, doc.ID, doc.Format, dirty, doc.UpdatedAt.UTC().UnixNano(), doc.ViewState)
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

// ListRecents returns a bounded, newest-first page of recent files. The query
// is a literal substring match against the stored path; SQL wildcard
// characters supplied by the caller do not change its meaning.
func (s *Store) ListRecents(limit, offset int, kind, query string) ([]Recent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("workspace store is not open")
	}
	if limit < 1 || limit > 101 || offset < 0 || offset > 10000 {
		return nil, errors.New("recent list bounds are invalid")
	}
	pattern := "%"
	if query != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
		pattern = "%" + escaped + "%"
	}
	rows, err := s.db.Query(`SELECT path, kind, opened_at FROM recents
		WHERE (? = '' OR kind = ?)
		AND (? = '' OR path COLLATE NOCASE LIKE ? ESCAPE '\')
		ORDER BY opened_at DESC, path ASC LIMIT ? OFFSET ?`, kind, kind, query, pattern, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Recent, 0, limit)
	for rows.Next() {
		var recent Recent
		var openedAt int64
		if err := rows.Scan(&recent.Path, &recent.Kind, &openedAt); err != nil {
			return nil, err
		}
		recent.OpenedAt = time.Unix(0, openedAt).UTC()
		items = append(items, recent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) LoadDocument(id string) (Document, bool, error) {
	doc, found, err := s.LoadDocumentMetadata(id)
	if err != nil || !found {
		return doc, found, err
	}
	payload, err := s.LoadDocumentPayload(id)
	if err != nil {
		return doc, false, err
	}
	doc.Payload = payload
	doc.PayloadSize = int64(len(payload))
	return doc, true, nil
}

func (s *Store) LoadDocumentMetadata(id string) (Document, bool, error) {
	var doc Document
	var dirty int
	var updated int64
	var sqlitePayloadLen int64
	err := s.db.QueryRow(`SELECT id,format,length(payload),dirty,updated_at,view_state FROM documents WHERE id=?`, id).Scan(&doc.ID, &doc.Format, &sqlitePayloadLen, &dirty, &updated, &doc.ViewState)
	if errors.Is(err, sql.ErrNoRows) {
		return doc, false, nil
	}
	if err != nil {
		return doc, false, err
	}
	doc.Dirty = dirty != 0
	doc.UpdatedAt = time.Unix(0, updated).UTC()
	if sqlitePayloadLen > 0 {
		doc.PayloadSize = sqlitePayloadLen
	} else if target := s.payloadPath(id); target != "" {
		if fi, err := os.Stat(target); err == nil {
			doc.PayloadSize = fi.Size()
		}
	}
	return doc, true, nil
}

func (s *Store) LoadDocumentPayload(id string) ([]byte, error) {
	var sqlitePayload []byte
	err := s.db.QueryRow(`SELECT payload FROM documents WHERE id=?`, id).Scan(&sqlitePayload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(sqlitePayload) > 0 {
		return sqlitePayload, nil
	}
	target := s.payloadPath(id)
	if target == "" {
		return []byte{}, nil
	}
	data, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []byte{}, nil
		}
		return nil, err
	}
	return data, nil
}

func (s *Store) DeleteDocument(id string) error {
	_, err := s.db.Exec(`DELETE FROM documents WHERE id=?`, id)
	if target := s.payloadPath(id); target != "" {
		_ = os.Remove(target)
	}
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

func (s *Store) UpdateDocumentMetadata(id string, state []byte, dirty bool, updatedAt time.Time) error {
	if id == "" {
		return errors.New("workspace document id is required")
	}
	if state == nil {
		state = []byte{}
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	dirtyInt := 0
	if dirty {
		dirtyInt = 1
	}
	result, err := s.db.Exec(`UPDATE documents SET view_state=?,dirty=?,updated_at=? WHERE id=?`, state, dirtyInt, updatedAt.UTC().UnixNano(), id)
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

// LoadHomeFolderSnapshot loads and validates the complete logical folder tree
// and its one-folder-per-document assignments.
func (s *Store) LoadHomeFolderSnapshot() (HomeFolderSnapshot, error) {
	if s == nil || s.db == nil {
		return HomeFolderSnapshot{}, errors.New("workspace store is not open")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return HomeFolderSnapshot{}, err
	}
	defer tx.Rollback()

	snapshot := HomeFolderSnapshot{}
	rows, err := tx.Query(`SELECT id, parent_id, name, sort_order, source_path FROM home_folders`)
	if err != nil {
		return HomeFolderSnapshot{}, err
	}
	for rows.Next() {
		var folder HomeFolder
		var parentID, sourcePath sql.NullString
		if err := rows.Scan(&folder.ID, &parentID, &folder.Name, &folder.SortOrder, &sourcePath); err != nil {
			rows.Close()
			return HomeFolderSnapshot{}, err
		}
		if parentID.Valid {
			folder.ParentID = parentID.String
		}
		if sourcePath.Valid {
			folder.SourcePath = sourcePath.String
		}
		snapshot.Folders = append(snapshot.Folders, folder)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return HomeFolderSnapshot{}, err
	}
	if err := rows.Close(); err != nil {
		return HomeFolderSnapshot{}, err
	}

	rows, err = tx.Query(`SELECT path, folder_id FROM home_document_folders ORDER BY path`)
	if err != nil {
		return HomeFolderSnapshot{}, err
	}
	for rows.Next() {
		var assignment HomeFolderAssignment
		if err := rows.Scan(&assignment.Path, &assignment.FolderID); err != nil {
			rows.Close()
			return HomeFolderSnapshot{}, err
		}
		snapshot.Assignments = append(snapshot.Assignments, assignment)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return HomeFolderSnapshot{}, err
	}
	if err := rows.Close(); err != nil {
		return HomeFolderSnapshot{}, err
	}
	if err := ValidateHomeFolderSnapshot(snapshot); err != nil {
		return HomeFolderSnapshot{}, fmt.Errorf("workspace home folder data is invalid: %w", err)
	}
	snapshot.Folders = orderedHomeFolders(snapshot.Folders)
	if err := tx.Commit(); err != nil {
		return HomeFolderSnapshot{}, err
	}
	return snapshot, nil
}

// SaveHomeFolderSnapshot atomically replaces all persisted folder nodes and
// document assignments after validating the new snapshot.
func (s *Store) SaveHomeFolderSnapshot(snapshot HomeFolderSnapshot) error {
	if s == nil || s.db == nil {
		return errors.New("workspace store is not open")
	}
	if err := ValidateHomeFolderSnapshot(snapshot); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM home_document_folders`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM home_folders`); err != nil {
		return err
	}
	for _, folder := range orderedHomeFolders(snapshot.Folders) {
		var parentID, sourcePath any
		if folder.ParentID != "" {
			parentID = folder.ParentID
		}
		if folder.SourcePath != "" {
			sourcePath = folder.SourcePath
		}
		if _, err := tx.Exec(`INSERT INTO home_folders(id, parent_id, name, sort_order, source_path) VALUES (?, ?, ?, ?, ?)`, folder.ID, parentID, folder.Name, folder.SortOrder, sourcePath); err != nil {
			return fmt.Errorf("save home folder %q: %w", folder.ID, err)
		}
	}
	for _, assignment := range snapshot.Assignments {
		if _, err := tx.Exec(`INSERT INTO home_document_folders(path, folder_id) VALUES (?, ?)`, assignment.Path, assignment.FolderID); err != nil {
			return fmt.Errorf("save home folder assignment for %q: %w", assignment.Path, err)
		}
	}
	return tx.Commit()
}

// StoreDiagnostics captures durable database geometry and metrics for profiling.
type StoreDiagnostics struct {
	Path                      string `json:"path"`
	FileSizeBytes             int64  `json:"file_size_bytes"`
	WALSizeBytes              int64  `json:"wal_size_bytes"`
	PageSize                  int64  `json:"page_size"`
	PageCount                 int64  `json:"page_count"`
	FreelistCount             int64  `json:"freelist_count"`
	DocumentsCount            int    `json:"documents_count"`
	DocumentsPayloadBytes     int64  `json:"documents_payload_bytes"`
	WordVersionsCount         int    `json:"word_versions_count"`
	WordVersionsSnapshotBytes int64  `json:"word_versions_snapshot_bytes"`
	RecentsCount              int    `json:"recents_count"`
	OpenConnections           int    `json:"open_connections"`
}

// Diagnostics gathers database file sizes, SQLite pragma metrics, and document table counts.
func (s *Store) Diagnostics() (StoreDiagnostics, error) {
	if s == nil || s.db == nil {
		return StoreDiagnostics{}, errors.New("workspace store is not open")
	}
	var diag StoreDiagnostics
	diag.Path = s.path
	if s.path != "" && s.path != ":memory:" {
		if fi, err := os.Stat(s.path); err == nil {
			diag.FileSizeBytes = fi.Size()
		}
		if fi, err := os.Stat(s.path + "-wal"); err == nil {
			diag.WALSizeBytes = fi.Size()
		}
	}
	_ = s.db.QueryRow(`PRAGMA page_size`).Scan(&diag.PageSize)
	_ = s.db.QueryRow(`PRAGMA page_count`).Scan(&diag.PageCount)
	_ = s.db.QueryRow(`PRAGMA freelist_count`).Scan(&diag.FreelistCount)
	var sqlitePayloadBytes int64
	_ = s.db.QueryRow(`SELECT count(*), coalesce(sum(length(payload)), 0) FROM documents`).Scan(&diag.DocumentsCount, &sqlitePayloadBytes)
	diag.DocumentsPayloadBytes = sqlitePayloadBytes
	if s.sessionsDir != "" {
		if entries, err := os.ReadDir(s.sessionsDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".dat") {
					if fi, err := entry.Info(); err == nil {
						diag.DocumentsPayloadBytes += fi.Size()
					}
				}
			}
		}
	}
	var tableCount int
	_ = s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='word_versions'`).Scan(&tableCount)
	if tableCount > 0 {
		_ = s.db.QueryRow(`SELECT count(*), coalesce(sum(length(snapshot)), 0) FROM word_versions`).Scan(&diag.WordVersionsCount, &diag.WordVersionsSnapshotBytes)
	}
	_ = s.db.QueryRow(`SELECT count(*) FROM recents`).Scan(&diag.RecentsCount)
	diag.OpenConnections = s.db.Stats().OpenConnections
	return diag, nil
}

// ShrinkMemory advises SQLite to release freed memory back to the allocator.
func (s *Store) ShrinkMemory() error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`PRAGMA shrink_memory`)
	return err
}

// Vacuum reorganizes the database file to reclaim free space from deleted rows.
func (s *Store) Vacuum() error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`VACUUM`)
	return err
}

// CleanupOrphanedSessions deletes unreferenced .dat files and temporary sessions from the sessions directory.
func (s *Store) CleanupOrphanedSessions() error {
	if s == nil || s.db == nil || s.sessionsDir == "" {
		return nil
	}
	entries, err := os.ReadDir(s.sessionsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	rows, err := s.db.Query(`SELECT id FROM documents`)
	if err != nil {
		return err
	}
	defer rows.Close()
	valid := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil && id != "" {
			valid[s.payloadPath(id)] = true
		}
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		fullPath := filepath.Join(s.sessionsDir, name)
		if strings.HasPrefix(name, ".tmp-session-") || (strings.HasSuffix(name, ".dat") && !valid[fullPath]) {
			_ = os.Remove(fullPath)
		}
	}
	return nil
}

// AutoMaintainSpace checks whether dead pages in the freelist exceed maintenance thresholds
// and reorganizes the database to reclaim space. It also cleans up orphaned session files.
func (s *Store) AutoMaintainSpace() (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	_ = s.CleanupOrphanedSessions()
	if s.path == "" || s.path == ":memory:" {
		return false, nil
	}
	var freelist, pageCount int64
	if err := s.db.QueryRow(`PRAGMA freelist_count`).Scan(&freelist); err != nil {
		return false, err
	}
	if err := s.db.QueryRow(`PRAGMA page_count`).Scan(&pageCount); err != nil {
		return false, err
	}
	// Reclaim if freelist has more than 1000 pages (~4MB), or over 200 pages (~800KB) representing > 10% of total
	if freelist > 1000 || (freelist > 200 && pageCount > 0 && float64(freelist)/float64(pageCount) > 0.10) {
		return true, s.Vacuum()
	}
	return false, nil
}


