# Dynamic control syntax (proposed)

This document records the feature contract for a proposed dynamic SQL control syntax. It is a design note for implementation work and does not mean the feature is available in released versions of `sqlc`.

## Syntax

Dynamic control blocks use bracket directives embedded in a named query:

```sql
-- name: ListProfiles :many
SELECT *
FROM profiles
[[IF @is_admin]]
  WHERE salary > 0
[[ELSE]]
  WHERE public_profile = true
[[END]]
ORDER BY created_at DESC;
```

Supported directives:

- `[[IF @param]]`
- `[[ELIF @param]]`
- `[[ELSE]]`
- `[[END]]`

Only allowed only to check nullable parameters are null:

- ✅: `[[IF sqlc.narg(param)]]`
- ✅: `[[ELIF sqlc.narg(param)]]`
- ❌: `[[IF sqlc.narg(param) > 4]]`
  - This may be [supported via 3VL](#nullable-parameters) in the future

Supports logic:

- `[[IF @A AND (!@B OR @C)]]`

Comparators:

- `[[IF @sort == "name"]]`
- `[[IF @limit > 10]]`
- `[[IF @mode != "public"]]`

Proposed comparator operators:

- `==`
- `!=`
- `<`
- `<=`
- `>`
- `>=`

These comparisons should be statically type checked before expansion.

- Both operands must have compatible types.
- If the operand types are compatible, the operator itself must also be valid
  for that type.
- For example, equality operators may be valid for strings, but ordered
  comparisons such as `<`, `<=`, `>`, and `>=` should be rejected for string
  controls.
- Type mismatches such as comparing a numeric control with a string literal
  should be rejected.
  
## Match

After the initial `[[IF]]` / `[[ELIF]]` / `[[ELSE]]` / `[[END]]`
implementation is complete, the control language may be extended with `match` dispatch.

### `match` example

```sql
[[MATCH sqlc.narg(sort)]]
[[CASE NULL]]
  ORDER BY size
[[CASE "name"]]
  ORDER BY name
[[CASE "price"]]
  ORDER BY price
[[DEFAULT]]
  ORDER BY created_at
[[END]]
```

Also allowed:

```sql
[[MATCH @color]]
[[CASE "red"]]
  ORDER BY priority
[[DEFAULT]]
  ORDER BY created_at
[[END]]
```

Proposed directives for this later phase:

- `[[MATCH @expr]]`
- `[[CASE @value]]`
- `[[DEFAULT]]`
- `[[END]]`

`match` is a typed multi-way branch over a control expression.

- Each `[[CASE]]` value must be type-compatible with the matched expression.
- `[[DEFAULT]]` is optional and acts as the fallback branch.
- The selected branch contributes a raw SQL fragment using the same expansion
  model as `[[IF]]` blocks.
- `[[MATCH @sort]]` matches against an expression.

## Semantics

- Dynamic control is a **pre-parse raw SQL expansion** feature.
- The dialect parser must only ever see concrete SQL after all dynamic control
  directives have been expanded away.
- Branch bodies are raw SQL fragments and may appear in arbitrary fragment
  position inside a query.
- `[[IF]]` begins a control block.
- `[[ELIF]]` adds another conditional branch to the same block.
- `[[ELSE]]` provides the fallback branch.
- `[[END]]` terminates the block.

The selected branch is determined by a named control such as `@is_admin`. Some directives may also introduce a new control parameter directly, for example `[[MATCH @color]]`.

## Structural rules

### If
- Every `[[IF]]` must have a matching `[[END]]`.
- `[[ELIF]]` and `[[ELSE]]` are only valid inside an open `[[IF]]` block.
- A block may contain zero or more `[[ELIF]]` clauses.
- A block may contain at most one `[[ELSE]]` clause.
- If `[[ELSE]]` is present, it must come after all `[[ELIF]]` clauses.

### Match
- Every `[[MATCH]]` must have a matching `[[END]]`.
- `[[CASE]]` and `[[DEFAULT]]` are only valid inside an open `[[MATCH]]`
  block.
- A `[[MATCH]]` block may contain zero or more `[[CASE]]` clauses.
- A `[[MATCH]]` block may contain at most one `[[DEFAULT]]` clause.
- If `[[DEFAULT]]` is present, it must come after all `[[CASE]]` clauses.

## Validation model

Each query induces multiple concrete SQL variants through a pre-parsing layer. Control expressions must be validated before variant expansion:

* Comparator expressions must be type-compatible on both sides.
* Comparator expressions must use an operator supported by the resolved operand type.
* `MATCH` expressions and all of their `CASE` values must agree on type.

Validation mode is selected with `@sqlc-dynamic-check`.

### Structural Definitions
To define the validation modes, we define the structure of the AST (Abstract Syntax Tree):
* **Leaf:** A raw SQL fragment.
* **Dynamic Block:** An `IF` or `MATCH` directive. A Block contains one or more **Arms**.
* **Arm:** A single branch within a Block (e.g., an `IF`, `ELIF`, `ELSE`, `CASE`, or `DEFAULT` clause). An Arm contains its own **Sequence** of Parts.
* **Part:** The smallest unit of syntax. It is either a **Leaf** or a **Dynamic Block**.
* **Sequence:** A list of adjacent Parts executed in order. A full query is a Sequence.

An `IF` block without an `ELSE` still has an implicit "Arm 0" containing an empty Sequence.

### Validation Modes

#### 1. `heuristic` (Default Mode)
The `heuristic` mode catches syntax-shape issues by validating the boundary between every pair of adjacent parts across all possible logic paths, without generating the exponentially large set of full query permutations.

For every Leaf in the AST, `sqlc` finds the immediately preceding Part. `sqlc` must check the current Leaf against every possible **Tail** Leaf (exit point) of that Block.

**Finding the "Immediate Predecessor"**
To find the predecessor(s) of a Leaf `P`:
* Keep going up to the parent Block until that Block is no longer the first in the sequence.
* The part immediately before is the "immediate predecessor."
* Note that some Leaves do not have an immediate predecessor.

**The Tail Resolution Algorithm:**
To find the set of Tails for any Part:
1.  **If the Part is a Raw SQL Fragment:** Its Tail is simply itself.
2.  **If the Part is a Dynamic Block:** Its Tails are the union of the Tails of all its Arms.
3.  **The Tail of an Arm:** is simply the Tail of the *last Part* in its Sequence.

**Example Traversal:**
Consider a Sequence where a Block `C` is immediately followed by Block `D`.
```text
[Sequence]
 ┌─ Block A
 │   ├─ Arm A.1
 │   │   └─ [Sequence]
 │   │       ├─ Block A.1.A ...
 │   │       └─ Leaf (1)
 │   └─ Arm A.2
 │       └─ [Sequence]
 │           ├─ Leaf (2)
 │           └─ Block A.2.A
 │               ├─ Arm A.2.A.1
 │               │   ├─ ...
 │               │   └─ Leaf (3)
 │               └─ Arm A.2.A.2
 │                   ├─ ...
 │                   └─ Leaf (4)
 └─ Block B
     ├─ Arm B.1
     │   └─ [Sequence]
     │       ├─ Leaf (5)
     │       └─ ...
     └─ Arm B.2
         └─ [Sequence]
             ├─ Leaf (6)
             └─ ...
```

To validate **Leaf (5)**, `sqlc` looks at the immediately preceding Part: Block `A`.
Using the algorithm to find the Tails of Block `A`:
* The last Part in Arm 1 is **Leaf (1)**.
* The last Part in Arm 2 is **Block A.2.A**. The Tails of Block `A.2.A` are the tails of its arms: **Leaf (3)** and **Leaf (4)**.

Therefore, the Tails of Block `A` are `{1, 3, 4}`.
The `heuristic` mode will check the syntax of:
* `Leaf (1)` immediately followed by `Leaf (5)`
* `Leaf (3)` immediately followed by `Leaf (5)`
* `Leaf (4)` immediately followed by `Leaf (5)`

As well as these following the same method:
* `Leaf (2)` immediately followed by `Leaf (3)`
* `Leaf (2)` immediately followed by `Leaf (4)`
* `Leaf (1)` immediately followed by `Leaf (6)`
* `Leaf (3)` immediately followed by `Leaf (6)`
* `Leaf (4)` immediately followed by `Leaf (6)`
* **Complexity:**
  * **Lower bound:** $\Omega(L + \sum A_i)$
  * **Worst case:** $O(NA^2)$
  * $A$ is the average number of arms per branch.
  * $L$ is the number of leaves in the entire query.
  * These have not been proven rigorously

#### 2. `weak-heuristic`
Checks each Block Arm in isolation to cover type-checking, but does not exhaustively catch inter-block syntax errors.
* For each Block and each of its Arms, `sqlc` constructs one traversal that selects that Arm.
* Ancestor Blocks are set to whatever Arms are required to reach the targeted Block.
* All other Blocks are held at Arm 0 (empty or default).
* **Complexity:** $O(\sum a_i)$ where Block $i$ has $a_i$ Arms.

#### 3. `exhaustive`
Validates every possible Arm assignment combination across every Dynamic Block simultaneously.
* Generates every conceivable concrete query variant.
* **Complexity:** $O(\prod a_i)$ where Block $i$ has $a_i$ Arms.

## Invariants

Accepted variants must preserve one stable generated API.

At minimum, all accepted variants must agree on:

- query name and command kind
- result column count and order
- result column types and nullability, modulo any existing normalization rules
- statement class and command semantics relevant to codegen
- any structural facts required for stable code generation, such as insert/copy
  targets when those affect emitted APIs

If variants disagree in a way that would change the generated method contract, the query must be rejected.

## Nullable Parameters

It's hard to know what these mean if the parameters are null:

- `[[IF sqlc.narg(param) > 4]]`
- `[[IF sqlc.narg(param1) > 10 AND sqlc.narg(param2) < 20]]`

If this is ever implemented in the future, it will follow [SQL's three-valued logic (3VL)](https://www.red-gate.com/simple-talk/databases/sql-server/learn/sql-and-the-snare-of-three-valued-logic/). The generator must output extra code and data types to handle this.

## Type checking

The type checking follows these steps:
1. Parameters will be automatically inferred from the SQL schema as usual.
2. Parameters that only appear in dynamic control will be int, string, or bool
3. SQL parameters that also appear in dynamic control must be compatible with the expressions it is in.
