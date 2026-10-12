package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"glance/internal/agent/provider"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
func response(request *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

func TestListModelsUsesNativeOllamaCatalog(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/tags" {
			t.Errorf("request method/path=%s %s", r.Method, r.URL.Path)
		}
		return response(r, `{"models":[{"name":"llama3:latest","model":"llama3:latest"},{"model":"qwen:7b"}]}`), nil
	})}
	models, err := New(Config{BaseURL: "http://ollama.test", HTTPClient: client}).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 || models[0] != (provider.ModelInfo{ID: "llama3:latest", Name: "llama3:latest"}) || models[1].ID != "qwen:7b" {
		t.Fatalf("models=%+v", models)
	}
}

func TestStreamSeparatesThinkingAndText(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Errorf("request method/path=%s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Model    string          `json:"model"`
			Stream   bool            `json:"stream"`
			Messages []ollamaMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Model != "llama3:latest" || !body.Stream || len(body.Messages) != 1 {
			t.Errorf("request body=%+v", body)
		}
		stream := `{"message":{"thinking":"先分析","content":"正文"},"done":false}` + "\n" +
			`{"message":{"tool_calls":[{"function":{"name":"document.inspect","arguments":{"id":"doc-1"}}}]},"done":true,"prompt_eval_count":5,"eval_count":2}` + "\n"
		return response(r, stream), nil
	})}
	var events []provider.ModelEvent
	err := New(Config{BaseURL: "http://ollama.test", HTTPClient: client}).Stream(context.Background(), provider.ModelRequest{RequestID: "local", Model: "llama3:latest", Messages: []provider.ModelMessage{{Role: "user", Content: "检查"}}}, func(event provider.ModelEvent) error { events = append(events, event); return nil })
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(events) != 5 || events[0].ReasoningDelta != "先分析" || events[1].TextDelta != "正文" || events[2].ToolCallDelta == nil || events[3].Usage == nil || !events[4].Done {
		t.Fatalf("events=%+v", events)
	}
}

func TestStopCancelsOnlyRequestedStream(t *testing.T) {
	started := make(chan string, 2)
	canceled := make(chan string, 2)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return nil, err
		}
		started <- body.Model
		<-r.Context().Done()
		canceled <- body.Model
		return nil, r.Context().Err()
	})}
	p := New(Config{BaseURL: "http://ollama.test", HTTPClient: client})
	results := make(chan error, 2)
	for _, model := range []string{"one", "two"} {
		model := model
		go func() {
			results <- p.Stream(context.Background(), provider.ModelRequest{RequestID: model, Model: model, Messages: []provider.ModelMessage{{Role: "user", Content: "x"}}}, func(provider.ModelEvent) error { return nil })
		}()
	}
	for range 2 {
		<-started
	}
	if err := p.Stop(context.Background(), "one"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case got := <-canceled:
		if got != "one" {
			t.Fatalf("canceled=%q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("requested stream was not canceled")
	}
	select {
	case got := <-canceled:
		t.Fatalf("unrelated stream %q was canceled", got)
	case <-time.After(50 * time.Millisecond):
	}
	if err := p.Stop(context.Background(), "two"); err != nil {
		t.Fatalf("Stop second: %v", err)
	}
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				t.Error("Stream returned nil after cancellation")
			}
		case <-time.After(time.Second):
			t.Fatal("streams did not end after cancellation")
		}
	}
}
