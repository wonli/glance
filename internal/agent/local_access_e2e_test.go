package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"glance/internal/agent/provider"
	"glance/internal/application"
	"glance/internal/localaccess"
)

const (
	localAccessFixtureText       = "AUTHORIZED_FILE_CONTENT_FOR_LOCAL_ACCESS_TEST"
	localAccessFixtureCommitBody = "PRIVATE_COMMIT_BODY_FOR_LOCAL_ACCESS_TEST"
	localAccessFixtureEmail      = "private-author@example.invalid"
)

type scriptedLocalAccessPrompt struct {
	root      string
	allow     bool
	mu        sync.Mutex
	selection *localaccess.SelectionRequest
	consent   *localaccess.ConsentRequest
}

func (p *scriptedLocalAccessPrompt) SelectDirectory(ctx context.Context, request localaccess.SelectionRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	p.mu.Lock()
	copy := request
	p.selection = &copy
	p.mu.Unlock()
	return p.root, nil
}

func (p *scriptedLocalAccessPrompt) ConfirmDirectory(ctx context.Context, request localaccess.ConsentRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	p.mu.Lock()
	copy := request
	p.consent = &copy
	p.mu.Unlock()
	return p.allow, nil
}

func TestAgentLocalAccessRequiresApprovalBeforeReading(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for this integration test")
	}
	root := createLocalAccessGitFixture(t)
	for _, allow := range []bool{false, true} {
		name := "denied"
		if allow {
			name = "allowed"
		}
		t.Run(name, func(t *testing.T) {
			runLocalAccessScenario(t, root, allow)
		})
	}
}

