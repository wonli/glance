// Package ocr provides an experimental, UI-independent cloud OCR adapter.
// Calling Submit uploads the supplied document to the configured service.
package ocr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const DefaultEndpoint = "https://paddleocr.aistudio-app.com/api/v2/ocr/jobs"
const DefaultModel = "PaddleOCR-VL-1.6"

var ErrAuthentication = errors.New("ocr: authentication rejected")
var ErrJobFailed = errors.New("ocr: remote job failed")
var ErrJobUnavailable = errors.New("ocr: remote job is unavailable")

type httpStatusError struct {
	status int
}

func (e *httpStatusError) Error() string { return fmt.Sprintf("ocr: HTTP status %d", e.status) }

// Client must be configured before use and then may be shared between workers.
// Token is supplied by the caller and is never persisted by this package.
type Client struct {
	Token        string
	Endpoint     string
	Model        string
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type Options struct {
	UseDocOrientationClassify bool `json:"useDocOrientationClassify"`
	UseDocUnwarping           bool `json:"useDocUnwarping"`
	UseChartRecognition       bool `json:"useChartRecognition"`
}

// Source accepts either a remote URL or a reader with a filename, never both.
type Source struct {
	URL    string
	Name   string
	Reader io.Reader
	// Size is an optional file length. Positive values let streamed multipart
	// requests carry Content-Length; zero leaves transport framing to net/http.
	Size int64
}

type Progress struct {
	TotalPages     int `json:"totalPages"`
	ExtractedPages int `json:"extractedPages"`
}
type Job struct {
	ID        string   `json:"jobId"`
	State     string   `json:"state"`
	Progress  Progress `json:"extractProgress"`
	ResultURL struct {
		JSON string `json:"jsonUrl"`
	} `json:"resultUrl"`
	Error string `json:"errorMsg"`
}

// Page preserves service output for later coordinate-aware alignment. Number is
// the one-based output sequence, not a printed page label in the document.
type Page struct {
	Number          int
	Markdown        string
	Images          map[string]string
	OutputImages    map[string]string
	Raw             json.RawMessage
	RawRecord       json.RawMessage
	RecordPageIndex int
}

func (c *Client) endpoint() string {
	if c.Endpoint != "" {
		return strings.TrimRight(c.Endpoint, "/")
	}
	return DefaultEndpoint
}
func (c *Client) model() string {
	if c.Model != "" {
		return c.Model
	}
	return DefaultModel
}
func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Minute}
}

