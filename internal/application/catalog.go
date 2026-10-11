package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"glance/internal/document"
)

var (
	ErrInvalidCapability         = errors.New("invalid capability registration")
	ErrDuplicateCapability       = errors.New("capability ID is already registered")
	ErrCapabilityHandlerMissing  = errors.New("capability handler is missing")
	ErrInvalidCapabilitySchema   = errors.New("capability JSON schema is invalid or unsupported")
	ErrCapabilityNotFound        = errors.New("capability is not registered")
	ErrCapabilityNotCommand      = errors.New("capability is not a command")
	ErrInvalidCommandArguments   = errors.New("command arguments do not match the capability schema")
	ErrRevisionConflict          = errors.New("document revision does not match expected revision")
	ErrExpectedRevisionRequired  = errors.New("command must include the expected document revision")
	ErrIdempotencyConflict       = errors.New("idempotency key was already used with a different request")
	ErrIdempotencyStoreFull      = errors.New("idempotency result store is full")
	ErrInvalidCommandResult      = errors.New("command handler returned invalid JSON")
	ErrInvalidIncomingMessage    = errors.New("incoming message is invalid")
	ErrEventBusClosed            = errors.New("application event bus is closed")
	ErrCapabilityUnsupported     = errors.New("capability does not support this document format")
	ErrViewContextMismatch       = errors.New("view context does not match the command target")
	ErrNoAttachedView            = errors.New("capability requires an attached desktop view")
	ErrHumanConfirmationRequired = errors.New("command requires human confirmation")
)

type CapabilityCategory string

const (
	CapabilityQuery    CapabilityCategory = "query"
	CapabilityCommand  CapabilityCategory = "command"
	CapabilityWorkflow CapabilityCategory = "workflow"
)

type RiskClass string

const (
	RiskLow      RiskClass = "low"
	RiskMedium   RiskClass = "medium"
	RiskHigh     RiskClass = "high"
	RiskCritical RiskClass = "critical"
)

type CapabilityDescriptor struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	Category         CapabilityCategory `json:"category"`
	InputSchema      json.RawMessage    `json:"input_schema"`
	OutputSchema     json.RawMessage    `json:"output_schema"`
	Formats          []Format           `json:"formats,omitempty"`
	Risk             RiskClass          `json:"risk"`
	RequiresDocument bool               `json:"requires_document,omitempty"`
	RequiresRevision bool               `json:"requires_revision,omitempty"`
	RequiresView     bool               `json:"requires_view,omitempty"`
}

type OperatorKind string

const (
	OperatorHuman         OperatorKind = "human"
	OperatorBuiltInAgent  OperatorKind = "built-in-agent"
	OperatorExternalAgent OperatorKind = "external-agent"
	OperatorSystem        OperatorKind = "system"
)

type Operator struct {
	ID   string       `json:"id"`
	Kind OperatorKind `json:"kind"`
}

// ViewContext carries serializable selection data. It never contains a Gio
// layout.Context, widget, or a pointer to window-owned state.
type ViewContext struct {
	WindowID   uint64            `json:"window_id,omitempty"`
	DocumentID string            `json:"document_id,omitempty"`
	Revision   uint64            `json:"revision,omitempty"`
	Selection  *SelectionContext `json:"selection,omitempty"`
}

type SelectionContext struct {
	TargetType string              `json:"target_type"`
	TargetID   string              `json:"target_id,omitempty"`
	Sheet      string              `json:"sheet,omitempty"`
	Range      *document.CellRange `json:"range,omitempty"`
}

type CapabilityContext struct {
	RequestID string
	Runtime   *Runtime
	Document  DocumentInfo
	Adapter   DocumentAdapter
	Operator  Operator
	View      ViewContext
}

type CapabilityResult struct {
	Data    json.RawMessage
	Changed bool
}

type CapabilityHandler func(context.Context, CapabilityContext, json.RawMessage) (CapabilityResult, error)

