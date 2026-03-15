package dynamic

import (
	"strings"
	"testing"
)

func TestParseDynamicQuerySimple(t *testing.T) {
	sql := `-- name: ListProfiles :many
SELECT *
FROM profiles
[[if is_admin]]
  WHERE salary > 0
[[else]]
  WHERE public_profile = true
[[endif]]
ORDER BY created_at DESC;
`

	query, err := ParseDynamicQuery(sql)
	if err != nil {
		t.Fatalf("ParseDynamicQuery returned error: %v", err)
	}
	if len(query.Parts) != 3 {
		t.Fatalf("expected 3 top-level parts, got %d", len(query.Parts))
	}
	if !strings.Contains(query.Parts[0].Text, "FROM profiles") {
		t.Fatalf("expected first text part to include query prefix, got %q", query.Parts[0].Text)
	}
	block := query.Parts[1].If
	if block == nil {
		t.Fatal("expected second part to be a dynamic if block")
	}
	if len(query.Controls) != 1 || query.Controls[0].Name != "is_admin" {
		t.Fatalf("unexpected controls: %+v", query.Controls)
	}
	if len(block.Arms) != 2 {
		t.Fatalf("expected 2 arms, got %d", len(block.Arms))
	}
	if block.Arms[0].Kind != DynamicArmKindIf || block.Arms[0].ConditionText != "is_admin" {
		t.Fatalf("unexpected first arm: %+v", block.Arms[0])
	}
	requireControlVariable(t, block.Arms[0].Condition, 1)
	if block.Arms[1].Kind != DynamicArmKindElse || block.Arms[1].ConditionText != "" || block.Arms[1].Condition != nil {
		t.Fatalf("unexpected second arm: %+v", block.Arms[1])
	}
	if !strings.Contains(block.Arms[0].Parts[0].Text, "salary > 0") {
		t.Fatalf("expected if arm body, got %+v", block.Arms[0].Parts)
	}
	if !strings.Contains(block.Arms[1].Parts[0].Text, "public_profile = true") {
		t.Fatalf("expected else arm body, got %+v", block.Arms[1].Parts)
	}
	if !strings.Contains(query.Parts[2].Text, "ORDER BY created_at DESC") {
		t.Fatalf("expected suffix text after endif, got %q", query.Parts[2].Text)
	}
}

func TestParseDynamicQueryNested(t *testing.T) {
	sql := `SELECT 1
[[if outer]]a[[if inner]]b[[else]]c[[endif]]d[[elif fallback]]e[[endif]]
`

	query, err := ParseDynamicQuery(sql)
	if err != nil {
		t.Fatalf("ParseDynamicQuery returned error: %v", err)
	}
	if len(query.Parts) != 3 {
		t.Fatalf("expected 3 top-level parts, got %d", len(query.Parts))
	}
	outer := query.Parts[1].If
	if outer == nil {
		t.Fatal("expected outer dynamic if block")
	}
	if len(outer.Arms) != 2 {
		t.Fatalf("expected 2 outer arms, got %d", len(outer.Arms))
	}
	if outer.Arms[0].ConditionText != "outer" || outer.Arms[1].ConditionText != "fallback" {
		t.Fatalf("unexpected outer arm conditions: %+v", outer.Arms)
	}
	if len(query.Controls) != 3 || query.Controls[0].Name != "outer" || query.Controls[1].Name != "inner" || query.Controls[2].Name != "fallback" {
		t.Fatalf("unexpected controls: %+v", query.Controls)
	}
	requireControlVariable(t, outer.Arms[0].Condition, 1)
	requireControlVariable(t, outer.Arms[1].Condition, 3)
	if len(outer.Arms[0].Parts) != 3 {
		t.Fatalf("expected nested arm to have text/if/text parts, got %d", len(outer.Arms[0].Parts))
	}
	inner := outer.Arms[0].Parts[1].If
	if inner == nil {
		t.Fatal("expected nested if block")
	}
	if len(inner.Arms) != 2 {
		t.Fatalf("expected 2 inner arms, got %d", len(inner.Arms))
	}
	if inner.Arms[0].ConditionText != "inner" {
		t.Fatalf("unexpected inner if condition: %q", inner.Arms[0].ConditionText)
	}
	requireControlVariable(t, inner.Arms[0].Condition, 2)
	if inner.Arms[1].Kind != DynamicArmKindElse {
		t.Fatalf("expected nested else arm, got %q", inner.Arms[1].Kind)
	}
}

