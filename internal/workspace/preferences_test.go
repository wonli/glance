package workspace

import (
	"path/filepath"
	"testing"
)

func TestPreferencesSurviveReopenAndUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.Preference("ai.sidebar.width_dp"); err != nil || found {
		t.Fatal("new preference must be absent")
	}
	if err := s.SetPreference("ai.sidebar.width_dp", "320"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPreference("ai.sidebar.width_dp", "380.5"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	value, found, err := s.Preference("ai.sidebar.width_dp")
	if err != nil || !found || value != "380.5" {
		t.Fatalf("reopened preference=%q found=%v err=%v", value, found, err)
	}
}

func TestPreferencesMigrateVersionFiveWithoutLosingDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDocument(Document{ID: "keep", Format: "txt", Payload: []byte("preserved"), Dirty: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP TABLE preferences`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP TABLE home_document_folders`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP TABLE home_folders`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 5`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	doc, found, err := s.LoadDocument("keep")
	if err != nil || !found || string(doc.Payload) != "preserved" || !doc.Dirty {
		t.Fatal("migration changed recovery document")
	}
	if err := s.SetPreference("ai.sidebar.width_dp", "340"); err != nil {
		t.Fatal(err)
	}
}
