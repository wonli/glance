package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"glance/internal/document"
	"glance/internal/jsonschema"
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
	var schema jsonschema.Definition
	if err := json.Unmarshal(raw, &schema); err != nil {
		return errors.New("schema must be a JSON object")
	}
	return validateSchemaDefinition(schema)
}

type schemaBranchGroup struct {
	keyword  string
	present  bool
	branches []jsonschema.Definition
}

type schemaNumericConstraint struct {
	keyword string
	raw     json.RawMessage
}

func validateSchemaDefinition(schema jsonschema.Definition) error {
	if schema.Has("type") {
		if schema.Type == nil || !validJSONType(*schema.Type) {
			return fmt.Errorf("unknown JSON type %v", schema.Type)
		}
	}
	if schema.Has("required") {
		if schema.Required == nil {
			return errors.New("required must be an array of strings")
		}
	}
	if schema.Has("properties") {
		if schema.Properties == nil {
			return errors.New("properties must be an object")
		}
		for name, property := range schema.Properties {
			if err := validateSchemaDefinition(property); err != nil {
				return fmt.Errorf("property %q: %w", name, err)
			}
		}
	}
	if schema.Has("items") {
		if schema.Items == nil {
			return errors.New("items must be a schema object")
		}
		if err := validateSchemaDefinition(*schema.Items); err != nil {
			return fmt.Errorf("items: %w", err)
		}
	}
	for _, group := range []schemaBranchGroup{
		{"anyOf", schema.Has("anyOf"), schema.AnyOf},
		{"allOf", schema.Has("allOf"), schema.AllOf},
		{"oneOf", schema.Has("oneOf"), schema.OneOf},
	} {
		if !group.present {
			continue
		}
		if len(group.branches) == 0 {
			return fmt.Errorf("%s must be a non-empty array of schemas", group.keyword)
		}
		for index, branch := range group.branches {
			if err := validateSchemaDefinition(branch); err != nil {
				return fmt.Errorf("%s[%d]: %w", group.keyword, index, err)
			}
		}
	}
	if schema.Has("additionalProperties") {
		if _, ok := jsonBool(schema.AdditionalProperties); !ok {
			return errors.New("additionalProperties must be a boolean")
		}
	}
	if schema.Has("enum") {
		if schema.Enum == nil || len(schema.Enum) == 0 {
			return errors.New("enum must be a non-empty array")
		}
	}
	for _, constraint := range []schemaNumericConstraint{
		{"minimum", schema.Minimum},
		{"maximum", schema.Maximum},
		{"exclusiveMinimum", schema.ExclusiveMinimum},
		{"exclusiveMaximum", schema.ExclusiveMaximum},
		{"multipleOf", schema.MultipleOf},
	} {
		if schema.Has(constraint.keyword) {
			value, ok := schemaNumber(constraint.raw)
			if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("%s must be a finite number", constraint.keyword)
			}
			if constraint.keyword == "multipleOf" && value <= 0 {
				return errors.New("multipleOf must be greater than zero")
			}
		}
	}
	for _, constraint := range []schemaNumericConstraint{
		{"minLength", schema.MinLength},
		{"maxLength", schema.MaxLength},
		{"minItems", schema.MinItems},
		{"maxItems", schema.MaxItems},
	} {
		if schema.Has(constraint.keyword) {
			value, ok := schemaInteger(constraint.raw)
			if !ok || value < 0 {
				return fmt.Errorf("%s must be a non-negative integer", constraint.keyword)
			}
		}
	}
	if schema.Has("pattern") {
		if schema.Pattern == nil {
			return errors.New("pattern must be a string")
		}
		if _, err := regexp.Compile(*schema.Pattern); err != nil {
			return fmt.Errorf("pattern is invalid: %w", err)
		}
	}
	if schema.Has("uniqueItems") {
		if schema.UniqueItems == nil {
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
	var schema jsonschema.Definition
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		return ErrInvalidCapabilitySchema
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(arguments) {
		return fmt.Errorf("%w: invalid JSON", ErrInvalidCommandArguments)
	}
	if err := validateJSONValue(schema, arguments, "arguments"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCommandArguments, err)
	}
	return nil
}