func TestParseDynamicQueryIgnoresQuotedAndCommentedDirectives(t *testing.T) {
	sql := "SELECT '[[if nope]]', \"[[else]]\", `[[endif]]`, $$[[if no]]$$, $tag$[[elif no]]$tag$\n" +
		"/* [[if block]] */ -- [[else]]\n" +
		"# [[endif]]\n" +
		"[[if yes]] kept [[endif]]"

	query, err := ParseDynamicQuery(sql)
	if err != nil {
		t.Fatalf("ParseDynamicQuery returned error: %v", err)
	}
	if len(query.Parts) != 2 {
		t.Fatalf("expected 2 top-level parts, got %d", len(query.Parts))
	}
	block := query.Parts[1].If
	if block == nil {
		t.Fatal("expected a real dynamic block after ignored literals/comments")
	}
	if len(block.Arms) != 1 {
		t.Fatalf("expected 1 arm, got %d", len(block.Arms))
	}
	if block.Arms[0].ConditionText != "yes" {
		t.Fatalf("expected real condition to be parsed, got %q", block.Arms[0].ConditionText)
	}
	requireControlVariable(t, block.Arms[0].Condition, 1)
	if !strings.Contains(block.Arms[0].Parts[0].Text, "kept") {
		t.Fatalf("expected arm text to be kept, got %+v", block.Arms[0].Parts)
	}
}

func TestParseDynamicQueryWithParams(t *testing.T) {
	sql := `SELECT *
[[if @limit > 10 || enabled]] WHERE x = 1 [[endif]]`

	query, err := ParseDynamicQueryWithParams(sql, map[string]ConditionParam{
		"limit": {Number: 4, Type: ConditionValueTypeInt},
	})
	if err != nil {
		t.Fatalf("ParseDynamicQueryWithParams returned error: %v", err)
	}
	if len(query.Controls) != 1 || query.Controls[0].Name != "enabled" {
		t.Fatalf("unexpected controls: %+v", query.Controls)
	}
	block := query.Parts[1].If
	if block == nil || len(block.Arms) != 1 {
		t.Fatalf("expected single dynamic block, got %+v", query.Parts)
	}
	if block.Arms[0].ConditionText != "@limit > 10 || enabled" {
		t.Fatalf("unexpected condition text: %q", block.Arms[0].ConditionText)
	}
	if block.Arms[0].Condition == nil || block.Arms[0].Condition.Or == nil || len(block.Arms[0].Condition.Or.Exprs) != 2 {
		t.Fatalf("expected parsed or condition, got %+v", block.Arms[0].Condition)
	}
	requireParamIntComparison(t, block.Arms[0].Condition.Or.Exprs[0], ComparatorGreater, 4, 10)
	requireControlVariable(t, block.Arms[0].Condition.Or.Exprs[1], 1)
}

func TestParseDynamicQueryInvalid(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{name: "elif without if", sql: `SELECT [[elif cond]]`, want: "unexpected [[elif]]"},
		{name: "else without if", sql: `SELECT [[else]]`, want: "unexpected [[else]]"},
		{name: "endif without if", sql: `SELECT [[endif]]`, want: "unexpected [[endif]]"},
		{name: "duplicate else", sql: `SELECT [[if cond]]a[[else]]b[[else]]c[[endif]]`, want: "duplicate [[else]]"},
		{name: "elif after else", sql: `SELECT [[if cond]]a[[else]]b[[elif other]]c[[endif]]`, want: "unexpected [[elif]] after [[else]]"},
		{name: "missing endif", sql: `SELECT [[if cond]]a`, want: "missing [[endif]]"},
		{name: "missing if condition", sql: `SELECT [[if]]a[[endif]]`, want: "missing condition for [[if]]"},
		{name: "missing elif condition", sql: `SELECT [[if cond]]a[[elif]]b[[endif]]`, want: "missing condition for [[elif]]"},
		{name: "extra else content", sql: `SELECT [[if cond]]a[[else nope]]b[[endif]]`, want: "unexpected extra content in [[else]]"},
		{name: "extra endif content", sql: `SELECT [[if cond]]a[[endif nope]]`, want: "unexpected extra content in [[endif]]"},
		{name: "unknown directive", sql: `SELECT [[wat cond]]`, want: "unknown dynamic control directive"},
		{name: "unterminated directive", sql: `SELECT [[if cond]`, want: "unterminated dynamic control directive"},
		{name: "invalid condition", sql: `SELECT [[if 5]]a[[endif]]`, want: "invalid condition for [[if]]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDynamicQuery(tc.sql)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %q", tc.want, err.Error())
			}
		})
	}
}
