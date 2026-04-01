from __future__ import annotations
from itertools import product
from typing import List, Union
from dataclasses import dataclass, field

@dataclass
class Node:
    parent: Node | None = None
    pos: int | None = None # order in parent
    children: List[Node] = field(default_factory=list)

class Leaf(Node):
    def __init__(self, sql: str):
        self.sql = sql
        self.name = self.sql
    
    def __repr__(self):
        return f"Leaf(\"{self.sql}\")"
    
class Block(Node):    
    def __init__(self, name: str, arms: List[Arm]):
        super().__init__(children=arms)
        self.name = name
        for i, arm in enumerate(arms):
            arm.parent = self
            arm.pos = i

Part = Union[Leaf, Block]

class Arm(Node):    
    def __init__(self, name: str, parts: List[Part]):
        super().__init__(children=parts)
        self.name = name
        for i, part in enumerate(parts):
            part.parent = self
            part.pos = i
    
class DynamicQuery(Node):
    def __init__(self, name: str, parts: List[Part]):
        super().__init__(children=parts)
        self.name = name
        for i, part in enumerate(parts):
            part.parent = self
            part.pos = i
            
    def find_heuristic_pairs(self):
        return self._find_heuristic_pairs_impl(self)[0]
        
    def _find_heuristic_pairs_impl(self, node: Node):
        """Returns pairs, heads, tails"""
        match node:
            case Leaf():
                return [], [node], [node]
            
            case Block():
                pairs = []
                heads = []
                tails = []
                for arm in node.children:
                    data = self._find_heuristic_pairs_impl(arm)
                    pairs.extend(data[0])
                    heads.extend(data[1])
                    tails.extend(data[2])
                    
                return pairs, heads, tails
            
            case Arm() | DynamicQuery():
                pairs = []
                heads = []
                tails = []
                results = []
                
                for i, part in enumerate(node.children):
                    result = self._find_heuristic_pairs_impl(part)
                    results.append(result)
                    pairs.extend(result[0])
                
                heads.extend(results[0][1])
                tails.extend(results[-1][2])
                
                for i in range(1, len(node.children)):
                    pairs.extend(product(results[i-1][2], results[i][1]))
                
                return pairs, heads, tails

tree = DynamicQuery("ExampleQuery", [
    Block("A", [
        Arm("A.1", [
            Leaf("l1"),
            Leaf("l2")
        ]),
        Arm("A.2", [
            Leaf("l3"),
            Block("A.2.B", [
                Arm("A.2.B.1", [
                    Leaf("l4"),
                    Leaf("l5")
                ]),
                Arm("A.2.B.2", [
                    Leaf("l6"),
                    Leaf("l7")
                ])
            ])
        ])
    ]),
    Block("B", [
        Arm("B.1", [
            Leaf("l8"),
            Leaf("l9")
        ]),
        Arm("B.2", [
            Leaf("l10"),
            Leaf("l11")
        ])
    ])
])

pairs = tree.find_heuristic_pairs()
print(f"Found {len(pairs)} heuristic pairs:")
for pair in pairs:
    print(f"  {pair[0].sql} <-> {pair[1].sql}")
