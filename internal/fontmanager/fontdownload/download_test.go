package fontdownload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"golang.org/x/image/font/gofont/goregular"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestEmptyProviderDoesNoNetwork(t *testing.T) {
	entries, e := (Manifest{}).List(context.Background())
	if e != nil || len(entries) != 0 {
		t.Fatalf("empty provider = %v %v", entries, e)
	}
}
func TestDownloadVerifyAndRestart(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(goregular.TTF) }))
	defer server.Close()
	sum := sha256.Sum256(goregular.TTF)
	entry := Entry{ID: "test", Family: "Go", Version: "1", URL: server.URL, SHA256: hex.EncodeToString(sum[:]), MaxBytes: 1 << 20, License: "test license"}
	d := New(t.TempDir(), server.Client())
	resource, e := d.Fetch(context.Background(), entry, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(resource.Path); e != nil {
		t.Fatal(e)
	}
	if _, e = d.Fetch(context.Background(), entry, nil); e != nil {
		t.Fatal(e)
	}
}
func TestInvalidDigestOrFaceNotPublished(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(goregular.TTF) }))
	defer server.Close()
	d := New(t.TempDir(), server.Client())
	_, e := d.Fetch(context.Background(), Entry{ID: "bad", Family: "Go", Version: "1", URL: server.URL, SHA256: "0000000000000000000000000000000000000000000000000000000000000000", MaxBytes: 1 << 20, License: "test"}, nil)
	if e == nil {
		t.Fatal("published invalid digest")
	}
}
func TestDownloadCancelAndRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := New(t.TempDir(), nil).Fetch(ctx, Entry{}, nil)
	if e == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestDownloadSizeAndRedirectLimits(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(goregular.TTF) }))
	defer server.Close()
	sum := sha256.Sum256(goregular.TTF)
	d := New(t.TempDir(), server.Client())
	entry := Entry{ID: "tiny", Family: "Go", Version: "1", URL: server.URL, SHA256: hex.EncodeToString(sum[:]), MaxBytes: 8, License: "test"}
	if _, err := d.Fetch(context.Background(), entry, nil); err == nil {
		t.Fatal("accepted oversize font")
	}
}
func TestDownloadRejectsWrongFamilyWithValidDigest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(goregular.TTF) }))
	defer server.Close()
	sum := sha256.Sum256(goregular.TTF)
	entry := Entry{ID: "wrong", Family: "Other Family", Version: "1", URL: server.URL, SHA256: hex.EncodeToString(sum[:]), MaxBytes: 1 << 20, License: "test"}
	if _, err := New(t.TempDir(), server.Client()).Fetch(context.Background(), entry, nil); err == nil {
		t.Fatal("accepted wrong family")
	}
}

func TestDownloadCachedResourceStillChecksManifestFamily(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(goregular.TTF) }))
	defer server.Close()
	sum := sha256.Sum256(goregular.TTF)
	entry := Entry{ID: "test", Family: "Go", Version: "1", URL: server.URL, SHA256: hex.EncodeToString(sum[:]), MaxBytes: 1 << 20, License: "test"}
	d := New(t.TempDir(), server.Client())
	if _, err := d.Fetch(context.Background(), entry, nil); err != nil {
		t.Fatal(err)
	}
	entry.Family = "Wrong family"
	if _, err := d.Fetch(context.Background(), entry, nil); err == nil {
		t.Fatal("cached font bypassed manifest family validation")
	}
}
