package evaluator

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func init() {
	Register("custom_code",
		"Scores a trace with a sandboxed expression over its input, output and telemetry.",
		newCustomCode)
}

// The custom_code evaluator runs a user-supplied expression in a sandbox. It is
// deliberately not a general programming language: there are no loops, no
// assignment, no I/O and no host access, so an expression cannot hang the
// worker or reach anything outside the target it is scoring. That rules out the
// remote-code-execution risk that shipping a real interpreter would carry in a
// self-hosted, multi-tenant deployment.
//
// Available variables:
//
//	input, output, expected      string
//	model                        string
//	cost, latency                number
//	tokens, input_tokens, output_tokens, observation_count, error_count   number
//	metadata.<key>               value from the trace metadata
//
// Available functions:
//
//	len(x) lower(s) upper(s) trim(s) number(x) round(x) abs(x) min(a,b) max(a,b)
//	contains(s,sub) startswith(s,p) endswith(s,p) matches(s,pattern) count(s,sub)
//	similarity(a,b) json_valid(s) if(cond,a,b)
//
// The expression's value becomes the score: a number is used directly, a
// boolean maps to 1 or 0, and a string produces a CATEGORICAL score.

type customCodeConfig struct {
	Expression string `json:"expression"`

	// Clamp bounds a numeric result to [0, 1], which keeps scores comparable
	// across evaluators on the same dashboard.
	Clamp bool `json:"clamp"`
}

type customCodeEvaluator struct {
	config customCodeConfig
	ast    exprNode
}

func newCustomCode(config json.RawMessage, _ Deps) (Evaluator, error) {
	var c customCodeConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if strings.TrimSpace(c.Expression) == "" {
		return nil, fmt.Errorf("`expression` is required")
	}

	ast, err := parseExpression(c.Expression)
	if err != nil {
		return nil, fmt.Errorf("invalid expression: %w", err)
	}

	return &customCodeEvaluator{config: c, ast: ast}, nil
}

func (e *customCodeEvaluator) Evaluate(_ context.Context, target Target) (Result, error) {
	value, err := e.ast.eval(&evalScope{target: target})
	if err != nil {
		return Result{}, fmt.Errorf("evaluating expression: %w", err)
	}

	switch typed := value.(type) {
	case bool:
		return boolean(typed, "expression evaluated to %v", typed), nil

	case float64:
		score := typed
		if e.config.Clamp {
			score = math.Max(0, math.Min(1, score))
		}
		return numeric(score, "expression evaluated to %g", typed), nil

	case string:
		return Result{
			Score:       0,
			StringValue: typed,
			DataType:    DataTypeCategorical,
			Reason:      fmt.Sprintf("expression evaluated to %q", truncate(typed, 80)),
		}, nil

	case nil:
		return Result{}, fmt.Errorf("expression evaluated to null")

	default:
		return Result{}, fmt.Errorf("expression produced unsupported type %T", value)
	}
}

// --- lexer ----------------------------------------------------------------

type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenNumber
	tokenString
	tokenIdent
	tokenOperator
	tokenLParen
	tokenRParen
	tokenComma
)

type token struct {
	kind tokenKind
	text string
	pos  int
}

// maxExpressionLength bounds parser work on a hostile configuration.
const maxExpressionLength = 4096

