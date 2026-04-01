package main

import (
	"fmt"
)

// Result captures the pairs, heads, and tails found during a traversal step
type Result struct {
	Pairs [][2]*Leaf
	Heads []*Leaf
	Tails []*Leaf
}

// Node interfaces and types
type Node interface {
	isNode()
}

type Leaf struct {
	SQL string
}
func (l *Leaf) isNode() {}

type Arm struct {
	Name     string
	Children []Node
}
func (a *Arm) isNode() {}

type Block struct {
	Name string
	Arms []*Arm
}
func (b *Block) isNode() {}

type DynamicQuery struct {
	Name     string
	Children []Node
}
func (d *DynamicQuery) isNode() {}

// findHeuristicPairs is the public entry point
func (d *DynamicQuery) findHeuristicPairs() [][2]*Leaf {
	res := findHeuristicPairsImpl(d)
	return res.Pairs
}

func findHeuristicPairsImpl(node Node) Result {
	switch n := node.(type) {
	case *Leaf:
		return Result{
			Heads: []*Leaf{n},
			Tails: []*Leaf{n},
		}

	case *Block:
		var res Result
		for _, arm := range n.Arms {
			data := findHeuristicPairsImpl(arm)
			res.Pairs = append(res.Pairs, data.Pairs...)
			res.Heads = append(res.Heads, data.Heads...)
			res.Tails = append(res.Tails, data.Tails...)
		}
		return res

	case *Arm, *DynamicQuery:
		var children []Node
		if a, ok := n.(*Arm); ok {
			children = a.Children
		} else if d, ok := n.(*DynamicQuery); ok {
			children = d.Children
		}

		if len(children) == 0 {
			return Result{}
		}

		var res Result
		results := make([]Result, 0, len(children))

		// First Pass: Recurse and collect internal results
		for _, child := range children {
			childRes := findHeuristicPairsImpl(child)
			results = append(results, childRes)
			res.Pairs = append(res.Pairs, childRes.Pairs...)
		}

		// Boundary Logic
		res.Heads = results[0].Heads
		res.Tails = results[len(results)-1].Tails

		// Second Pass: Handshake between adjacent children
		for i := 1; i < len(results); i++ {
			prevTails := results[i-1].Tails
			currHeads := results[i].Heads
			
			// Cartesian product
			for _, t := range prevTails {
				for _, h := range currHeads {
					res.Pairs = append(res.Pairs, [2]*Leaf{t, h})
				}
			}
		}
		return res
	}
	return Result{}
}

func main() {
	tree := &DynamicQuery{
		Name: "ExampleQuery",
		Children: []Node{
			&Block{
				Name: "A",
				Arms: []*Arm{
					{Name: "A.1", Children: []Node{&Leaf{"l1"}, &Leaf{"l2"}}},
					{Name: "A.2", Children: []Node{
						&Leaf{"l3"},
						&Block{
							Name: "A.2.B",
							Arms: []*Arm{
								{Name: "A.2.B.1", Children: []Node{&Leaf{"l4"}, &Leaf{"l5"}}},
								{Name: "A.2.B.2", Children: []Node{&Leaf{"l6"}, &Leaf{"l7"}}},
							},
						},
					}},
				},
			},
			&Block{
				Name: "B",
				Arms: []*Arm{
					{Name: "B.1", Children: []Node{&Leaf{"l8"}, &Leaf{"l9"}}},
					{Name: "B.2", Children: []Node{&Leaf{"l10"}, &Leaf{"l11"}}},
				},
			},
		},
	}

	pairs := tree.findHeuristicPairs()
	fmt.Printf("Found %d heuristic pairs:\n", len(pairs))
	for _, p := range pairs {
		fmt.Printf("  %s <-> %s\n", p[0].SQL, p[1].SQL)
	}
}
