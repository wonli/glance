// Package fontdownload manages validated fonts used only by Glance.
package fontdownload

import "context"

type Entry struct {
	ID, Family, Version, URL, SHA256, License string
	MaxBytes                                  int64
}
type Provider interface {
	List(context.Context) ([]Entry, error)
}
type Manifest []Entry

func (m Manifest) List(ctx context.Context) ([]Entry, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return append([]Entry(nil), m...), nil
}

// DefaultManifest intentionally has no unverified public font resources.
func DefaultManifest() Manifest { return nil }

type Progress struct{ Received, Total int64 }
