package evaluator

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The json_schema evaluator validates against a deliberate subset of JSON
// Schema draft 2020-12, chosen to cover what LLM output contracts actually use
// while keeping the implementation dependency-free:
//
//	type            string | number | integer | boolean | object | array | null,
//	                or a list of those
//	required        list of required property names
//	properties      per-property subschemas
//	items           subschema applied to every array element
//	enum            allowed values
//	const           exact required value
//	minimum         maximum          exclusiveMinimum   exclusiveMaximum
//	minLength       maxLength        pattern
//	minItems        maxItems         uniqueItems
//	minProperties   maxProperties    additionalProperties (bool or subschema)
//	anyOf           allOf            oneOf              not
//	nullable        (accepts null in addition to `type`)
//
// Keywords outside this list are ignored rather than rejected, so a schema
// carrying $id, title, description or examples still validates its data.

type jsonSchemaConfig struct {
	Field  string          `json:"field"`
	Schema json.RawMessage `json:"schema"`

	// PartialCredit scores the fraction of checks that passed instead of a
	// pass/fail verdict, which is more informative when comparing prompts.
	PartialCredit bool `json:"partial_credit"`
}

type jsonSchemaEvaluator struct {
	config jsonSchemaConfig
	schema *schemaNode
}

func newJSONSchema(config json.RawMessage, _ Deps) (Evaluator, error) {
	var c jsonSchemaConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if len(c.Schema) == 0 {
		return nil, fmt.Errorf("`schema` is required")
	}

	schema, err := compileSchema(c.Schema)
	if err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	return &jsonSchemaEvaluator{config: c, schema: schema}, nil
}

func (e *jsonSchemaEvaluator) Evaluate(_ context.Context, target Target) (Result, error) {
	text := strings.TrimSpace(fieldOf(target, e.config.Field))
	if text == "" {
		return boolean(false, "field is empty"), nil
	}

	var value interface{}
	if err := json.Unmarshal([]byte(stripCodeFence(text)), &value); err != nil {
		return boolean(false, "field is not valid JSON: %v", err), nil
	}

	v := &validation{}
	e.schema.validate(value, "$", v)

	if len(v.errors) == 0 {
		return boolean(true, "matches schema (%d checks)", v.checks), nil
	}

	reason := fmt.Sprintf("%d violation(s): %s", len(v.errors), strings.Join(v.errors, "; "))
	if !e.config.PartialCredit {
		return boolean(false, "%s", reason), nil
	}

	passed := v.checks - len(v.errors)
	score := 0.0
	if v.checks > 0 && passed > 0 {
		score = float64(passed) / float64(v.checks)
	}
	return numeric(score, "%s", reason), nil
}

// validation accumulates the outcome of a schema walk.
type validation struct {
	checks int
	errors []string
}

// maxReportedErrors bounds the score comment so one very wrong document cannot
// produce an unbounded string.
const maxReportedErrors = 10

func (v *validation) fail(path, format string, args ...interface{}) {
	if len(v.errors) >= maxReportedErrors {
		return
	}
	v.errors = append(v.errors, path+": "+fmt.Sprintf(format, args...))
}

// schemaNode is a compiled schema.
type schemaNode struct {
	types    []string
	nullable bool

	properties           map[string]*schemaNode
	required             []string
	items                *schemaNode
	additionalAllowed    bool
	additionalSchema     *schemaNode
	hasAdditionalKeyword bool

	enum     []interface{}
	constVal interface{}
	hasConst bool

	minimum          *float64
	maximum          *float64
	exclusiveMinimum *float64
	exclusiveMaximum *float64

	minLength *int
	maxLength *int
	pattern   *regexp.Regexp

	minItems    *int
	maxItems    *int
	uniqueItems bool

	minProperties *int
	maxProperties *int

	anyOf []*schemaNode
	allOf []*schemaNode
	oneOf []*schemaNode
	not   *schemaNode
}

