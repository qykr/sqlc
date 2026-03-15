package dynamic

import (
	"strings"
	"testing"
)

func TestParseConditionPrecedence(t *testing.T) {
	cond, controls, err := ParseCondition(`A && (!B || @C > 5)`, map[string]ConditionParam{
		"C": {Number: 7, Type: ConditionValueTypeInt},
	})
	if err != nil {
		t.Fatalf("ParseCondition returned error: %v", err)
	}
	if len(controls) != 2 || controls[0].Name != "A" || controls[1].Name != "B" {
		t.Fatalf("unexpected controls: %+v", controls)
	}
	if cond.And == nil || len(cond.And.Exprs) != 2 {
		t.Fatalf("expected top-level and node, got %+v", cond)
	}
	requireControlVariable(t, cond.And.Exprs[0], 1)
	right := cond.And.Exprs[1]
	if right.Or == nil || len(right.Or.Exprs) != 2 {
		t.Fatalf("expected nested or node, got %+v", right)
	}
	if right.Or.Exprs[0].Not == nil {
		t.Fatalf("expected first nested expr to be not, got %+v", right.Or.Exprs[0])
	}
	requireControlVariable(t, right.Or.Exprs[0].Not.Expr, 2)
	requireParamIntComparison(t, right.Or.Exprs[1], ComparatorGreater, 7, 5)
}

func TestParseConditionStringEquality(t *testing.T) {
	cond, controls, err := ParseCondition(`@sort == "name"`, map[string]ConditionParam{
		"sort": {Number: 2, Type: ConditionValueTypeString},
	})
	if err != nil {
		t.Fatalf("ParseCondition returned error: %v", err)
	}
	if len(controls) != 0 {
		t.Fatalf("expected no controls, got %+v", controls)
	}
	if cond.Comparison == nil {
		t.Fatalf("expected comparison node, got %+v", cond)
	}
	if cond.Comparison.Symbol != ComparatorEquals {
		t.Fatalf("expected equals comparator, got %v", cond.Comparison.Symbol)
	}
	if got := cond.Comparison.Left.ParamNumber; got == nil || *got != 2 {
		t.Fatalf("expected left operand to be param 2, got %+v", cond.Comparison.Left)
	}
	if got := cond.Comparison.Right.ConstStr; got == nil || *got != "name" {
		t.Fatalf("expected right operand to be string literal, got %+v", cond.Comparison.Right)
	}
}

func TestParseConditionBoolParameter(t *testing.T) {
	cond, controls, err := ParseCondition(`@flag || A`, map[string]ConditionParam{
		"flag": {Number: 3, Type: ConditionValueTypeBool},
	})
	if err != nil {
		t.Fatalf("ParseCondition returned error: %v", err)
	}
	if len(controls) != 1 || controls[0].Name != "A" {
		t.Fatalf("unexpected controls: %+v", controls)
	}
	if cond.Or == nil || len(cond.Or.Exprs) != 2 {
		t.Fatalf("expected or node, got %+v", cond)
	}
	requireParamVariable(t, cond.Or.Exprs[0], 3)
	requireControlVariable(t, cond.Or.Exprs[1], 1)
}

func TestParseConditionInvalid(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		params map[string]ConditionParam
		want   string
	}{
		{name: "unknown parameter", input: `@missing`, want: "unknown parameter @missing"},
		{name: "non-bool bare parameter", input: `@count && A`, params: map[string]ConditionParam{"count": {Number: 1, Type: ConditionValueTypeInt}}, want: "parameter variables used as conditions must be bool"},
		{name: "string ordering", input: `@sort > "name"`, params: map[string]ConditionParam{"sort": {Number: 1, Type: ConditionValueTypeString}}, want: "operator > is not supported for string values"},
		{name: "bare literal", input: `5`, want: "expected boolean variable or comparison"},
		{name: "missing close paren", input: `(@n > 1`, params: map[string]ConditionParam{"n": {Number: 1, Type: ConditionValueTypeInt}}, want: "expected ')'"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ParseCondition(tc.input, tc.params)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if got := err.Error(); !strings.Contains(got, tc.want) {
				t.Fatalf("expected error containing %q, got %q", tc.want, got)
			}
		})
	}
}

func requireControlVariable(t *testing.T, cond *Condition, want int32) {
	t.Helper()
	if cond.Variable == nil || cond.Variable.ControlID == nil || *cond.Variable.ControlID != want {
		t.Fatalf("expected control variable %d, got %+v", want, cond)
	}
}

func requireParamVariable(t *testing.T, cond *Condition, want int32) {
	t.Helper()
	if cond.Variable == nil || cond.Variable.ParamNumber == nil || *cond.Variable.ParamNumber != want {
		t.Fatalf("expected param variable %d, got %+v", want, cond)
	}
}

func requireParamIntComparison(t *testing.T, cond *Condition, want Comparator, param int32, literal int64) {
	t.Helper()
	if cond.Comparison == nil {
		t.Fatalf("expected comparison, got %+v", cond)
	}
	if cond.Comparison.Symbol != want {
		t.Fatalf("expected comparator %v, got %v", want, cond.Comparison.Symbol)
	}
	if got := cond.Comparison.Left.ParamNumber; got == nil || *got != param {
		t.Fatalf("expected left param %d, got %+v", param, cond.Comparison.Left)
	}
	if got := cond.Comparison.Right.ConstInt; got == nil || *got != literal {
		t.Fatalf("expected right const int %d, got %+v", literal, cond.Comparison.Right)
	}
}