func (c *Client) request(ctx context.Context, method, target, contentType string, body io.Reader, auth bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		if upload, ok := body.(*streamingUploadBody); ok {
			_ = upload.Close()
			_ = upload.wait()
		}
		return nil, errors.New("ocr: invalid request")
	}
	if sized, ok := body.(interface{ ContentLength() int64 }); ok {
		req.ContentLength = sized.ContentLength()
	}
	if closer, ok := body.(io.ReadCloser); ok {
		req.Body = closer
	}
	if auth {
		if strings.TrimSpace(c.Token) == "" {
			return nil, errors.New("ocr: missing API token")
		}
		req.Header.Set("Authorization", "bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	// Do not forward credentials through redirects, or include signed URLs/tokens in errors.
	hc := *c.httpClient()
	hc.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := hc.Do(req)
	if upload, ok := body.(*streamingUploadBody); ok {
		if err != nil {
			_ = upload.Close()
		}
		uploadErr := upload.wait()
		if err == nil && uploadErr != nil {
			resp.Body.Close()
			return nil, errors.New("ocr: reading source failed")
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("ocr: HTTP request failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		if auth && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return nil, fmt.Errorf("%w (HTTP status %d)", ErrAuthentication, resp.StatusCode)
		}
		return nil, &httpStatusError{status: resp.StatusCode}
	}
	return resp, nil
}

func decodeJob(resp *http.Response) (Job, error) {
	defer resp.Body.Close()
	var envelope struct {
		Data Job `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil {
		return Job{}, errors.New("ocr: invalid job response")
	}
	return envelope.Data, nil
}

func (c *Client) Submit(ctx context.Context, source Source, options Options) (Job, error) {
	if strings.TrimSpace(c.Token) == "" {
		return Job{}, errors.New("ocr: missing API token")
	}
	if (source.URL != "") == (source.Reader != nil) {
		return Job{}, errors.New("ocr: specify exactly one URL or file reader")
	}
	var body bytes.Buffer
	var contentType string
	if source.URL != "" {
		u, err := url.Parse(source.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Job{}, errors.New("ocr: invalid source URL")
		}
		err = json.NewEncoder(&body).Encode(map[string]any{"fileUrl": source.URL, "model": c.model(), "optionalPayload": options})
		if err != nil {
			return Job{}, err
		}
		contentType = "application/json"
	} else {
		if source.Name == "" {
			return Job{}, errors.New("ocr: file name required")
		}
		var upload *streamingUploadBody
		var err error
		upload, contentType, err = newStreamingUpload(source, c.model(), options)
		if err != nil {
			return Job{}, err
		}
		resp, err := c.request(ctx, http.MethodPost, c.endpoint(), contentType, upload, true)
		if err != nil {
			return Job{}, err
		}
		job, err := decodeJob(resp)
		if err == nil && job.ID == "" {
			err = errors.New("ocr: submission returned no job ID")
		}
		return job, err
	}
	resp, err := c.request(ctx, http.MethodPost, c.endpoint(), contentType, &body, true)
	if err != nil {
		return Job{}, err
	}
	job, err := decodeJob(resp)
	if err == nil && job.ID == "" {
		err = errors.New("ocr: submission returned no job ID")
	}
	return job, err
}

type streamingUploadBody struct {
	*io.PipeReader
	length int64
	done   chan error
}

func (b *streamingUploadBody) ContentLength() int64 { return b.length }
func (b *streamingUploadBody) wait() error          { return <-b.done }

func newStreamingUpload(source Source, model string, options Options) (*streamingUploadBody, string, error) {
	var encoded bytes.Buffer
	writer := multipart.NewWriter(&encoded)
	if err := writer.WriteField("model", model); err != nil {
		return nil, "", err
	}
	optional, err := json.Marshal(options)
	if err != nil {
		return nil, "", err
	}
	if err = writer.WriteField("optionalPayload", string(optional)); err != nil {
		return nil, "", err
	}
	if _, err = writer.CreateFormFile("file", filepath.Base(source.Name)); err != nil {
		return nil, "", err
	}
	fileStart := encoded.Len()
	if err = writer.Close(); err != nil {
		return nil, "", err
	}
	payload := append([]byte(nil), encoded.Bytes()[:fileStart]...)
	suffix := append([]byte(nil), encoded.Bytes()[fileStart:]...)
	contentType := writer.FormDataContentType()
	length := int64(-1)
	if source.Size > 0 {
		length = int64(len(payload)) + source.Size + int64(len(suffix))
	}

	reader, pipe := io.Pipe()
	body := &streamingUploadBody{PipeReader: reader, length: length, done: make(chan error, 1)}
	go func() {
		if _, writeErr := pipe.Write(payload); writeErr != nil {
			pipe.CloseWithError(writeErr)
			body.done <- writeErr
			return
		}
		if _, writeErr := io.Copy(pipe, source.Reader); writeErr != nil {
			pipe.CloseWithError(writeErr)
			body.done <- errors.New("ocr: reading source failed")
			return
		}
		if _, writeErr := pipe.Write(suffix); writeErr != nil {
			pipe.CloseWithError(writeErr)
			body.done <- writeErr
			return
		}
		pipe.Close()
		body.done <- nil
	}()
	return body, contentType, nil
}

func (c *Client) Status(ctx context.Context, id string) (Job, error) {
	if id == "" {
		return Job{}, errors.New("ocr: job ID required")
	}
	resp, err := c.request(ctx, http.MethodGet, c.endpoint()+"/"+url.PathEscape(id), "", nil, true)
	if err != nil {
		return Job{}, classifyUnavailableJobError(err)
	}
	return decodeJob(resp)
}

// Wait polls until completion or cancellation. It never resubmits a job.
func (c *Client) Wait(ctx context.Context, id string, progress func(Job)) (Job, error) {
	interval := c.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	for {
		job, err := c.Status(ctx, id)
		if err != nil {
			return Job{}, err
		}
		if progress != nil {
			progress(job)
		}
		switch job.State {
		case "done":
			if job.ResultURL.JSON == "" {
				return Job{}, errors.New("ocr: completed job has no result URL")
			}
			return job, nil
		case "failed":
			return Job{}, ErrJobFailed
		case "pending", "running":
		default:
			return Job{}, errors.New("ocr: unknown job state")
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Job{}, ctx.Err()
		case <-timer.C:
		}
	}
}

// Results reads JSONL in service order. Image URLs are retained, never downloaded.
// No API authorization header is sent to the result storage host.
func (c *Client) Results(ctx context.Context, job Job) ([]Page, error) {
	if job.ResultURL.JSON == "" {
		return nil, errors.New("ocr: result URL required")
	}
	resp, err := c.request(ctx, http.MethodGet, job.ResultURL.JSON, "", nil, false)
	if err != nil {
		return nil, classifyUnavailableJobError(err)
	}
	defer resp.Body.Close()
	return ParseResults(resp.Body)
}

func classifyUnavailableJobError(err error) error {
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) && (statusErr.status == http.StatusNotFound || statusErr.status == http.StatusGone) {
		return fmt.Errorf("%w (HTTP status %d)", ErrJobUnavailable, statusErr.status)
	}
	return err
}

func ParseResults(r io.Reader) ([]Page, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	var pages []Page
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var row struct {
			Result struct {
				Layout []json.RawMessage `json:"layoutParsingResults"`
			} `json:"result"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil || len(row.Result.Layout) == 0 {
			return nil, errors.New("ocr: invalid result JSONL record")
		}
		record := append(json.RawMessage(nil), scanner.Bytes()...)
		for index, raw := range row.Result.Layout {
			var entry struct {
				Markdown *struct {
					Text   *string           `json:"text"`
					Images map[string]string `json:"images"`
				} `json:"markdown"`
				OutputImages map[string]string `json:"outputImages"`
			}
			if err := json.Unmarshal(raw, &entry); err != nil || entry.Markdown == nil || entry.Markdown.Text == nil {
				return nil, errors.New("ocr: page missing markdown text")
			}
			pages = append(pages, Page{Number: len(pages) + 1, Markdown: *entry.Markdown.Text, Images: entry.Markdown.Images, OutputImages: entry.OutputImages, Raw: append(json.RawMessage(nil), raw...), RawRecord: record, RecordPageIndex: index})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("ocr: reading JSONL failed")
	}
	if len(pages) == 0 {
		return nil, errors.New("ocr: empty results")
	}
	return pages, nil
}
