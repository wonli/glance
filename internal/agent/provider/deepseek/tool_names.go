package deepseek

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"glance/internal/agent/provider"
)

var functionNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// DeepSeek function names cannot contain the dots used by MCP capabilities.
// Keep aliases inside the adapter, including prior assistant tool calls, so
// persistence, confirmation cards and MCP execution retain the original names.
type toolNameCodec struct {
	toWire   map[string]string
	fromWire map[string]string
}

func newToolNameCodec(request provider.ModelRequest) toolNameCodec {
	names := make(map[string]bool)
	for _, tool := range request.Tools {
		names[tool.Function.Name] = true
	}
	for _, message := range request.Messages {
		if message.Name != "" {
			names[message.Name] = true
		}
		for _, call := range message.ToolCalls {
			names[call.Function.Name] = true
		}
	}
	codec := toolNameCodec{toWire: make(map[string]string), fromWire: make(map[string]string)}
	var invalid []string
	for name := range names {
		if functionNamePattern.MatchString(name) {
			codec.toWire[name], codec.fromWire[name] = name, name
		} else {
			invalid = append(invalid, name)
		}
	}
	sort.Strings(invalid)
	for _, name := range invalid {
		for salt := 0; ; salt++ {
			hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", name, salt)))
			alias := fmt.Sprintf("glance_%x", hash[:28])
			if _, used := codec.fromWire[alias]; used {
				continue
			}
			codec.toWire[name], codec.fromWire[alias] = alias, name
			break
		}
	}
	return codec
}

func (c toolNameCodec) tools(tools []provider.ModelTool) []provider.ModelTool {
	result := append([]provider.ModelTool(nil), tools...)
	for i := range result {
		result[i].Function.Name = c.toWire[result[i].Function.Name]
	}
	return result
}

func (c toolNameCodec) messages(messages []chatMessage) []chatMessage {
	for i := range messages {
		if messages[i].Name != "" {
			messages[i].Name = c.toWire[messages[i].Name]
		}
		messages[i].ToolCalls = append([]provider.ModelToolCall(nil), messages[i].ToolCalls...)
		for j := range messages[i].ToolCalls {
			call := &messages[i].ToolCalls[j]
			call.Function.Name = c.toWire[call.Function.Name]
		}
	}
	return messages
}

func (c toolNameCodec) emitter(emit func(provider.ModelEvent) error) func(provider.ModelEvent) error {
	// Buffer only partial aliases; leave ordinary tool delta delivery intact.
	names := make(map[int]string)
	return func(event provider.ModelEvent) error {
		if event.ToolCallDelta != nil && event.ToolCallDelta.Name != "" {
			delta := *event.ToolCallDelta
			name := names[delta.Index] + delta.Name
			if len(name) > 64 {
				return fmt.Errorf("%w: tool function name exceeds 64 bytes", provider.ErrInvalidResponse)
			}
			partial := false
			for wire := range c.fromWire {
				if len(wire) > len(name) && strings.HasPrefix(wire, name) {
					partial = true
					break
				}
			}
			if partial {
				names[delta.Index] = name
				delta.Name = ""
			} else if original, ok := c.fromWire[name]; ok {
				delta.Name = original
				delete(names, delta.Index)
			} else {
				delta.Name = name
				delete(names, delta.Index)
			}
			event.ToolCallDelta = &delta
		}
		if event.Done || event.FinishReason != "" {
			indices := make([]int, 0, len(names))
			for index := range names {
				indices = append(indices, index)
			}
			sort.Ints(indices)
			for _, index := range indices {
				name := names[index]
				if original, ok := c.fromWire[name]; ok {
					name = original
				}
				if err := emit(provider.ModelEvent{ToolCallDelta: &provider.ToolCallDelta{Index: index, Name: name}}); err != nil {
					return err
				}
				delete(names, index)
			}
		}
		return emit(event)
	}
}
