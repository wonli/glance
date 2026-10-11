package application

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryCommandRejectsStaleRevisionBeforeHandlerExecution(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}

	commands := 0
	for _, descriptor := range runtime.Catalog().Descriptors() {
		if descriptor.Category != CapabilityCommand {
			continue
		}
		commands++
		arguments, err := exampleArguments(descriptor.InputSchema)
		if err != nil {
			t.Errorf("%s: build schema-valid arguments: %v", descriptor.ID, err)
			continue
		}
		if err := validateArguments(descriptor.InputSchema, arguments); err != nil {
			t.Errorf("%s: generated arguments do not satisfy its schema: %v; arguments=%s", descriptor.ID, err, arguments)
			continue
		}
		_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "stale-command-" + descriptor.ID, DocumentID: doc.DocumentID,
			CapabilityID: descriptor.ID, ExpectedRevision: doc.Revision + 1,
			Operator:  Operator{ID: "test-human", Kind: OperatorHuman},
			View:      ViewContext{WindowID: 1, DocumentID: doc.DocumentID, Revision: doc.Revision + 1},
			Arguments: arguments,
		})
		if !errors.Is(err, ErrRevisionConflict) {
			t.Errorf("%s: stale command error = %v, want revision conflict", descriptor.ID, err)
		}
		current, infoErr := runtime.GetDocument(ctx, doc.DocumentID)
		if infoErr != nil {
			t.Fatalf("GetDocument after %s: %v", descriptor.ID, infoErr)
		}
		if current.Revision != doc.Revision || current.Dirty != doc.Dirty {
			t.Errorf("%s ran despite stale revision: revision=%d dirty=%v; initial revision=%d dirty=%v", descriptor.ID, current.Revision, current.Dirty, doc.Revision, doc.Dirty)
		}
	}
	if commands == 0 {
		t.Fatal("capability catalog has no commands")
	}
	t.Logf("validated stale-revision rejection for %d registered commands", commands)
}

func TestEveryCommandDispatchesSchemaValidRequestWithoutPartialFailure(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	documents := make(map[Format]DocumentInfo)
	commandCount, successCount, errorCount := 0, 0, 0
	for _, descriptor := range runtime.Catalog().Descriptors() {
		if descriptor.Category != CapabilityCommand {
			continue
		}
		commandCount++
		format := FormatXLSX
		if len(descriptor.Formats) > 0 {
			format = descriptor.Formats[0]
		}
		if format == FormatXLS {
			// Legacy XLS is intentionally read-only and cannot be created as a
			// blank command target.
			errorCount++
			continue
		}
		doc, exists := documents[format]
		if !exists {
			doc, err = runtime.CreateDocument(ctx, format)
			if err != nil {
				t.Fatalf("CreateDocument(%s) for %s: %v", format, descriptor.ID, err)
			}
			documents[format] = doc
		}
		arguments, err := exampleArguments(descriptor.InputSchema)
		if err != nil {
			t.Errorf("%s: build schema-valid arguments: %v", descriptor.ID, err)
			continue
		}
		arguments = isolateGeneratedPaths(t, arguments, descriptor.ID)
		if err := validateArguments(descriptor.InputSchema, arguments); err != nil {
			t.Errorf("%s: generated arguments do not satisfy its schema: %v; arguments=%s", descriptor.ID, err, arguments)
			continue
		}
		before, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil {
			t.Fatalf("GetDocument before %s: %v", descriptor.ID, err)
		}
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "dispatch-command-" + descriptor.ID, DocumentID: doc.DocumentID,
			CapabilityID: descriptor.ID, ExpectedRevision: before.Revision,
			Operator:  Operator{ID: "test-human", Kind: OperatorHuman},
			View:      ViewContext{WindowID: 1, DocumentID: doc.DocumentID, Revision: before.Revision},
			Arguments: arguments,
		})
		after, getErr := runtime.GetDocument(ctx, doc.DocumentID)
		if getErr != nil {
			t.Fatalf("GetDocument after %s: %v", descriptor.ID, getErr)
		}
		if invokeErr != nil {
			errorCount++
			if after.Revision != before.Revision || after.Dirty != before.Dirty {
				t.Errorf("%s returned error after partially changing document: err=%v revision=%d→%d dirty=%v→%v", descriptor.ID, invokeErr, before.Revision, after.Revision, before.Dirty, after.Dirty)
			}
			continue
		}
		successCount++
		if result.DocumentID != doc.DocumentID || result.Revision != after.Revision || result.Revision < before.Revision || result.Revision > before.Revision+1 {
			t.Errorf("%s returned inconsistent command result: result=%+v before=%+v after=%+v", descriptor.ID, result, before, after)
		}
	}
	if commandCount == 0 {
		t.Fatal("capability catalog has no commands")
	}
	t.Logf("dispatched %d registered commands: %d completed, %d returned validated errors", commandCount, successCount, errorCount)
}

