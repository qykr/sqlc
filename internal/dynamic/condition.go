package dynamic

import (
	"fmt"
	"strconv"
	"strings"
)

type Control struct {
	ID   int32
	Name string
}

// ConditionValueType describes the externally-provided type of a dynamic
// condition operand.
type ConditionValueType string

const (
	ConditionValueTypeBool   ConditionValueType = "bool"
	ConditionValueTypeInt    ConditionValueType = "int"
	ConditionValueTypeDouble ConditionValueType = "double"
	ConditionValueTypeString ConditionValueType = "string"
)

// ConditionParam describes a dynamic control parameter
type ConditionParam struct {
	Number int32
	Type   ConditionValueType
}

type Condition struct {
	Comparison *Comparison
	Variable   *Variable
	Not        *Not
	And        *And
	Or         *Or
}

type Variable struct {
	ControlID   *int32
	ParamNumber *int32
}

type ComparisonValue struct {
	ControlID   *int32
	ParamNumber *int32
	ConstInt    *int64
	ConstDouble *float64
	ConstStr    *string
	ConstBool   *bool
}

type Comparison struct {
	Symbol Comparator
	Left   ComparisonValue
	Right  ComparisonValue
}

type Comparator int

const (
	ComparatorEquals Comparator = iota
	ComparatorNotEquals
	ComparatorGreater
	ComparatorGreaterEqual
	ComparatorLess
	ComparatorLessEqual
)

func (c Comparator) String() string {
	switch c {
	case ComparatorEquals:
		return "=="
	case ComparatorNotEquals:
		return "!="
	case ComparatorGreater:
		return ">"
	case ComparatorGreaterEqual:
		return ">="
	case ComparatorLess:
		return "<"
	case ComparatorLessEqual:
		return "<="
	default:
		return fmt.Sprintf("Comparator(%d)", c)
	}
}

type Not struct {
	Expr *Condition
}

type And struct {
	Exprs []*Condition
}

type Or struct {
	Exprs []*Condition
}

func ParseCondition(input string, params map[string]ConditionParam) (*Condition, []Control, error) {
	registry := &conditionControlRegistry{ids: map[string]int32{}}
	cond, err := parseConditionWithRegistry(input, params, registry)
	if err != nil {
		return nil, nil, err
	}
	return cond, append([]Control(nil), registry.controls...), nil
}

func parseConditionWithRegistry(input string, params map[string]ConditionParam, registry *conditionControlRegistry) (*Condition, error) {
	if registry == nil {
		registry = &conditionControlRegistry{ids: map[string]int32{}}
	}
	p := conditionParser{
		input:    input,
		params:   params,
		controls: registry,
	}
	cond, err := p.parse()
	if err != nil {
		return nil, err
	}
	return cond, nil
}

type conditionControlRegistry struct {
	ids      map[string]int32
	controls []Control
}

func (r *conditionControlRegistry) idFor(name string) int32 {
	if id, ok := r.ids[name]; ok {
		return id
	}
	id := int32(len(r.controls) + 1)
	r.ids[name] = id
	r.controls = append(r.controls, Control{ID: id, Name: name})
	return id
}

type parsedConditionOperand struct {
	value    ComparisonValue
	variable *Variable
	typ      ConditionValueType
}

type conditionParser struct {
	input    string
	i        int
	params   map[string]ConditionParam
	controls *conditionControlRegistry
}

func (p *conditionParser) parse() (*Condition, error) {
	cond, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if !p.eof() {
		return nil, p.errorf("unexpected trailing content")
	}
	return cond, nil
}

func (p *conditionParser) parseOr() (*Condition, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	exprs := []*Condition{left}
	for {
		if !p.consume("||") {
			break
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, right)
	}
	if len(exprs) == 1 {
		return left, nil
	}
	return &Condition{Or: &Or{Exprs: exprs}}, nil
}

func (p *conditionParser) parseAnd() (*Condition, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	exprs := []*Condition{left}
	for {
		if !p.consume("&&") {
			break
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, right)
	}
	if len(exprs) == 1 {
		return left, nil
	}
	return &Condition{And: &And{Exprs: exprs}}, nil
}

func (p *conditionParser) parseUnary() (*Condition, error) {
	if p.consume("!") {
		expr, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &Condition{Not: &Not{Expr: expr}}, nil
	}
	return p.parsePrimary()
}