func tokenize(src string) ([]token, error) {
	if len(src) > maxExpressionLength {
		return nil, fmt.Errorf("expression exceeds %d characters", maxExpressionLength)
	}

	var tokens []token
	i := 0
	for i < len(src) {
		r, size := utf8.DecodeRuneInString(src[i:])

		switch {
		case unicode.IsSpace(r):
			i += size

		case r == '(':
			tokens = append(tokens, token{kind: tokenLParen, text: "(", pos: i})
			i += size

		case r == ')':
			tokens = append(tokens, token{kind: tokenRParen, text: ")", pos: i})
			i += size

		case r == ',':
			tokens = append(tokens, token{kind: tokenComma, text: ",", pos: i})
			i += size

		case r == '"' || r == '\'':
			text, width, err := scanString(src[i:], r)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: tokenString, text: text, pos: i})
			i += width

		case unicode.IsDigit(r):
			text, width := scanNumber(src[i:])
			tokens = append(tokens, token{kind: tokenNumber, text: text, pos: i})
			i += width

		case r == '_' || unicode.IsLetter(r):
			text, width := scanIdent(src[i:])
			tokens = append(tokens, token{kind: tokenIdent, text: text, pos: i})
			i += width

		default:
			op, width := scanOperator(src[i:])
			if width == 0 {
				return nil, fmt.Errorf("unexpected character %q at position %d", string(r), i)
			}
			tokens = append(tokens, token{kind: tokenOperator, text: op, pos: i})
			i += width
		}
	}

	tokens = append(tokens, token{kind: tokenEOF, pos: len(src)})
	return tokens, nil
}

func scanString(src string, quote rune) (string, int, error) {
	var sb strings.Builder
	i := utf8.RuneLen(quote)

	for i < len(src) {
		r, size := utf8.DecodeRuneInString(src[i:])
		switch {
		case r == quote:
			return sb.String(), i + size, nil
		case r == '\\' && i+size < len(src):
			next, nextSize := utf8.DecodeRuneInString(src[i+size:])
			switch next {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			default:
				sb.WriteRune(next)
			}
			i += size + nextSize
		default:
			sb.WriteRune(r)
			i += size
		}
	}

	return "", 0, fmt.Errorf("unterminated string literal")
}

func scanNumber(src string) (string, int) {
	i := 0
	seenDot := false
	for i < len(src) {
		c := src[i]
		if c >= '0' && c <= '9' {
			i++
			continue
		}
		if c == '.' && !seenDot {
			seenDot = true
			i++
			continue
		}
		break
	}
	return src[:i], i
}

func scanIdent(src string) (string, int) {
	i := 0
	for i < len(src) {
		r, size := utf8.DecodeRuneInString(src[i:])
		if r != '_' && r != '.' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		i += size
	}
	return src[:i], i
}

// multiCharOperators are matched before single-character ones so that `<=`
// never lexes as `<` followed by `=`.
var multiCharOperators = []string{"==", "!=", "<=", ">=", "&&", "||"}

func scanOperator(src string) (string, int) {
	for _, op := range multiCharOperators {
		if strings.HasPrefix(src, op) {
			return op, len(op)
		}
	}
	if strings.ContainsRune("+-*/%<>!", rune(src[0])) {
		return src[:1], 1
	}
	return "", 0
}

// --- parser ---------------------------------------------------------------

// binaryPrecedence orders the infix operators; higher binds tighter.
var binaryPrecedence = map[string]int{
	"||": 1,
	"&&": 2,
	"==": 3, "!=": 3,
	"<": 4, "<=": 4, ">": 4, ">=": 4,
	"+": 5, "-": 5,
	"*": 6, "/": 6, "%": 6,
}

type parser struct {
	tokens []token
	pos    int
}

func parseExpression(src string) (exprNode, error) {
	tokens, err := tokenize(src)
	if err != nil {
		return nil, err
	}

	p := &parser{tokens: tokens}
	node, err := p.parseBinary(0)
	if err != nil {
		return nil, err
	}
	if p.current().kind != tokenEOF {
		return nil, fmt.Errorf("unexpected trailing input at position %d", p.current().pos)
	}
	return node, nil
}

func (p *parser) current() token { return p.tokens[p.pos] }

func (p *parser) advance() token {
	tok := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return tok
}

func (p *parser) parseBinary(minPrecedence int) (exprNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}

	for {
		tok := p.current()
		if tok.kind != tokenOperator {
			return left, nil
		}
		precedence, ok := binaryPrecedence[tok.text]
		if !ok || precedence < minPrecedence {
			return left, nil
		}

		p.advance()
		right, err := p.parseBinary(precedence + 1)
		if err != nil {
			return nil, err
		}
		left = &binaryNode{op: tok.text, left: left, right: right}
	}
}

