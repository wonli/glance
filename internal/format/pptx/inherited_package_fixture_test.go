package pptx

import (
	"archive/zip"
	"bytes"

	"io"

	"os"
	"path/filepath"

	"strings"
	"testing"
)

func writePPTXFixture(t *testing.T, replacements map[string][]byte) string {
	t.Helper()
	seed, err := NewBlank()
	if err != nil {
		t.Fatalf("create seed presentation: %v", err)
	}
	seedPath := filepath.Join(t.TempDir(), "seed.pptx")
	if err := seed.SaveAs(seedPath); err != nil {
		_ = seed.Close()
		t.Fatalf("save seed presentation: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed presentation: %v", err)
	}

	seedFile, err := os.Open(seedPath)
	if err != nil {
		t.Fatalf("open seed package: %v", err)
	}
	info, err := seedFile.Stat()
	if err != nil {
		_ = seedFile.Close()
		t.Fatalf("stat seed package: %v", err)
	}
	reader, err := zip.NewReader(seedFile, info.Size())
	if err != nil {
		_ = seedFile.Close()
		t.Fatalf("read seed package: %v", err)
	}

	targetPath := filepath.Join(t.TempDir(), "fixture.pptx")
	targetFile, err := os.Create(targetPath)
	if err != nil {
		_ = seedFile.Close()
		t.Fatalf("create fixture package: %v", err)
	}
	writer := zip.NewWriter(targetFile)
	seen := make(map[string]bool, len(replacements))
	for _, part := range reader.File {
		rc, err := part.Open()
		if err != nil {
			t.Fatalf("open seed part %s: %v", part.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read seed part %s: %v", part.Name, err)
		}
		if replacement, ok := replacements[part.Name]; ok {
			data = replacement
			seen[part.Name] = true
		}
		if part.Name == "[Content_Types].xml" {
			data = addImageContentTypes(data)
		}
		header := &zip.FileHeader{Name: part.Name, Method: zip.Deflate}
		out, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("write fixture part %s: %v", part.Name, err)
		}
		if _, err := out.Write(data); err != nil {
			t.Fatalf("write fixture part %s bytes: %v", part.Name, err)
		}
	}
	for name, data := range replacements {
		if seen[name] {
			continue
		}
		out, err := writer.Create(name)
		if err != nil {
			t.Fatalf("add fixture part %s: %v", name, err)
		}
		if _, err := out.Write(data); err != nil {
			t.Fatalf("write fixture part %s bytes: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close fixture ZIP: %v", err)
	}
	if err := targetFile.Close(); err != nil {
		t.Fatalf("close fixture package: %v", err)
	}
	if err := seedFile.Close(); err != nil {
		t.Fatalf("close seed package: %v", err)
	}
	return targetPath
}

func addImageContentTypes(data []byte) []byte {
	var defaults strings.Builder
	for _, item := range []struct{ ext, contentType string }{
		{"png", "image/png"}, {"jpg", "image/jpeg"}, {"jpeg", "image/jpeg"}, {"gif", "image/gif"},
	} {
		if !bytes.Contains(data, []byte(`Extension="`+item.ext+`"`)) {
			defaults.WriteString(`<Default Extension="` + item.ext + `" ContentType="` + item.contentType + `"/>`)
		}
	}
	if defaults.Len() == 0 {
		return data
	}
	return bytes.Replace(data, []byte(`</Types>`), append([]byte(defaults.String()), []byte(`</Types>`)...), 1)
}
