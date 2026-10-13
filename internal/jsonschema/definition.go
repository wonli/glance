package jsonschema

import (
	"encoding/json"
	"errors"
)

// Definition models the JSON Schema keywords understood by Glance. Property
// names are dynamic, so Properties is a map; each property value remains a
// typed schema definition. Unknown keywords are retained as raw JSON so schema
// adapters can preserve extensions they do not interpret.
type Definition struct {
	Type                 *string               `json:"type,omitempty"`
	Title                string                `json:"title,omitempty"`
	Description          string                `json:"description,omitempty"`
	Properties           map[string]Definition `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	AdditionalProperties json.RawMessage       `json:"additionalProperties,omitempty"`
	Items                *Definition           `json:"items,omitempty"`
	AnyOf                []Definition          `json:"anyOf,omitempty"`
	AllOf                []Definition          `json:"allOf,omitempty"`
	OneOf                []Definition          `json:"oneOf,omitempty"`
	Enum                 []json.RawMessage     `json:"enum,omitempty"`
	Const                json.RawMessage       `json:"const,omitempty"`
	Minimum              json.RawMessage       `json:"minimum,omitempty"`
	Maximum              json.RawMessage       `json:"maximum,omitempty"`
	ExclusiveMinimum     json.RawMessage       `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     json.RawMessage       `json:"exclusiveMaximum,omitempty"`
	MultipleOf           json.RawMessage       `json:"multipleOf,omitempty"`
	MinLength            json.RawMessage       `json:"minLength,omitempty"`
	MaxLength            json.RawMessage       `json:"maxLength,omitempty"`
	MinItems             json.RawMessage       `json:"minItems,omitempty"`
	MaxItems             json.RawMessage       `json:"maxItems,omitempty"`
	Pattern              *string               `json:"pattern,omitempty"`
	UniqueItems          *bool                 `json:"uniqueItems,omitempty"`
	extra                map[string]json.RawMessage
	present              map[string]bool
}

// Has reports whether a keyword appeared in the input, including when it was
// explicitly set to null. That distinction lets validators reject malformed
// null values instead of treating them as omitted fields.
func (d Definition) Has(keyword string) bool {
	return d.present[keyword]
}

// Without returns a copy with one composed-schema keyword removed.
func (d Definition) Without(keyword string) Definition {
	if d.present != nil {
		present := make(map[string]bool, len(d.present))
		for key, value := range d.present {
			present[key] = value
		}
		delete(present, keyword)
		d.present = present
	}
	switch keyword {
	case "anyOf":
		d.AnyOf = nil
	case "allOf":
		d.AllOf = nil
	case "oneOf":
		d.OneOf = nil
	}
	return d
}

func (d *Definition) UnmarshalJSON(data []byte) error {
	if d == nil {
		return errors.New("cannot unmarshal a schema into a nil receiver")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("schema must be a JSON object")
	}
	type plainDefinition Definition
	var decoded plainDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*d = Definition(decoded)
	d.present = make(map[string]bool, len(fields))
	d.extra = make(map[string]json.RawMessage)
	for keyword, raw := range fields {
		d.present[keyword] = true
		if !knownKeyword(keyword) {
			d.extra[keyword] = append(json.RawMessage(nil), raw...)
		}
	}
	return nil
}

func (d Definition) MarshalJSON() ([]byte, error) {
	type plainDefinition Definition
	encoded, err := json.Marshal(plainDefinition(d))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	for keyword, raw := range d.extra {
		if _, replaced := fields[keyword]; !replaced {
			fields[keyword] = append(json.RawMessage(nil), raw...)
		}
	}
	return json.Marshal(fields)
}

func knownKeyword(keyword string) bool {
	switch keyword {
	case "type", "title", "description", "properties", "required", "additionalProperties", "items",
		"anyOf", "allOf", "oneOf", "enum", "const", "minimum", "maximum", "exclusiveMinimum",
		"exclusiveMaximum", "multipleOf", "minLength", "maxLength", "minItems", "maxItems", "pattern",
		"uniqueItems":
		return true
	default:
		return false
	}
}

// RawBool encodes a boolean keyword without introducing a generic JSON value.
func RawBool(value bool) json.RawMessage {
	if value {
		return json.RawMessage("true")
	}
	return json.RawMessage("false")
}

// StringType returns a schema type value suitable for a Definition.
func StringType(value string) *string {
	return &value
}
