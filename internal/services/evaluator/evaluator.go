// Package evaluator implements the scoring plugins used by the evaluation
// engine.
//
// An evaluator turns a Target — the input, output and telemetry of one trace or
// dataset item — into a Result. Evaluators register themselves by name in an
// init function, so adding one means adding a file to this package and nothing
// else: the service layer, the API and the database schema are untouched.
package evaluator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Score data types, matching the values stored on the scores table.
const (
	DataTypeNumeric     = "NUMERIC"
	DataTypeCategorical = "CATEGORICAL"
	DataTypeBoolean     = "BOOLEAN"
)

// Target is the material an evaluator scores.
type Target struct {
	// Input, Output and Expected are decoded to plain text where the stored
	// value is a JSON string, and left as compact JSON otherwise.
	Input    string
	Output   string
	Expected string

	Model          string
	Cost           float64
	InputTokens    int64
	OutputTokens   int64
	TotalTokens    int64
	LatencySeconds float64

	// Metadata is the trace metadata, decoded when it is a JSON object.
	Metadata map[string]interface{}

	// ObservationCount and ErrorCount summarise the trace's observation tree.
	ObservationCount int
	ErrorCount       int
}

// Result is one evaluator's verdict.
type Result struct {
	Score       float64 `json:"score"`
	StringValue string  `json:"string_value,omitempty"`
	DataType    string  `json:"data_type"`
	Reason      string  `json:"reason"`
}

// numeric builds a NUMERIC result.
func numeric(score float64, format string, args ...interface{}) Result {
	return Result{
		Score:    score,
		DataType: DataTypeNumeric,
		Reason:   fmt.Sprintf(format, args...),
	}
}

// boolean builds a BOOLEAN result, scored 1 for pass and 0 for fail.
func boolean(pass bool, format string, args ...interface{}) Result {
	score := 0.0
	if pass {
		score = 1
	}
	return Result{
		Score:    score,
		DataType: DataTypeBoolean,
		Reason:   fmt.Sprintf(format, args...),
	}
}

// Evaluator scores a target.
type Evaluator interface {
	Evaluate(ctx context.Context, target Target) (Result, error)
}

// Deps are the shared resources an evaluator may need. Evaluators that reach
// out over the network (LLM judge, embedding similarity) use the provided
// client so timeouts stay under the deployment's control.
type Deps struct {
	HTTPClient *http.Client
}

// Factory builds an evaluator from its JSON configuration.
type Factory func(config json.RawMessage, deps Deps) (Evaluator, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]registration)
)

type registration struct {
	factory     Factory
	description string
	hidden      bool
}

// Register adds an evaluator to the registry. It panics on a duplicate name,
// because that can only be a programming error at init time.
func Register(name, description string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("evaluator %q registered twice", name))
	}
	registry[name] = registration{factory: factory, description: description}
}

// RegisterAlias adds a second name for an already-registered evaluator. Aliases
// keep configurations written against older evaluator ids working; they are
// omitted from Descriptors so the documented list stays canonical.
func RegisterAlias(alias, target string) {
	registryMu.Lock()
	defer registryMu.Unlock()

	entry, ok := registry[target]
	if !ok {
		panic(fmt.Sprintf("alias %q points at unregistered evaluator %q", alias, target))
	}
	if _, exists := registry[alias]; exists {
		panic(fmt.Sprintf("evaluator %q registered twice", alias))
	}
	registry[alias] = registration{factory: entry.factory, description: entry.description, hidden: true}
}

// Build constructs a registered evaluator from its configuration.
func Build(name string, config json.RawMessage, deps Deps) (Evaluator, error) {
	registryMu.RLock()
	entry, ok := registry[normalizeName(name)]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown evaluator %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}

	evaluator, err := entry.factory(config, deps)
	if err != nil {
		return nil, fmt.Errorf("configuring evaluator %q: %w", name, err)
	}
	return evaluator, nil
}

// Descriptor documents one registered evaluator.
type Descriptor struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Names returns every registered evaluator name, sorted.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registry))
	for name, entry := range registry {
		if entry.hidden {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Descriptors returns every registered evaluator with its description, sorted
// by name. The API exposes this so a UI can offer the available evaluators
// without hardcoding the list.
func Descriptors() []Descriptor {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]Descriptor, 0, len(registry))
	for name, entry := range registry {
		if entry.hidden {
			continue
		}
		out = append(out, Descriptor{Name: name, Description: entry.description})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Exists reports whether a name is registered.
func Exists(name string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := registry[normalizeName(name)]
	return ok
}

// normalizeName accepts the stored uppercase form as well as the canonical
// lowercase evaluator id, so `LLM_JUDGE` and `llm_judge` resolve alike.
func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// fieldOf selects the target text an evaluator is configured to read.
func fieldOf(target Target, field string) string {
	switch strings.ToLower(field) {
	case "input":
		return target.Input
	case "expected", "expected_output":
		return target.Expected
	case "output", "":
		return target.Output
	default:
		return target.Output
	}
}

// DefaultHTTPClient builds the client used by network-backed evaluators.
func DefaultHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Timeout: timeout}
}
