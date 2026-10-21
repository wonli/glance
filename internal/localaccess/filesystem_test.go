package localaccess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrokerListsOnlyDirectChildrenAndSkipsGit(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "00-subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "00-subdir", "nested.txt"), []byte("nested"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 505; i++ {
		name := fmt.Sprintf("entry-%03d.txt", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	broker, scope := newAuthorizedBroker(t, root, "turn-list")
	entries, err := broker.ListDirectory(context.Background(), "turn-list", scope.ID, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 500 {
		t.Fatalf("listed %d entries, want the 500-entry limit", len(entries))
	}
	if entries[0].Name != "00-subdir" || !entries[0].IsDirectory {
		t.Fatalf("first entry = %#v, want the direct child directory", entries[0])
	}
	if entries[len(entries)-1].Name != "entry-498.txt" {
		t.Fatalf("last bounded entry = %q, want entry-498.txt", entries[len(entries)-1].Name)
	}
	for _, entry := range entries {
		if entry.Name == ".git" || entry.Name == ".env" || strings.Contains(entry.RelativePath, "nested.txt") {
			t.Fatalf("directory listing exposed filtered or non-direct entry %#v", entry)
		}
	}
}

func TestBrokerRejectsTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}

	broker, scope := newAuthorizedBroker(t, root, "turn-path")
	for _, relativePath := range []string{"../secret.txt", "escape/secret.txt"} {
		if _, err := broker.ReadTextFile(context.Background(), "turn-path", scope.ID, relativePath); !errors.Is(err, ErrPathOutsideScope) {
			t.Fatalf("reading %q = %v, want ErrPathOutsideScope", relativePath, err)
		}
	}
	if _, err := broker.ReadTextFile(context.Background(), "turn-path", scope.ID, filepath.Join(root, "inside.txt")); !errors.Is(err, ErrInvalidRelativePath) {
		t.Fatalf("reading an absolute path = %v, want ErrInvalidRelativePath", err)
	}
	if _, err := broker.ReadTextFile(context.Background(), "turn-path", scope.ID, "inside.txt"); err != nil {
		t.Fatalf("reading a file inside the selected root: %v", err)
	}
}

func TestBrokerReadTextFileRejectsUnsupportedAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	fixtures := map[string][]byte{
		".env":             []byte("API_TOKEN=not-for-model"),
		"credentials.json": []byte(`{"token":"not-for-model"}`),
		"binary.bin":       {0x00, 0x01, 0x02},
		"invalid-utf8.txt": {0xff, 0xfe},
		"valid.txt":        []byte("中文内容\n"),
	}
	for name, contents := range fixtures {
		if err := os.WriteFile(filepath.Join(root, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	large := strings.Repeat("x", 1<<20+1)
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(large), 0o600); err != nil {
		t.Fatal(err)
	}
	specialPath, err := createSpecialFile(filepath.Join(root, "special"))
	if err != nil {
		t.Skipf("special file creation is unavailable: %v", err)
	}
	if specialPath == "" {
		t.Skip("this platform does not provide a portable special-file fixture")
	}

	broker, scope := newAuthorizedBroker(t, root, "turn-file")
	for _, name := range []string{".env", "credentials.json"} {
		if _, err := broker.ReadTextFile(context.Background(), "turn-file", scope.ID, name); !errors.Is(err, ErrSensitiveFile) {
			t.Fatalf("reading sensitive file %q = %v, want ErrSensitiveFile", name, err)
		}
	}
	for _, name := range []string{"binary.bin", "invalid-utf8.txt", filepath.Base(specialPath)} {
		if _, err := broker.ReadTextFile(context.Background(), "turn-file", scope.ID, name); !errors.Is(err, ErrUnsupportedFile) {
			t.Fatalf("reading unsupported file %q = %v, want ErrUnsupportedFile", name, err)
		}
	}
	if _, err := broker.ReadTextFile(context.Background(), "turn-file", scope.ID, "large.txt"); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("reading oversized file = %v, want ErrFileTooLarge", err)
	}
	got, err := broker.ReadTextFile(context.Background(), "turn-file", scope.ID, "valid.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "中文内容\n" || got.Size != int64(len("中文内容\n")) {
		t.Fatalf("read text file = %#v, want exact UTF-8 content and byte size", got)
	}
}
