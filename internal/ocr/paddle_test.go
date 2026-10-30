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

func TestSubmitClassifiesAuthenticationRejections(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := Client{Token: "synthetic-token", Endpoint: "https://ocr.example/jobs", HTTPClient: &http.Client{Transport: statusRoundTripper(status)}}
			_, err := client.Submit(context.Background(), Source{URL: "https://example.test/book.pdf"}, Options{})
			if !errors.Is(err, ErrAuthentication) {
				t.Fatalf("HTTP %d must be identifiable as an authentication rejection, got %v", status, err)
			}
		})
	}
}

func TestWaitClassifiesTerminalRemoteFailure(t *testing.T) {
	client := Client{
		Token: "synthetic-token", Endpoint: "https://ocr.example/jobs", PollInterval: time.Millisecond,
		HTTPClient: &http.Client{Transport: newResponseRoundTripper(http.StatusOK, `{"data":{"state":"failed","errorMsg":"recognition failed"}}`)},
	}
	_, err := client.Wait(context.Background(), "job", nil)
	if !errors.Is(err, ErrJobFailed) {
		t.Fatalf("remote failed state should be terminal and classifiable, got %v", err)
	}
}

func TestStatusClassifiesExpiredRemoteJobs(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := Client{Token: "synthetic-token", Endpoint: "https://ocr.example/jobs", HTTPClient: &http.Client{Transport: statusRoundTripper(status)}}
			_, err := client.Status(context.Background(), "expired-job")
			if !errors.Is(err, ErrJobUnavailable) {
				t.Fatalf("HTTP %d for a saved job should be terminal, got %v", status, err)
			}
		})
	}
}

func TestResultsClassifiesExpiredStorageWithoutMislabelingAuthentication(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		wantAuth   bool
		wantAbsent bool
	}{
		{name: "forbidden signed URL", status: http.StatusForbidden},
		{name: "missing signed URL result", status: http.StatusNotFound, wantAbsent: true},
		{name: "expired signed URL result", status: http.StatusGone, wantAbsent: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := Client{Token: "synthetic-token", Endpoint: "https://ocr.example/jobs", HTTPClient: &http.Client{Transport: statusRoundTripper(test.status)}}
			_, err := client.Results(context.Background(), Job{ResultURL: struct {
				JSON string `json:"jsonUrl"`
			}{JSON: "https://results.example/signed.jsonl"}})
			if errors.Is(err, ErrAuthentication) != test.wantAuth {
				t.Fatalf("signed result URL HTTP %d authentication classification: err=%v", test.status, err)
			}
			if errors.Is(err, ErrJobUnavailable) != test.wantAbsent {
				t.Fatalf("signed result URL HTTP %d unavailable classification: err=%v", test.status, err)
			}
		})
	}
}

type statusRoundTripper int

func (status statusRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: int(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("rejected")),
		Request:    request,
	}, nil
}

type responseRoundTripper struct {
	status int
	body   string
}

func newResponseRoundTripper(status int, body string) responseRoundTripper {
	return responseRoundTripper{status: status, body: body}
}

func (r responseRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: r.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(r.body)), Request: request}, nil
}

func TestParseResultsPreservesCompleteRecordForMultiplePages(t *testing.T) {
	input := `{"extra":{"future":[1,2]},"result":{"metadata":{"x":true},"layoutParsingResults":[{"markdown":{"text":"A"},"extra":1},{"markdown":{"text":"B"},"extra":2}]}}`
	pages, err := ParseResults(strings.NewReader(input))
	if err != nil || len(pages) != 2 {
		t.Fatalf("pages=%d err=%v", len(pages), err)
	}
	for i, page := range pages {
		if string(page.RawRecord) != input || page.RecordPageIndex != i {
			t.Fatalf("page %d lost original record or index", i)
		}
	}
}
