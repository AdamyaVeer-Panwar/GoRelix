# GoRelix

GoRelix is a relational database engine built from first principles in Go.

## Goals

- Understand and implement database internals.
- Provide durable, page-based storage.
- Build a query execution engine and a documented SQL subset.
- Implement indexing, transactions, and crash recovery incrementally.
- Prioritize correctness, testability, and explicit engineering contracts.

## Current Status

Initial repository setup.

Database storage, SQL execution, indexing, and transaction
management are not implemented yet.

## Development

Run the test suite:

    go test ./...

Run static analysis:

    go vet ./...

Build the executable:

    go build -o bin/gorelix ./cmd/gorelix

## Engineering Principles

- Correctness before optimization.
- Explicit on-disk formats instead of raw Go struct layouts.
- Tests for behavioral contracts and failure conditions.
- Small, reviewable changes.
- Dependencies introduced only when justified.
- Document significant architectural decisions.

## Initial Milestone

Implement a disk manager that can create or open a database file,
read and write fixed-size pages, and preserve page contents across
closing and reopening the file.