type CapabilityRegistration struct {
	Descriptor CapabilityDescriptor
	Handler    CapabilityHandler
}

type registeredCapability struct {
	descriptor CapabilityDescriptor
	handler    CapabilityHandler
}

type Catalog struct {
	mu            sync.RWMutex
	registrations map[string]registeredCapability
}

func NewCatalog() *Catalog {
	return &Catalog{registrations: make(map[string]registeredCapability)}
}

func (c *Catalog) Register(registration CapabilityRegistration) error {
	if c == nil {
		return ErrInvalidCapability
	}
	descriptor := cloneCapabilityDescriptor(registration.Descriptor)
	if !capabilityIDPattern.MatchString(descriptor.ID) || strings.TrimSpace(descriptor.Name) == "" || strings.TrimSpace(descriptor.Description) == "" {
		return fmt.Errorf("%w: ID, name, and operation description are required", ErrInvalidCapability)
	}
	if descriptor.Category != CapabilityQuery && descriptor.Category != CapabilityCommand && descriptor.Category != CapabilityWorkflow {
		return fmt.Errorf("%w: unknown category %q", ErrInvalidCapability, descriptor.Category)
	}
	if descriptor.Risk != RiskLow && descriptor.Risk != RiskMedium && descriptor.Risk != RiskHigh && descriptor.Risk != RiskCritical {
		return fmt.Errorf("%w: unknown risk class %q", ErrInvalidCapability, descriptor.Risk)
	}
	if registration.Handler == nil {
		return ErrCapabilityHandlerMissing
	}
	if len(descriptor.InputSchema) == 0 {
		descriptor.InputSchema = json.RawMessage(`{}`)
	}
	if len(descriptor.OutputSchema) == 0 {
		descriptor.OutputSchema = json.RawMessage(`{}`)
	}
	if err := validateSchemaDocument(descriptor.InputSchema); err != nil {
		return fmt.Errorf("%w: input schema: %v", ErrInvalidCapabilitySchema, err)
	}
	if err := validateSchemaDocument(descriptor.OutputSchema); err != nil {
		return fmt.Errorf("%w: output schema: %v", ErrInvalidCapabilitySchema, err)
	}
	sort.Slice(descriptor.Formats, func(i, j int) bool { return descriptor.Formats[i] < descriptor.Formats[j] })
	for index := 1; index < len(descriptor.Formats); index++ {
		if descriptor.Formats[index] == descriptor.Formats[index-1] {
			return fmt.Errorf("%w: duplicate format %q", ErrInvalidCapability, descriptor.Formats[index])
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.registrations[descriptor.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateCapability, descriptor.ID)
	}
	c.registrations[descriptor.ID] = registeredCapability{descriptor: descriptor, handler: registration.Handler}
	return nil
}

func (c *Catalog) Descriptors() []CapabilityDescriptor {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	items := make([]CapabilityDescriptor, 0, len(c.registrations))
	for _, registration := range c.registrations {
		items = append(items, cloneCapabilityDescriptor(registration.descriptor))
	}
	c.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func (c *Catalog) Descriptor(id string) (CapabilityDescriptor, bool) {
	registration, ok := c.lookup(id)
	if !ok {
		return CapabilityDescriptor{}, false
	}
	return cloneCapabilityDescriptor(registration.descriptor), true
}

func (c *Catalog) lookup(id string) (registeredCapability, bool) {
	if c == nil {
		return registeredCapability{}, false
	}
	c.mu.RLock()
	registration, ok := c.registrations[id]
	c.mu.RUnlock()
	return registration, ok
}

func cloneCapabilityDescriptor(source CapabilityDescriptor) CapabilityDescriptor {
	source.InputSchema = append(json.RawMessage(nil), source.InputSchema...)
	source.OutputSchema = append(json.RawMessage(nil), source.OutputSchema...)
	source.Formats = append([]Format(nil), source.Formats...)
	return source
}

var capabilityIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

func validateSchemaDocument(raw json.RawMessage) error {
	var schema map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if !json.Valid(raw) || decoder.Decode(&schema) != nil || schema == nil {
		return errors.New("schema must be a JSON object")
	}
	return validateSchemaDefinition(schema)
}

func validateSchemaDefinition(schema map[string]any) error {
	if rawType, exists := schema["type"]; exists {
		kind, ok := rawType.(string)
		if !ok || !validJSONType(kind) {
			return fmt.Errorf("unknown JSON type %v", rawType)
		}
	}
	if required, exists := schema["required"]; exists {
		items, ok := required.([]any)
		if !ok {
			return errors.New("required must be an array of strings")
		}
		for _, item := range items {
			if _, ok := item.(string); !ok {
				return errors.New("required must contain only strings")
			}
		}
	}
	if properties, exists := schema["properties"]; exists {
		items, ok := properties.(map[string]any)
		if !ok {
			return errors.New("properties must be an object")
		}
		for name, rawProperty := range items {
			property, ok := rawProperty.(map[string]any)
			if !ok {
				return fmt.Errorf("property %q must be a schema object", name)
			}
			if err := validateSchemaDefinition(property); err != nil {
				return fmt.Errorf("property %q: %w", name, err)
			}
		}
	}
	if items, exists := schema["items"]; exists {
		itemSchema, ok := items.(map[string]any)
		if !ok {
			return errors.New("items must be a schema object")
		}
		if err := validateSchemaDefinition(itemSchema); err != nil {
			return fmt.Errorf("items: %w", err)
		}
	}
	for _, keyword := range []string{"anyOf", "allOf", "oneOf"} {
		if rawBranches, exists := schema[keyword]; exists {
			branches, ok := rawBranches.([]any)
			if !ok || len(branches) == 0 {
				return fmt.Errorf("%s must be a non-empty array of schemas", keyword)
			}
			for index, rawBranch := range branches {
				branch, ok := rawBranch.(map[string]any)
				if !ok {
					return fmt.Errorf("%s[%d] must be a schema object", keyword, index)
				}
				if err := validateSchemaDefinition(branch); err != nil {
					return fmt.Errorf("%s[%d]: %w", keyword, index, err)
				}
			}
		}
	}
	if additional, exists := schema["additionalProperties"]; exists {
		if _, ok := additional.(bool); !ok {
			return errors.New("additionalProperties must be a boolean")
		}
	}
	if rawEnum, exists := schema["enum"]; exists {
		values, ok := rawEnum.([]any)
		if !ok || len(values) == 0 {
			return errors.New("enum must be a non-empty array")
		}
	}
	for _, keyword := range []string{"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf"} {
		if raw, exists := schema[keyword]; exists {
			value, ok := schemaNumber(raw)
			if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("%s must be a finite number", keyword)
			}
			if keyword == "multipleOf" && value <= 0 {
				return errors.New("multipleOf must be greater than zero")
			}
		}
	}
	for _, keyword := range []string{"minLength", "maxLength", "minItems", "maxItems"} {
		if raw, exists := schema[keyword]; exists {
			value, ok := schemaInteger(raw)
			if !ok || value < 0 {
				return fmt.Errorf("%s must be a non-negative integer", keyword)
			}
		}
	}
	if raw, exists := schema["pattern"]; exists {
		pattern, ok := raw.(string)
		if !ok {
			return errors.New("pattern must be a string")
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("pattern is invalid: %w", err)
		}
	}
	if raw, exists := schema["uniqueItems"]; exists {
		if _, ok := raw.(bool); !ok {
			return errors.New("uniqueItems must be a boolean")
		}
	}
	return nil
}

func validJSONType(value string) bool {
	switch value {
	case "object", "array", "string", "number", "integer", "boolean", "null":
		return true
	default:
		return false
	}
}

func validateArguments(schemaRaw, arguments json.RawMessage) error {
	var schema map[string]any
	schemaDecoder := json.NewDecoder(bytes.NewReader(schemaRaw))
	schemaDecoder.UseNumber()
	if err := schemaDecoder.Decode(&schema); err != nil {
		return ErrInvalidCapabilitySchema
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCommandArguments, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrInvalidCommandArguments
	}
	if err := validateJSONValue(schema, value, "arguments"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCommandArguments, err)
	}
	return nil
}

