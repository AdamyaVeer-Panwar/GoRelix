# ADR 0001: Project Foundation

- Status: Accepted
- Date: 2026-10-27

## Context

GoRelix is a single-node relational database engine developed
to establish correctness and understand database internals.

## Decisions

1. Use Go and its standard library initially.
2. Keep the executable entry point separate from database internals.
3. Keep implementation packages under internal/.
4. Implement subsystems in dependency order.
5. Define and test behavioral contracts before adding features.
6. Encode persistent data explicitly; never persist raw Go struct layouts.
7. Require tests for storage correctness and failure conditions.
8. Avoid dependencies without a concrete engineering justification.

## Initial Scope

The first database subsystem will be a disk manager.

Its contract will define page size, page addressing, file lifecycle,
read/write behavior, invalid inputs, and error propagation.

## Non-Goals

The initial storage milestone excludes SQL parsing, indexes,
transactions, write-ahead logging, recovery, and concurrency control.

These will be implemented in later milestones.

## Validation

The project must build and pass its automated tests.

The disk manager must eventually demonstrate that page contents
remain intact after closing and reopening a database file.