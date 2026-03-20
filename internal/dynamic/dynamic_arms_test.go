package dynamic

import "testing"

const (
	dynamicArmSQLSimple = `SELECT 1 [[if outer]]a[[else]]b[[endif]]`

	dynamicArmSQLFallback = `SELECT 1
[[if left]]a[[else]]b[[endif]]
[[if right]]c[[elif middle]]d[[else]]e[[endif]]
`

	dynamicArmSQLArmTreeNested = `SELECT 1
[[if outer]]a[[if inner]]b[[else]]c[[endif]]d[[elif fallback]]e[[endif]]
`

	dynamicArmSQLWeakNested = `SELECT 1
[[if outer]]a[[if inner]]b[[elif inner_fallback]]c[[else]]d[[endif]]e[[else]]f[[endif]]
`

	dynamicArmSQLHeuristicNested = `SELECT 1
[[if outer]]a[[if inner]]b[[else]]c[[endif]]d[[else]]e[[endif]]
[[if sibling]]f[[else]]g[[endif]]
`

	dynamicArmSQLDedup = `SELECT 1[[if outer]][[else]][[endif]]`
)

func mustParseDynamicQueryForArms(t *testing.T, sql string) DynamicQuery {
	t.Helper()

	query, err := ParseDynamicQuery(sql, nil)
	if err != nil {
		t.Fatalf("ParseDynamicQuery returned error: %v", err)
	}

	return query
}

func mustParseDynamicArmTree(t *testing.T, sql string) *DynamicArmSkeleton {
	t.Helper()

	return mustParseDynamicQueryForArms(t, sql).ArmTree()
}

func assertStaticQueries(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d static queries, got %d: %q", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected static query %d to be %q, got %q", i, want[i], got[i])
		}
	}
}

func TestDynamicQueryArmTreeSimple(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLSimple)
	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 top-level block, got %d", len(tree.Children))
	}
	if len(tree.Children[0].Children) != 2 {
		t.Fatalf("expected top-level block to have 2 arms, got %d", len(tree.Children[0].Children))
	}
	if len(tree.Children[0].Children[0].Children) != 0 {
		t.Fatalf("expected first arm to have no nested blocks, got %d", len(tree.Children[0].Children[0].Children))
	}
}

func TestDynamicQueryArmTreeNested(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLArmTreeNested)
	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 top-level block, got %d", len(tree.Children))
	}

	outer := tree.Children[0]
	if len(outer.Children) != 2 {
		t.Fatalf("expected outer block to have 2 arms, got %d", len(outer.Children))
	}
	if len(outer.Children[0].Children) != 1 {
		t.Fatalf("expected first outer arm to contain 1 nested block, got %d", len(outer.Children[0].Children))
	}
	if len(outer.Children[1].Children) != 0 {
		t.Fatalf("expected second outer arm to contain 0 nested blocks, got %d", len(outer.Children[1].Children))
	}

	inner := outer.Children[0].Children[0]
	if len(inner.Children) != 2 {
		t.Fatalf("expected inner block to have 2 arms, got %d", len(inner.Children))
	}
	if len(inner.Children[0].Children) != 0 || len(inner.Children[1].Children) != 0 {
		t.Fatalf("expected inner arms to have no nested blocks, got %d and %d", len(inner.Children[0].Children), len(inner.Children[1].Children))
	}
}

func TestDynamicArmSkeletonWeakTraversalsSimple(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLSimple)
	t.Logf("skeleton:\n%s", tree)

	traversals := tree.WeakTraversals()
	if len(traversals) != 2 {
		t.Fatalf("expected 2 weak traversals, got %d", len(traversals))
	}

	want := []int{0, 1}
	for i, traversal := range traversals {
		t.Logf("traversal[%d]:\n%s", i, traversal)
		if len(traversal.Children) != 1 {
			t.Fatalf("expected traversal %d to have 1 top-level block, got %d", i, len(traversal.Children))
		}
		if traversal.Children[0].Arm != want[i] {
			t.Fatalf("expected traversal %d to select arm %d, got %d", i, want[i], traversal.Children[0].Arm)
		}
		if len(traversal.Children[0].Children) != 0 {
			t.Fatalf("expected traversal %d to have no nested blocks, got %d", i, len(traversal.Children[0].Children))
		}
	}
}