func isolateGeneratedPaths(t *testing.T, arguments json.RawMessage, capabilityID string) json.RawMessage {
	t.Helper()
	var value any
	if err := json.Unmarshal(arguments, &value); err != nil {
		t.Fatalf("decode generated arguments for %s: %v", capabilityID, err)
	}
	base := filepath.Join(t.TempDir(), strings.NewReplacer(".", "-", "/", "-").Replace(capabilityID))
	var rewrite func(any) any
	rewrite = func(value any) any {
		switch current := value.(type) {
		case map[string]any:
			for key, child := range current {
				if strings.Contains(strings.ToLower(key), "path") {
					if _, ok := child.(string); ok {
						current[key] = base + ".tmp"
						continue
					}
				}
				current[key] = rewrite(child)
			}
			return current
		case []any:
			for index, child := range current {
				current[index] = rewrite(child)
			}
			return current
		default:
			return value
		}
	}
	encoded, err := json.Marshal(rewrite(value))
	if err != nil {
		t.Fatalf("encode isolated arguments for %s: %v", capabilityID, err)
	}
	return encoded
}

func exampleArguments(schemaJSON json.RawMessage) (json.RawMessage, error) {
	var schema map[string]any
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return nil, err
	}
	value, ok := exampleSchemaValue(schema)
	if !ok {
		return nil, ErrInvalidCommandArguments
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func exampleSchemaValue(schema map[string]any) (any, bool) {
	if constant, ok := schema["const"]; ok {
		return constant, true
	}
	if values, ok := schema["enum"].([]any); ok && len(values) > 0 {
		return values[0], true
	}
	if branches, ok := schema["allOf"].([]any); ok {
		composed := cloneSchemaMap(schema)
		delete(composed, "allOf")
		for _, branch := range branches {
			branchSchema, ok := branch.(map[string]any)
			if !ok {
				return nil, false
			}
			composed = mergeExampleSchemas(composed, branchSchema)
		}
		return exampleSchemaValue(composed)
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		branches, ok := schema[key].([]any)
		if !ok {
			continue
		}
		for _, branch := range branches {
			branchSchema, ok := branch.(map[string]any)
			if !ok {
				continue
			}
			composed := cloneSchemaMap(schema)
			delete(composed, key)
			composed = mergeExampleSchemas(composed, branchSchema)
			if value, ok := exampleSchemaValue(composed); ok {
				return value, true
			}
		}
		return nil, false
	}
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "object":
		properties, _ := schema["properties"].(map[string]any)
		required, _ := schema["required"].([]any)
		object := make(map[string]any, len(required))
		for _, rawName := range required {
			name, ok := rawName.(string)
			if !ok {
				return nil, false
			}
			property, ok := properties[name].(map[string]any)
			if !ok {
				return nil, false
			}
			value, ok := exampleSchemaValue(property)
			if !ok {
				return nil, false
			}
			object[name] = value
		}
		return object, true
	case "array":
		minimum, _ := schema["minItems"].(float64)
		count := int(minimum)
		items, ok := schema["items"].(map[string]any)
		if count > 0 && !ok {
			return nil, false
		}
		array := make([]any, count)
		for index := range array {
			var value any
			var ok bool
			if unique, _ := schema["uniqueItems"].(bool); unique {
				value, ok = uniqueExampleSchemaValue(items, index)
			} else {
				value, ok = exampleSchemaValue(items)
			}
			if !ok {
				return nil, false
			}
			array[index] = value
		}
		return array, true
	case "string":
		if pattern, _ := schema["pattern"].(string); pattern == "^#[0-9A-Fa-f]{6}$" {
			return "#000000", true
		}
		minimum, _ := schema["minLength"].(float64)
		if minimum < 1 {
			minimum = 1
		}
		return repeatX(int(minimum)), true
	case "integer", "number":
		minimum, hasMinimum := schema["minimum"].(float64)
		if !hasMinimum {
			minimum = 1
		}
		if exclusive, ok := schema["exclusiveMinimum"].(float64); ok && minimum <= exclusive {
			minimum = exclusive + 1
		}
		if multipleOf, ok := schema["multipleOf"].(float64); ok && multipleOf > 0 {
			minimum = math.Ceil(minimum/multipleOf) * multipleOf
		}
		if maximum, ok := schema["maximum"].(float64); ok && minimum > maximum {
			return nil, false
		}
		if typeName == "integer" {
			return math.Ceil(minimum), true
		}
		return minimum, true
	case "boolean":
		return false, true
	case "null":
		return nil, true
	default:
		return map[string]any{}, true
	}
}

