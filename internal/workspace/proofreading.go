package workspace

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// ProofreadingProject separates resumable working data from confirmed Git history.
type ProofreadingProject struct {
	ID                  string    `json:"id"`
	Title               string    `json:"title"`
	Directory           string    `json:"directory"`
	SourceA             string    `json:"source_a"`
	SourceAHash         string    `json:"source_a_hash,omitempty"`
	SourceAOriginalPath string    `json:"source_a_original_path,omitempty"`
	SourceB             string    `json:"source_b"`
	SourceBName         string    `json:"source_b_name,omitempty"`
	SourceBOriginalPath string    `json:"source_b_original_path,omitempty"`
	PageCount           int       `json:"page_count"`
	ReviewedCount       int       `json:"reviewed_count"`
	CurrentPage         int       `json:"current_page"`
	UpdatedAt           time.Time `json:"updated_at"`
	Payload             []byte    `json:"-"`
	RemoteURL           string    `json:"remote_url,omitempty"`
	CredentialRef       string    `json:"credential_ref,omitempty"`
}

func (s *Store) SaveProofreadingProject(p ProofreadingProject) error {
	if p.ID == "" {
		return errors.New("proofreading project ID is empty")
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now().UTC()
	}
	meta, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO proofreading_projects(id,metadata,payload,updated_at) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET metadata=json_set(CAST(excluded.metadata AS TEXT),'$.remote_url',COALESCE(json_extract(CAST(proofreading_projects.metadata AS TEXT),'$.remote_url'),'')),payload=excluded.payload,updated_at=excluded.updated_at`, p.ID, meta, p.Payload, p.UpdatedAt.UnixNano())
	return err
}
func (s *Store) LoadProofreadingProject(id string) (ProofreadingProject, bool, error) {
	var p ProofreadingProject
	var meta, payload []byte
	err := s.db.QueryRow(`SELECT metadata,payload FROM proofreading_projects WHERE id=?`, id).Scan(&meta, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	err = json.Unmarshal(meta, &p)
	p.Payload = payload
	return p, err == nil, err
}
func (s *Store) ListProofreadingProjects() ([]ProofreadingProject, error) {
	rows, err := s.db.Query(`SELECT metadata FROM proofreading_projects ORDER BY updated_at DESC,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ProofreadingProject
	for rows.Next() {
		var data []byte
		var p ProofreadingProject
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// SetProofreadingRemote updates only metadata owned by the remote settings form.
func (s *Store) SetProofreadingRemote(id, remote string) error {
	result, err := s.db.Exec(`UPDATE proofreading_projects SET metadata=json_set(CAST(metadata AS TEXT),'$.remote_url',?) WHERE id=?`, remote, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return errors.New("proofreading project not found")
	}
	return err
}

func (s *Store) LoadProofreadingProjectMetadata(id string) (ProofreadingProject, bool, error) {
	var p ProofreadingProject
	var meta []byte
	err := s.db.QueryRow(`SELECT metadata FROM proofreading_projects WHERE id=?`, id).Scan(&meta)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	err = json.Unmarshal(meta, &p)
	return p, err == nil, err
}

func (s *Store) DeleteProofreadingProject(id string) error {
	if id == "" {
		return errors.New("proofreading project ID is empty")
	}
	_, err := s.db.Exec(`DELETE FROM proofreading_projects WHERE id=?`, id)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`DELETE FROM preferences WHERE key=?`, "proofreading.pushed."+id)
	return nil
}
