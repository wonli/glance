package fontcatalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func testFont(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, goregular.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFamilyAliasesAndAspects(t *testing.T) {
	path := testFont(t, "test.ttf")
	faces, err := Inspect(context.Background(), path, System)
	if err != nil {
		t.Fatal(err)
	}
	if len(faces) != 1 || faces[0].Family != "Go" {
		t.Fatalf("faces = %+v", faces)
	}
	faces[0].Aliases = append(faces[0].Aliases, "测试字体")
	snapshot := NewSnapshot(1, faces)
	if got := snapshot.Search("测试"); len(got) != 1 || got[0].Name != "Go" {
		t.Fatalf("search = %+v", got)
	}
	resolved, err := snapshot.Resolve(Request{Family: "测试字体", Weight: 400, Runes: []rune("Hello")})
	if err != nil || resolved.Face.ID != faces[0].ID || resolved.Substitute {
		t.Fatalf("resolve = %+v, %v", resolved, err)
	}
}

func TestSnapshotIsolation(t *testing.T) {
	faces, _ := Inspect(context.Background(), testFont(t, "test.ttf"), Private)
	snapshot := NewSnapshot(7, faces)
	faces[0].Family = "changed"
	families := snapshot.Families()
	families[0].Name = "changed"
	families[0].Aliases[0] = "changed"
	if got := snapshot.Families(); got[0].Name != "Go" || got[0].Aliases[0] == "changed" {
		t.Fatalf("mutable snapshot: %+v", got)
	}
}

func TestSameNameDifferentContent(t *testing.T) {
	faces, _ := Inspect(context.Background(), testFont(t, "test.ttf"), System)
	other := faces[0]
	other.ID = "private-different-content"
	other.Source = Private
	snapshot := NewSnapshot(1, append(faces, other))
	if got := snapshot.Families(); len(got) != 1 || len(got[0].Faces) != 2 {
		t.Fatalf("families = %+v", got)
	}
	resolved, err := snapshot.Resolve(Request{Family: "Go", Weight: 400})
	if err != nil || resolved.Face.Source != System {
		t.Fatalf("resolve = %+v %v", resolved, err)
	}
}

func TestCollectionFaceIdentity(t *testing.T) {
	snapshot := NewSnapshot(1, []Face{{ID: "a#0", Family: "Example", Index: 0}, {ID: "a#1", Family: "Example", Index: 1, Weight: 700}})
	got, err := snapshot.Resolve(Request{Family: "Example", Weight: 700})
	if err != nil || got.Face.Index != 1 {
		t.Fatalf("resolve = %+v %v", got, err)
	}
}

func TestPreferredSourceResolvesConflict(t *testing.T) {
	s := NewSnapshot(1, []Face{{ID: "system", Family: "Example", Source: System, Weight: 400}, {ID: "private", Family: "Example", Source: Private, Weight: 400}})
	s, err := s.Prefer("private")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Resolve(Request{Family: "Example", Weight: 400})
	if err != nil || r.Face.ID != "private" {
		t.Fatalf("preferred source ignored: %+v %v", r, err)
	}
}

func TestResolveWithAliasFallback(t *testing.T) {
	// Snapshot has Arial and PingFang SC, but not Calibri.
	s := NewSnapshot(1, []Face{
		{ID: "cjk", Family: "PingFang SC", Source: System, Weight: 400},
		{ID: "western", Family: "Arial", Source: System, Weight: 400},
	})
	// Requesting Calibri should prefer Arial over PingFang SC via alias candidates.
	r, err := s.Resolve(Request{Family: "Calibri", Weight: 400})
	if err != nil {
		t.Fatal(err)
	}
	if r.Face.Family != "Arial" {
		t.Fatalf("Resolve(Calibri) = %q; want Arial alias substitute", r.Face.Family)
	}
	if !r.Substitute {
		t.Fatalf("Resolve(Calibri) Substitute = false; want true")
	}
}