func runLocalAccessScenario(t *testing.T, root string, allow bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prompt := &scriptedLocalAccessPrompt{root: root, allow: allow}
	broker := localaccess.NewBroker(prompt)
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{LocalAccessBroker: broker})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	store := newMemoryConversationStore()
	var scopeID string
	var toolRequests int
	model := &scriptedProvider{t: t}
	model.steps = []func(provider.ModelRequest) []provider.ModelEvent{
		func(request provider.ModelRequest) []provider.ModelEvent {
			if len(request.Messages) != 1 || request.Messages[0].Role != "user" || !strings.Contains(request.Messages[0].Content, "用户第一条消息：\n总结本月工作") {
				t.Errorf("conversation-title request did not contain only the user's request: %+v", request.Messages)
			}
			if containsModelMessages(request.Messages, localAccessFixtureText) || containsModelMessages(request.Messages, "October project milestone") {
				t.Error("local content reached the provider while generating a conversation title")
			}
			return []provider.ModelEvent{{TextDelta: "本月工作总结"}}
		},
		func(request provider.ModelRequest) []provider.ModelEvent {
			if len(request.Messages) == 0 || request.Messages[len(request.Messages)-1].Role != "user" || request.Messages[len(request.Messages)-1].Content != "总结本月工作，并检查这个目录中的 Git 提交记录。" {
				t.Errorf("first provider request did not start with the user's prompt: %+v", request.Messages)
			}
			toolRequests++
			return localAccessToolCall("request-directory", "local.request_directory_access", `{"purpose":"汇总本月工作"}`)
		},
	}
	if allow {
		model.steps = append(model.steps,
			func(request provider.ModelRequest) []provider.ModelEvent {
				if containsModelMessages(request.Messages, localAccessFixtureText) {
					t.Error("file contents reached the model before a read tool was authorized")
				}
				data := localAccessToolData(t, request, "local.request_directory_access")
				if err := json.Unmarshal(data["scope_id"], &scopeID); err != nil || scopeID == "" {
					t.Errorf("authorized tool result did not contain an opaque scope ID: %s (error %v)", data["scope_id"], err)
				}
				if containsModelMessages(request.Messages, root) {
					t.Errorf("absolute folder path escaped into model context before file reads: %s", root)
				}
				toolRequests++
				arguments, _ := json.Marshal(map[string]string{"scope_id": scopeID, "relative_path": "monthly.md"})
				return localAccessToolCall("read-file", "local.read_text_file", string(arguments))
			},
			func(request provider.ModelRequest) []provider.ModelEvent {
				if !containsModelMessages(request.Messages, localAccessFixtureText) {
					t.Error("approved file content was not returned to the model after authorization")
				}
				if containsModelMessages(request.Messages, localAccessFixtureCommitBody) || containsModelMessages(request.Messages, localAccessFixtureEmail) {
					t.Error("Git body or author email leaked before the Git metadata query")
				}
				toolRequests++
				arguments, _ := json.Marshal(map[string]any{"scope_id": scopeID, "limit": 10})
				return localAccessToolCall("list-commits", "local.list_git_commits", string(arguments))
			},
			func(request provider.ModelRequest) []provider.ModelEvent {
				if !containsModelMessages(request.Messages, localAccessFixtureText) || !containsModelMessages(request.Messages, "October project milestone") {
					t.Error("approved file or Git metadata was not returned to the model")
				}
				for _, private := range []string{localAccessFixtureCommitBody, localAccessFixtureEmail, "diff --git"} {
					if containsModelMessages(request.Messages, private) {
						t.Errorf("local Git query returned forbidden field %q", private)
					}
				}
				return []provider.ModelEvent{{TextDelta: "已根据获准的本地文件和提交记录完成总结。", Done: true}}
			},
		)
	} else {
		model.steps = append(model.steps, func(request provider.ModelRequest) []provider.ModelEvent {
			if containsModelMessages(request.Messages, localAccessFixtureText) || containsModelMessages(request.Messages, "October project milestone") {
				t.Error("denied local content reached the model")
			}
			if !containsModelMessages(request.Messages, "directory_access_denied") {
				t.Errorf("model did not receive the denial result: %+v", request.Messages)
			}
			return []provider.ModelEvent{{TextDelta: "我无法读取该目录；可以根据你在对话中提供的信息继续总结。", Done: true}}
		})
	}

	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ended := make(chan string, 1)
	host, err := NewHost(ctx, model, client.Session, HostOptions{
		Model: "local-access-test", ConversationStore: store, MaxToolRounds: 8,
		OnTurnEnded: func(turnID string) {
			runtime.ReleaseAgentTurn(turnID)
			ended <- turnID
		},
	})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	t.Cleanup(host.Close)
	conversation, err := host.CreateConversation(ctx, "local-access-document")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	userPrompt := "总结本月工作，并检查这个目录中的 Git 提交记录。"
	if err := host.Send(ctx, conversation.ID, userPrompt, application.ViewContext{WindowID: 72, DocumentID: "local-access-document"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	turnID := <-ended
	if toolRequests != map[bool]int{false: 1, true: 3}[allow] {
		t.Fatalf("model issued %d local tool calls after the initial request; allow=%v", toolRequests, allow)
	}

	prompt.mu.Lock()
	selection, consent := prompt.selection, prompt.consent
	prompt.mu.Unlock()
	if selection == nil || selection.WindowID != 72 || selection.TurnID != turnID || selection.UserRequestExcerpt != userPrompt {
		t.Fatalf("directory picker did not receive trusted current-turn context: %+v turn=%q", selection, turnID)
	}
	if consent == nil || consent.WindowID != 72 || consent.TurnID != turnID || consent.FolderPath != root {
		t.Fatalf("consent prompt did not receive the selected directory/current turn: %+v", consent)
	}

	record, found, err := store.Load(ctx, "local-access-document", conversation.ID)
	if err != nil || !found {
		t.Fatalf("load persisted conversation: found=%v error=%v", found, err)
	}
	var persisted string
	for _, message := range record.Messages {
		persisted += message.Content + "\n"
	}
	if allow {
		if !strings.Contains(persisted, "scope_id") {
			t.Fatalf("tool result with opaque scope handle was not persisted: %s", persisted)
		}
		if !strings.Contains(persisted, localAccessFixtureText) || !strings.Contains(persisted, "October project milestone") {
			t.Fatalf("approved text/Git results were not stored in conversation: %s", persisted)
		}
		if strings.Contains(persisted, root) || strings.Contains(persisted, localAccessFixtureCommitBody) || strings.Contains(persisted, localAccessFixtureEmail) {
			t.Fatalf("conversation persisted an absolute path or forbidden Git content: %s", persisted)
		}
		if _, err := broker.ReadTextFile(context.Background(), turnID, scopeID, "monthly.md"); !errors.Is(err, localaccess.ErrTurnExpired) {
			t.Fatalf("scope remained usable after turn end: %v", err)
		}
	} else {
		if strings.Contains(persisted, localAccessFixtureText) || strings.Contains(persisted, "October project milestone") || strings.Contains(persisted, "scope_id") || scopeID != "" {
			t.Fatalf("denied content or a usable scope was persisted: %s scope=%q", persisted, scopeID)
		}
	}
}

func createLocalAccessGitFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "monthly.md"), []byte(localAccessFixtureText), 0o600); err != nil {
		t.Fatalf("write local file fixture: %v", err)
	}
	commands := [][]string{
		{"init", "-q"},
		{"add", "monthly.md"},
		{"-c", "user.name=Local Access Author", "-c", "user.email=" + localAccessFixtureEmail, "-c", "commit.gpgsign=false", "commit", "-m", "October project milestone", "-m", localAccessFixtureCommitBody},
	}
	for _, args := range commands {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, output)
		}
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("canonicalize local Git fixture root: %v", err)
	}
	return canonicalRoot
}

func localAccessToolCall(id, name, arguments string) []provider.ModelEvent {
	return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: id, Type: "function", Name: name, ArgumentsDelta: arguments}}}
}

func localAccessToolData(t *testing.T, request provider.ModelRequest, toolName string) map[string]json.RawMessage {
	t.Helper()
	for index := len(request.Messages) - 1; index >= 0; index-- {
		message := request.Messages[index]
		if message.Role != "tool" || message.ToolName != toolName {
			continue
		}
		var envelope struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(message.Content), &envelope); err != nil {
			t.Fatalf("decode %s result: %v (%s)", toolName, err, message.Content)
		}
		return envelope.Data
	}
	t.Fatalf("provider request omitted result for tool %s: %+v", toolName, request.Messages)
	return nil
}

func containsModelMessages(messages []provider.ModelMessage, value string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, value) {
			return true
		}
		for _, call := range message.ToolCalls {
			if strings.Contains(call.Function.Arguments, value) {
				return true
			}
		}
	}
	return false
}
