package storage

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrInvalidDatabaseLayout = errors.New(
		"database file layout does not match metadata",
	)

	ErrDatabaseCapacityExceeded = errors.New(
		"database allocation capacity exceeded",
	)

	ErrDatabaseFailed = errors.New("database is in a failed state")
)

// Database manages database metadata and the underlying disk manager.
//
// Page allocation, deallocation, transactions, and crash recovery
// are not implemented yet.
type Database struct {
	mu       sync.Mutex
	disk     *DiskManager
	metadata Metadata
	closed   bool
	failed   bool
}

// OpenDatabase opens an existing GoRelix database or initializes an empty file.
//
// An empty file receives metadata in physical page 0.
// A nonempty file must contain valid metadata and a consistent page layout.
//
// Invalid existing files are rejected rather than overwritten.
func OpenDatabase(path string) (*Database, error) {
	disk, err := OpenDiskManager(path)
	if err != nil {
		return nil, err
	}

	// Close the disk manager whenever initialization or validation fails.
	fail := func(cause error) (*Database, error) {
		closeErr := disk.Close()
		return nil, errors.Join(cause, closeErr)
	}

	pageCount, err := disk.PageCount()
	if err != nil {
		return fail(fmt.Errorf("get database page count: %w", err))
	}

	// Initialize a new database.
	if pageCount == 0 {
		metadata := NewMetadata()

		page, err := EncodeMetadata(metadata)
		if err != nil {
			return fail(fmt.Errorf("encode initial metadata: %w", err))
		}

		// AppendPage accepts *Page.
		id, err := disk.AppendPage(page)
		if err != nil {
			return fail(fmt.Errorf("initialize metadata page: %w", err))
		}

		if id != MetadataPageID {
			return fail(fmt.Errorf(
				"initialize metadata: expected page ID %d, got %d",
				MetadataPageID,
				id,
			))
		}

		if err := disk.Sync(); err != nil {
			return fail(fmt.Errorf("sync initial metadata: %w", err))
		}

		return &Database{
			disk:     disk,
			metadata: metadata,
		}, nil
	}

	// Existing databases must contain metadata at physical page 0.
	page, err := disk.ReadPage(MetadataPageID)
	if err != nil {
		return fail(fmt.Errorf("read database metadata page: %w", err))
	}

	// ReadPage returns *Page, so pass the dereferenced value.
	metadata, err := DecodeMetadata(page)
	if err != nil {
		return fail(fmt.Errorf("decode database metadata: %w", err))
	}

	// Reject files that exceed the bitmap's representable capacity.
	physicalDataPages := pageCount - 1

	if physicalDataPages > uint64(metadata.BitmapCapacity) {
		return fail(fmt.Errorf(
			"%w: physical data pages=%d, capacity=%d",
			ErrDatabaseCapacityExceeded,
			physicalDataPages,
			metadata.BitmapCapacity,
		))
	}

	// Every physical data page must be marked allocated.
	for physicalID := uint64(1); physicalID < pageCount; physicalID++ {
		bitmapIndex := physicalID - 1
		byteIndex := bitmapIndex / 8
		bitIndex := bitmapIndex % 8

		if metadata.Bitmap[byteIndex]&(byte(1)<<bitIndex) == 0 {
			return fail(fmt.Errorf(
				"%w: physical page %d is not marked allocated",
				ErrInvalidDatabaseLayout,
				physicalID,
			))
		}
	}

	// For this initial allocator design, every allocated page must
	// correspond to a physical page in the file.
	if uint64(metadata.AllocatedCount) != physicalDataPages {
		return fail(fmt.Errorf(
			"%w: allocated count=%d, physical data pages=%d",
			ErrInvalidDatabaseLayout,
			metadata.AllocatedCount,
			physicalDataPages,
		))
	}

	return &Database{
		disk:     disk,
		metadata: metadata,
	}, nil
}

// Metadata returns a snapshot of the current database metadata.
//
// The bitmap is copied so callers cannot modify the internal allocation state.
func (db *Database) Metadata() Metadata {
	db.mu.Lock()
	defer db.mu.Unlock()

	metadata := db.metadata
	metadata.Bitmap = append([]byte(nil), db.metadata.Bitmap...)

	return metadata
}

// Close synchronizes and closes the underlying database file.
//
// Repeated calls after a successful close are safe.
func (db *Database) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return nil
	}

	if err := db.disk.Close(); err != nil {
		return err
	}

	db.closed = true
	return nil
}