func TestDynamicArmSkeletonWeakTraversalsFallback(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLFallback)
	t.Logf("skeleton:\n%s", tree)

	traversals := tree.WeakTraversals()
	if len(traversals) != 4 {
		t.Fatalf("expected 4 weak traversals, got %d", len(traversals))
	}

	want := [][2]int{{0, 0}, {1, 0}, {0, 1}, {0, 2}}
	for i, traversal := range traversals {
		t.Logf("traversal[%d]:\n%s", i, traversal)
		if len(traversal.Children) != 2 {
			t.Fatalf("expected traversal %d to have 2 top-level blocks, got %d", i, len(traversal.Children))
		}
		if traversal.Children[0].Arm != want[i][0] || traversal.Children[1].Arm != want[i][1] {
			t.Fatalf("expected traversal %d arms %v, got [%d %d]", i, want[i], traversal.Children[0].Arm, traversal.Children[1].Arm)
		}
	}
}

func TestDynamicArmSkeletonWeakTraversalsNested(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLWeakNested)
	t.Logf("skeleton:\n%s", tree)

	traversals := tree.WeakTraversals()
	if len(traversals) != 4 {
		t.Fatalf("expected 4 weak traversals, got %d", len(traversals))
	}
	for i, traversal := range traversals {
		t.Logf("traversal[%d]:\n%s", i, traversal)
	}

	if traversals[0].Children[0].Arm != 0 {
		t.Fatalf("expected traversal 0 outer block to select arm 0, got %d", traversals[0].Children[0].Arm)
	}
	if len(traversals[0].Children[0].Children) != 1 {
		t.Fatalf("expected traversal 0 to include 1 nested block, got %d", len(traversals[0].Children[0].Children))
	}
	if traversals[0].Children[0].Children[0].Arm != 0 {
		t.Fatalf("expected traversal 0 inner block to select arm 0, got %d", traversals[0].Children[0].Children[0].Arm)
	}

	outer := traversals[1].Children[0]
	if outer.Arm != 1 {
		t.Fatalf("expected traversal 1 outer block to select arm 1, got %d", outer.Arm)
	}
	if len(outer.Children) != 0 {
		t.Fatalf("expected traversal 1 to descend only through selected outer arm, got %d nested blocks", len(outer.Children))
	}

	innerOne := traversals[2].Children[0]
	if innerOne.Arm != 0 {
		t.Fatalf("expected traversal 2 outer block to stay on arm 0, got %d", innerOne.Arm)
	}
	if len(innerOne.Children) != 1 || innerOne.Children[0].Arm != 1 {
		t.Fatalf("expected traversal 2 to select inner arm 1, got %+v", innerOne.Children)
	}

	innerTwo := traversals[3].Children[0]
	if innerTwo.Arm != 0 {
		t.Fatalf("expected traversal 3 outer block to stay on arm 0, got %d", innerTwo.Arm)
	}
	if len(innerTwo.Children) != 1 || innerTwo.Children[0].Arm != 2 {
		t.Fatalf("expected traversal 3 to select inner arm 2, got %+v", innerTwo.Children)
	}
}

func TestDynamicArmSkeletonHeuristicTraversalsSimple(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLSimple)
	t.Logf("skeleton:\n%s", tree)

	traversals := tree.HeuristicTraversals()
	if len(traversals) != 2 {
		t.Fatalf("expected 2 heuristic traversals, got %d", len(traversals))
	}

	want := []int{0, 1}
	for i, traversal := range traversals {
		t.Logf("traversal[%d]:\n%s", i, traversal)
		if len(traversal.Children) != 1 {
			t.Fatalf("expected traversal %d to have 1 top-level block, got %d", i, len(traversal.Children))
		}
		if traversal.Children[0].Arm != want[i] {
			t.Fatalf("expected traversal %d to select arm %d, got %d", i, want[i], traversal.Children[0].Arm)
		}
	}
}

