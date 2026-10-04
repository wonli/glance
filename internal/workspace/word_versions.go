package workspace

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxWordVersionsPerPath  = 50
	maxWordVersionBytes     = int64(512 << 20)
	maxWordVersionBlobBytes = int64(512 << 20)
	wordVersionAutoInterval = 5 * time.Minute
)

var ErrWordVersionTooLarge = errors.New("Word version snapshot exceeds 512 MiB")

type WordVersionSource string

const (
	WordVersionOpened        WordVersionSource = "opened"
	WordVersionManualSave    WordVersionSource = "manual-save"
	WordVersionAutoSave      WordVersionSource = "auto-save"
	WordVersionRestoreBefore WordVersionSource = "restore-before"
)

type WordVersion struct {
	ID          int64
	PathKey     string
	DisplayPath string
	Source      WordVersionSource
	CreatedAt   time.Time
	SHA256      string
	SizeBytes   int64
}

type WordVersionInput struct {
	PathKey     string
	DisplayPath string
	Source      WordVersionSource
	CapturedAt  time.Time
	Snapshot    []byte
}

type wordVersionPolicy struct {
	maxVersions      int
	maxBytes         int64
	maxSnapshotBytes int64
	autoSaveInterval time.Duration
}

func defaultWordVersionPolicy() wordVersionPolicy {
	return wordVersionPolicy{
		maxVersions:      maxWordVersionsPerPath,
		maxBytes:         maxWordVersionBytes,
		maxSnapshotBytes: maxWordVersionBlobBytes,
		autoSaveInterval: wordVersionAutoInterval,
	}
}

func validateWordVersionSize(size int64) error {
	return validateWordVersionSizeLimit(size, maxWordVersionBlobBytes)
}

func validateWordVersionSizeLimit(size, limit int64) error {
	if size < 0 {
		return fmt.Errorf("Word version snapshot has negative size %d", size)
	}
	if size > limit {
		return ErrWordVersionTooLarge
	}
	return nil
}

