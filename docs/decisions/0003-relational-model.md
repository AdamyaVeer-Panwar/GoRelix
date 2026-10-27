# ADR 0003: Logical Relational Model

- Status: Accepted
- Date: 2026-10-28

## Context

GoRelix needs a logical representation of columns, schemas, and rows
before records can be encoded or stored on disk.

## Decisions

1. Define the logical model in internal/relational.
2. Support INT64, FLOAT64, BOOL, and STRING.
3. Represent nullability explicitly on each column.
4. Represent NULL separately from ordinary values.
5. Preserve schema column order.
6. Reject duplicate column names.
7. Require row value count to match schema column count.
8. Require exact data-type matches; no implicit conversion.
9. Keep schema and row fields private.
10. Defensively copy caller-provided slices.
11. Reject non-finite FLOAT64 values during row validation.
12. Validate logical values independently of database-file access.

## Separation of concerns

- The relational model defines logical data and schema conformance.
- Record encoding will convert logical values into bytes in a later phase.
- The storage subsystem will read and write pages.
- Physical page IDs and byte offsets do not belong in the logical model.

## Guarantees

- Invalid schema definitions are rejected.
- Invalid rows produce explicit errors.
- Valid rows conform to their schema.
- Schema and row slices cannot be modified through returned slice references.

## Limitations

- No SQL parser.
- No table catalog or persistent schemas.
- No binary record encoding.
- No indexes, transactions, or query execution.

## Validation

Automated tests cover schema construction, column lookup, row validation,
nullability, type mismatch, invalid floating-point values, and defensive
copying.