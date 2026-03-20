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
  - This may be [supported](#nullable-parameters) in the future

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
[[MATCH @sort]]
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

Each query induces multiple concrete SQL variants through a pre-parsing layer.

Control expressions must be validated before variant expansion.

- Comparator expressions must be type-compatible on both sides.
- Comparator expressions must use an operator supported by the resolved operand type.
- `match` expressions and all of their `case` values must agree on type.

- Validation mode is selected with `@sqlc-dynamic-check`.
- Every dynamic block is treated as a set of arms.
  - For `match`, each `case` arm and the optional `default` arm participate.
  - For `if` / `elif` / `else`, each clause is an arm.
  - An `if` block without an `else` still has an implicit arm 0 that expands to nothing.
- The default mode is `heuristic`.
  - `heuristic` checks a small constant-size set of arm assignments aimed at
    catching common syntax-shape issues.
  - For a block with arms `A`, `B`, `C`, ... this means checking the uniform assignments `AAAAAA`, `BBBBBB`, `CCCCCC`, ... and, for every pair of arms, alternating assignments such as `ABABAB` and `BABABA`.
  - Nested blocks are validated bottom-up. When validating an outer block, nested blocks that are not currently being explored are held at arm 0. That keeps the search bounded while still probing mixed-arm syntax shapes.
  - The complexity is $O(a_\text{max}^2)$, where $a_\text{max}$ is the max number of arms in a branch.
- `weak-heuristic` checks each block arm in isolation.
  - This covers almost all type checking, but does not try to catch many syntax issues.
  - For each block and each of its arms, sqlc constructs one traversal that selects that arm.
  - Ancestor blocks are set to whatever arms are required to reach the targeted block.
  - All other blocks are held at arm 0.
  - The complexity is $O(\sum a_i)$ where block $i$ has $a_i$ arms.
- `exhaustive` validates every possible arm assignment across every dynamic block.
  - The complexity is $O(\prod a_i)$ where block $i$ has $a_i$ arms.

Each generated concrete variant is then processed through the normal `sqlc` pipeline: parsing, validation, parameter rewriting, analysis, and codegen.

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