// rawSchema is the on-the-wire form before compilation.
type rawSchema struct {
	Type                 json.RawMessage            `json:"type"`
	Nullable             bool                       `json:"nullable"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Required             []string                   `json:"required"`
	Items                json.RawMessage            `json:"items"`
	AdditionalProperties json.RawMessage            `json:"additionalProperties"`
	Enum                 []interface{}              `json:"enum"`
	Const                json.RawMessage            `json:"const"`
	Minimum              *float64                   `json:"minimum"`
	Maximum              *float64                   `json:"maximum"`
	ExclusiveMinimum     *float64                   `json:"exclusiveMinimum"`
	ExclusiveMaximum     *float64                   `json:"exclusiveMaximum"`
	MinLength            *int                       `json:"minLength"`
	MaxLength            *int                       `json:"maxLength"`
	Pattern              string                     `json:"pattern"`
	MinItems             *int                       `json:"minItems"`
	MaxItems             *int                       `json:"maxItems"`
	UniqueItems          bool                       `json:"uniqueItems"`
	MinProperties        *int                       `json:"minProperties"`
	MaxProperties        *int                       `json:"maxProperties"`
	AnyOf                []json.RawMessage          `json:"anyOf"`
	AllOf                []json.RawMessage          `json:"allOf"`
	OneOf                []json.RawMessage          `json:"oneOf"`
	Not                  json.RawMessage            `json:"not"`
	Defs                 map[string]json.RawMessage `json:"$defs"`
}

// compileSchema turns a raw schema document into a validating tree.
func compileSchema(raw json.RawMessage) (*schemaNode, error) {
	// `true` and `false` are valid schemas meaning "anything" and "nothing".
	var asBool bool
	if err := json.Unmarshal(raw, &asBool); err == nil {
		if asBool {
			return &schemaNode{additionalAllowed: true}, nil
		}
		return &schemaNode{not: &schemaNode{additionalAllowed: true}}, nil
	}

	var rs rawSchema
	if err := json.Unmarshal(raw, &rs); err != nil {
		return nil, err
	}

	node := &schemaNode{
		required:         rs.Required,
		nullable:         rs.Nullable,
		enum:             rs.Enum,
		minimum:          rs.Minimum,
		maximum:          rs.Maximum,
		exclusiveMinimum: rs.ExclusiveMinimum,
		exclusiveMaximum: rs.ExclusiveMaximum,
		minLength:        rs.MinLength,
		maxLength:        rs.MaxLength,
		minItems:         rs.MinItems,
		maxItems:         rs.MaxItems,
		uniqueItems:      rs.UniqueItems,
		minProperties:    rs.MinProperties,
		maxProperties:    rs.MaxProperties,

		// Absent `additionalProperties` means additional properties are allowed.
		additionalAllowed: true,
	}

	if len(rs.Type) > 0 {
		types, err := parseTypeKeyword(rs.Type)
		if err != nil {
			return nil, err
		}
		node.types = types
	}

	if rs.Pattern != "" {
		compiled, err := regexp.Compile(rs.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", rs.Pattern, err)
		}
		node.pattern = compiled
	}

	if len(rs.Const) > 0 {
		var v interface{}
		if err := json.Unmarshal(rs.Const, &v); err != nil {
			return nil, fmt.Errorf("invalid const: %w", err)
		}
		node.constVal = v
		node.hasConst = true
	}

	if len(rs.Properties) > 0 {
		node.properties = make(map[string]*schemaNode, len(rs.Properties))
		for name, sub := range rs.Properties {
			compiled, err := compileSchema(sub)
			if err != nil {
				return nil, fmt.Errorf("property %q: %w", name, err)
			}
			node.properties[name] = compiled
		}
	}

	if len(rs.Items) > 0 {
		compiled, err := compileSchema(rs.Items)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		node.items = compiled
	}

	if len(rs.AdditionalProperties) > 0 {
		node.hasAdditionalKeyword = true
		var allowed bool
		if err := json.Unmarshal(rs.AdditionalProperties, &allowed); err == nil {
			node.additionalAllowed = allowed
		} else {
			compiled, err := compileSchema(rs.AdditionalProperties)
			if err != nil {
				return nil, fmt.Errorf("additionalProperties: %w", err)
			}
			node.additionalSchema = compiled
			node.additionalAllowed = true
		}
	}

	var err error
	if node.anyOf, err = compileList(rs.AnyOf, "anyOf"); err != nil {
		return nil, err
	}
	if node.allOf, err = compileList(rs.AllOf, "allOf"); err != nil {
		return nil, err
	}
	if node.oneOf, err = compileList(rs.OneOf, "oneOf"); err != nil {
		return nil, err
	}
	if len(rs.Not) > 0 {
		if node.not, err = compileSchema(rs.Not); err != nil {
			return nil, fmt.Errorf("not: %w", err)
		}
	}

	return node, nil
}

func compileList(raws []json.RawMessage, keyword string) ([]*schemaNode, error) {
	if len(raws) == 0 {
		return nil, nil
	}
	out := make([]*schemaNode, 0, len(raws))
	for i, raw := range raws {
		compiled, err := compileSchema(raw)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", keyword, i, err)
		}
		out = append(out, compiled)
	}
	return out, nil
}

// parseTypeKeyword accepts both `"type": "string"` and `"type": ["string","null"]`.
func parseTypeKeyword(raw json.RawMessage) ([]string, error) {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many, nil
	}
	return nil, fmt.Errorf("`type` must be a string or list of strings")
}

// validate walks a value against the schema, recording failures.
func (n *schemaNode) validate(value interface{}, path string, v *validation) {
	if n == nil {
		return
	}

	if len(n.types) > 0 {
		v.checks++
		if !n.matchesType(value) {
			v.fail(path, "expected type %s, got %s", strings.Join(n.types, "|"), jsonTypeOf(value))
			// The remaining keywords assume the declared type; reporting them
			// too would bury the actual problem.
			return
		}
	}

	if n.hasConst {
		v.checks++
		if !deepEqualJSON(value, n.constVal) {
			v.fail(path, "expected const %v", n.constVal)
		}
	}

	if len(n.enum) > 0 {
		v.checks++
		matched := false
		for _, candidate := range n.enum {
			if deepEqualJSON(value, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			v.fail(path, "value %v is not in enum", value)
		}
	}

	switch typed := value.(type) {
	case string:
		n.validateString(typed, path, v)
	case float64:
		n.validateNumber(typed, path, v)
	case []interface{}:
		n.validateArray(typed, path, v)
	case map[string]interface{}:
		n.validateObject(typed, path, v)
	}

	for _, sub := range n.allOf {
		sub.validate(value, path, v)
	}

	if len(n.anyOf) > 0 {
		v.checks++
		if !anyMatches(n.anyOf, value) {
			v.fail(path, "value matches none of the anyOf schemas")
		}
	}

	if len(n.oneOf) > 0 {
		v.checks++
		matches := 0
		for _, sub := range n.oneOf {
			if matchesSchema(sub, value) {
				matches++
			}
		}
		if matches != 1 {
			v.fail(path, "expected exactly one oneOf match, got %d", matches)
		}
	}

	if n.not != nil {
		v.checks++
		if matchesSchema(n.not, value) {
			v.fail(path, "value matches a forbidden schema")
		}
	}
}

func (n *schemaNode) validateString(s, path string, v *validation) {
	length := utf8.RuneCountInString(s)
	if n.minLength != nil {
		v.checks++
		if length < *n.minLength {
			v.fail(path, "string length %d below minLength %d", length, *n.minLength)
		}
	}
	if n.maxLength != nil {
		v.checks++
		if length > *n.maxLength {
			v.fail(path, "string length %d above maxLength %d", length, *n.maxLength)
		}
	}
	if n.pattern != nil {
		v.checks++
		if !n.pattern.MatchString(s) {
			v.fail(path, "string does not match pattern %s", n.pattern.String())
		}
	}
}

func (n *schemaNode) validateNumber(f float64, path string, v *validation) {
	if n.minimum != nil {
		v.checks++
		if f < *n.minimum {
			v.fail(path, "%g is below minimum %g", f, *n.minimum)
		}
	}
	if n.maximum != nil {
		v.checks++
		if f > *n.maximum {
			v.fail(path, "%g is above maximum %g", f, *n.maximum)
		}
	}
	if n.exclusiveMinimum != nil {
		v.checks++
		if f <= *n.exclusiveMinimum {
			v.fail(path, "%g is not above exclusiveMinimum %g", f, *n.exclusiveMinimum)
		}
	}
	if n.exclusiveMaximum != nil {
		v.checks++
		if f >= *n.exclusiveMaximum {
			v.fail(path, "%g is not below exclusiveMaximum %g", f, *n.exclusiveMaximum)
		}
	}
}

func (n *schemaNode) validateArray(items []interface{}, path string, v *validation) {
	if n.minItems != nil {
		v.checks++
		if len(items) < *n.minItems {
			v.fail(path, "array has %d items, below minItems %d", len(items), *n.minItems)
		}
	}
	if n.maxItems != nil {
		v.checks++
		if len(items) > *n.maxItems {
			v.fail(path, "array has %d items, above maxItems %d", len(items), *n.maxItems)
		}
	}
	if n.uniqueItems {
		v.checks++
		if hasDuplicates(items) {
			v.fail(path, "array items are not unique")
		}
	}
	if n.items != nil {
		for i, item := range items {
			n.items.validate(item, fmt.Sprintf("%s[%d]", path, i), v)
		}
	}
}

func (n *schemaNode) validateObject(obj map[string]interface{}, path string, v *validation) {
	for _, name := range n.required {
		v.checks++
		if _, present := obj[name]; !present {
			v.fail(path, "missing required property %q", name)
		}
	}

	if n.minProperties != nil {
		v.checks++
		if len(obj) < *n.minProperties {
			v.fail(path, "object has %d properties, below minProperties %d", len(obj), *n.minProperties)
		}
	}
	if n.maxProperties != nil {
		v.checks++
		if len(obj) > *n.maxProperties {
			v.fail(path, "object has %d properties, above maxProperties %d", len(obj), *n.maxProperties)
		}
	}

	for name, value := range obj {
		sub, declared := n.properties[name]
		if declared {
			sub.validate(value, path+"."+name, v)
			continue
		}
		if !n.hasAdditionalKeyword {
			continue
		}
		if !n.additionalAllowed {
			v.checks++
			v.fail(path, "additional property %q is not allowed", name)
			continue
		}
		if n.additionalSchema != nil {
			n.additionalSchema.validate(value, path+"."+name, v)
		}
	}
}

// matchesType reports whether a value satisfies the schema's `type` keyword.
func (n *schemaNode) matchesType(value interface{}) bool {
	if value == nil && n.nullable {
		return true
	}
	actual := jsonTypeOf(value)
	for _, want := range n.types {
		switch want {
		case actual:
			return true
		case "number":
			if actual == "integer" {
				return true
			}
		case "integer":
			if f, ok := value.(float64); ok && f == math.Trunc(f) {
				return true
			}
		}
	}
	return false
}

// matchesSchema reports whether a value validates cleanly, without recording
// the failures on the caller's validation.
func matchesSchema(n *schemaNode, value interface{}) bool {
	probe := &validation{}
	n.validate(value, "$", probe)
	return len(probe.errors) == 0
}

func anyMatches(nodes []*schemaNode, value interface{}) bool {
	for _, n := range nodes {
		if matchesSchema(n, value) {
			return true
		}
	}
	return false
}

// jsonTypeOf names a decoded JSON value's type. Whole floats report as
// "integer" so the integer keyword can be checked without a separate pass.
func jsonTypeOf(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		if typed == math.Trunc(typed) {
			return "integer"
		}
		return "number"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "unknown"
	}
}

// deepEqualJSON compares two decoded JSON values structurally.
func deepEqualJSON(a, b interface{}) bool {
	left, errA := json.Marshal(a)
	right, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(left) == string(right)
}

func hasDuplicates(items []interface{}) bool {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		encoded, err := json.Marshal(item)
		if err != nil {
			continue
		}
		if _, dup := seen[string(encoded)]; dup {
			return true
		}
		seen[string(encoded)] = struct{}{}
	}
	return false
}