func validateJSONValue(schema map[string]any, value any, path string) error {
	if expected, exists := schema["const"]; exists && !equalJSONValues(expected, value) {
		return fmt.Errorf("%s must equal the declared constant", path)
	}
	if rawEnum, exists := schema["enum"]; exists {
		values, ok := rawEnum.([]any)
		if !ok {
			return ErrInvalidCapabilitySchema
		}
		matched := false
		for _, expected := range values {
			if equalJSONValues(expected, value) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s must match one of the declared values", path)
		}
	}
	if rawType, exists := schema["type"]; exists {
		kind, ok := rawType.(string)
		if !ok || !validJSONType(kind) {
			return ErrInvalidCapabilitySchema
		}
		if !matchesJSONType(kind, value) {
			return fmt.Errorf("%s must be %s", path, kind)
		}
	}
	if number, ok := schemaRational(value); ok {
		for _, keyword := range []string{"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf"} {
			rawBound, exists := schema[keyword]
			if !exists {
				continue
			}
			bound, valid := schemaRational(rawBound)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			comparison := number.Cmp(bound)
			switch keyword {
			case "minimum":
				if comparison < 0 {
					return fmt.Errorf("%s must be at least %v", path, bound)
				}
			case "maximum":
				if comparison > 0 {
					return fmt.Errorf("%s must be at most %v", path, bound)
				}
			case "exclusiveMinimum":
				if comparison <= 0 {
					return fmt.Errorf("%s must be greater than %v", path, bound)
				}
			case "exclusiveMaximum":
				if comparison >= 0 {
					return fmt.Errorf("%s must be less than %v", path, bound)
				}
			case "multipleOf":
				if !new(big.Rat).Quo(number, bound).IsInt() {
					return fmt.Errorf("%s must be a multiple of %v", path, bound)
				}
			}
		}
	}
	if text, ok := value.(string); ok {
		length := utf8.RuneCountInString(text)
		if raw, exists := schema["minLength"]; exists {
			minimum, valid := schemaInteger(raw)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			if int64(length) < minimum {
				return fmt.Errorf("%s must contain at least %d characters", path, minimum)
			}
		}
		if raw, exists := schema["maxLength"]; exists {
			maximum, valid := schemaInteger(raw)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			if int64(length) > maximum {
				return fmt.Errorf("%s must contain at most %d characters", path, maximum)
			}
		}
		if raw, exists := schema["pattern"]; exists {
			pattern, valid := raw.(string)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return ErrInvalidCapabilitySchema
			}
			if !compiled.MatchString(text) {
				return fmt.Errorf("%s does not match the required pattern", path)
			}
		}
	}
	if object, ok := value.(map[string]any); ok {
		properties, _ := schema["properties"].(map[string]any)
		for _, requiredValue := range stringArray(schema["required"]) {
			if _, exists := object[requiredValue]; !exists {
				return fmt.Errorf("%s.%s is required", path, requiredValue)
			}
		}
		for name, child := range object {
			propertySchemaRaw, exists := properties[name]
			if !exists {
				if schema["additionalProperties"] == false {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
				continue
			}
			propertySchema, ok := propertySchemaRaw.(map[string]any)
			if !ok {
				return ErrInvalidCapabilitySchema
			}
			if err := validateJSONValue(propertySchema, child, path+"."+name); err != nil {
				return err
			}
		}
	}
	if array, ok := value.([]any); ok {
		if raw, exists := schema["minItems"]; exists {
			minimum, valid := schemaInteger(raw)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			if int64(len(array)) < minimum {
				return fmt.Errorf("%s must contain at least %d items", path, minimum)
			}
		}
		if raw, exists := schema["maxItems"]; exists {
			maximum, valid := schemaInteger(raw)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			if int64(len(array)) > maximum {
				return fmt.Errorf("%s must contain at most %d items", path, maximum)
			}
		}
		if unique, _ := schema["uniqueItems"].(bool); unique {
			for index := range array {
				for prior := 0; prior < index; prior++ {
					if equalJSONValues(array[index], array[prior]) {
						return fmt.Errorf("%s[%d] duplicates an earlier item", path, index)
					}
				}
			}
		}
		if itemSchema, exists := schema["items"].(map[string]any); exists {
			for index, item := range array {
				if err := validateJSONValue(itemSchema, item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	}
	for _, keyword := range []string{"anyOf", "allOf", "oneOf"} {
		rawBranches, exists := schema[keyword]
		if !exists {
			continue
		}
		branches, ok := rawBranches.([]any)
		if !ok || len(branches) == 0 {
			return ErrInvalidCapabilitySchema
		}
		matched := 0
		for _, rawBranch := range branches {
			branch, valid := rawBranch.(map[string]any)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			if validateJSONValue(branch, value, path) == nil {
				matched++
			}
		}
		switch keyword {
		case "anyOf":
			if matched == 0 {
				return fmt.Errorf("%s must match at least one alternative", path)
			}
		case "allOf":
			if matched != len(branches) {
				return fmt.Errorf("%s must match every required schema", path)
			}
		case "oneOf":
			if matched != 1 {
				return fmt.Errorf("%s must match exactly one alternative", path)
			}
		}
	}
	return nil
}

func schemaNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case float64:
		return number, !math.IsNaN(number) && !math.IsInf(number, 0)
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}

func schemaRational(value any) (*big.Rat, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, ok := new(big.Rat).SetString(number.String())
		return parsed, ok
	case float64:
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, false
		}
		return new(big.Rat).SetFloat64(number), true
	case int:
		return new(big.Rat).SetInt64(int64(number)), true
	case int64:
		return new(big.Rat).SetInt64(number), true
	default:
		return nil, false
	}
}