func (p *conditionParser) parsePrimary() (*Condition, error) {
	p.skipSpace()
	if p.eof() {
		return nil, p.errorf("expected condition")
	}
	if p.consume("(") {
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.consume(")") {
			return nil, p.errorf("expected ')' to close grouped condition")
		}
		return expr, nil
	}

	left, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	if cmp, ok := p.consumeComparator(); ok {
		right, err := p.parseOperand()
		if err != nil {
			return nil, err
		}
		if err := validateComparisonTypes(left.typ, right.typ, cmp); err != nil {
			return nil, p.errorf("%s", err)
		}
		return &Condition{Comparison: &Comparison{Symbol: cmp, Left: left.value, Right: right.value}}, nil
	}
	if left.variable == nil {
		return nil, p.errorf("expected boolean variable or comparison")
	}
	if left.typ != ConditionValueTypeBool {
		return nil, p.errorf("parameter variables used as conditions must be bool")
	}
	return &Condition{Variable: left.variable}, nil
}

func (p *conditionParser) parseOperand() (parsedConditionOperand, error) {
	p.skipSpace()
	if p.eof() {
		return parsedConditionOperand{}, p.errorf("expected operand")
	}

	if p.consume("@") {
		name, err := p.parseIdentifier("expected parameter name after '@'")
		if err != nil {
			return parsedConditionOperand{}, err
		}
		param, ok := p.params[name]
		if !ok {
			return parsedConditionOperand{}, p.errorf("unknown parameter @%s", name)
		}
		if param.Type == "" {
			return parsedConditionOperand{}, p.errorf("parameter @%s is missing a condition type", name)
		}
		paramNumber := param.Number
		return parsedConditionOperand{
			typ:      param.Type,
			variable: &Variable{ParamNumber: &paramNumber},
			value:    ComparisonValue{ParamNumber: &paramNumber},
		}, nil
	}

	if p.peekStringStart() {
		value, err := p.parseStringLiteral()
		if err != nil {
			return parsedConditionOperand{}, err
		}
		constStr := value
		return parsedConditionOperand{
			typ:   ConditionValueTypeString,
			value: ComparisonValue{ConstStr: &constStr},
		}, nil
	}

	if p.peekNumberStart() {
		return p.parseNumericLiteral()
	}

	name, err := p.parseIdentifier("expected operand")
	if err != nil {
		return parsedConditionOperand{}, err
	}
	if name == "true" || name == "false" {
		value := name == "true"
		return parsedConditionOperand{
			typ:   ConditionValueTypeBool,
			value: ComparisonValue{ConstBool: &value},
		}, nil
	}
	id := p.controls.idFor(name)
	controlID := id
	return parsedConditionOperand{
		typ:      ConditionValueTypeBool,
		variable: &Variable{ControlID: &controlID},
		value:    ComparisonValue{ControlID: &controlID},
	}, nil
}

func (p *conditionParser) parseNumericLiteral() (parsedConditionOperand, error) {
	start := p.i
	if !p.eof() && (p.input[p.i] == '+' || p.input[p.i] == '-') {
		p.i++
	}
	digitsBefore := p.consumeDigits()
	hasDot := false
	digitsAfter := 0
	if !p.eof() && p.input[p.i] == '.' {
		hasDot = true
		p.i++
		digitsAfter = p.consumeDigits()
	}
	if digitsBefore == 0 && digitsAfter == 0 {
		return parsedConditionOperand{}, p.errorf("invalid numeric literal")
	}
	literal := p.input[start:p.i]
	if hasDot {
		value, err := strconv.ParseFloat(literal, 64)
		if err != nil {
			return parsedConditionOperand{}, p.errorf("invalid numeric literal %q", literal)
		}
		constDouble := value
		return parsedConditionOperand{
			typ:   ConditionValueTypeDouble,
			value: ComparisonValue{ConstDouble: &constDouble},
		}, nil
	}
	value, err := strconv.ParseInt(literal, 10, 64)
	if err != nil {
		return parsedConditionOperand{}, p.errorf("invalid integer literal %q", literal)
	}
	constInt := value
	return parsedConditionOperand{
		typ:   ConditionValueTypeInt,
		value: ComparisonValue{ConstInt: &constInt},
	}, nil
}