func uniqueExampleSchemaValue(schema map[string]any, index int) (any, bool) {
	if values, ok := schema["enum"].([]any); ok {
		if index >= len(values) {
			return nil, false
		}
		return values[index], true
	}
	if index == 0 {
		return exampleSchemaValue(schema)
	}
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "integer", "number":
		value, ok := exampleSchemaValue(schema)
		if !ok {
			return nil, false
		}
		base, ok := value.(float64)
		if !ok {
			return nil, false
		}
		step := 1.0
		if multipleOf, ok := schema["multipleOf"].(float64); ok && multipleOf > 0 {
			step = multipleOf
		}
		candidate := base + float64(index)*step
		if maximum, ok := schema["maximum"].(float64); ok && candidate > maximum {
			return nil, false
		}
		if typeName == "integer" {
			return math.Round(candidate), true
		}
		return candidate, true
	default:
		return nil, false
	}
}

func cloneSchemaMap(schema map[string]any) map[string]any {
	cloned := make(map[string]any, len(schema))
	for key, value := range schema {
		cloned[key] = value
	}
	return cloned
}

func mergeExampleSchemas(base, overlay map[string]any) map[string]any {
	merged := cloneSchemaMap(base)
	for key, value := range overlay {
		switch key {
		case "properties":
			properties := make(map[string]any)
			if existing, ok := merged[key].(map[string]any); ok {
				for name, property := range existing {
					properties[name] = property
				}
			}
			if additions, ok := value.(map[string]any); ok {
				for name, property := range additions {
					properties[name] = property
				}
			}
			merged[key] = properties
		case "required":
			seen := make(map[string]bool)
			var required []any
			for _, group := range []any{merged[key], value} {
				if names, ok := group.([]any); ok {
					for _, rawName := range names {
						if name, ok := rawName.(string); ok && !seen[name] {
							seen[name] = true
							required = append(required, name)
						}
					}
				}
			}
			merged[key] = required
		default:
			merged[key] = value
		}
	}
	return merged
}

func repeatX(count int) string {
	if count < 1 {
		count = 1
	}
	if count > 1024 {
		count = 1024
	}
	bytes := make([]byte, count)
	for index := range bytes {
		bytes[index] = 'x'
	}
	return string(bytes)
}