func schemaInteger(value any) (int64, bool) {
	number, ok := schemaRational(value)
	if !ok || !number.IsInt() || !number.Num().IsInt64() {
		return 0, false
	}
	return number.Num().Int64(), true
}

func equalJSONValues(left, right any) bool {
	if leftNumber, ok := schemaRational(left); ok {
		if rightNumber, ok := schemaRational(right); ok {
			return leftNumber.Cmp(rightNumber) == 0
		}
	}
	if reflect.DeepEqual(left, right) {
		return true
	}
	leftArray, leftIsArray := left.([]any)
	rightArray, rightIsArray := right.([]any)
	if leftIsArray && rightIsArray {
		if len(leftArray) != len(rightArray) {
			return false
		}
		for index := range leftArray {
			if !equalJSONValues(leftArray[index], rightArray[index]) {
				return false
			}
		}
		return true
	}
	leftObject, leftIsObject := left.(map[string]any)
	rightObject, rightIsObject := right.(map[string]any)
	if leftIsObject && rightIsObject {
		if len(leftObject) != len(rightObject) {
			return false
		}
		for key, leftValue := range leftObject {
			rightValue, exists := rightObject[key]
			if !exists || !equalJSONValues(leftValue, rightValue) {
				return false
			}
		}
		return true
	}
	return false
}

func matchesJSONType(kind string, value any) bool {
	switch kind {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		parsed, ok := schemaRational(number)
		return ok && parsed.IsInt()
	case "number":
		_, ok := value.(json.Number)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}

func stringArray(raw any) []string {
	items, _ := raw.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			out = append(out, value)
		}
	}
	return out
}
