package fontdownload

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"glance/internal/fontmanager/fontcatalog"
)

type downloadJob struct {
	done     chan struct{}
	resource fontcatalog.PrivateResource
	err      error
}
type Downloader struct {
	root   string
	client *http.Client
	mu     sync.Mutex
	jobs   map[string]*downloadJob
	slots  chan struct{}
}

func New(root string, client *http.Client) *Downloader {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &Downloader{root: root, client: client, jobs: map[string]*downloadJob{}, slots: make(chan struct{}, 2)}
}
func (d *Downloader) Fetch(ctx context.Context, entry Entry, progress func(Progress)) (fontcatalog.PrivateResource, error) {
	if e := ctx.Err(); e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	u, e := url.Parse(entry.URL)
	digest, e2 := hex.DecodeString(entry.SHA256)
	if e != nil || e2 != nil || len(digest) != 32 || strings.ToLower(entry.SHA256) != entry.SHA256 || u.Scheme != "https" || u.Host == "" || u.User != nil || entry.Family == "" || entry.Version == "" || entry.License == "" || entry.ID == "" || entry.MaxBytes <= 0 || entry.MaxBytes > fontcatalog.MaxFontBytes {
		return fontcatalog.PrivateResource{}, errors.New("invalid trusted font manifest entry")
	}
	jobKey := fmt.Sprintf("%s:%s:%d", entry.SHA256, strings.ToLower(entry.Family), entry.MaxBytes)
	d.mu.Lock()
	if job := d.jobs[jobKey]; job != nil {
		d.mu.Unlock()
		select {
		case <-job.done:
			return job.resource, job.err
		case <-ctx.Done():
			return fontcatalog.PrivateResource{}, ctx.Err()
		}
	}
	job := &downloadJob{done: make(chan struct{})}
	d.jobs[jobKey] = job
	d.mu.Unlock()
	resource, err := d.fetch(ctx, entry, u, progress)
	d.mu.Lock()
	job.resource, job.err = resource, err
	delete(d.jobs, jobKey)
	close(job.done)
	d.mu.Unlock()
	return resource, err
}
func (d *Downloader) fetch(ctx context.Context, entry Entry, u *url.URL, progress func(Progress)) (fontcatalog.PrivateResource, error) {
	select {
	case d.slots <- struct{}{}:
		defer func() { <-d.slots }()
	case <-ctx.Done():
		return fontcatalog.PrivateResource{}, ctx.Err()
	}
	objects := filepath.Join(d.root, "objects")
	if e := os.MkdirAll(objects, 0700); e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	dest := filepath.Join(objects, entry.SHA256)
	if faces, e := fontcatalog.Inspect(ctx, dest, fontcatalog.Private); e == nil && len(faces) > 0 && faces[0].Digest == entry.SHA256 {
		if !strings.EqualFold(faces[0].Family, entry.Family) || faces[0].Size > entry.MaxBytes {
			return fontcatalog.PrivateResource{}, errors.New("cached font does not match manifest")
		}
		if e := validateStaticTTF(dest); e != nil {
			return fontcatalog.PrivateResource{}, e
		}
		return d.publishRecord(entry, dest)
	}
	jobctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	request, e := http.NewRequestWithContext(jobctx, http.MethodGet, entry.URL, nil)
	if e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	client := *d.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || !strings.EqualFold(req.URL.Host, u.Host) {
			return errors.New("font redirect outside trusted origin")
		}
		return nil
	}
	response, e := client.Do(request)
	if e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fontcatalog.PrivateResource{}, fmt.Errorf("font server returned %d", response.StatusCode)
	}
	if response.ContentLength > entry.MaxBytes {
		return fontcatalog.PrivateResource{}, errors.New("font response exceeds size limit")
	}
	tmp, e := os.CreateTemp(objects, ".font-stage-*")
	if e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	hash := sha256.New()
	writer := io.MultiWriter(tmp, hash)
	reader := io.LimitReader(response.Body, entry.MaxBytes+1)
	buf := make([]byte, 32<<10)
	var received int64
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			received += int64(n)
			if received > entry.MaxBytes {
				return fontcatalog.PrivateResource{}, errors.New("font response exceeds size limit")
			}
			if _, e := writer.Write(buf[:n]); e != nil {
				return fontcatalog.PrivateResource{}, e
			}
			if progress != nil {
				progress(Progress{received, response.ContentLength})
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fontcatalog.PrivateResource{}, err
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
		return fontcatalog.PrivateResource{}, errors.New("font checksum mismatch")
	}
	if e := tmp.Sync(); e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	if e := tmp.Close(); e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	if e := validateStaticTTF(tmp.Name()); e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	faces, e := fontcatalog.Inspect(ctx, tmp.Name(), fontcatalog.Private)
	if e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	matched := false
	for _, f := range faces {
		if strings.EqualFold(f.Family, entry.Family) {
			matched = true
		}
	}
	if !matched {
		return fontcatalog.PrivateResource{}, errors.New("font family does not match manifest")
	}
	if e := ctx.Err(); e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	if e := os.Rename(tmp.Name(), dest); e != nil {
		return fontcatalog.PrivateResource{}, e
	}
	return d.publishRecord(entry, dest)
}
func (d *Downloader) publishRecord(entry Entry, path string) (fontcatalog.PrivateResource, error) {
	dir := filepath.Join(d.root, "licenses")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fontcatalog.PrivateResource{}, err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fontcatalog.PrivateResource{}, err
	}
	if err = fontcatalog.WriteAtomic(filepath.Join(dir, entry.SHA256+".json"), data); err != nil {
		return fontcatalog.PrivateResource{}, err
	}
	return fontcatalog.PrivateResource{Path: path}, nil
}
func (d *Downloader) CleanStaging() error {
	matches, err := filepath.Glob(filepath.Join(d.root, "objects", ".font-stage-*"))
	if err != nil {
		return err
	}
	for _, p := range matches {
		if e := os.Remove(p); e != nil {
			return e
		}
	}
	return nil
}

func validateStaticTTF(path string) error {
	fontBytes, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if len(fontBytes) < 12 || binary.BigEndian.Uint32(fontBytes[:4]) != 0x00010000 {
		return errors.New("download requires a static TrueType font")
	}
	parsed, e := font.ParseTTC(bytes.NewReader(fontBytes))
	if e != nil || len(parsed) != 1 {
		return errors.New("invalid TrueType font")
	}
	loader, e := ot.NewLoader(bytes.NewReader(fontBytes))
	if e != nil {
		return e
	}
	if variation, e := loader.RawTable(ot.MustNewTag("fvar")); e == nil && len(variation) > 0 {
		return errors.New("variable font downloads are not supported")
	}
	return nil
}
