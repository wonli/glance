package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"glance/internal/agent/provider"
	"glance/internal/application"
)

func TestToolRoundBudgetAllowsLastToolAndFinalAnswer(t *testing.T) {
	r, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(context.Background()) })
	steps := []func(provider.ModelRequest) []provider.ModelEvent{
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "list", Type: "function", Name: "document.list", ArgumentsDelta: "{}"}}}
		},
		func(req provider.ModelRequest) []provider.ModelEvent {
			if len(req.Tools) != 0 || req.Messages[len(req.Messages)-1].Role != "tool" {
				t.Error("final answer request must include tool result and no further tools")
			}
			return []provider.ModelEvent{{TextDelta: "已完成查询。"}, {Done: true}}
		},
	}
	model := &scriptedProvider{t: t, steps: append(append([]func(provider.ModelRequest) []provider.ModelEvent(nil), steps...), steps...)}
	h := newTestAgentHost(t, r, model, HostOptions{MaxToolRounds: 1, MaxToolCalls: 1})
	for _, id := range []ConversationID{"fresh-one", "fresh-two"} {
		if err := h.Send(context.Background(), id, "查询", application.ViewContext{}); err != nil {
			t.Fatalf("fresh conversation %s rejected within budget: %v", id, err)
		}
	}
}

func TestToolRoundBudgetRejectsExtraToolAfterLastRound(t *testing.T) {
	r, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(context.Background()) })
	tool := func(provider.ModelRequest) []provider.ModelEvent {
		return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "list", Type: "function", Name: "document.list", ArgumentsDelta: "{}"}}}
	}
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{tool, tool}}
	h := newTestAgentHost(t, r, model, HostOptions{MaxToolRounds: 1})
	err = h.Send(context.Background(), "bounded", "查询", application.ViewContext{})
	if !errors.Is(err, ErrToolRoundLimit) {
		t.Fatalf("excess tool error=%v", err)
	}
	s, _ := h.Snapshot("bounded")
	if len(s.Messages) != 3 || s.Messages[2].Role != "tool" {
		t.Fatal("allowed tool result missing, or extra tool ran")
	}
}

func TestDefaultToolBudgetSupportsMultistepDocumentTask(t *testing.T) {
	r, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(context.Background()) })
	var steps []func(provider.ModelRequest) []provider.ModelEvent
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("list-%d", i)
		steps = append(steps, func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: id, Type: "function", Name: "document.list", ArgumentsDelta: "{}"}}}
		})
	}
	steps = append(steps, func(provider.ModelRequest) []provider.ModelEvent {
		return []provider.ModelEvent{{TextDelta: "已完成。"}, {Done: true}}
	})
	model := &scriptedProvider{t: t, steps: steps}
	h := newTestAgentHost(t, r, model, HostOptions{})
	if err := h.Send(context.Background(), "multistep", "多步检查", application.ViewContext{}); err != nil {
		t.Fatal(err)
	}
}

func TestToolCallBudgetLeavesAnswerOpportunity(t *testing.T) {
	r, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(context.Background()) })
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "list", Type: "function", Name: "document.list", ArgumentsDelta: "{}"}}}
		},
		func(req provider.ModelRequest) []provider.ModelEvent {
			if len(req.Tools) != 0 {
				t.Error("exhausted call budget still advertises executable tools")
			}
			return []provider.ModelEvent{{TextDelta: "已完成查询。"}, {Done: true}}
		},
	}}
	h := newTestAgentHost(t, r, model, HostOptions{MaxToolRounds: 4, MaxToolCalls: 1})
	if err := h.Send(context.Background(), "call-budget", "查询", application.ViewContext{}); err != nil {
		t.Fatal(err)
	}
}
