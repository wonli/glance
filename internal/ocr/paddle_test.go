package ocr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJobLifecycle(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(fmt.Sprint(local), func(t *testing.T) {
			polls := 0
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/result" {
					if r.Header.Get("Authorization") != "" {
						t.Error("token leaked to result request")
					}
					fmt.Fprintln(w, `{"result":{"layoutParsingResults":[{"markdown":{"text":"卷一\n天地玄黃","images":{}},"parsing_res_list":[{"block_bbox":[1,2,3,4]}]}]}}`)
					fmt.Fprintln(w, `{"result":{"layoutParsingResults":[{"markdown":{"text":"宇宙洪荒"}}]}}`)
					return
				}
				if r.Header.Get("Authorization") != "bearer synthetic-token" {
					t.Error("missing authorization")
				}
				if r.Method == "POST" {
					if local {
						if r.ContentLength <= int64(len("synthetic PDF")) {
							t.Errorf("multipart Content-Length=%d; expected file and framing", r.ContentLength)
						}
						if err := r.ParseMultipartForm(1 << 20); err != nil {
							t.Fatal(err)
						}
						defer r.MultipartForm.RemoveAll()
						f, h, err := r.FormFile("file")
						if err != nil {
							t.Fatal(err)
						}
						if h.Filename != "sample.pdf" {
							t.Error(h.Filename)
						}
						fileData, readErr := io.ReadAll(f)
						if readErr != nil || string(fileData) != "synthetic PDF" {
							t.Errorf("multipart file=%q, err=%v", fileData, readErr)
						}
						f.Close()
						if r.FormValue("model") != DefaultModel {
							t.Error("model")
						}
					} else {
						if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
							t.Error("content type")
						}
					}
					fmt.Fprint(w, `{"data":{"jobId":"job-1"}}`)
					return
				}
				polls++
				if polls == 1 {
					fmt.Fprint(w, `{"data":{"state":"running"}}`)
					return
				}
				fmt.Fprintf(w, `{"data":{"state":"done","resultUrl":{"jsonUrl":%q}}}`, server.URL+"/result")
			}))
			defer server.Close()
			c := Client{Token: "synthetic-token", Endpoint: server.URL + "/jobs", PollInterval: time.Millisecond}
			src := Source{URL: "https://example.com/a.pdf"}
			if local {
				src = Source{Name: "sample.pdf", Reader: strings.NewReader("synthetic PDF"), Size: int64(len("synthetic PDF"))}
			}
			job, err := c.Submit(context.Background(), src, Options{})
			if err != nil {
				t.Fatal(err)
			}
			job, err = c.Wait(context.Background(), job.ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			pages, err := c.Results(context.Background(), job)
			if err != nil {
				t.Fatal(err)
			}
			if len(pages) != 2 || pages[0].Markdown != "卷一\n天地玄黃" || pages[1].Number != 2 || !strings.Contains(string(pages[0].Raw), "block_bbox") {
				t.Fatalf("bad pages: %+v", pages)
			}
		})
	}
}

func TestCancellationAndFailure(t *testing.T) {
	for _, state := range []string{"running", "failed", "unexpected", "done"} {
		t.Run(state, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"data":{"state":%q}}`, state) }))
			defer s.Close()
			c := Client{Token: "synthetic", Endpoint: s.URL, PollInterval: time.Hour}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			_, err := c.Wait(ctx, "id", nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if state == "running" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
		})
	}
}

func TestRejectInvalidResults(t *testing.T) {
	for _, input := range []string{"", `{}`, `not json`, `{"result":{"layoutParsingResults":[{}]}}`} {
		if _, err := ParseResults(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}
