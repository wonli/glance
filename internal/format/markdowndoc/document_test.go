package markdowndoc

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMarkdownSourceRoundTrip(t *testing.T) {
	for _, source := range []string{"", "# 中文\n\n**内容**\n", "\ufeff# 标题\r\n正文\r\n"} {
		t.Run(source, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.md")
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			d, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			snapshot, err := d.Snapshot()
			if err != nil || !bytes.Equal(snapshot, []byte(source)) {
				t.Fatalf("snapshot %q, %v", snapshot, err)
			}
			if err := d.SetText(d.Text()); err != nil || d.Dirty() {
				t.Fatalf("no-op changed dirty state: %v", err)
			}
			if err := d.Save(); err != nil {
				t.Fatal(err)
			}
			actual, _ := os.ReadFile(path)
			if string(actual) != source {
				t.Fatalf("saved %q want %q", actual, source)
			}
			restored, err := OpenSnapshot(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			recovered, _ := restored.Snapshot()
			if !bytes.Equal(recovered, snapshot) {
				t.Fatal("recovery lost source")
			}
		})
	}
}

func TestMarkdownNoOpAndSaveFailure(t *testing.T) {
	d := NewBlank()
	defer d.Close()
	d.SetDirty(false)
	if err := d.SetText("中文\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveAs(filepath.Join(t.TempDir(), "missing", "out.md")); err == nil {
		t.Fatal("expected failure")
	}
	if !d.Dirty() || d.Text() != "中文\r\n" {
		t.Fatal("failed save lost draft")
	}
	path := filepath.Join(t.TempDir(), "out.markdown")
	if err := d.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.Text() != "中文\r\n" || d.Dirty() {
		t.Fatal("edited text failed round trip")
	}
}

func TestMarkdownRejectsInvalidUTF8(t *testing.T) {
	if _, err := OpenSnapshot([]byte{0xff}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}
