package fontcatalog

import (
	"bytes"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/go-text/typesetting/font"
)

type Request struct {
	Family string
	Weight int
	Italic bool
	Runes  []rune
}
type Resolution struct {
	RequestedFamily string
	Face            Face
	Substitute      bool
	Reason          string
}

func matchFamily(f Face, name string) bool {
	if strings.EqualFold(f.Family, name) {
		return true
	}
	for _, a := range f.Aliases {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}

func resolutionScore(f Face, req Request, weight int) int {
	score := 0
	if !matchFamily(f, req.Family) {
		score += 100000
	}
	distance := f.Weight - weight
	if distance < 0 {
		distance = -distance
	}
	score += distance
	if f.Italic != req.Italic {
		score += 1000
	}
	return score
}

func makeResolution(req Request, face Face) Resolution {
	face = cloneFace(face)
	substitute := !matchFamily(face, req.Family)
	reason := ""
	if substitute {
		reason = "requested family unavailable or lacks required characters"
	}
	return Resolution{RequestedFamily: req.Family, Face: face, Substitute: substitute, Reason: reason}
}

func (s Snapshot) Resolve(req Request) (Resolution, error) {
	weight := req.Weight
	if weight == 0 {
		weight = 400
	}
	if len(req.Runes) == 0 {
		// The common family-picker path only needs the best face. Keep the
		// snapshot's stable order for score ties without cloning and sorting all
		// faces on every selection.
		var best Face
		bestScore := 0
		found := false
		for _, face := range s.faces {
			if face.Unavailable != "" {
				continue
			}
			score := resolutionScore(face, req, weight)
			if !found || score < bestScore {
				best = face
				bestScore = score
				found = true
			}
		}
		if !found {
			return Resolution{}, errors.New("no matching usable font")
		}
		return makeResolution(req, best), nil
	}

	candidates := s.Faces()
	sort.SliceStable(candidates, func(i, j int) bool {
		return resolutionScore(candidates[i], req, weight) < resolutionScore(candidates[j], req, weight)
	})
	for _, f := range candidates {
		if f.Unavailable != "" {
			continue
		}
		if len(req.Runes) > 0 {
			r, e := s.OpenFace(f.ID)
			if e != nil {
				continue
			}
			data, e := io.ReadAll(r)
			r.Close()
			if e != nil {
				continue
			}
			faces, e := font.ParseTTC(bytes.NewReader(data))
			if e != nil || int(f.Index) >= len(faces) {
				continue
			}
			covered := true
			for _, ch := range req.Runes {
				if _, ok := faces[f.Index].NominalGlyph(ch); !ok {
					covered = false
					break
				}
			}
			if !covered {
				continue
			}
		}
		return makeResolution(req, f), nil
	}
	return Resolution{}, errors.New("no matching usable font")
}
