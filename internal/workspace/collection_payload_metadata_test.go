package workspace

import (
	"bytes"
	"path/filepath"
	"testing"
)

func openPDFCollectionFixture(t *testing.T, payload []byte) (*Store, *Collection) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close workspace store: %v", err)
		}
	})
	collection, err := OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.AddDocument(Session{
		Tab:       Tab{ID: "tab-1", DocumentID: "doc-1", Kind: "file", Title: "sample.pdf"},
		Format:    "pdf",
		Payload:   payload,
		ViewState: []byte("initial view"),
	}, true); err != nil {
		t.Fatal(err)
	}
	return store, collection
}

func TestCollectionGetMetadataDoesNotHydratePayload(t *testing.T) {
	payload := payloadStreamFixture(4096)
	store, collection := openPDFCollectionFixture(t, payload)
	collection.ReleasePayload("tab-1")

	session, ok := collection.GetMetadata("tab-1")
	if !ok || session == nil {
		t.Fatal("GetMetadata() did not return the session")
	}
	if session.Payload != nil {
		t.Fatalf("metadata query hydrated %d payload bytes", len(session.Payload))
	}
	if !session.SnapshotPresent || session.PayloadSize != int64(len(payload)) {
		t.Fatalf("snapshot metadata = present:%v size:%d, want present size %d", session.SnapshotPresent, session.PayloadSize, len(payload))
	}
	doc, found, err := store.LoadDocumentMetadata("doc-1")
	if err != nil || !found || doc.PayloadSize != int64(len(payload)) {
		t.Fatalf("stored document metadata = %+v, found:%v, err:%v", doc, found, err)
	}
}

func TestCollectionSnapshotPresenceIncludesEmptySnapshot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close workspace store: %v", err)
		}
	})
	collection, err := OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.AddDocument(Session{
		Tab:     Tab{ID: "tab-empty", DocumentID: "doc-empty", Kind: "file", Title: "empty.md"},
		Format:  "markdown",
		Payload: []byte{},
	}, true); err != nil {
		t.Fatal(err)
	}
	collection.ReleasePayload("tab-empty")
	session, ok := collection.GetMetadata("tab-empty")
	if !ok || !session.SnapshotPresent || session.PayloadSize != 0 || session.Payload != nil {
		t.Fatalf("empty snapshot metadata = %+v, found:%v", session, ok)
	}
}

func TestCollectionUpdatePDFSnapshotDoesNotRetainPayload(t *testing.T) {
	initial := payloadStreamFixture(32)
	store, collection := openPDFCollectionFixture(t, initial)
	updated := payloadStreamFixture(2*payloadStreamChunkSize + 19)
	viewState := []byte("updated view")
	if err := collection.UpdatePDFSnapshot("tab-1", updated, viewState, true); err != nil {
		t.Fatal(err)
	}
	session, ok := collection.GetMetadata("tab-1")
	if !ok {
		t.Fatal("updated session not found")
	}
	if session.Payload != nil {
		t.Fatalf("PDF snapshot update retained %d payload bytes", len(session.Payload))
	}
	if !session.SnapshotPresent || session.PayloadSize != int64(len(updated)) || !session.Dirty || !bytes.Equal(session.ViewState, viewState) {
		t.Fatalf("updated metadata = %+v", session)
	}
	got, err := store.LoadDocumentPayload("doc-1")
	if err != nil || !bytes.Equal(got, updated) {
		t.Fatalf("stored snapshot differs: len=%d err=%v", len(got), err)
	}
}

func TestCollectionUpdatePDFMetadataPreservesPayload(t *testing.T) {
	payload := payloadStreamFixture(1024)
	store, collection := openPDFCollectionFixture(t, payload)
	collection.ReleasePayload("tab-1")
	viewState := []byte("scroll position")
	if err := collection.UpdatePDFMetadata("tab-1", viewState, false); err != nil {
		t.Fatal(err)
	}
	session, ok := collection.GetMetadata("tab-1")
	if !ok || session.Payload != nil || session.Dirty || !bytes.Equal(session.ViewState, viewState) {
		t.Fatalf("metadata-only result = %+v", session)
	}
	got, err := store.LoadDocumentPayload("doc-1")
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("metadata update changed stored payload: len=%d err=%v", len(got), err)
	}
}
