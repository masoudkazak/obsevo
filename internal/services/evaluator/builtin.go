package evaluator

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

func init() {
	Register("exact_match", "Scores 1 when the output equals the expected value.", newExactMatch)
	Register("keyword", "Scores the fraction of configured keywords present (or absent).", newKeyword)
	Register("regex", "Scores 1 when a regular expression matches the selected field.", newRegex)
	Register("numeric_range", "Scores 1 when a numeric attribute falls inside a range.", newNumericRange)
	Register("json_valid", "Scores 1 when the selected field parses as JSON.", newJSONValid)
	Register("json_schema", "Validates the selected field against a JSON Schema subset.", newJSONSchema)
	Register("length", "Scores 1 when the field length falls inside a range.", newLength)
	Register("contains", "Scores 1 when the field contains a substring.", newContains)

	// Names used by evaluator configurations written before the registry existed.
	RegisterAlias("length_check", "length")
	RegisterAlias("keyword_check", "keyword")
	RegisterAlias("regex_check", "regex")
}

// --- exact match ----------------------------------------------------------

type exactMatchConfig struct {
	Field         string `json:"field"`
	Expected      string `json:"expected"`
	CaseSensitive bool   `json:"case_sensitive"`
	TrimSpace     bool   `json:"trim_space"`
}

type exactMatchEvaluator struct{ config exactMatchConfig }

func newExactMatch(config json.RawMessage, _ Deps) (Evaluator, error) {
	var c exactMatchConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &exactMatchEvaluator{config: c}, nil
}

func (e *exactMatchEvaluator) Evaluate(_ context.Context, target Target) (Result, error) {
	actual := fieldOf(target, e.config.Field)

	// An expected value in the config wins; otherwise compare against the
	// dataset item's expected output, which is the usual experiment setup.
	expected := e.config.Expected
	if expected == "" {
		expected = target.Expected
	}
	if expected == "" {
		return Result{}, fmt.Errorf("exact_match needs an `expected` value in config or an expected output on the target")
	}

	if e.config.TrimSpace {
		actual = strings.TrimSpace(actual)
		expected = strings.TrimSpace(expected)
	}
	if !e.config.CaseSensitive {
		actual = strings.ToLower(actual)
		expected = strings.ToLower(expected)
	}

	return boolean(actual == expected, "expected %q, got %q", truncate(expected, 80), truncate(actual, 80)), nil
}

// --- keyword --------------------------------------------------------------

type keywordConfig struct {
	Keywords      []string `json:"keywords"`
	Field         string   `json:"field"`
	Mode          string   `json:"mode"` // "contains" (default) or "not_contains"
	CaseSensitive bool     `json:"case_sensitive"`
}

type keywordEvaluator struct{ config keywordConfig }

func newKeyword(config json.RawMessage, _ Deps) (Evaluator, error) {
	var c keywordConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if len(c.Keywords) == 0 {
		return nil, fmt.Errorf("`keywords` must not be empty")
	}
	return &keywordEvaluator{config: c}, nil
}

func (e *keywordEvaluator) Evaluate(_ context.Context, target Target) (Result, error) {
	text := fieldOf(target, e.config.Field)
	if !e.config.CaseSensitive {
		text = strings.ToLower(text)
	}

	found := 0
	var missing []string
	for _, kw := range e.config.Keywords {
		needle := kw
		if !e.config.CaseSensitive {
			needle = strings.ToLower(kw)
		}
		if strings.Contains(text, needle) {
			found++
		} else {
			missing = append(missing, kw)
		}
	}

	score := float64(found) / float64(len(e.config.Keywords))
	reason := fmt.Sprintf("found %d/%d keywords", found, len(e.config.Keywords))
	if e.config.Mode == "not_contains" {
		score = 1 - score
		reason = fmt.Sprintf("%d/%d forbidden keywords present", found, len(e.config.Keywords))
	} else if len(missing) > 0 {
		reason += fmt.Sprintf(" (missing: %s)", strings.Join(missing, ", "))
	}

	return numeric(score, "%s", reason), nil
}

// --- regex ----------------------------------------------------------------

type regexConfig struct {
	Pattern string `json:"pattern"`
	Field   string `json:"field"`
	Negate  bool   `json:"negate"`
}

type regexEvaluator struct {
	config  regexConfig
	pattern *regexp.Regexp
}

func newRegex(config json.RawMessage, _ Deps) (Evaluator, error) {
	var c regexConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if c.Pattern == "" {
		return nil, fmt.Errorf("`pattern` is required")
	}
	compiled, err := regexp.Compile(c.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}
	return &regexEvaluator{config: c, pattern: compiled}, nil
}

func (e *regexEvaluator) Evaluate(_ context.Context, target Target) (Result, error) {
	matched := e.pattern.MatchString(fieldOf(target, e.config.Field))
	pass := matched != e.config.Negate
	return boolean(pass, "pattern=%s matched=%v", e.config.Pattern, matched), nil
}

// --- numeric range --------------------------------------------------------

type numericRangeConfig struct {
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	Field string   `json:"field"`
}

type numericRangeEvaluator struct{ config numericRangeConfig }

