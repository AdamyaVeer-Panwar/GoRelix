# ADR 0002: Disk Manager and Logical Pages

- Status: Accepted
- Date: 2026-10-27

## Context

GoRelix requires a persistent storage foundation before introducing
records, indexes, query execution, and transactions.

## Decisions

1. Use a fixed logical page size of 4096 bytes.
2. Represent page identifiers using uint64.
3. Address page N at byte offset N * PageSize.
4. Represent a page as a fixed-size Go array.
5. Append new pages sequentially; do not implement page reclamation yet.
6. Reject files whose size is not a multiple of PageSize.
7. Reject reads and writes outside the existing page range.
8. Serialize DiskManager operations with a mutex.
9. Expose explicit Sync behavior and synchronize during Close.
10. Propagate storage errors to callers.

## Guarantees

- Each successfully appended page receives a distinct sequential page ID.
- Existing pages can be read and overwritten.
- Normal close and reopen can be tested for persisted page contents.
- Concurrent appends through one DiskManager are serialized.

## Limitations

- Page overwrites are not transactionally atomic.
- There is no WAL, crash recovery, or transaction isolation.
- Pages cannot be reclaimed or reused.
- The file does not yet have a versioned database header.
- Sync cannot guarantee more than the underlying OS and filesystem provide.

## Validation

Automated tests cover page reads and writes, append behavior, persistence
after reopening, invalid page IDs, malformed file lengths, lifecycle
errors, and concurrent appends.