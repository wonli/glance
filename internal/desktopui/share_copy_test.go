package desktopui

import (
	"errors"
	"strings"
	"testing"
)

func TestEnsureShareCopyExtension(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		extension string
		want      string
	}{
		{name: "append missing extension", path: "shared", extension: "xlsx", want: "shared.xlsx"},
		{name: "preserve matching extension case insensitively", path: "shared.XLSX", extension: "xlsx", want: "shared.XLSX"},
		{name: "append after another extension", path: "shared.docx", extension: "xlsx", want: "shared.docx.xlsx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ensureShareCopyExtension(tt.path, tt.extension); got != tt.want {
				t.Fatalf("ensureShareCopyExtension(%q, %q) = %q, want %q", tt.path, tt.extension, got, tt.want)
			}
		})
	}
}

func TestSaveShareCopySnapshotMaintainsCallbackOrderAndErrorPrecedence(t *testing.T) {
	t.Run("snapshot failure skips later callbacks", func(t *testing.T) {
		calls := []string{}
		snapshotErr := errors.New("snapshot failed")
		gotErr := saveShareCopySnapshot(
			"copy.xlsx", "XLSX",
			func() ([]byte, error) {
				calls = append(calls, "snapshot")
				return nil, snapshotErr
			},
			func() error {
				calls = append(calls, "revalidate")
				return nil
			},
			func([]byte) (*shareCopyTestDocument, error) {
				calls = append(calls, "open")
				return &shareCopyTestDocument{calls: &calls}, nil
			},
			func() (func(), error) {
				calls = append(calls, "guard")
				return nil, nil
			},
		)
		if gotErr != snapshotErr {
			t.Fatalf("snapshot error = %v, want original %v", gotErr, snapshotErr)
		}
		assertShareCopyCalls(t, calls, []string{"snapshot"})
	})

	t.Run("stale snapshot skips open", func(t *testing.T) {
		calls := []string{}
		staleErr := errors.New("snapshot went stale")
		gotErr := saveShareCopySnapshot(
			"copy.xlsx", "XLSX",
			func() ([]byte, error) {
				calls = append(calls, "snapshot")
				return []byte("snapshot"), nil
			},
			func() error {
				calls = append(calls, "revalidate")
				return staleErr
			},
			func([]byte) (*shareCopyTestDocument, error) {
				calls = append(calls, "open")
				return &shareCopyTestDocument{calls: &calls}, nil
			},
			func() (func(), error) {
				calls = append(calls, "guard")
				return nil, nil
			},
		)
		if gotErr != staleErr {
			t.Fatalf("stale error = %v, want original %v", gotErr, staleErr)
		}
		assertShareCopyCalls(t, calls, []string{"snapshot", "revalidate"})
	})

	t.Run("open error has format prefix and does not close", func(t *testing.T) {
		calls := []string{}
		openErr := errors.New("invalid snapshot")
		gotErr := saveShareCopySnapshot(
			"copy.xlsx", "XLSX",
			func() ([]byte, error) {
				calls = append(calls, "snapshot")
				return []byte("snapshot"), nil
			},
			func() error {
				calls = append(calls, "revalidate")
				return nil
			},
			func([]byte) (*shareCopyTestDocument, error) {
				calls = append(calls, "open")
				return nil, openErr
			},
			func() (func(), error) {
				calls = append(calls, "guard")
				return nil, nil
			},
		)
		if got, want := gotErr.Error(), "校验 XLSX 副本：invalid snapshot"; got != want {
			t.Fatalf("open error = %q, want %q", got, want)
		}
		assertShareCopyCalls(t, calls, []string{"snapshot", "revalidate", "open"})
	})

	t.Run("save error takes precedence over close error", func(t *testing.T) {
		calls := []string{}
		saveErr := errors.New("save failed")
		doc := &shareCopyTestDocument{calls: &calls, saveErr: saveErr, closeErr: errors.New("close failed")}
		guardCalls := 0
		gotErr := saveShareCopySnapshot(
			"copy.xlsx", "XLSX",
			func() ([]byte, error) {
				calls = append(calls, "snapshot")
				return []byte("snapshot"), nil
			},
			func() error {
				calls = append(calls, "revalidate")
				return nil
			},
			func([]byte) (*shareCopyTestDocument, error) {
				calls = append(calls, "open")
				return doc, nil
			},
			func() (func(), error) {
				guardCalls++
				calls = append(calls, "guard")
				return func() { calls = append(calls, "release") }, nil
			},
		)
		if gotErr != saveErr {
			t.Fatalf("save/close error = %v, want save error %v", gotErr, saveErr)
		}
		if guardCalls != 1 {
			t.Fatalf("commit guard called %d times, want 1", guardCalls)
		}
		assertShareCopyCalls(t, calls, []string{"snapshot", "revalidate", "open", "save:copy.xlsx", "guard", "release", "close"})
	})

	t.Run("close error is returned after successful save", func(t *testing.T) {
		calls := []string{}
		closeErr := errors.New("close failed")
		doc := &shareCopyTestDocument{calls: &calls, closeErr: closeErr}
		gotErr := saveShareCopySnapshot(
			"copy.xlsx", "XLSX",
			func() ([]byte, error) {
				calls = append(calls, "snapshot")
				return []byte("snapshot"), nil
			},
			func() error {
				calls = append(calls, "revalidate")
				return nil
			},
			func([]byte) (*shareCopyTestDocument, error) {
				calls = append(calls, "open")
				return doc, nil
			},
			func() (func(), error) {
				calls = append(calls, "guard")
				return nil, nil
			},
		)
		if gotErr != closeErr {
			t.Fatalf("close error = %v, want %v", gotErr, closeErr)
		}
		assertShareCopyCalls(t, calls, []string{"snapshot", "revalidate", "open", "save:copy.xlsx", "guard", "close"})
	})
}

