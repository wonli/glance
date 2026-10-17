package application

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/jsonschema"
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
	if !json.Valid(arguments) {
		t.Fatalf("generated arguments for %s are invalid JSON", capabilityID)
	}
	base := filepath.Join(t.TempDir(), strings.NewReplacer(".", "-", "/", "-").Replace(capabilityID))
	encoded, err := rewriteGeneratedPaths(arguments, base)
	if err != nil {
		t.Fatalf("encode isolated arguments for %s: %v", capabilityID, err)
	}
	return encoded
}

func rewriteGeneratedPaths(value json.RawMessage, base string) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value, &fields); err == nil && fields != nil {
		for key, child := range fields {
			if strings.Contains(strings.ToLower(key), "path") {
				var path string
				if json.Unmarshal(child, &path) == nil {
					fields[key], _ = json.Marshal(base + ".tmp")
					continue
				}
			}
			rewritten, err := rewriteGeneratedPaths(child, base)
			if err != nil {
				return nil, err
			}
			fields[key] = rewritten
		}
		return json.Marshal(fields)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(value, &items); err == nil && items != nil {
		for index, item := range items {
			rewritten, err := rewriteGeneratedPaths(item, base)
			if err != nil {
				return nil, err
			}
			items[index] = rewritten
		}
		return json.Marshal(items)
	}
	return append(json.RawMessage(nil), value...), nil
}