func validateJSONValue(schema jsonschema.Definition, value json.RawMessage, path string) error {
	if schema.Has("const") && !equalJSONValues(schema.Const, value) {
		return fmt.Errorf("%s must equal the declared constant", path)
	}
	if schema.Has("enum") {
		if schema.Enum == nil {
			return ErrInvalidCapabilitySchema
		}
		matched := false
		for _, expected := range schema.Enum {
			if equalJSONValues(expected, value) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s must match one of the declared values", path)
		}
	}
	if schema.Has("type") {
		if schema.Type == nil || !validJSONType(*schema.Type) {
			return ErrInvalidCapabilitySchema
		}
		if !matchesJSONType(*schema.Type, value) {
			return fmt.Errorf("%s must be %s", path, *schema.Type)
		}
	}
	if number, ok := schemaRational(value); ok {
		for _, constraint := range []schemaNumericConstraint{
			{"minimum", schema.Minimum},
			{"maximum", schema.Maximum},
			{"exclusiveMinimum", schema.ExclusiveMinimum},
			{"exclusiveMaximum", schema.ExclusiveMaximum},
			{"multipleOf", schema.MultipleOf},
		} {
			if !schema.Has(constraint.keyword) {
				continue
			}
			bound, valid := schemaRational(constraint.raw)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			comparison := number.Cmp(bound)
			switch constraint.keyword {
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
	if text, ok := jsonString(value); ok {
		length := utf8.RuneCountInString(text)
		if schema.Has("minLength") {
			minimum, valid := schemaInteger(schema.MinLength)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			if int64(length) < minimum {
				return fmt.Errorf("%s must contain at least %d characters", path, minimum)
			}
		}
		if schema.Has("maxLength") {
			maximum, valid := schemaInteger(schema.MaxLength)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			if int64(length) > maximum {
				return fmt.Errorf("%s must contain at most %d characters", path, maximum)
			}
		}
		if schema.Has("pattern") {
			if schema.Pattern == nil {
				return ErrInvalidCapabilitySchema
			}
			compiled, err := regexp.Compile(*schema.Pattern)
			if err != nil {
				return ErrInvalidCapabilitySchema
			}
			if !compiled.MatchString(text) {
				return fmt.Errorf("%s does not match the required pattern", path)
			}
		}
	}
	if object, ok := jsonObject(value); ok {
		properties := schema.Properties
		if schema.Has("properties") && properties == nil {
			return ErrInvalidCapabilitySchema
		}
		required := schema.Required
		if schema.Has("required") && required == nil {
			return ErrInvalidCapabilitySchema
		}
		for _, requiredValue := range required {
			if _, exists := object[requiredValue]; !exists {
				return fmt.Errorf("%s.%s is required", path, requiredValue)
			}
		}
		for name, child := range object {
			propertySchema, exists := properties[name]
			if !exists {
				if schema.Has("additionalProperties") {
					allowed, valid := jsonBool(schema.AdditionalProperties)
					if !valid {
						return ErrInvalidCapabilitySchema
					}
					if !allowed {
						return fmt.Errorf("%s.%s is not allowed", path, name)
					}
				}
				continue
			}
			if err := validateJSONValue(propertySchema, child, path+"."+name); err != nil {
				return err
			}
		}
	}
	if array, ok := jsonArray(value); ok {
		if schema.Has("minItems") {
			minimum, valid := schemaInteger(schema.MinItems)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			if int64(len(array)) < minimum {
				return fmt.Errorf("%s must contain at least %d items", path, minimum)
			}
		}
		if schema.Has("maxItems") {
			maximum, valid := schemaInteger(schema.MaxItems)
			if !valid {
				return ErrInvalidCapabilitySchema
			}
			if int64(len(array)) > maximum {
				return fmt.Errorf("%s must contain at most %d items", path, maximum)
			}
		}
		if schema.Has("uniqueItems") {
			if schema.UniqueItems == nil {
				return ErrInvalidCapabilitySchema
			}
			if *schema.UniqueItems {
				for index := range array {
					for prior := 0; prior < index; prior++ {
						if equalJSONValues(array[index], array[prior]) {
							return fmt.Errorf("%s[%d] duplicates an earlier item", path, index)
						}
					}
				}
			}
		}
		if schema.Has("items") {
			if schema.Items == nil {
				return ErrInvalidCapabilitySchema
			}
			for index, item := range array {
				if err := validateJSONValue(*schema.Items, item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	}
	for _, group := range []schemaBranchGroup{
		{"anyOf", schema.Has("anyOf"), schema.AnyOf},
		{"allOf", schema.Has("allOf"), schema.AllOf},
		{"oneOf", schema.Has("oneOf"), schema.OneOf},
	} {
		if !group.present {
			continue
		}
		if len(group.branches) == 0 {
			return ErrInvalidCapabilitySchema
		}
		matched := 0
		for _, branch := range group.branches {
			if validateJSONValue(branch, value, path) == nil {
				matched++
			}
		}
		switch group.keyword {
		case "anyOf":
			if matched == 0 {
				return fmt.Errorf("%s must match at least one alternative", path)
			}
		case "allOf":
			if matched != len(group.branches) {
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

func schemaNumber(value json.RawMessage) (float64, bool) {
	number, ok := jsonNumber(value)
	if !ok {
		return 0, false
	}
	parsed, err := number.Float64()
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

func schemaRational(value json.RawMessage) (*big.Rat, bool) {
	number, ok := jsonNumber(value)
	if !ok {
		return nil, false
	}
	parsed, ok := new(big.Rat).SetString(number.String())
	return parsed, ok
}

func schemaInteger(value json.RawMessage) (int64, bool) {
	number, ok := schemaRational(value)
	if !ok || !number.IsInt() || !number.Num().IsInt64() {
		return 0, false
	}
	return number.Num().Int64(), true
}

func equalJSONValues(left, right json.RawMessage) bool {
	if leftNumber, ok := schemaRational(left); ok {
		if rightNumber, ok := schemaRational(right); ok {
			return leftNumber.Cmp(rightNumber) == 0
		}
	}
	leftArray, leftIsArray := jsonArray(left)
	rightArray, rightIsArray := jsonArray(right)
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
	leftObject, leftIsObject := jsonObject(left)
	rightObject, rightIsObject := jsonObject(right)
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
	if leftString, leftIsString := jsonString(left); leftIsString {
		rightString, rightIsString := jsonString(right)
		return rightIsString && leftString == rightString
	}
	if leftBool, leftIsBool := jsonBool(left); leftIsBool {
		rightBool, rightIsBool := jsonBool(right)
		return rightIsBool && leftBool == rightBool
	}
	return bytes.Equal(bytes.TrimSpace(left), bytes.TrimSpace(right)) && json.Valid(left) && json.Valid(right)
}

func matchesJSONType(kind string, value json.RawMessage) bool {
	switch kind {
	case "object":
		_, ok := jsonObject(value)
		return ok
	case "array":
		_, ok := jsonArray(value)
		return ok
	case "string":
		_, ok := jsonString(value)
		return ok
	case "integer":
		parsed, ok := schemaRational(value)
		return ok && parsed.IsInt()
	case "number":
		_, ok := jsonNumber(value)
		return ok
	case "boolean":
		_, ok := jsonBool(value)
		return ok
	case "null":
		return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
	default:
		return false
	}
}

func jsonObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if !json.Valid(raw) {
		return nil, false
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, false
	}
	return object, true
}

func jsonArray(raw json.RawMessage) ([]json.RawMessage, bool) {
	if !json.Valid(raw) {
		return nil, false
	}
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err != nil || array == nil {
		return nil, false
	}
	return array, true
}

func jsonString(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return "", false
	}
	var value string
	err := json.Unmarshal(raw, &value)
	return value, err == nil
}

func jsonBool(raw json.RawMessage) (bool, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || (trimmed[0] != 't' && trimmed[0] != 'f') {
		return false, false
	}
	var value bool
	err := json.Unmarshal(raw, &value)
	return value, err == nil
}

func jsonNumber(raw json.RawMessage) (json.Number, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || !bytes.ContainsRune([]byte("-0123456789"), rune(trimmed[0])) {
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value json.Number
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	return value, true
}

func stringArray(raw json.RawMessage) ([]string, bool) {
	items, ok := jsonArray(raw)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		value, valid := jsonString(item)
		if !valid {
			return nil, false
		}
		out = append(out, value)
	}
	return out, true
}
