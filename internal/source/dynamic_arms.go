package source

import (
	"fmt"
	"strings"
)

// ArmTree converts a parsed dynamic query into a recursive arm-count tree.
func (q DynamicQuery) ArmTree() *DynamicArmSkeleton {
	return dynamicArmTreeFromParts(q.Parts)
}

// WeakTraversals returns the weak-heuristic traversals for this arm tree.
//
// Each traversal targets one block arm, sets the ancestor path needed to reach
// that block, and holds every other block at arm 0.
func (t *DynamicArmSkeleton) WeakTraversals() []*DynamicArmTraversalTree {
	var targets []dynamicArmTraversalTarget
	t.collectWeakTraversalTargets(&targets)
	if len(targets) == 0 {
		return []*DynamicArmTraversalTree{{}}
	}

	traversals := make([]*DynamicArmTraversalTree, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		appendUniqueDynamicArmTraversal(&traversals, seen, &DynamicArmTraversalTree{
			Children: t.weakTraversalAtBlocksLevel(target),
		})
	}
	return traversals
}

// HeuristicTraversals returns the heuristic traversals for this arm tree.
//
// Each traversal targets one blocks level at a time, applies either a uniform
// arm assignment or an alternating arm pair at that level, chooses ancestor
// arms as needed to reach the targeted level, and holds all unrelated blocks at
// arm 0.
func (t *DynamicArmSkeleton) HeuristicTraversals() []*DynamicArmTraversalTree {
	var targets []dynamicArmPatternTraversalTarget
	t.collectHeuristicTraversalTargets(&targets)
	if len(targets) == 0 {
		return []*DynamicArmTraversalTree{{}}
	}

	traversals := make([]*DynamicArmTraversalTree, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		appendUniqueDynamicArmTraversal(&traversals, seen, &DynamicArmTraversalTree{
			Children: t.heuristicTraversalAtBlocksLevel(target),
		})
	}
	return traversals
}

// ExhaustiveTraversals returns every reachable arm assignment across the full
// dynamic arm tree.
func (t *DynamicArmSkeleton) ExhaustiveTraversals() []*DynamicArmTraversalTree {
	combinations := t.exhaustiveTraversalCombinationsAtBlocksLevel()
	traversals := make([]*DynamicArmTraversalTree, 0, len(combinations))
	for _, blocks := range combinations {
		traversals = append(traversals, &DynamicArmTraversalTree{Children: blocks})
	}
	return traversals
}

type dynamicArmTraversalTarget struct {
	block *DynamicArmSkeleton
	arm   int
}

type dynamicArmPatternTraversalTarget struct {
	level *DynamicArmSkeleton
	arms  map[*DynamicArmSkeleton]int
}

func dynamicArmTreeFromParts(parts []DynamicPart) *DynamicArmSkeleton {
	tree := &DynamicArmSkeleton{}
	for _, part := range parts {
		if part.If == nil {
			continue
		}
		tree.Children = append(tree.Children, part.If.armTree())
	}
	return tree
}

func (b *DynamicIfBlock) armTree() *DynamicArmSkeleton {
	tree := &DynamicArmSkeleton{}
	for _, arm := range b.Arms {
		tree.Children = append(tree.Children, dynamicArmTreeFromParts(arm.Parts))
	}
	return tree
}

func (t *DynamicArmSkeleton) collectWeakTraversalTargets(targets *[]dynamicArmTraversalTarget) {
	if t == nil {
		return
	}

	for _, block := range t.Children {
		for arm := range block.Children {
			*targets = append(*targets, dynamicArmTraversalTarget{block: block, arm: arm})
		}
		for _, armNode := range block.Children {
			armNode.collectWeakTraversalTargets(targets)
		}
	}
}

func (t *DynamicArmSkeleton) collectHeuristicTraversalTargets(targets *[]dynamicArmPatternTraversalTarget) {
	if t == nil {
		return
	}

	for _, pattern := range t.heuristicPatternsAtBlocksLevel() {
		arms := make(map[*DynamicArmSkeleton]int, len(t.Children))
		for i, block := range t.Children {
			arms[block] = pattern[i]
		}
		*targets = append(*targets, dynamicArmPatternTraversalTarget{
			level: t,
			arms:  arms,
		})
	}

	for _, block := range t.Children {
		for _, armNode := range block.Children {
			armNode.collectHeuristicTraversalTargets(targets)
		}
	}
}

func (t *DynamicArmSkeleton) heuristicPatternsAtBlocksLevel() [][]int {
	if t == nil || len(t.Children) == 0 {
		return nil
	}

	maxArms := t.maxArmCountAtBlocksLevel()
	patterns := make([][]int, 0, maxArms)
	for arm := range maxArms {
		pattern := make([]int, len(t.Children))
		for i := range pattern {
			pattern[i] = arm
		}
		patterns = append(patterns, pattern)
	}

	if len(t.Children) < 2 {
		return patterns
	}

	for left := range maxArms {
		for right := left + 1; right < maxArms; right++ {
			patterns = append(patterns,
				alternatingArmPattern(len(t.Children), left, right),
				alternatingArmPattern(len(t.Children), right, left),
			)
		}
	}

	return patterns
}

func (t *DynamicArmSkeleton) maxArmCountAtBlocksLevel() int {
	maxArms := 0
	for _, block := range t.Children {
		if len(block.Children) > maxArms {
			maxArms = len(block.Children)
		}
	}
	return maxArms
}

func alternatingArmPattern(width, first, second int) []int {
	pattern := make([]int, width)
	for i := range pattern {
		if i%2 == 0 {
			pattern[i] = first
			continue
		}
		pattern[i] = second
	}
	return pattern
}

