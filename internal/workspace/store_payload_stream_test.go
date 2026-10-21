package workspace

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const payloadStreamChunkSize = 1 << 20

func payloadStreamFixture(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i*31 + 7)
	}
	return data
}

func openPayloadStreamStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "workspace.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close workspace store: %v", err)
		}
	})
	return store, path
}

func savePayloadStreamFixture(t *testing.T, store *Store, payload []byte) {
	t.Helper()
	if err := store.SaveDocument(Document{ID: "doc-1", Format: "pdf", Payload: payload}); err != nil {
		t.Fatal(err)
	}
}

func TestStreamDocumentPayloadExactChunks(t *testing.T) {
	store, _ := openPayloadStreamStore(t)
	want := payloadStreamFixture(2*payloadStreamChunkSize + 123)
	savePayloadStreamFixture(t, store, want)

	var got bytes.Buffer
	var chunkSizes []int
	dst := writerFunc(func(p []byte) (int, error) {
		chunkSizes = append(chunkSizes, len(p))
		return got.Write(p)
	})
	n, err := store.StreamDocumentPayload("doc-1", dst)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(want)) {
		t.Fatalf("streamed bytes = %d, want %d", n, len(want))
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("streamed payload bytes differ from stored payload")
	}
	if len(chunkSizes) != 3 || chunkSizes[0] != payloadStreamChunkSize || chunkSizes[1] != payloadStreamChunkSize || chunkSizes[2] != 123 {
		t.Fatalf("chunk sizes = %v, want [%d %d 123]", chunkSizes, payloadStreamChunkSize, payloadStreamChunkSize)
	}
	if _, err := store.StreamDocumentPayload("missing", io.Discard); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing payload error = %v, want sql.ErrNoRows", err)
	}
}

func TestStreamDocumentPayloadAllowsEmptyBlob(t *testing.T) {
	store, _ := openPayloadStreamStore(t)
	if err := store.SaveDocument(Document{ID: "empty", Format: "pdf", Payload: []byte{}}); err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	n, err := store.StreamDocumentPayload("empty", &got)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || got.Len() != 0 {
		t.Fatalf("empty BLOB stream = %d bytes, buffer length %d", n, got.Len())
	}
}

func TestStreamDocumentPayloadRequiresFilesystemDatabase(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close in-memory workspace store: %v", err)
		}
	})
	if _, err := store.StreamDocumentPayload("doc-1", io.Discard); err == nil || !strings.Contains(err.Error(), "filesystem") {
		t.Fatalf("in-memory stream error = %v, want filesystem database error", err)
	}
}

func TestStreamDocumentPayloadUsesStableTransactionSnapshot(t *testing.T) {
	store, path := openPayloadStreamStore(t)
	if _, err := store.db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		t.Fatal(err)
	}
	want := payloadStreamFixture(2*payloadStreamChunkSize + 71)
	replacement := bytes.Repeat([]byte{0xA5}, len(want))
	savePayloadStreamFixture(t, store, want)

	writerDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	writerDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := writerDB.Close(); err != nil {
			t.Errorf("close writer connection: %v", err)
		}
	})
	writer := &replacePayloadOnFirstWrite{db: writerDB, replacement: replacement}
	n, err := store.StreamDocumentPayload("doc-1", writer)
	if err != nil {
		t.Fatal(err)
	}
	if !writer.updated {
		t.Fatal("writer connection did not replace the payload during streaming")
	}
	if n != int64(len(want)) || !bytes.Equal(writer.buf.Bytes(), want) {
		t.Fatal("stream did not preserve the original read transaction snapshot")
	}
	stored, err := store.LoadDocumentPayload("doc-1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, replacement) {
		t.Fatal("concurrent replacement was not committed")
	}
}

