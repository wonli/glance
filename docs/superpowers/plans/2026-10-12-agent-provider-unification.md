# Agent Provider 统一与文档会话 Implementation Plan

> **For agentic workers:** Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** 统一 Provider 的 apic 流式、模型列表和终止契约，并实现按文档隔离、跨重启恢复、支持思考内容和会话标题的 Agent 聊天。

**Architecture:** `internal/agent/provider` 根包只定义中立 DTO/接口，每个服务商目录实现其协议。Agent 通过 prompts 模板和 workspace-backed 会话仓库运行，桌面 runtime 装配 Provider/存储，Gio UI 处理模型发现与会话展示。

**Tech Stack:** Go、`github.com/wonli/apic/v2`、`text/template`/`embed.FS`、SQLite (`modernc.org/sqlite`)、Gio。

**Spec:** `docs/superpowers/specs/2026-10-12-agent-provider-apic-design.md`

## Global Constraints

- 每个服务商单独放在 `internal/agent/provider/{minimax,kimi,deepseek,openai,ollama}`；不保留旧 Provider 包装器。
- apic 每请求独立 client/context，通过 `OnEvent`/`ReceiveEvent` 回调发出中立事件；Debug 保持关闭。
- 思考内容使用独立 `ReasoningDelta`，不并入正文或一般跨模型历史；协议明确要求时由对应 Provider 映射必要的私有元数据。
- 会话必须按 `DocumentID` 隔离，存入 workspace SQLite，并在应用重启后恢复。
- 新会话首轮必须由模型生成标题；标题成功前不发起普通回复。
- 所有 Agent prompts 位于 `internal/agent/prompts/`，用 Go 模板渲染并嵌入二进制。
- API Key 只在显式保存时写入系统凭据库；设置页模型发现不得记录密钥或发送文档/聊天内容。
- Provider `Stop` 优先尝试正式支持的远程终止操作，同时取消本地流；终止 context 独立且有时限。

## Review Focus

- SSE/NDJSON 单块同时返回 thinking 和正文时，两类事件都保留并按 thinking 后正文发送。
- 用户编辑 API Key、Provider 或 BaseURL 时，取消旧列表请求并拒绝应用迟到结果。
- 同一文档多个历史会话及多个文档之间的 transcript/context 不串线。
- 保存失败、标题请求失败、普通流取消时，不能丢弃已确认持久化的用户输入或工具往返。
- 停止当前 provider 流不能误取消同一 Provider 实例上的其他文档会话。

---

### Task 1: Provider 契约与 apic 服务商实现

**Files:**
- Modify: `internal/agent/provider/provider.go`, `internal/agent/provider/errors.go`
- Implement apic requests, callbacks, and response parsing independently under each provider directory; do not add a shared protocol parser.
- Create: `internal/agent/provider/minimax/*`, `internal/agent/provider/openai/*`
- Modify/Create: `internal/agent/provider/kimi/*`, `internal/agent/provider/deepseek/*`, `internal/agent/provider/ollama/*`
- Remove/replace: `internal/agent/provider/chat_completions.go`, `internal/agent/provider/ollama.go`, old draft constructors as call sites migrate
- Test: provider package-local `*_test.go` files

**Interfaces:**
- Produces: `ModelProvider{ListModels(ctx) ([]ModelInfo,error), Stream(ctx, ModelRequest, emit) error, Stop(ctx, requestID) error}`; `ModelRequest.RequestID` is local-only; `ModelEvent` has separate `TextDelta`/`ReasoningDelta`.
- Each service package binds endpoint, credentials, HTTP client, list-model decoder, stream decoder, and optional native remote-stop request.

- [x] Add tests for normalized model lists, auth/status handling, reasoning and text channels, tool/usage/done events, and cancellation.
- [x] Add Stop isolation tests proving only the requested active stream ends; current protocols expose no remote-stop operation.
- [x] Implement independent per-provider apic adapters and response parsers without global `apic.Init()` or Debug.
- [x] Run `go test ./internal/agent/provider/...` and inspect every package result.

### Task 2: Central Go prompt templates

**Files:**
- Create: `internal/agent/prompts/*.tmpl`, `internal/agent/prompts/prompts.go`
- Test: `internal/agent/prompts/prompts_test.go`
- Modify: `internal/agent/host.go` and title-generation caller to consume rendered prompts

