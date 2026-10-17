package docx

import (
	"bytes"
	"os"
	"sort"
	"testing"
	"unicode/utf8"

	"glance/internal/document"
)

func TestFormatBookmarkedWordSample(t *testing.T) {
	path := os.Getenv("GLANCE_WORD_FORMAT_SAMPLE")
	if path == "" {
		t.Skip("set GLANCE_WORD_FORMAT_SAMPLE to a local DOCX reproduction")
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	groups := map[string][]wordTextTarget{}
	for _, target := range d.wordTextTargets {
		groups[target.paragraphID] = append(groups[target.paragraphID], target)
	}
	for _, targets := range groups {
		if len(targets) < 2 {
			continue
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i].text.start < targets[j].text.start })
		first, last := targets[0], targets[len(targets)-1]
		var markers [][]byte
		for _, child := range first.paragraph.children {
			if (child.name.Local == "bookmarkStart" || child.name.Local == "bookmarkEnd") && child.start > first.text.start && child.end < last.text.end {
				markers = append(markers, append([]byte(nil), d.parts[d.documentIdx].data[child.start:child.end]...))
			}
		}
		if len(markers) == 0 {
			continue
		}
		startID, endID := "", ""
		for id, target := range d.wordTextTargets {
			if target.text == first.text {
				startID = id
			}
			if target.text == last.text {
				endID = id
			}
		}
		startOffset, endOffset := 0, utf8.RuneCountInString(last.text.text)
		for _, size := range []float64{16, 18, 12} {
			result, err := d.SetTextSelectionStyle(startID, startOffset, endID, endOffset, document.WordTextFormatPatch{FontSize: &size})
			if err != nil {
				t.Fatal(err)
			}
			startID, startOffset, endID, endOffset = result.StartRunID, result.StartOffset, result.EndRunID, result.EndOffset
			for _, marker := range markers {
				if bytes.Count(d.parts[d.documentIdx].data, marker) != 1 {
					t.Fatal("bookmark changed during repeated font-size edits")
				}
			}
		}
		t.Logf("repeated font-size edits preserved %d bookmark markers in the sample", len(markers))
		return
	}
	t.Fatal("sample has no editable text span containing a bookmark")
}