func TestStreamDocumentPayloadPropagatesWriterError(t *testing.T) {
	store, _ := openPayloadStreamStore(t)
	savePayloadStreamFixture(t, store, payloadStreamFixture(payloadStreamChunkSize+10))
	wantErr := errors.New("destination failed")
	if _, err := store.StreamDocumentPayload("doc-1", errorWriter{err: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("stream error = %v, want %v", err, wantErr)
	}
	if _, err := store.StreamDocumentPayload("doc-1", shortWriter{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error = %v, want %v", err, io.ErrShortWrite)
	}
}

func TestSaveDocumentStreamExactPayloadAndMetadata(t *testing.T) {
	store, _ := openPayloadStreamStore(t)
	initial := payloadStreamFixture(31)
	savePayloadStreamFixture(t, store, initial)

	want := payloadStreamFixture(2*payloadStreamChunkSize + 123)
	updatedAt := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	err := store.SaveDocumentStream(Document{
		ID: "doc-1", Format: "pdf", ViewState: []byte("page=7"), Dirty: true, UpdatedAt: updatedAt,
	}, bytes.NewReader(want), int64(len(want)))
	if err != nil {
		t.Fatal(err)
	}

	got, found, err := store.LoadDocument("doc-1")
	if err != nil || !found {
		t.Fatalf("LoadDocument() found=%v err=%v", found, err)
	}
	if !bytes.Equal(got.Payload, want) || got.PayloadSize != int64(len(want)) {
		t.Fatalf("stored payload size=%d, want exact %d-byte payload", len(got.Payload), len(want))
	}
	if got.Format != "pdf" || !got.Dirty || !bytes.Equal(got.ViewState, []byte("page=7")) || !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("stored metadata = %+v", got)
	}
}

func TestSaveDocumentStreamFailurePreservesPriorDocument(t *testing.T) {
	store, _ := openPayloadStreamStore(t)
	prior := payloadStreamFixture(2048)
	savePayloadStreamFixture(t, store, prior)
	updatedAt := time.Date(2026, 10, 21, 12, 1, 0, 0, time.UTC)
	wantErr := errors.New("snapshot source failed")
	source := &readerThenError{remaining: payloadStreamChunkSize + 17, err: wantErr}
	if err := store.SaveDocumentStream(Document{
		ID: "doc-1", Format: "pdf", ViewState: []byte("new view"), Dirty: true, UpdatedAt: updatedAt,
	}, source, payloadStreamChunkSize+99); !errors.Is(err, wantErr) {
		t.Fatalf("SaveDocumentStream error = %v, want source error", err)
	}

	got, found, err := store.LoadDocument("doc-1")
	if err != nil || !found {
		t.Fatalf("LoadDocument() found=%v err=%v", found, err)
	}
	if !bytes.Equal(got.Payload, prior) || got.Format != "pdf" || got.Dirty || len(got.ViewState) != 0 {
		t.Fatalf("failed stream changed previous document: %+v", got)
	}
}

func TestSaveDocumentStreamRejectsUnexpectedLengthAndRollsBack(t *testing.T) {
	store, _ := openPayloadStreamStore(t)
	prior := payloadStreamFixture(512)
	savePayloadStreamFixture(t, store, prior)

	if err := store.SaveDocumentStream(Document{ID: "doc-1", Format: "pdf"}, bytes.NewReader([]byte("too long")), 2); err == nil {
		t.Fatal("SaveDocumentStream accepted a source longer than the declared size")
	}
	got, err := store.LoadDocumentPayload("doc-1")
	if err != nil || !bytes.Equal(got, prior) {
		t.Fatalf("length mismatch changed prior payload: len=%d err=%v", len(got), err)
	}
}

func TestSaveDocumentStreamKeepsGoAllocationBounded(t *testing.T) {
	store, _ := openPayloadStreamStore(t)
	size := int64(32 << 20)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if err := store.SaveDocumentStream(Document{ID: "large", Format: "pdf"}, &generatedPayloadReader{remaining: size}, size); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	if allocated > 8<<20 {
		t.Fatalf("streaming %d-byte payload allocated %d Go bytes, want below 8 MiB", size, allocated)
	}
}

type readerThenError struct {
	remaining int
	err       error
}

type generatedPayloadReader struct {
	remaining int64
	position  byte
}

func (r *generatedPayloadReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	for i := range p {
		p[i] = r.position
		r.position++
	}
	r.remaining -= int64(len(p))
	return len(p), nil
}

func (r *readerThenError) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, r.err
	}
	if len(p) > r.remaining {
		p = p[:r.remaining]
	}
	for i := range p {
		p[i] = byte(i)
	}
	r.remaining -= len(p)
	return len(p), nil
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

type replacePayloadOnFirstWrite struct {
	db          *sql.DB
	replacement []byte
	buf         bytes.Buffer
	updated     bool
}

func (w *replacePayloadOnFirstWrite) Write(p []byte) (int, error) {
	if !w.updated {
		if _, err := w.db.Exec(`UPDATE documents SET payload=? WHERE id=?`, w.replacement, "doc-1"); err != nil {
			return 0, err
		}
		w.updated = true
	}
	return w.buf.Write(p)
}
