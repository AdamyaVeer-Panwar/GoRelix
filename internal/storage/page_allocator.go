package storage

import (
	"errors"
	"fmt"
)

var (
	ErrDatabaseClosed       = errors.New("database is closed")
	ErrPageNotAllocated     = errors.New("page is not allocated")
	ErrPageAlreadyAllocated = errors.New("page is already allocated")
)

// AllocatePage allocates a new data page and persists the updated metadata.
//
// Allocation order:
// 1. Append a formatted data page.
// 2. Update the allocation bitmap.
// 3. Persist the metadata page.
// 4. Sync the disk.
//
// Crash-atomic allocation will be implemented with WAL in a later phase.
func (db *Database) AllocatePage() (PageID, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return 0, ErrDatabaseClosed
	}
	if db.failed {
		return 0, ErrDatabaseFailed
	}
	if db.metadata.AllocatedCount >= db.metadata.BitmapCapacity {
		return 0, ErrDatabaseCapacityExceeded
	}

	pageCount, err := db.disk.PageCount()
	if err != nil {
		return 0, fmt.Errorf("get page count: %w", err)
	}
	if pageCount == 0 {
		db.failed = true
		return 0, fmt.Errorf(
			"%w: metadata page is missing",
			ErrInvalidDatabaseLayout,
		)
	}
	if pageCount-1 != uint64(db.metadata.AllocatedCount) {
		db.failed = true
		return 0, fmt.Errorf(
			"%w: physical data pages=%d, allocated pages=%d",
			ErrInvalidDatabaseLayout,
			pageCount-1,
			db.metadata.AllocatedCount,
		)
	}
	if pageCount >= MaxDatabasePages {
		return 0, ErrDatabaseCapacityExceeded
	}

	id := PageID(pageCount)

	page, err := FormatPage(id, PageTypeData, 0)
	if err != nil {
		return 0, fmt.Errorf("format data page %d: %w", id, err)
	}

	appendedID, err := db.disk.AppendPage(page)
	if err != nil {
		return 0, fmt.Errorf("append data page: %w", err)
	}
	if appendedID != id {
		db.failed = true
		return 0, fmt.Errorf(
			"%w: expected page ID %d, appended page ID %d",
			ErrInvalidDatabaseLayout,
			id,
			appendedID,
		)
	}

	bitIndex := uint64(id) - 1
	byteIndex := bitIndex / 8
	bitOffset := uint(bitIndex % 8)

	if byteIndex >= uint64(len(db.metadata.Bitmap)) {
		db.failed = true
		return 0, ErrDatabaseCapacityExceeded
	}

	// Build a candidate metadata value without modifying the current state.
	candidate := db.metadata
	candidate.Bitmap = append([]byte(nil), db.metadata.Bitmap...)
	candidate.Bitmap[byteIndex] |= byte(1 << bitOffset)
	candidate.AllocatedCount++

	metadataPage, err := EncodeMetadata(candidate)
	if err != nil {
		// A data page has already been appended.
		db.failed = true
		return 0, fmt.Errorf("encode updated metadata: %w", err)
	}

	if err := db.disk.WritePage(MetadataPageID, metadataPage); err != nil {
		// The on-disk state is now uncertain.
		db.failed = true
		return 0, fmt.Errorf("persist allocation metadata: %w", err)
	}

	if err := db.disk.Sync(); err != nil {
		// Persistence cannot be confirmed.
		db.failed = true
		return 0, fmt.Errorf("sync allocated page: %w", err)
	}

	// Publish the new in-memory state only after persistence succeeds.
	db.metadata = candidate

	return id, nil
}
