package workspace

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestProofreadingProjectReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p := ProofreadingProject{ID: "task", Title: "书", Directory: "/book", Payload: []byte{0, 1, 255}}
	if err = s.SaveProofreadingProject(p); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, found, err := s.LoadProofreadingProject("task")
	if err != nil || !found || !bytes.Equal(got.Payload, p.Payload) {
		t.Fatal(got, found, err)
	}
	list, err := s.ListProofreadingProjects()
	if err != nil || len(list) != 1 || list[0].Payload != nil {
		t.Fatal(list, err)
	}
}

func TestProofreadingRemoteUpdateDoesNotOverwriteWorkingPayload(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := ProofreadingProject{ID: "task", Payload: []byte("newer progress")}
	if err = s.SaveProofreadingProject(p); err != nil {
		t.Fatal(err)
	}
	if err = s.SetProofreadingRemote("task", "ssh://git@example.com/book"); err != nil {
		t.Fatal(err)
	}
	p.Payload = []byte("latest progress")
	if err = s.SaveProofreadingProject(p); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.LoadProofreadingProject("task")
	if err != nil || string(got.Payload) != "latest progress" || got.RemoteURL != "ssh://git@example.com/book" {
		t.Fatal(got, err)
	}
}