func TestDynamicArmSkeletonHeuristicTraversalsFallback(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLFallback)
	t.Logf("skeleton:\n%s", tree)

	traversals := tree.HeuristicTraversals()
	if len(traversals) != 6 {
		t.Fatalf("expected 6 heuristic traversals, got %d", len(traversals))
	}

	want := [][2]int{{0, 0}, {1, 1}, {0, 2}, {0, 1}, {1, 0}, {1, 2}}
	for i, traversal := range traversals {
		t.Logf("traversal[%d]:\n%s", i, traversal)
		if len(traversal.Children) != 2 {
			t.Fatalf("expected traversal %d to have 2 top-level blocks, got %d", i, len(traversal.Children))
		}
		if traversal.Children[0].Arm != want[i][0] || traversal.Children[1].Arm != want[i][1] {
			t.Fatalf("expected traversal %d arms %v, got [%d %d]", i, want[i], traversal.Children[0].Arm, traversal.Children[1].Arm)
		}
	}
}

func TestDynamicArmSkeletonHeuristicTraversalsNested(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLHeuristicNested)
	t.Logf("skeleton:\n%s", tree)

	traversals := tree.HeuristicTraversals()
	if len(traversals) != 5 {
		t.Fatalf("expected 5 heuristic traversals, got %d", len(traversals))
	}
	for i, traversal := range traversals {
		t.Logf("traversal[%d]:\n%s", i, traversal)
	}

	first := traversals[0].Children
	if first[0].Arm != 0 || first[1].Arm != 0 {
		t.Fatalf("expected traversal 0 top-level arms [0 0], got [%d %d]", first[0].Arm, first[1].Arm)
	}
	if len(first[0].Children) != 1 || first[0].Children[0].Arm != 0 {
		t.Fatalf("expected traversal 0 to keep nested inner arm 0, got %+v", first[0].Children)
	}

	second := traversals[1].Children
	if second[0].Arm != 1 || second[1].Arm != 1 {
		t.Fatalf("expected traversal 1 top-level arms [1 1], got [%d %d]", second[0].Arm, second[1].Arm)
	}

	third := traversals[2].Children
	if third[0].Arm != 0 || third[1].Arm != 1 {
		t.Fatalf("expected traversal 2 top-level arms [0 1], got [%d %d]", third[0].Arm, third[1].Arm)
	}
	if len(third[0].Children) != 1 || third[0].Children[0].Arm != 0 {
		t.Fatalf("expected traversal 2 to keep nested inner arm 0, got %+v", third[0].Children)
	}

	fourth := traversals[3].Children
	if fourth[0].Arm != 1 || fourth[1].Arm != 0 {
		t.Fatalf("expected traversal 3 top-level arms [1 0], got [%d %d]", fourth[0].Arm, fourth[1].Arm)
	}

	fifth := traversals[4].Children
	if fifth[0].Arm != 0 || fifth[1].Arm != 0 {
		t.Fatalf("expected traversal 4 top-level arms [0 0], got [%d %d]", fifth[0].Arm, fifth[1].Arm)
	}
	if len(fifth[0].Children) != 1 || fifth[0].Children[0].Arm != 1 {
		t.Fatalf("expected traversal 4 to select nested inner arm 1, got %+v", fifth[0].Children)
	}
}

func TestDynamicArmSkeletonExhaustiveTraversalsSimple(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLSimple)
	t.Logf("skeleton:\n%s", tree)

	traversals := tree.ExhaustiveTraversals()
	if len(traversals) != 2 {
		t.Fatalf("expected 2 exhaustive traversals, got %d", len(traversals))
	}

	want := []int{0, 1}
	for i, traversal := range traversals {
		t.Logf("traversal[%d]:\n%s", i, traversal)
		if len(traversal.Children) != 1 {
			t.Fatalf("expected traversal %d to have 1 top-level block, got %d", i, len(traversal.Children))
		}
		if traversal.Children[0].Arm != want[i] {
			t.Fatalf("expected traversal %d to select arm %d, got %d", i, want[i], traversal.Children[0].Arm)
		}
	}
}