**Interfaces:**
- Produces typed render functions for the Agent system instruction and first-message title instruction; templates load through `embed.FS` and execute with `missingkey=error`.

- [x] Add tests for template substitutions, missing required data, and user text containing `{{...}}` remaining literal data.
- [x] Implement templates and render functions; remove inline Agent/title instruction strings.
- [x] Run `go test ./internal/agent/prompts`.

### Task 3: Durable document-scoped conversation repository

**Files:**
- Modify: `internal/workspace/store.go`
- Create: `internal/workspace/agent_conversations.go`
- Modify: `internal/workspace/word_versions_test.go`, `internal/workspace/store_test.go`
- Create: `internal/workspace/agent_conversations_test.go`
- Create/modify: `internal/agent/conversations.go`, `internal/agent/conversations_test.go`

**Interfaces:**
- Produces neutral conversation record/repository APIs: `CreateConversation`, `ListConversations`, `LoadConversation`, `SaveConversation`, scoped by document ID.
- SQLite schema migrates v4 to v5 with independent conversation records, indexed by document and ordered by update time.

- [x] Add migration tests and repository tests for document-scoped history, persistence, and cross-document access rejection.
- [x] Implement transactional v5 migration and workspace storage methods while preserving existing document/workspace rows.
- [x] Run `go test ./internal/workspace`.

### Task 4: AgentHost context, title, events, and persistence lifecycle

**Files:**
- Modify: `internal/agent/host.go`, `internal/agent/events.go`, `internal/agent/errors.go`
- Modify: `internal/agent/host_test.go`

**Interfaces:**
- AgentHost loads the selected persisted context per send, persists each committed boundary, emits reasoning separately, generates the first title through a tool-free provider request, and sends `RequestID` to Stream/Stop.

- [x] Add tests for restart/provider replacement restore, title success/failure/retry, reasoning forwarding, Stop request IDs, and persistence failures.
- [x] Implement repository-backed messages and turn lifecycle without changing MCP tool/confirmation semantics.
- [x] Run `go test ./internal/agent/...`.

### Task 5: Runtime and document-bound chat/history UI

**Files:**
- Modify: `internal/desktopui/runtime.go`, `internal/desktopui/agent_ui_state.go`, `internal/desktopui/agent_chat_ui.go`
- Create focused files for conversation history/title chooser and reasoning/answer presentation
- Modify: `internal/desktopui/agent_chat_ui_test.go`, `internal/desktopui/runtime_agent_test.go`

**Interfaces:**
- Runtime exposes history list/load/create calls independent of the active AgentHost; UI uses active document ID, supports new chat/history switching, and hydrates persisted transcript.

- [x] Add tests for document-scoped history persistence, transcript restore, and provider replacement.
- [x] Add event-sequence tests for separate reasoning, collapse on answer, and persisted collapsed history.
- [x] Implement the UI and runtime flow with Gio operations/background calls outside layout; keep stable widgets in state.
- [x] Run focused desktop UI tests and the full package with sandbox loopback-only tests skipped.

### Task 6: Provider settings model discovery

**Files:**
- Modify: `internal/desktopui/agent_settings_page.go`, `internal/desktopui/agent_settings_ui.go`, `internal/desktopui/agent_ui_state.go`, `internal/desktopui/runtime.go`
- Modify/Create: `internal/desktopui/runtime_agent_test.go`, settings model-discovery tests

**Interfaces:**
- Runtime creates a temporary provider from the current draft and calls `ListModels`; UI stores normalized options and request generation/cancel state.

- [x] Add tests for draft-key provider construction, lookup fingerprints, required credentials, and stale result rejection.
- [x] Implement short debounce, per-request cancellation, snapshot/generation checks, and model select options; persist secrets only on explicit save.
- [x] Run focused settings tests and verify Ollama lookup is requested without a key.

### Task 7: Integration verification

**Files:**
- Modify: focused provider/Agent/runtime/UI tests only as gaps are found

- [x] Run all targeted package suites from the tasks above.
- [x] Run `go test ./...`; record loopback-bind failures in `.superpowers/sdd/2026-10-12-agent-provider-unification/progress.md`.
- [x] Run `git diff --check` and inspect `git status`/diff for leaked secrets and stale source imports.
- [x] Do not send a real provider request without the user selecting a configured provider for that live check.
