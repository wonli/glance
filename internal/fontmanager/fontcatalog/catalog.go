// Package fontcatalog owns format-neutral font metadata and immutable catalogs.
package fontcatalog

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Source string

const (
	System  Source = "system"
	Private Source = "private"
	Online  Source = "online"
)

type ResourceID string

type Face struct {
	ID          ResourceID
	Family      string
	Aliases     []string
	Path        string
	Index       uint32
	Weight      int
	Italic      bool
	Source      Source
	Size        int64
	ModTime     int64
	Digest      string
	Preferred   bool
	Unavailable string `json:",omitempty"`
}
type Family struct {
	Name    string
	Aliases []string
	Faces   []Face
}
type Diagnostic struct{ Path, Message string }
type Snapshot struct {
	Version   uint64
	CheckedAt time.Time
	faces     []Face
}

func cloneFace(f Face) Face { f.Aliases = append([]string(nil), f.Aliases...); return f }
func NewSnapshot(version uint64, faces []Face) Snapshot {
	s := Snapshot{Version: version, faces: make([]Face, len(faces))}
	for i, f := range faces {
		s.faces[i] = cloneFace(f)
	}
	sort.SliceStable(s.faces, func(i, j int) bool {
		a, b := s.faces[i], s.faces[j]
		if a.Preferred != b.Preferred {
			return a.Preferred
		}
		if a.Source != b.Source {
			return a.Source == System
		}
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Index < b.Index
	})
	return s
}
func (s Snapshot) Faces() []Face {
	out := make([]Face, len(s.faces))
	for i, f := range s.faces {
		out[i] = cloneFace(f)
	}
	return out
}
func (s Snapshot) Families() []Family { return s.Search("") }
func (s Snapshot) Search(query string) []Family {
	query = strings.ToLower(strings.TrimSpace(query))
	var out []Family
	indices := map[string]int{}
	for _, f := range s.faces {
		key := strings.ToLower(f.Family)
		i, ok := indices[key]
		if !ok {
			i = len(out)
			indices[key] = i
			out = append(out, Family{Name: f.Family, Aliases: []string{f.Family}})
		}
		out[i].Faces = append(out[i].Faces, cloneFace(f))
		for _, alias := range f.Aliases {
			found := false
			for _, v := range out[i].Aliases {
				if v == alias {
					found = true
					break
				}
			}
			if !found {
				out[i].Aliases = append(out[i].Aliases, alias)
			}
		}
	}
	filtered := out[:0]
	for _, f := range out {
		for _, name := range f.Aliases {
			if strings.Contains(strings.ToLower(name), query) {
				filtered = append(filtered, f)
				break
			}
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name) })
	return filtered
}

// Prefer binds a local rendering preference without rewriting document names.
func (s Snapshot) Prefer(id ResourceID) (Snapshot, error) {
	var chosen *Face
	for _, f := range s.faces {
		if f.ID == id && f.Unavailable == "" {
			v := f
			chosen = &v
			break
		}
	}
	if chosen == nil {
		return s, fmt.Errorf("font resource unavailable: %s", id)
	}
	faces := s.Faces()
	for i := range faces {
		f := &faces[i]
		if strings.EqualFold(f.Family, chosen.Family) {
			f.Preferred = f.Source == chosen.Source && (chosen.Source == System || f.Path == chosen.Path)
		}
	}
	next := NewSnapshot(s.Version, faces)
	next.CheckedAt = s.CheckedAt
	return next, nil
}