// AddWordVersion stores one full DOCX package. The boolean is true only when
// a new content hash was inserted; duplicates and rate-limited autosaves
// return false without changing the timeline.
func (s *Store) AddWordVersion(input WordVersionInput) (WordVersion, bool, error) {
	if s == nil || s.db == nil {
		return WordVersion{}, false, errors.New("workspace store is unavailable")
	}
	input.PathKey = strings.TrimSpace(input.PathKey)
	input.DisplayPath = strings.TrimSpace(input.DisplayPath)
	if input.PathKey == "" {
		return WordVersion{}, false, errors.New("Word version path key is empty")
	}
	if input.DisplayPath == "" {
		input.DisplayPath = input.PathKey
	}
	if !validWordVersionSource(input.Source) {
		return WordVersion{}, false, fmt.Errorf("unsupported Word version source %q", input.Source)
	}

	policy := s.wordVersionPolicy
	if policy.maxVersions == 0 {
		policy = defaultWordVersionPolicy()
	}
	size := int64(len(input.Snapshot))
	if err := validateWordVersionSizeLimit(size, policy.maxSnapshotBytes); err != nil {
		return WordVersion{}, false, err
	}
	if size > policy.maxBytes {
		return WordVersion{}, false, fmt.Errorf("%w: snapshot is larger than this path's history limit", ErrWordVersionTooLarge)
	}
	if input.CapturedAt.IsZero() {
		input.CapturedAt = time.Now().UTC()
	} else {
		input.CapturedAt = input.CapturedAt.UTC()
	}
	digest := sha256.Sum256(input.Snapshot)
	hash := fmt.Sprintf("%x", digest)

	tx, err := s.db.Begin()
	if err != nil {
		return WordVersion{}, false, err
	}
	defer tx.Rollback()

	if existing, found, err := queryWordVersionByHash(tx, input.PathKey, hash); err != nil {
		return WordVersion{}, false, err
	} else if found {
		if err := tx.Commit(); err != nil {
			return WordVersion{}, false, err
		}
		return existing, false, nil
	}

	if input.Source == WordVersionAutoSave {
		var lastAutoSave sql.NullInt64
		err := tx.QueryRow(
			`SELECT MAX(created_at) FROM word_versions WHERE path_key = ? AND source = ?`,
			input.PathKey, WordVersionAutoSave,
		).Scan(&lastAutoSave)
		if err != nil {
			return WordVersion{}, false, err
		}
		if lastAutoSave.Valid && input.CapturedAt.Before(time.Unix(0, lastAutoSave.Int64).Add(policy.autoSaveInterval)) {
			if err := tx.Commit(); err != nil {
				return WordVersion{}, false, err
			}
			return WordVersion{}, false, nil
		}
	}

	result, err := tx.Exec(
		`INSERT INTO word_versions(path_key, display_path, source, created_at, sha256, size_bytes, payload)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		input.PathKey, input.DisplayPath, input.Source, input.CapturedAt.UnixNano(), hash, size, input.Snapshot,
	)
	if err != nil {
		return WordVersion{}, false, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return WordVersion{}, false, err
	}
	version := WordVersion{
		ID: id, PathKey: input.PathKey, DisplayPath: input.DisplayPath, Source: input.Source,
		CreatedAt: input.CapturedAt, SHA256: hash, SizeBytes: size,
	}
	if err := pruneWordVersions(tx, input.PathKey, id, policy); err != nil {
		return WordVersion{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return WordVersion{}, false, err
	}
	return version, true, nil
}

// ListWordVersions returns newest-first metadata without loading package BLOBs.
func (s *Store) ListWordVersions(pathKey string) ([]WordVersion, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("workspace store is unavailable")
	}
	pathKey = strings.TrimSpace(pathKey)
	if pathKey == "" {
		return nil, errors.New("Word version path key is empty")
	}
	rows, err := s.db.Query(
		`SELECT id, path_key, display_path, source, created_at, sha256, size_bytes
		 FROM word_versions WHERE path_key = ? ORDER BY created_at DESC, id DESC`,
		pathKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []WordVersion
	for rows.Next() {
		version, err := scanWordVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return versions, nil
}

// LoadWordVersion reads one full package only after the caller selects it.
func (s *Store) LoadWordVersion(pathKey string, id int64) (WordVersion, []byte, error) {
	if s == nil || s.db == nil {
		return WordVersion{}, nil, errors.New("workspace store is unavailable")
	}
	pathKey = strings.TrimSpace(pathKey)
	if pathKey == "" {
		return WordVersion{}, nil, errors.New("Word version path key is empty")
	}
	var version WordVersion
	var createdAt int64
	var payload []byte
	err := s.db.QueryRow(
		`SELECT id, path_key, display_path, source, created_at, sha256, size_bytes, payload
		 FROM word_versions WHERE path_key = ? AND id = ?`,
		pathKey, id,
	).Scan(
		&version.ID, &version.PathKey, &version.DisplayPath, &version.Source, &createdAt,
		&version.SHA256, &version.SizeBytes, &payload,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WordVersion{}, nil, fmt.Errorf("Word version %d was not found", id)
		}
		return WordVersion{}, nil, err
	}
	version.CreatedAt = time.Unix(0, createdAt).UTC()
	return version, payload, nil
}

type wordVersionScanner interface {
	Scan(dest ...any) error
}

func scanWordVersion(scanner wordVersionScanner) (WordVersion, error) {
	var version WordVersion
	var createdAt int64
	err := scanner.Scan(
		&version.ID, &version.PathKey, &version.DisplayPath, &version.Source, &createdAt,
		&version.SHA256, &version.SizeBytes,
	)
	if err != nil {
		return WordVersion{}, err
	}
	version.CreatedAt = time.Unix(0, createdAt).UTC()
	return version, nil
}

func queryWordVersionByHash(tx *sql.Tx, pathKey, hash string) (WordVersion, bool, error) {
	row := tx.QueryRow(
		`SELECT id, path_key, display_path, source, created_at, sha256, size_bytes
		 FROM word_versions WHERE path_key = ? AND sha256 = ?`,
		pathKey, hash,
	)
	version, err := scanWordVersion(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WordVersion{}, false, nil
		}
		return WordVersion{}, false, err
	}
	return version, true, nil
}

func pruneWordVersions(tx *sql.Tx, pathKey string, protectedID int64, policy wordVersionPolicy) error {
	for {
		var count int
		var totalBytes int64
		if err := tx.QueryRow(
			`SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM word_versions WHERE path_key = ?`,
			pathKey,
		).Scan(&count, &totalBytes); err != nil {
			return err
		}
		if count <= policy.maxVersions && totalBytes <= policy.maxBytes {
			return nil
		}
		var oldestID int64
		err := tx.QueryRow(
			`SELECT id FROM word_versions
			 WHERE path_key = ? AND id <> ?
			 ORDER BY created_at ASC, id ASC LIMIT 1`,
			pathKey, protectedID,
		).Scan(&oldestID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("Word version retention limit cannot keep the protected snapshot")
			}
			return err
		}
		if _, err := tx.Exec(`DELETE FROM word_versions WHERE path_key = ? AND id = ?`, pathKey, oldestID); err != nil {
			return err
		}
	}
}

func validWordVersionSource(source WordVersionSource) bool {
	switch source {
	case WordVersionOpened, WordVersionManualSave, WordVersionAutoSave, WordVersionRestoreBefore:
		return true
	default:
		return false
	}
}
