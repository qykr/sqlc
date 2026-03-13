# Dynamic control syntax (proposed)

This document records the feature contract for a proposed dynamic SQL control
syntax. It is a design note for implementation work and does not mean the
feature is available in released versions of `sqlc`.

## Syntax

Dynamic control blocks use bracket directives embedded in a named query:

```sql
-- name: ListProfiles :many
SELECT *
FROM profiles
[[if is_admin]]
  WHERE salary > 0
[[else]]
  WHERE public_profile = true
[[endif]]
ORDER BY created_at DESC;
```

Supported directives:

- `[[if cond]]`
- `[[elif cond]]`
- `[[else]]`
- `[[endif]]`

True if parameter is not null:

- `[[if sqlc.narg(param)]]`
- `[[elif sqlc.narg(param)]]`

Supports logic:

- `[[if A && (!B && @C)]]`

## Follow-on proposal after basic `if` blocks

After the initial `[[if]]` / `[[elif]]` / `[[else]]` / `[[endif]]`
implementation is complete, the control language may be extended with typed
comparators and `match` dispatch.

### Comparator examples

```sql
[[if @sort == "name"]]
[[if @limit > 10]]
[[if @mode != "public"]]
```

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

### `match` example

```sql
[[match @sort]]
[[case "name"]]
  ORDER BY name
[[case "price"]]
  ORDER BY price
[[default]]
  ORDER BY created_at
[[endmatch]]
```

Also allowed:

```sql
[[match color]]
[[case "red"]]
  ORDER BY priority
[[default]]
  ORDER BY created_at
[[endmatch]]
```

Proposed directives for this later phase:

- `[[match expr]]`
- `[[case value]]`
- `[[default]]`
- `[[endmatch]]`

`match` is a typed multi-way branch over a control expression.

- Each `[[case]]` value must be type-compatible with the matched expression.
- `[[default]]` is optional and acts as the fallback branch.
- The selected branch contributes a raw SQL fragment using the same expansion
  model as `[[if]]` blocks.
- `[[match @sort]]` matches against an existing control expression.
- `[[match color]]` is also valid and introduces a new control parameter named
  `color`.

## Semantics

- Dynamic control is a **pre-parse raw SQL expansion** feature.
- The dialect parser must only ever see concrete SQL after all dynamic control
  directives have been expanded away.
- Branch bodies are raw SQL fragments and may appear in arbitrary fragment
  position inside a query.
- `[[if]]` begins a control block.
- `[[elif]]` adds another conditional branch to the same block.
- `[[else]]` provides the fallback branch.
- `[[endif]]` terminates the block.

The selected branch is determined by a named control such as `is_admin`.
Those controls are distinct from ordinary SQL parameters: they decide which SQL
fragment is present in the final query text rather than supplying a placeholder
value to the database. Some directives may also introduce a new control
parameter directly, for example `[[match color]]`.

## Structural rules

- Every `[[if]]` must have a matching `[[endif]]`.
- `[[elif]]` and `[[else]]` are only valid inside an open `[[if]]` block.
- A block may contain zero or more `[[elif]]` clauses.
- A block may contain at most one `[[else]]` clause.
- If `[[else]]` is present, it must come after all `[[elif]]` clauses.
- Every `[[match]]` must have a matching `[[endmatch]]`.
- `[[case]]` and `[[default]]` are only valid inside an open `[[match]]`
  block.
- A `[[match]]` block may contain zero or more `[[case]]` clauses.
- A `[[match]]` block may contain at most one `[[default]]` clause.
- If `[[default]]` is present, it must come after all `[[case]]` clauses.
- Nesting is allowed unless a later implementation decision explicitly removes
  it; nested blocks must still be properly balanced.

## Validation model

Each query induces multiple concrete SQL variants through a pre-parsing layer.

Control expressions must be validated before variant expansion.

- Comparator expressions must be type-compatible on both sides.
- Comparator expressions must use an operator supported by the resolved operand
  type.
- `match` expressions and all of their `case` values must agree on type.

- Validation mode is selected with `@sqlc-dynamic-check`.
- The default mode is `heuristic`.
  - `heuristic` checks a small constant-size mask set aimed at catching common
    syntax-shape issues, using masks `10101010`, `01010101`, `00000000`, and `11111111`.
- `weak-heuristic` checks only the all-disabled and all-enabled masks,
  `00000` and `111111`.
  - This covers almost all type checking, but does not try to catch syntax
    issues such as dangling `AND`.
- `exhaustive` validates all branch combinations with O(2^n) complexity.

Each generated concrete variant is then processed through the normal `sqlc`
pipeline: parsing, validation, parameter rewriting, analysis, and codegen.

## Invariants

Accepted variants must preserve one stable generated API.

At minimum, all accepted variants must agree on:

- query name and command kind
- result column count and order
- result column types and nullability, modulo any existing normalization rules
- statement class and command semantics relevant to codegen
- any structural facts required for stable code generation, such as insert/copy
  targets when those affect emitted APIs

If variants disagree in a way that would change the generated method contract,
the query must be rejected.

## Deferred decisions

The following are intentionally not locked down by this document:

- the exact heuristic mask family beyond the starter sets above
- the final generated API shape for control booleans
- parameter unioning for params that appear only in some branches
- the protobuf/codegen representation used to carry dynamic-control metadata

Those decisions belong to later implementation tasks.