func newNumericRange(config json.RawMessage, _ Deps) (Evaluator, error) {
	var c numericRangeConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if c.Min == nil && c.Max == nil {
		return nil, fmt.Errorf("at least one of `min` or `max` is required")
	}
	return &numericRangeEvaluator{config: c}, nil
}

func (e *numericRangeEvaluator) Evaluate(_ context.Context, target Target) (Result, error) {
	value, err := numericField(target, e.config.Field)
	if err != nil {
		return Result{}, err
	}

	inRange := true
	if e.config.Min != nil && value < *e.config.Min {
		inRange = false
	}
	if e.config.Max != nil && value > *e.config.Max {
		inRange = false
	}

	return boolean(inRange, "%s=%g range=[%s, %s]",
		fieldLabel(e.config.Field), value, boundLabel(e.config.Min), boundLabel(e.config.Max)), nil
}

// numericField reads the numeric attribute a range evaluator is pointed at.
func numericField(target Target, field string) (float64, error) {
	switch strings.ToLower(field) {
	case "cost", "":
		return target.Cost, nil
	case "tokens", "total_tokens":
		return float64(target.TotalTokens), nil
	case "input_tokens":
		return float64(target.InputTokens), nil
	case "output_tokens":
		return float64(target.OutputTokens), nil
	case "latency", "latency_seconds":
		return target.LatencySeconds, nil
	case "output_length":
		return float64(utf8.RuneCountInString(target.Output)), nil
	case "observation_count":
		return float64(target.ObservationCount), nil
	case "error_count":
		return float64(target.ErrorCount), nil
	default:
		return 0, fmt.Errorf("numeric_range does not support field %q", field)
	}
}

func fieldLabel(field string) string {
	if field == "" {
		return "cost"
	}
	return field
}

func boundLabel(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%g", *v)
}

// --- JSON validity --------------------------------------------------------

type jsonValidConfig struct {
	Field string `json:"field"`
}

type jsonValidEvaluator struct{ config jsonValidConfig }

func newJSONValid(config json.RawMessage, _ Deps) (Evaluator, error) {
	var c jsonValidConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &jsonValidEvaluator{config: c}, nil
}

func (e *jsonValidEvaluator) Evaluate(_ context.Context, target Target) (Result, error) {
	text := strings.TrimSpace(fieldOf(target, e.config.Field))
	if text == "" {
		return boolean(false, "field is empty"), nil
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(stripCodeFence(text)), &parsed); err != nil {
		return boolean(false, "invalid JSON: %v", err), nil
	}
	return boolean(true, "valid JSON"), nil
}

// --- length ---------------------------------------------------------------

type lengthConfig struct {
	MinLength int    `json:"min_length"`
	MaxLength int    `json:"max_length"`
	Field     string `json:"field"`
	Unit      string `json:"unit"` // "characters" (default) or "words"
}

type lengthEvaluator struct{ config lengthConfig }

func newLength(config json.RawMessage, _ Deps) (Evaluator, error) {
	var c lengthConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &lengthEvaluator{config: c}, nil
}

func (e *lengthEvaluator) Evaluate(_ context.Context, target Target) (Result, error) {
	text := fieldOf(target, e.config.Field)

	length := utf8.RuneCountInString(text)
	unit := "characters"
	if strings.EqualFold(e.config.Unit, "words") {
		length = len(strings.Fields(text))
		unit = "words"
	}

	switch {
	case e.config.MinLength > 0 && length < e.config.MinLength:
		return boolean(false, "length=%d %s, below minimum %d", length, unit, e.config.MinLength), nil
	case e.config.MaxLength > 0 && length > e.config.MaxLength:
		return boolean(false, "length=%d %s, above maximum %d", length, unit, e.config.MaxLength), nil
	default:
		return boolean(true, "length=%d %s", length, unit), nil
	}
}

// --- contains -------------------------------------------------------------

type containsConfig struct {
	Substring     string `json:"substring"`
	Field         string `json:"field"`
	CaseSensitive bool   `json:"case_sensitive"`
	Negate        bool   `json:"negate"`
}

type containsEvaluator struct{ config containsConfig }

func newContains(config json.RawMessage, _ Deps) (Evaluator, error) {
	var c containsConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if c.Substring == "" {
		return nil, fmt.Errorf("`substring` is required")
	}
	return &containsEvaluator{config: c}, nil
}

func (e *containsEvaluator) Evaluate(_ context.Context, target Target) (Result, error) {
	text := fieldOf(target, e.config.Field)
	needle := e.config.Substring
	if !e.config.CaseSensitive {
		text = strings.ToLower(text)
		needle = strings.ToLower(needle)
	}

	found := strings.Contains(text, needle)
	return boolean(found != e.config.Negate, "substring %q present=%v", truncate(e.config.Substring, 40), found), nil
}

// --- shared helpers -------------------------------------------------------

// stripCodeFence removes a Markdown code fence, which models routinely wrap
// around JSON they were asked to emit bare.
func stripCodeFence(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}

	trimmed = strings.TrimPrefix(trimmed, "```")
	if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 {
		// Drop the language tag on the opening fence, if any.
		if !strings.Contains(trimmed[:idx], "{") && !strings.Contains(trimmed[:idx], "[") {
			trimmed = trimmed[idx+1:]
		}
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(trimmed), "```"))
}

// truncate shortens text for inclusion in a score comment.
func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}