type shareCopyTestDocument struct {
	calls    *[]string
	saveErr  error
	closeErr error
}

func (d *shareCopyTestDocument) SaveAsWithCommitGuard(path string, guard func() (func(), error)) error {
	*d.calls = append(*d.calls, "save:"+path)
	release, err := guard()
	if err != nil {
		return err
	}
	if release != nil {
		defer release()
	}
	return d.saveErr
}

func (d *shareCopyTestDocument) Close() error {
	*d.calls = append(*d.calls, "close")
	return d.closeErr
}

func assertShareCopyCalls(t *testing.T, got, want []string) {
	t.Helper()
	if gotValue, wantValue := strings.Join(got, ","), strings.Join(want, ","); gotValue != wantValue {
		t.Fatalf("callback order = %q, want %q", gotValue, wantValue)
	}
}

func TestApplyShareCopyResultPreservesCompletionStatus(t *testing.T) {
	for _, tt := range []struct {
		format string
		path   string
		want   string
	}{
		{format: "XLSX", path: "shared.xlsx", want: "共享 XLSX 副本已导出：shared.xlsx"},
		{format: "DOCX", path: "shared.docx", want: "共享 DOCX 副本已导出：shared.docx"},
	} {
		t.Run(tt.format+" success", func(t *testing.T) {
			state := newDocumentSessionState("source." + strings.ToLower(tt.format))
			app := &App{documentSessionState: state}
			app.applyShareCopyResult(state, tt.path, nil, tt.format)
			if app.status != tt.want {
				t.Fatalf("success status = %q, want %q", app.status, tt.want)
			}
		})
	}

	t.Run("active error", func(t *testing.T) {
		state := newDocumentSessionState("source.xlsx")
		app := &App{documentSessionState: state}
		app.applyShareCopyResult(state, "shared.xlsx", errors.New("disk full"), "XLSX")
		if want := "导出共享副本失败：disk full"; app.status != want {
			t.Fatalf("error status = %q, want %q", app.status, want)
		}
	})

	t.Run("closed result leaves status unchanged", func(t *testing.T) {
		state := newDocumentSessionState("source.xlsx")
		state.closed.Store(true)
		app := &App{documentSessionState: state, status: "existing status"}
		app.applyShareCopyResult(state, "shared.xlsx", nil, "XLSX")
		if app.status != "existing status" {
			t.Fatalf("closed result changed status to %q", app.status)
		}
	})

	t.Run("noncurrent result leaves status unchanged", func(t *testing.T) {
		state := newDocumentSessionState("source.xlsx")
		app := &App{documentSessionState: newDocumentSessionState("other.xlsx"), status: "existing status"}
		app.applyShareCopyResult(state, "shared.xlsx", nil, "XLSX")
		if app.status != "existing status" {
			t.Fatalf("noncurrent result changed status to %q", app.status)
		}
	})
}
