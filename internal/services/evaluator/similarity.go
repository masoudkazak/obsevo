package evaluator

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode"
)

func init() {
	Register("semantic_similarity",
		"Scores lexical overlap between two fields (cosine, Jaccard or Levenshtein).",
		newSemanticSimilarity)
}

// semantic_similarity compares two fields without calling a model. It is
// lexical, not neural: it measures token and character overlap. That is enough
// to catch drift and regressions in a self-hosted deployment with no embedding
// service, and it costs nothing per evaluation. For true semantic distance use
// the embedding_similarity evaluator, which calls an embeddings API.

type semanticSimilarityConfig struct {
	// Left and Right name the fields to compare; they default to comparing the
	// output against the dataset item's expected output.
	Left   string `json:"left"`
	Right  string `json:"right"`
	Method string `json:"method"` // cosine (default) | jaccard | levenshtein

	// NGram > 1 compares overlapping word n-grams, which rewards word order.
	NGram int `json:"ngram"`

	// Threshold, when set, converts the similarity into a pass/fail score.
	Threshold *float64 `json:"threshold"`
}

type semanticSimilarityEvaluator struct{ config semanticSimilarityConfig }

func newSemanticSimilarity(config json.RawMessage, _ Deps) (Evaluator, error) {
	var c semanticSimilarityConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if c.Left == "" {
		c.Left = "output"
	}
	if c.Right == "" {
		c.Right = "expected"
	}
	if c.Method == "" {
		c.Method = "cosine"
	}
	switch c.Method {
	case "cosine", "jaccard", "levenshtein":
	default:
		return nil, fmt.Errorf("unknown method %q (want cosine, jaccard or levenshtein)", c.Method)
	}
	if c.NGram < 1 {
		c.NGram = 1
	}
	return &semanticSimilarityEvaluator{config: c}, nil
}

func (e *semanticSimilarityEvaluator) Evaluate(_ context.Context, target Target) (Result, error) {
	left := fieldOf(target, e.config.Left)
	right := fieldOf(target, e.config.Right)

	if left == "" || right == "" {
		return Result{}, fmt.Errorf("semantic_similarity needs both %q and %q to be non-empty",
			e.config.Left, e.config.Right)
	}

	var similarity float64
	switch e.config.Method {
	case "jaccard":
		similarity = jaccardSimilarity(left, right, e.config.NGram)
	case "levenshtein":
		similarity = levenshteinSimilarity(left, right)
	default:
		similarity = ngramCosineSimilarity(left, right, e.config.NGram)
	}

	if e.config.Threshold != nil {
		return boolean(similarity >= *e.config.Threshold,
			"%s similarity %.4f (threshold %.4f)", e.config.Method, similarity, *e.config.Threshold), nil
	}

	return numeric(similarity, "%s similarity %.4f", e.config.Method, similarity), nil
}

// --- similarity measures --------------------------------------------------

// tokenize splits text into lowercase word tokens.
func tokenizeWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// ngrams builds overlapping word n-grams; n == 1 returns the tokens themselves.
func ngrams(tokens []string, n int) []string {
	if n <= 1 || len(tokens) < n {
		return tokens
	}
	out := make([]string, 0, len(tokens)-n+1)
	for i := 0; i+n <= len(tokens); i++ {
		out = append(out, strings.Join(tokens[i:i+n], " "))
	}
	return out
}

// termFrequencies counts occurrences of each term.
func termFrequencies(terms []string) map[string]float64 {
	counts := make(map[string]float64, len(terms))
	for _, term := range terms {
		counts[term]++
	}
	return counts
}

// ngramCosineSimilarity is the cosine of the two term-frequency vectors.
func ngramCosineSimilarity(a, b string, n int) float64 {
	left := termFrequencies(ngrams(tokenizeWords(a), n))
	right := termFrequencies(ngrams(tokenizeWords(b), n))

	if len(left) == 0 || len(right) == 0 {
		return 0
	}

	var dot, leftNorm, rightNorm float64
	for term, count := range left {
		dot += count * right[term]
		leftNorm += count * count
	}
	for _, count := range right {
		rightNorm += count * count
	}

	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

// tokenCosineSimilarity is the unigram cosine, exposed to custom expressions.
func tokenCosineSimilarity(a, b string) float64 {
	return ngramCosineSimilarity(a, b, 1)
}

// jaccardSimilarity is the size of the term intersection over the union.
func jaccardSimilarity(a, b string, n int) float64 {
	left := termFrequencies(ngrams(tokenizeWords(a), n))
	right := termFrequencies(ngrams(tokenizeWords(b), n))

	if len(left) == 0 && len(right) == 0 {
		return 1
	}

	intersection := 0
	for term := range left {
		if _, ok := right[term]; ok {
			intersection++
		}
	}
	union := len(left) + len(right) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// maxLevenshteinInput caps the quadratic edit-distance computation. Longer
// inputs fall back to the cosine measure rather than stalling the worker.
const maxLevenshteinInput = 2000

// levenshteinSimilarity is 1 minus the normalized edit distance.
func levenshteinSimilarity(a, b string) float64 {
	left := []rune(a)
	right := []rune(b)

	if len(left) > maxLevenshteinInput || len(right) > maxLevenshteinInput {
		return ngramCosineSimilarity(a, b, 1)
	}
	if len(left) == 0 && len(right) == 0 {
		return 1
	}

	distance := levenshteinDistance(left, right)
	longest := len(left)
	if len(right) > longest {
		longest = len(right)
	}
	return 1 - float64(distance)/float64(longest)
}

// levenshteinDistance computes edit distance with a single rolling row.
func levenshteinDistance(a, b []rune) int {
	if len(a) < len(b) {
		a, b = b, a
	}

	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}

	current := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = minInt(
				current[j-1]+1,     // insertion
				previous[j]+1,      // deletion
				previous[j-1]+cost, // substitution
			)
		}
		previous, current = current, previous
	}

	return previous[len(b)]
}

func minInt(values ...int) int {
	smallest := values[0]
	for _, v := range values[1:] {
		if v < smallest {
			smallest = v
		}
	}
	return smallest
}