func (t *DynamicArmSkeleton) exhaustiveTraversalCombinationsAtBlocksLevel() [][]*DynamicArmTraversal {
	if t == nil || len(t.Children) == 0 {
		return [][]*DynamicArmTraversal{{}}
	}

	combinations := [][]*DynamicArmTraversal{{}}
	for _, block := range t.Children {
		combinations = crossJoinDynamicArmTraversals(combinations, block.exhaustiveTraversalsForBlock())
	}
	return combinations
}

func (t *DynamicArmSkeleton) exhaustiveTraversalsForBlock() []*DynamicArmTraversal {
	if t == nil || len(t.Children) == 0 {
		return nil
	}

	traversals := make([]*DynamicArmTraversal, 0, len(t.Children))
	for arm, armNode := range t.Children {
		for _, nested := range armNode.exhaustiveTraversalCombinationsAtBlocksLevel() {
			traversals = append(traversals, &DynamicArmTraversal{
				Arm:      arm,
				Children: nested,
			})
		}
	}
	return traversals
}

func crossJoinDynamicArmTraversals(prefixes [][]*DynamicArmTraversal, suffixes []*DynamicArmTraversal) [][]*DynamicArmTraversal {
	if len(prefixes) == 0 || len(suffixes) == 0 {
		return nil
	}

	joined := make([][]*DynamicArmTraversal, 0, len(prefixes)*len(suffixes))
	for _, prefix := range prefixes {
		for _, suffix := range suffixes {
			combination := make([]*DynamicArmTraversal, len(prefix), len(prefix)+1)
			copy(combination, prefix)
			combination = append(combination, suffix)
			joined = append(joined, combination)
		}
	}
	return joined
}

func (t *DynamicArmSkeleton) weakTraversalAtBlocksLevel(target dynamicArmTraversalTarget) []*DynamicArmTraversal {
	if t == nil {
		return nil
	}

	traversals := make([]*DynamicArmTraversal, 0, len(t.Children))
	for _, block := range t.Children {
		traversals = append(traversals, block.weakTraversalForBlock(target))
	}
	return traversals
}

func (t *DynamicArmSkeleton) weakTraversalForBlock(target dynamicArmTraversalTarget) *DynamicArmTraversal {
	selectedArm := t.selectedArmForWeakTraversal(target)

	traversal := &DynamicArmTraversal{Arm: selectedArm}
	if len(t.Children) == 0 {
		return traversal
	}

	traversal.Children = t.Children[selectedArm].weakTraversalAtBlocksLevel(target)
	return traversal
}

func (t *DynamicArmSkeleton) heuristicTraversalAtBlocksLevel(target dynamicArmPatternTraversalTarget) []*DynamicArmTraversal {
	if t == nil {
		return nil
	}

	traversals := make([]*DynamicArmTraversal, 0, len(t.Children))
	for _, block := range t.Children {
		traversals = append(traversals, block.heuristicTraversalForBlock(target))
	}
	return traversals
}

func (t *DynamicArmSkeleton) heuristicTraversalForBlock(target dynamicArmPatternTraversalTarget) *DynamicArmTraversal {
	selectedArm := t.selectedArmForHeuristicTraversal(target)

	traversal := &DynamicArmTraversal{Arm: selectedArm}
	if len(t.Children) == 0 {
		return traversal
	}

	traversal.Children = t.Children[selectedArm].heuristicTraversalAtBlocksLevel(target)
	return traversal
}

func (t *DynamicArmSkeleton) selectedArmForWeakTraversal(target dynamicArmTraversalTarget) int {
	if len(t.Children) == 0 {
		return 0
	}
	if t == target.block {
		if target.arm < len(t.Children) {
			return target.arm
		}
		return 0
	}

	for arm, armNode := range t.Children {
		if armNode.containsBlock(target.block) {
			return arm
		}
	}
	return 0
}

func (t *DynamicArmSkeleton) selectedArmForHeuristicTraversal(target dynamicArmPatternTraversalTarget) int {
	if len(t.Children) == 0 {
		return 0
	}
	if arm, ok := target.arms[t]; ok {
		if arm < len(t.Children) {
			return arm
		}
		return 0
	}

	for arm, armNode := range t.Children {
		if armNode.containsBlock(target.level) {
			return arm
		}
	}
	return 0
}

func (t *DynamicArmSkeleton) containsBlock(target *DynamicArmSkeleton) bool {
	if t == nil {
		return false
	}
	if t == target {
		return true
	}
	for _, child := range t.Children {
		if child.containsBlock(target) {
			return true
		}
	}
	return false
}

func (t *DynamicArmTraversalTree) signature() string {
	var b strings.Builder
	writeDynamicArmTraversalSignature(&b, t.Children)
	return b.String()
}

func appendUniqueDynamicArmTraversal(traversals *[]*DynamicArmTraversalTree, seen map[string]struct{}, traversal *DynamicArmTraversalTree) {
	signature := traversal.signature()
	if _, ok := seen[signature]; ok {
		return
	}
	seen[signature] = struct{}{}
	*traversals = append(*traversals, traversal)
}

func writeDynamicArmTraversalSignature(b *strings.Builder, blocks []*DynamicArmTraversal) {
	b.WriteByte('[')
	for _, block := range blocks {
		fmt.Fprintf(b, "%d", block.Arm)
		writeDynamicArmTraversalSignature(b, block.Children)
		b.WriteByte(';')
	}
	b.WriteByte(']')
}
