package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestCatalogRequiresUniqueDescribedCapabilitiesAndHandlers(t *testing.T) {
	valid := CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.cell.set", Name: "设置单元格", Description: "将指定单元格设置为文本值。",
			Category: CapabilityCommand, InputSchema: json.RawMessage(`{"type":"object"}`),
			OutputSchema: json.RawMessage(`{"type":"object"}`), Formats: []Format{FormatXLSX}, Risk: RiskLow,
		},
		Handler: func(context.Context, CapabilityContext, json.RawMessage) (CapabilityResult, error) {
			return CapabilityResult{}, nil
		},
	}
	catalog := NewCatalog()
	if err := catalog.Register(valid); err != nil {
		t.Fatalf("Register valid capability: %v", err)
	}
	if err := catalog.Register(valid); !errors.Is(err, ErrDuplicateCapability) {
		t.Fatalf("duplicate registration error = %v, want ErrDuplicateCapability", err)
	}

	withoutHandler := valid
	withoutHandler.Descriptor.ID = "spreadsheet.cell.clear"
	withoutHandler.Handler = nil
	if err := catalog.Register(withoutHandler); !errors.Is(err, ErrCapabilityHandlerMissing) {
		t.Fatalf("missing handler error = %v, want ErrCapabilityHandlerMissing", err)
	}
	invalidSchema := valid
	invalidSchema.Descriptor.ID = "spreadsheet.cell.invalid"
	invalidSchema.Descriptor.InputSchema = json.RawMessage(`{"type":`)
	if err := catalog.Register(invalidSchema); !errors.Is(err, ErrInvalidCapabilitySchema) {
		t.Fatalf("invalid schema error = %v, want ErrInvalidCapabilitySchema", err)
	}

	descriptors := catalog.Descriptors()
	if len(descriptors) != 1 || descriptors[0].ID != "spreadsheet.cell.set" || descriptors[0].Description == "" {
		t.Fatalf("catalog did not expose its registered description: %+v", descriptors)
	}
	descriptors[0].Formats[0] = FormatPDF
	if catalog.Descriptors()[0].Formats[0] != FormatXLSX {
		t.Fatal("mutating a returned descriptor changed the registered capability")
	}
}