func TestDynamicArmSkeletonExhaustiveTraversalsFallback(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLFallback)
	t.Logf("skeleton:\n%s", tree)

	traversals := tree.ExhaustiveTraversals()
	if len(traversals) != 6 {
		t.Fatalf("expected 6 exhaustive traversals, got %d", len(traversals))
	}

	want := [][2]int{{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 1}, {1, 2}}
	for i, traversal := range traversals {
		t.Logf("traversal[%d]:\n%s", i, traversal)
		if len(traversal.Children) != 2 {
			t.Fatalf("expected traversal %d to have 2 top-level blocks, got %d", i, len(traversal.Children))
		}
		if traversal.Children[0].Arm != want[i][0] || traversal.Children[1].Arm != want[i][1] {
			t.Fatalf("expected traversal %d arms %v, got [%d %d]", i, want[i], traversal.Children[0].Arm, traversal.Children[1].Arm)
		}
	}
}

func TestDynamicArmSkeletonExhaustiveTraversalsNested(t *testing.T) {
	tree := mustParseDynamicArmTree(t, dynamicArmSQLHeuristicNested)
	t.Logf("skeleton:\n%s", tree)

	traversals := tree.ExhaustiveTraversals()
	if len(traversals) != 6 {
		t.Fatalf("expected 6 exhaustive traversals, got %d", len(traversals))
	}
	for i, traversal := range traversals {
		t.Logf("traversal[%d]:\n%s", i, traversal)
	}

	first := traversals[0].Children
	if first[0].Arm != 0 || first[1].Arm != 0 {
		t.Fatalf("expected traversal 0 top-level arms [0 0], got [%d %d]", first[0].Arm, first[1].Arm)
	}
	if len(first[0].Children) != 1 || first[0].Children[0].Arm != 0 {
		t.Fatalf("expected traversal 0 nested inner arm 0, got %+v", first[0].Children)
	}

	second := traversals[1].Children
	if second[0].Arm != 0 || second[1].Arm != 1 {
		t.Fatalf("expected traversal 1 top-level arms [0 1], got [%d %d]", second[0].Arm, second[1].Arm)
	}
	if len(second[0].Children) != 1 || second[0].Children[0].Arm != 0 {
		t.Fatalf("expected traversal 1 nested inner arm 0, got %+v", second[0].Children)
	}

	third := traversals[2].Children
	if third[0].Arm != 0 || third[1].Arm != 0 {
		t.Fatalf("expected traversal 2 top-level arms [0 0], got [%d %d]", third[0].Arm, third[1].Arm)
	}
	if len(third[0].Children) != 1 || third[0].Children[0].Arm != 1 {
		t.Fatalf("expected traversal 2 nested inner arm 1, got %+v", third[0].Children)
	}

	fourth := traversals[3].Children
	if fourth[0].Arm != 0 || fourth[1].Arm != 1 {
		t.Fatalf("expected traversal 3 top-level arms [0 1], got [%d %d]", fourth[0].Arm, fourth[1].Arm)
	}
	if len(fourth[0].Children) != 1 || fourth[0].Children[0].Arm != 1 {
		t.Fatalf("expected traversal 3 nested inner arm 1, got %+v", fourth[0].Children)
	}

	fifth := traversals[4].Children
	if fifth[0].Arm != 1 || fifth[1].Arm != 0 {
		t.Fatalf("expected traversal 4 top-level arms [1 0], got [%d %d]", fifth[0].Arm, fifth[1].Arm)
	}
	if len(fifth[0].Children) != 0 {
		t.Fatalf("expected traversal 4 to have no nested blocks under outer arm 1, got %+v", fifth[0].Children)
	}

	sixth := traversals[5].Children
	if sixth[0].Arm != 1 || sixth[1].Arm != 1 {
		t.Fatalf("expected traversal 5 top-level arms [1 1], got [%d %d]", sixth[0].Arm, sixth[1].Arm)
	}
	if len(sixth[0].Children) != 0 {
		t.Fatalf("expected traversal 5 to have no nested blocks under outer arm 1, got %+v", sixth[0].Children)
	}
}