func (p *parser) parseUnary() (exprNode, error) {
	tok := p.current()
	if tok.kind == tokenOperator && (tok.text == "!" || tok.text == "-") {
		p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unaryNode{op: tok.text, operand: operand}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (exprNode, error) {
	tok := p.advance()

	switch tok.kind {
	case tokenNumber:
		f, err := strconv.ParseFloat(tok.text, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q at position %d", tok.text, tok.pos)
		}
		return &literalNode{value: f}, nil

	case tokenString:
		return &literalNode{value: tok.text}, nil

	case tokenIdent:
		switch tok.text {
		case "true":
			return &literalNode{value: true}, nil
		case "false":
			return &literalNode{value: false}, nil
		case "null":
			return &literalNode{value: nil}, nil
		}
		if p.current().kind == tokenLParen {
			return p.parseCall(tok.text)
		}
		return &variableNode{name: tok.text}, nil

	case tokenLParen:
		inner, err := p.parseBinary(0)
		if err != nil {
			return nil, err
		}
		if p.current().kind != tokenRParen {
			return nil, fmt.Errorf("missing closing parenthesis at position %d", p.current().pos)
		}
		p.advance()
		return inner, nil

	case tokenEOF, tokenOperator, tokenRParen, tokenComma:
		return nil, fmt.Errorf("unexpected token %q at position %d", tok.text, tok.pos)

	default:
		return nil, fmt.Errorf("unexpected token at position %d", tok.pos)
	}
}

func (p *parser) parseCall(name string) (exprNode, error) {
	p.advance() // consume '('

	var args []exprNode
	if p.current().kind != tokenRParen {
		for {
			arg, err := p.parseBinary(0)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)

			if p.current().kind != tokenComma {
				break
			}
			p.advance()
		}
	}

	if p.current().kind != tokenRParen {
		return nil, fmt.Errorf("missing closing parenthesis for %s() at position %d", name, p.current().pos)
	}
	p.advance()

	fn, ok := builtinFunctions[name]
	if !ok {
		return nil, fmt.Errorf("unknown function %q", name)
	}
	if len(args) < fn.minArgs || len(args) > fn.maxArgs {
		return nil, fmt.Errorf("%s() takes %d-%d arguments, got %d", name, fn.minArgs, fn.maxArgs, len(args))
	}

	return &callNode{name: name, fn: fn, args: args}, nil
}

// --- evaluation -----------------------------------------------------------

type evalScope struct {
	target Target
}

type exprNode interface {
	eval(scope *evalScope) (interface{}, error)
}

type literalNode struct{ value interface{} }

func (n *literalNode) eval(*evalScope) (interface{}, error) { return n.value, nil }

type variableNode struct{ name string }

func (n *variableNode) eval(scope *evalScope) (interface{}, error) {
	t := scope.target

	if strings.HasPrefix(n.name, "metadata.") {
		key := strings.TrimPrefix(n.name, "metadata.")
		if t.Metadata == nil {
			return nil, nil
		}
		return t.Metadata[key], nil
	}

	switch n.name {
	case "input":
		return t.Input, nil
	case "output":
		return t.Output, nil
	case "expected":
		return t.Expected, nil
	case "model":
		return t.Model, nil
	case "cost":
		return t.Cost, nil
	case "latency":
		return t.LatencySeconds, nil
	case "tokens", "total_tokens":
		return float64(t.TotalTokens), nil
	case "input_tokens":
		return float64(t.InputTokens), nil
	case "output_tokens":
		return float64(t.OutputTokens), nil
	case "observation_count":
		return float64(t.ObservationCount), nil
	case "error_count":
		return float64(t.ErrorCount), nil
	default:
		return nil, fmt.Errorf("unknown variable %q", n.name)
	}
}

type unaryNode struct {
	op      string
	operand exprNode
}

func (n *unaryNode) eval(scope *evalScope) (interface{}, error) {
	value, err := n.operand.eval(scope)
	if err != nil {
		return nil, err
	}

	switch n.op {
	case "!":
		return !truthy(value), nil
	case "-":
		f, err := toNumber(value)
		if err != nil {
			return nil, err
		}
		return -f, nil
	default:
		return nil, fmt.Errorf("unknown unary operator %q", n.op)
	}
}

type binaryNode struct {
	op          string
	left, right exprNode
}

func (n *binaryNode) eval(scope *evalScope) (interface{}, error) {
	// Short-circuit before evaluating the right operand.
	if n.op == "&&" || n.op == "||" {
		left, err := n.left.eval(scope)
		if err != nil {
			return nil, err
		}
		if n.op == "&&" && !truthy(left) {
			return false, nil
		}
		if n.op == "||" && truthy(left) {
			return true, nil
		}
		right, err := n.right.eval(scope)
		if err != nil {
			return nil, err
		}
		return truthy(right), nil
	}

	left, err := n.left.eval(scope)
	if err != nil {
		return nil, err
	}
	right, err := n.right.eval(scope)
	if err != nil {
		return nil, err
	}

	switch n.op {
	case "==":
		return deepEqualJSON(left, right), nil
	case "!=":
		return !deepEqualJSON(left, right), nil
	}

	// `+` concatenates when either side is a string.
	if n.op == "+" {
		if ls, ok := left.(string); ok {
			return ls + toString(right), nil
		}
		if rs, ok := right.(string); ok {
			return toString(left) + rs, nil
		}
	}

	// Comparisons fall back to lexicographic order for two strings.
	if ls, lok := left.(string); lok {
		if rs, rok := right.(string); rok {
			return compareStrings(n.op, ls, rs)
		}
	}

	lf, err := toNumber(left)
	if err != nil {
		return nil, err
	}
	rf, err := toNumber(right)
	if err != nil {
		return nil, err
	}

	switch n.op {
	case "+":
		return lf + rf, nil
	case "-":
		return lf - rf, nil
	case "*":
		return lf * rf, nil
	case "/":
		if rf == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return lf / rf, nil
	case "%":
		if rf == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return math.Mod(lf, rf), nil
	case "<":
		return lf < rf, nil
	case "<=":
		return lf <= rf, nil
	case ">":
		return lf > rf, nil
	case ">=":
		return lf >= rf, nil
	default:
		return nil, fmt.Errorf("unknown operator %q", n.op)
	}
}

func compareStrings(op, left, right string) (interface{}, error) {
	switch op {
	case "<":
		return left < right, nil
	case "<=":
		return left <= right, nil
	case ">":
		return left > right, nil
	case ">=":
		return left >= right, nil
	default:
		return nil, fmt.Errorf("operator %q is not defined for strings", op)
	}
}

type callNode struct {
	name string
	fn   builtinFunction
	args []exprNode
}

func (n *callNode) eval(scope *evalScope) (interface{}, error) {
	// if() is lazy so the untaken branch is never evaluated.
	if n.name == "if" {
		cond, err := n.args[0].eval(scope)
		if err != nil {
			return nil, err
		}
		if truthy(cond) {
			return n.args[1].eval(scope)
		}
		return n.args[2].eval(scope)
	}

	args := make([]interface{}, len(n.args))
	for i, arg := range n.args {
		value, err := arg.eval(scope)
		if err != nil {
			return nil, err
		}
		args[i] = value
	}

	result, err := n.fn.call(args)
	if err != nil {
		return nil, fmt.Errorf("%s(): %w", n.name, err)
	}
	return result, nil
}

// --- built-in functions ---------------------------------------------------

type builtinFunction struct {
	minArgs int
	maxArgs int
	call    func(args []interface{}) (interface{}, error)
}

var builtinFunctions map[string]builtinFunction

func init() {
	builtinFunctions = map[string]builtinFunction{
		"len": {1, 1, func(a []interface{}) (interface{}, error) {
			return float64(utf8.RuneCountInString(toString(a[0]))), nil
		}},
		"lower": {1, 1, func(a []interface{}) (interface{}, error) {
			return strings.ToLower(toString(a[0])), nil
		}},
		"upper": {1, 1, func(a []interface{}) (interface{}, error) {
			return strings.ToUpper(toString(a[0])), nil
		}},
		"trim": {1, 1, func(a []interface{}) (interface{}, error) {
			return strings.TrimSpace(toString(a[0])), nil
		}},
		"contains": {2, 2, func(a []interface{}) (interface{}, error) {
			return strings.Contains(toString(a[0]), toString(a[1])), nil
		}},
		"startswith": {2, 2, func(a []interface{}) (interface{}, error) {
			return strings.HasPrefix(toString(a[0]), toString(a[1])), nil
		}},
		"endswith": {2, 2, func(a []interface{}) (interface{}, error) {
			return strings.HasSuffix(toString(a[0]), toString(a[1])), nil
		}},
		"count": {2, 2, func(a []interface{}) (interface{}, error) {
			return float64(strings.Count(toString(a[0]), toString(a[1]))), nil
		}},
		"matches": {2, 2, func(a []interface{}) (interface{}, error) {
			pattern, err := regexp.Compile(toString(a[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid pattern: %w", err)
			}
			return pattern.MatchString(toString(a[0])), nil
		}},
		"similarity": {2, 2, func(a []interface{}) (interface{}, error) {
			return tokenCosineSimilarity(toString(a[0]), toString(a[1])), nil
		}},
		"json_valid": {1, 1, func(a []interface{}) (interface{}, error) {
			var parsed interface{}
			return json.Unmarshal([]byte(stripCodeFence(toString(a[0]))), &parsed) == nil, nil
		}},
		"number": {1, 1, func(a []interface{}) (interface{}, error) {
			return toNumber(a[0])
		}},
		"abs": {1, 1, func(a []interface{}) (interface{}, error) {
			f, err := toNumber(a[0])
			return math.Abs(f), err
		}},
		"round": {1, 2, func(a []interface{}) (interface{}, error) {
			f, err := toNumber(a[0])
			if err != nil {
				return nil, err
			}
			places := 0.0
			if len(a) == 2 {
				if places, err = toNumber(a[1]); err != nil {
					return nil, err
				}
			}
			scale := math.Pow(10, places)
			return math.Round(f*scale) / scale, nil
		}},
		"min": {2, 2, func(a []interface{}) (interface{}, error) {
			return applyNumeric(a, math.Min)
		}},
		"max": {2, 2, func(a []interface{}) (interface{}, error) {
			return applyNumeric(a, math.Max)
		}},
		// if() is handled lazily in callNode.eval; this entry only declares arity.
		"if": {3, 3, func(a []interface{}) (interface{}, error) { return a[1], nil }},
	}
}

func applyNumeric(args []interface{}, fn func(a, b float64) float64) (interface{}, error) {
	left, err := toNumber(args[0])
	if err != nil {
		return nil, err
	}
	right, err := toNumber(args[1])
	if err != nil {
		return nil, err
	}
	return fn(left, right), nil
}

// truthy applies the language's definition of truth: false, zero, the empty
// string and null are false; everything else is true.
func truthy(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return typed != ""
	default:
		return true
	}
}

func toNumber(value interface{}) (float64, error) {
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case bool:
		if typed {
			return 1, nil
		}
		return 0, nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, fmt.Errorf("cannot use %q as a number", truncate(typed, 40))
		}
		return f, nil
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("cannot use %T as a number", value)
	}
}

func toString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return string(encoded)
	}
}