func (p *conditionParser) parseIdentifier(message string) (string, error) {
	p.skipSpace()
	if p.eof() || !(isASCIIAlpha(p.input[p.i]) || p.input[p.i] == '_') {
		return "", p.errorf("%s", message)
	}
	start := p.i
	p.i++
	for !p.eof() {
		ch := p.input[p.i]
		if !(isASCIIAlpha(ch) || isASCIIDigit(ch) || ch == '_') {
			break
		}
		p.i++
	}
	return p.input[start:p.i], nil
}

func (p *conditionParser) parseStringLiteral() (string, error) {
	delim := p.input[p.i]
	p.i++
	var b strings.Builder
	for !p.eof() {
		ch := p.input[p.i]
		switch ch {
		case '\\':
			if p.i+1 >= len(p.input) {
				return "", p.errorf("unterminated string literal")
			}
			next := p.input[p.i+1]
			switch next {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(next)
			}
			p.i += 2
		case delim:
			if p.i+1 < len(p.input) && p.input[p.i+1] == delim {
				b.WriteByte(delim)
				p.i += 2
				continue
			}
			p.i++
			return b.String(), nil
		default:
			b.WriteByte(ch)
			p.i++
		}
	}
	return "", p.errorf("unterminated string literal")
}

func (p *conditionParser) consumeComparator() (Comparator, bool) {
	switch {
	case p.consume("=="):
		return ComparatorEquals, true
	case p.consume("!="):
		return ComparatorNotEquals, true
	case p.consume(">="):
		return ComparatorGreaterEqual, true
	case p.consume("<="):
		return ComparatorLessEqual, true
	case p.consume(">"):
		return ComparatorGreater, true
	case p.consume("<"):
		return ComparatorLess, true
	default:
		return 0, false
	}
}

func (p *conditionParser) consume(text string) bool {
	p.skipSpace()
	if strings.HasPrefix(p.input[p.i:], text) {
		p.i += len(text)
		return true
	}
	return false
}

func (p *conditionParser) consumeDigits() int {
	start := p.i
	for !p.eof() && isASCIIDigit(p.input[p.i]) {
		p.i++
	}
	return p.i - start
}

func (p *conditionParser) peekNumberStart() bool {
	p.skipSpace()
	if p.eof() {
		return false
	}
	ch := p.input[p.i]
	if isASCIIDigit(ch) {
		return true
	}
	if ch == '.' {
		return p.i+1 < len(p.input) && isASCIIDigit(p.input[p.i+1])
	}
	if ch == '+' || ch == '-' {
		if p.i+1 >= len(p.input) {
			return false
		}
		next := p.input[p.i+1]
		if isASCIIDigit(next) {
			return true
		}
		return next == '.' && p.i+2 < len(p.input) && isASCIIDigit(p.input[p.i+2])
	}
	return false
}

func (p *conditionParser) peekStringStart() bool {
	p.skipSpace()
	return !p.eof() && (p.input[p.i] == '\'' || p.input[p.i] == '"')
}

func (p *conditionParser) skipSpace() {
	for !p.eof() {
		switch p.input[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *conditionParser) eof() bool {
	return p.i >= len(p.input)
}

func (p *conditionParser) errorf(format string, args ...any) error {
	return fmt.Errorf("invalid dynamic condition at offset %d: %s", p.i, fmt.Sprintf(format, args...))
}

func validateComparisonTypes(left, right ConditionValueType, cmp Comparator) error {
	if left == right {
		return validateComparatorForType(left, cmp)
	}
	if isNumericConditionType(left) && isNumericConditionType(right) {
		return validateComparatorForType(ConditionValueTypeDouble, cmp)
	}
	return fmt.Errorf("incompatible comparison types %q and %q", left, right)
}

func validateComparatorForType(typ ConditionValueType, cmp Comparator) error {
	switch typ {
	case ConditionValueTypeInt, ConditionValueTypeDouble:
		return nil
	case ConditionValueTypeBool, ConditionValueTypeString:
		if cmp == ComparatorEquals || cmp == ComparatorNotEquals {
			return nil
		}
		return fmt.Errorf("operator %s is not supported for %s values", cmp, typ)
	default:
		return fmt.Errorf("unsupported comparison type %q", typ)
	}
}

func isNumericConditionType(typ ConditionValueType) bool {
	return typ == ConditionValueTypeInt || typ == ConditionValueTypeDouble
}
