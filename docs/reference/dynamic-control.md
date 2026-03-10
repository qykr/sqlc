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

- `[[if @param]]`
- `[[elif @param]]`

Supports logic:

- `[[if A && (!B && @C)]]`

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
value to the database.

## Structural rules

- Every `[[if]]` must have a matching `[[endif]]`.
- `[[elif]]` and `[[else]]` are only valid inside an open `[[if]]` block.
- A block may contain zero or more `[[elif]]` clauses.
- A block may contain at most one `[[else]]` clause.
- If `[[else]]` is present, it must come after all `[[elif]]` clauses.
- Nesting is allowed unless a later implementation decision explicitly removes
  it; nested blocks must still be properly balanced.

## Validation model

Each query induces multiple concrete SQL variants through a pre-parsing layer.

- The default validation mode is exhaustive validation across all branch
  combinations with O(2^n) complexity.
- A separate query-level opt-in flag may allow a constant-size heuristic mask
  set instead of exhaustive enumeration.
  - For example, if you have 8 if clauses, you would only need to check these: `00000000`, `11111111`, `10101010`, `01010101`. The majority of the errors likely come from dangling keywords like `AND`. The type checking should also be very accurate with just this.

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

- the exact query annotation that enables heuristic validation
- the final generated API shape for control booleans
- parameter unioning for params that appear only in some branches
- the protobuf/codegen representation used to carry dynamic-control metadata

Those decisions belong to later implementation tasks.