func exampleArguments(schemaJSON json.RawMessage) (json.RawMessage, error) {
	var schema jsonschema.Definition
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

type schemaComposition struct {
	branches []jsonschema.Definition
	present  bool
}

func exampleSchemaValue(schema jsonschema.Definition) (json.RawMessage, bool) {
	if schema.Has("const") {
		return append(json.RawMessage(nil), schema.Const...), true
	}
	if schema.Has("enum") {
		if len(schema.Enum) == 0 {
			return nil, false
		}
		return append(json.RawMessage(nil), schema.Enum[0]...), true
	}
	if schema.Has("allOf") {
		composed := schema.Without("allOf")
		for _, branch := range schema.AllOf {
			composed = mergeExampleSchemas(composed, branch)
		}
		return exampleSchemaValue(composed)
	}
	for _, composition := range []schemaComposition{
		{schema.OneOf, schema.Has("oneOf")},
		{schema.AnyOf, schema.Has("anyOf")},
	} {
		if !composition.present {
			continue
		}
		for _, branch := range composition.branches {
			composed := schema.Without("oneOf").Without("anyOf")
			composed = mergeExampleSchemas(composed, branch)
			if value, ok := exampleSchemaValue(composed); ok {
				return value, true
			}
		}
		return nil, false
	}
	typeName := ""
	if schema.Type != nil {
		typeName = *schema.Type
	}
	switch typeName {
	case "object":
		object := make(map[string]json.RawMessage, len(schema.Required))
		for _, name := range schema.Required {
			property, exists := schema.Properties[name]
			if !exists {
				return nil, false
			}
			value, ok := exampleSchemaValue(property)
			if !ok {
				return nil, false
			}
			object[name] = value
		}
		encoded, err := json.Marshal(object)
		return encoded, err == nil
	case "array":
		count := 0
		if schema.Has("minItems") {
			minimum, ok := schemaInteger(schema.MinItems)
			if !ok {
				return nil, false
			}
			count = int(minimum)
		}
		if count > 0 && schema.Items == nil {
			return nil, false
		}
		array := make([]json.RawMessage, count)
		for index := range array {
			var value json.RawMessage
			var ok bool
			if schema.UniqueItems != nil && *schema.UniqueItems {
				value, ok = uniqueExampleSchemaValue(*schema.Items, index)
			} else {
				value, ok = exampleSchemaValue(*schema.Items)
			}
			if !ok {
				return nil, false
			}
			array[index] = value
		}
		encoded, err := json.Marshal(array)
		return encoded, err == nil
	case "string":
		if schema.Pattern != nil && *schema.Pattern == "^#[0-9A-Fa-f]{6}$" {
			encoded, err := json.Marshal("#000000")
			return encoded, err == nil
		}
		if schema.Pattern != nil && *schema.Pattern == `^\d{4}-\d{2}-\d{2}$` {
			encoded, err := json.Marshal("2000-01-01")
			return encoded, err == nil
		}
		if schema.Pattern != nil && *schema.Pattern == `^([01][0-9]|2[0-3]):[0-5][0-9]$` {
			encoded, err := json.Marshal("09:30")
			return encoded, err == nil
		}
		minimum := int64(1)
		if schema.Has("minLength") {
			parsed, ok := schemaInteger(schema.MinLength)
			if !ok {
				return nil, false
			}
			minimum = max(1, parsed)
		}
		encoded, err := json.Marshal(repeatX(int(minimum)))
		return encoded, err == nil
	case "integer", "number":
		minimum := 1.0
		if schema.Has("minimum") {
			parsed, ok := schemaNumber(schema.Minimum)
			if !ok {
				return nil, false
			}
			minimum = parsed
		}
		if schema.Has("exclusiveMinimum") {
			exclusive, ok := schemaNumber(schema.ExclusiveMinimum)
			if !ok {
				return nil, false
			}
			if minimum <= exclusive {
				minimum = exclusive + 1
			}
		}
		if schema.Has("multipleOf") {
			multipleOf, ok := schemaNumber(schema.MultipleOf)
			if !ok || multipleOf <= 0 {
				return nil, false
			}
			minimum = math.Ceil(minimum/multipleOf) * multipleOf
		}
		if schema.Has("maximum") {
			maximum, ok := schemaNumber(schema.Maximum)
			if !ok || minimum > maximum {
				return nil, false
			}
		}
		if typeName == "integer" {
			minimum = math.Ceil(minimum)
		}
		encoded, err := json.Marshal(minimum)
		return encoded, err == nil
	case "boolean":
		encoded, err := json.Marshal(false)
		return encoded, err == nil
	case "null":
		return json.RawMessage("null"), true
	default:
		if len(schema.Properties) > 0 || len(schema.Required) > 0 {
			return exampleSchemaValue(jsonschema.Definition{Type: jsonschema.StringType("object"), Properties: schema.Properties, Required: schema.Required})
		}
		return json.RawMessage("{}"), true
	}
}

func uniqueExampleSchemaValue(schema jsonschema.Definition, index int) (json.RawMessage, bool) {
	if schema.Has("enum") {
		if index >= len(schema.Enum) {
			return nil, false
		}
		return append(json.RawMessage(nil), schema.Enum[index]...), true
	}
	if index == 0 {
		return exampleSchemaValue(schema)
	}
	typeName := ""
	if schema.Type != nil {
		typeName = *schema.Type
	}
	switch typeName {
	case "integer", "number":
		encoded, ok := exampleSchemaValue(schema)
		if !ok {
			return nil, false
		}
		var base float64
		if err := json.Unmarshal(encoded, &base); err != nil {
			return nil, false
		}
		step := 1.0
		if schema.Has("multipleOf") {
			multipleOf, valid := schemaNumber(schema.MultipleOf)
			if !valid || multipleOf <= 0 {
				return nil, false
			}
			step = multipleOf
		}
		candidate := base + float64(index)*step
		if schema.Has("maximum") {
			maximum, valid := schemaNumber(schema.Maximum)
			if !valid || candidate > maximum {
				return nil, false
			}
		}
		if typeName == "integer" {
			candidate = math.Round(candidate)
		}
		encoded, err := json.Marshal(candidate)
		return encoded, err == nil
	default:
		return nil, false
	}
}

func mergeExampleSchemas(base, overlay jsonschema.Definition) jsonschema.Definition {
	merged := base
	if overlay.Type != nil {
		merged.Type = overlay.Type
	}
	if overlay.Title != "" {
		merged.Title = overlay.Title
	}
	if overlay.Description != "" {
		merged.Description = overlay.Description
	}
	if overlay.Properties != nil {
		properties := make(map[string]jsonschema.Definition, len(base.Properties)+len(overlay.Properties))
		for name, property := range base.Properties {
			properties[name] = property
		}
		for name, property := range overlay.Properties {
			properties[name] = property
		}
		merged.Properties = properties
	}
	if overlay.Required != nil {
		merged.Required = append(append([]string(nil), base.Required...), overlay.Required...)
		merged.Required = uniqueStrings(merged.Required)
	}
	if len(overlay.AdditionalProperties) > 0 {
		merged.AdditionalProperties = overlay.AdditionalProperties
	}
	if overlay.Items != nil {
		merged.Items = overlay.Items
	}
	if overlay.AnyOf != nil {
		merged.AnyOf = overlay.AnyOf
	}
	if overlay.AllOf != nil {
		merged.AllOf = overlay.AllOf
	}
	if overlay.OneOf != nil {
		merged.OneOf = overlay.OneOf
	}
	if overlay.Enum != nil {
		merged.Enum = overlay.Enum
	}
	if len(overlay.Const) > 0 {
		merged.Const = overlay.Const
	}
	if len(overlay.Minimum) > 0 {
		merged.Minimum = overlay.Minimum
	}
	if len(overlay.Maximum) > 0 {
		merged.Maximum = overlay.Maximum
	}
	if len(overlay.ExclusiveMinimum) > 0 {
		merged.ExclusiveMinimum = overlay.ExclusiveMinimum
	}
	if len(overlay.ExclusiveMaximum) > 0 {
		merged.ExclusiveMaximum = overlay.ExclusiveMaximum
	}
	if len(overlay.MultipleOf) > 0 {
		merged.MultipleOf = overlay.MultipleOf
	}
	if len(overlay.MinLength) > 0 {
		merged.MinLength = overlay.MinLength
	}
	if len(overlay.MaxLength) > 0 {
		merged.MaxLength = overlay.MaxLength
	}
	if len(overlay.MinItems) > 0 {
		merged.MinItems = overlay.MinItems
	}
	if len(overlay.MaxItems) > 0 {
		merged.MaxItems = overlay.MaxItems
	}
	if overlay.Pattern != nil {
		merged.Pattern = overlay.Pattern
	}
	if overlay.UniqueItems != nil {
		merged.UniqueItems = overlay.UniqueItems
	}
	encoded, err := json.Marshal(merged)
	if err == nil {
		var normalized jsonschema.Definition
		if json.Unmarshal(encoded, &normalized) == nil {
			return normalized
		}
	}
	return merged
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
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