func TestValidateArgumentsEnforcesDeclaredSchemaConstraints(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"mode":{"type":"string","enum":["portrait","landscape"]},"width":{"type":"number","minimum":1,"maximum":3000},"name":{"type":"string","minLength":2,"maxLength":4,"pattern":"^[A-Z][a-z]+$"},"ids":{"type":"array","minItems":2,"maxItems":3,"uniqueItems":true,"items":{"type":"integer","minimum":1,"maximum":9}}},"required":["mode","width","name","ids"],"anyOf":[{"required":["mode"]},{"required":["width"]}],"additionalProperties":false}`)
	valid := json.RawMessage(`{"mode":"landscape","width":792,"name":"Memo","ids":[1,2]}`)
	if err := validateArguments(schema, valid); err != nil {
		t.Fatalf("valid schema arguments rejected: %v", err)
	}
	invalid := []struct {
		name string
		args string
	}{
		{"enum", `{"mode":"diagonal","width":792,"name":"Memo","ids":[1,2]}`},
		{"minimum", `{"mode":"portrait","width":0,"name":"Memo","ids":[1,2]}`},
		{"maximum", `{"mode":"portrait","width":3001,"name":"Memo","ids":[1,2]}`},
		{"minimum length", `{"mode":"portrait","width":792,"name":"A","ids":[1,2]}`},
		{"maximum length", `{"mode":"portrait","width":792,"name":"Longer","ids":[1,2]}`},
		{"pattern", `{"mode":"portrait","width":792,"name":"memo","ids":[1,2]}`},
		{"minimum items", `{"mode":"portrait","width":792,"name":"Memo","ids":[1]}`},
		{"maximum items", `{"mode":"portrait","width":792,"name":"Memo","ids":[1,2,3,4]}`},
		{"unique items", `{"mode":"portrait","width":792,"name":"Memo","ids":[1,1]}`},
		{"item bounds", `{"mode":"portrait","width":792,"name":"Memo","ids":[1,10]}`},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if err := validateArguments(schema, json.RawMessage(test.args)); !errors.Is(err, ErrInvalidCommandArguments) {
				t.Fatalf("invalid %s arguments error=%v, want ErrInvalidCommandArguments", test.name, err)
			}
		})
	}
	constSchema := json.RawMessage(`{"type":"object","properties":{"enabled":{"type":"boolean","const":false}},"required":["enabled"],"additionalProperties":false}`)
	if err := validateArguments(constSchema, json.RawMessage(`{"enabled":false}`)); err != nil {
		t.Fatalf("matching const rejected: %v", err)
	}
	if err := validateArguments(constSchema, json.RawMessage(`{"enabled":true}`)); !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("const mismatch error=%v, want ErrInvalidCommandArguments", err)
	}
	anyOfSchema := json.RawMessage(`{"type":"object","properties":{"mode":{"type":"string"},"width":{"type":"number"}},"anyOf":[{"required":["mode"]},{"required":["width"]}],"additionalProperties":false}`)
	for _, arguments := range []string{`{"mode":"portrait"}`, `{"width":792}`, `{"mode":"portrait","width":792}`} {
		if err := validateArguments(anyOfSchema, json.RawMessage(arguments)); err != nil {
			t.Errorf("anyOf rejected %s: %v", arguments, err)
		}
	}
	if err := validateArguments(anyOfSchema, json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("anyOf mismatch error=%v, want ErrInvalidCommandArguments", err)
	}
	exclusiveSchema := json.RawMessage(`{"type":"number","exclusiveMinimum":0}`)
	if err := validateArguments(exclusiveSchema, json.RawMessage(`0`)); !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("exclusiveMinimum boundary error=%v, want ErrInvalidCommandArguments", err)
	}
	integerSchema := json.RawMessage(`{"type":"integer"}`)
	if err := validateArguments(integerSchema, json.RawMessage(`1.0`)); err != nil {
		t.Fatalf("mathematically integral JSON number rejected: %v", err)
	}
	if err := validateArguments(integerSchema, json.RawMessage(`9007199254740992.1`)); !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("fractional large integer error=%v, want ErrInvalidCommandArguments", err)
	}
	multipleSchema := json.RawMessage(`{"type":"number","multipleOf":0.1}`)
	if err := validateArguments(multipleSchema, json.RawMessage(`0.3`)); err != nil {
		t.Fatalf("exact decimal multiple rejected: %v", err)
	}
	if err := validateArguments(multipleSchema, json.RawMessage(`0.31`)); !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("non-multiple error=%v, want ErrInvalidCommandArguments", err)
	}
	unicodeSchema := json.RawMessage(`{"type":"string","minLength":2,"maxLength":2}`)
	if err := validateArguments(unicodeSchema, json.RawMessage(`"甲"`)); !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("single Unicode code point error=%v, want ErrInvalidCommandArguments", err)
	}
	if err := validateArguments(unicodeSchema, json.RawMessage(`"甲乙"`)); err != nil {
		t.Fatalf("two Unicode code points rejected: %v", err)
	}
	allOfSchema := json.RawMessage(`{"type":"integer","allOf":[{"minimum":1},{"maximum":3}]}`)
	if err := validateArguments(allOfSchema, json.RawMessage(`2`)); err != nil {
		t.Fatalf("allOf rejected value satisfying every branch: %v", err)
	}
	if err := validateArguments(allOfSchema, json.RawMessage(`4`)); !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("allOf mismatch error=%v, want ErrInvalidCommandArguments", err)
	}
	oneOfSchema := json.RawMessage(`{"type":"integer","oneOf":[{"minimum":1},{"maximum":2}]}`)
	if err := validateArguments(oneOfSchema, json.RawMessage(`3`)); err != nil {
		t.Fatalf("oneOf rejected value matching exactly one branch: %v", err)
	}
	if err := validateArguments(oneOfSchema, json.RawMessage(`2`)); !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("oneOf overlap error=%v, want ErrInvalidCommandArguments", err)
	}
}