func TestDynamicQueryExpandTraversalNested(t *testing.T) {
	query := mustParseDynamicQueryForArms(t, dynamicArmSQLHeuristicNested)
	traversal := &DynamicArmTraversalTree{Children: []*DynamicArmTraversal{
		{
			Arm:      0,
			Children: []*DynamicArmTraversal{{Arm: 1}},
		},
		{Arm: 0},
	}}

	t.Logf("traversal:\n%s", traversal)

	staticQuery, err := query.ExpandTraversal(traversal)
	if err != nil {
		t.Fatalf("ExpandTraversal returned error: %v", err)
	}

	t.Logf("static query:\n%s", staticQuery)

	if staticQuery != "SELECT 1\nacd\nf\n" {
		t.Fatalf("unexpected static query: %q", staticQuery)
	}
}

func TestDynamicQueryWeakStaticQueries(t *testing.T) {
	query := mustParseDynamicQueryForArms(t, dynamicArmSQLFallback)

	queries, err := query.WeakStaticQueries()
	if err != nil {
		t.Fatalf("WeakStaticQueries returned error: %v", err)
	}

	for i, staticQuery := range queries {
		t.Logf("static query[%d]:\n%s", i, staticQuery)
	}

	assertStaticQueries(t, queries, []string{
		"SELECT 1\na\nc\n",
		"SELECT 1\nb\nc\n",
		"SELECT 1\na\nd\n",
		"SELECT 1\na\ne\n",
	})
}

func TestDynamicQueryHeuristicStaticQueries(t *testing.T) {
	query := mustParseDynamicQueryForArms(t, dynamicArmSQLFallback)

	queries, err := query.HeuristicStaticQueries()
	if err != nil {
		t.Fatalf("HeuristicStaticQueries returned error: %v", err)
	}

	for i, staticQuery := range queries {
		t.Logf("static query[%d]:\n%s", i, staticQuery)
	}

	assertStaticQueries(t, queries, []string{
		"SELECT 1\na\nc\n",
		"SELECT 1\nb\nd\n",
		"SELECT 1\na\ne\n",
		"SELECT 1\na\nd\n",
		"SELECT 1\nb\nc\n",
		"SELECT 1\nb\ne\n",
	})
}

func TestDynamicQueryExhaustiveStaticQueries(t *testing.T) {
	query := mustParseDynamicQueryForArms(t, dynamicArmSQLHeuristicNested)

	queries, err := query.ExhaustiveStaticQueries()
	if err != nil {
		t.Fatalf("ExhaustiveStaticQueries returned error: %v", err)
	}

	for i, staticQuery := range queries {
		t.Logf("static query[%d]:\n%s", i, staticQuery)
	}

	assertStaticQueries(t, queries, []string{
		"SELECT 1\nabd\nf\n",
		"SELECT 1\nabd\ng\n",
		"SELECT 1\nacd\nf\n",
		"SELECT 1\nacd\ng\n",
		"SELECT 1\ne\nf\n",
		"SELECT 1\ne\ng\n",
	})
}

func TestDynamicQueryExpandTraversalsDedup(t *testing.T) {
	query := mustParseDynamicQueryForArms(t, dynamicArmSQLDedup)

	queries, err := query.ExpandTraversals(query.ArmTree().ExhaustiveTraversals())
	if err != nil {
		t.Fatalf("ExpandTraversals returned error: %v", err)
	}

	for i, staticQuery := range queries {
		t.Logf("static query[%d]:\n%s", i, staticQuery)
	}

	assertStaticQueries(t, queries, []string{"SELECT 1"})
}
