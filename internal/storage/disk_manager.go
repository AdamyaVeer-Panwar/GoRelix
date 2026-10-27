package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const PageSize = 4096

type PageID uint64

type Page [PageSize]byte

var (
	ErrInvalidPath     = errors.New("invalid database path")
	ErrClosed          = errors.New("disk manager is closed")
	ErrPageOutOfRange  = errors.New("page ID out of range")
	ErrCorruptFileSize = errors.New(
		"database file size is not a multiple of page size",
	)
	ErrNotRegularFile = errors.New(
		"database path is not a regular file",
	)
)

// DiskManager provides synchronized, fixed-size page I/O.
// It supports appending pages and accessing existing pages.
// It does not implement page reclamation, transactions, or recovery.
type DiskManager struct {
	mu     sync.Mutex
	file   *os.File
	closed bool
}

// OpenDiskManager opens an existing database file or creates a new one.
// Existing files are never truncated. Files containing incomplete pages
// are rejected rather than silently repaired.
func OpenDiskManager(path string) (*DiskManager, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrInvalidPath
	}

	file, err := os.OpenFile(
		path,
		os.O_RDWR|os.O_CREATE,
		0o600,
	)
	if err != nil {
		return nil, fmt.Errorf("open database file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat database file: %w", err)
	}

	if !info.Mode().IsRegular() {
		file.Close()
		return nil, ErrNotRegularFile
	}

	if info.Size()%int64(PageSize) != 0 {
		file.Close()

		return nil, fmt.Errorf(
			"%w: size=%d, page_size=%d",
			ErrCorruptFileSize,
			info.Size(),
			PageSize,
		)
	}

	return &DiskManager{
		file: file,
	}, nil
}

// PageCount returns the number of complete pages in the database file.
func (dm *DiskManager) PageCount() (uint64, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if err := dm.ensureOpen(); err != nil {
		return 0, err
	}

	return dm.pageCountLocked()
}

// ReadPage reads an existing page into a fixed-size Page value.
func (dm *DiskManager) ReadPage(id PageID) (Page, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if err := dm.ensureOpen(); err != nil {
		return Page{}, err
	}

	count, err := dm.pageCountLocked()
	if err != nil {
		return Page{}, err
	}

	if uint64(id) >= count {
		return Page{}, fmt.Errorf(
			"%w: requested=%d, page_count=%d",
			ErrPageOutOfRange,
			id,
			count,
		)
	}

	offset, err := pageOffset(id)
	if err != nil {
		return Page{}, err
	}

	var page Page

	n, err := dm.file.ReadAt(page[:], offset)
	if err != nil {
		return Page{}, fmt.Errorf("read page %d: %w", id, err)
	}

	if n != len(page) {
		return Page{}, fmt.Errorf(
			"read page %d: %w",
			id,
			io.ErrUnexpectedEOF,
		)
	}

	return page, nil
}

// WritePage overwrites an existing page.
//
// It cannot create a new page or extend the file.
func (dm *DiskManager) WritePage(id PageID, page Page) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if err := dm.ensureOpen(); err != nil {
		return err
	}

	count, err := dm.pageCountLocked()
	if err != nil {
		return err
	}

	if uint64(id) >= count {
		return fmt.Errorf(
			"%w: requested=%d, page_count=%d",
			ErrPageOutOfRange,
			id,
			count,
		)
	}

	offset, err := pageOffset(id)
	if err != nil {
		return err
	}

	n, err := dm.file.WriteAt(page[:], offset)
	if err != nil {
		return fmt.Errorf("write page %d: %w", id, err)
	}

	if n != len(page) {
		return fmt.Errorf(
			"write page %d: %w",
			id,
			io.ErrShortWrite,
		)
	}

	return nil
}

// AppendPage writes a new page at the end of the file.
//
// The returned PageID is the new page's identifier.
// Failed partial appends attempt to restore the original file length.
func (dm *DiskManager) AppendPage(page Page) (PageID, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if err := dm.ensureOpen(); err != nil {
		return 0, err
	}

	count, err := dm.pageCountLocked()
	if err != nil {
		return 0, err
	}

	id := PageID(count)

	offset, err := pageOffset(id)
	if err != nil {
		return 0, err
	}

	originalSize := int64(count) * int64(PageSize)

	n, writeErr := dm.file.WriteAt(page[:], offset)

	if writeErr == nil && n != len(page) {
		writeErr = io.ErrShortWrite
	}

	if writeErr != nil {
		if rollbackErr := dm.file.Truncate(originalSize); rollbackErr != nil {
			writeErr = errors.Join(
				writeErr,
				fmt.Errorf("restore file length: %w", rollbackErr),
			)
		}

		return 0, fmt.Errorf(
			"append page %d: %w",
			id,
			writeErr,
		)
	}

	return id, nil
}

// Sync requests that pending file contents be committed to stable storage,
// subject to the guarantees of the operating system and filesystem.
func (dm *DiskManager) Sync() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if err := dm.ensureOpen(); err != nil {
		return err
	}

	if err := dm.file.Sync(); err != nil {
		return fmt.Errorf("sync database file: %w", err)
	}

	return nil
}

// Close synchronizes and closes the database file.
//
// Close is idempotent: calling it again after a successful close is safe.
func (dm *DiskManager) Close() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.closed {
		return nil
	}

	var syncErr error
	var closeErr error

	if err := dm.file.Sync(); err != nil {
		syncErr = fmt.Errorf("sync database file before close: %w", err)
	}

	if err := dm.file.Close(); err != nil {
		closeErr = fmt.Errorf("close database file: %w", err)
	}

	dm.closed = true

	return errors.Join(syncErr, closeErr)
}

func (dm *DiskManager) ensureOpen() error {
	if dm.closed {
		return ErrClosed
	}

	return nil
}

func (dm *DiskManager) pageCountLocked() (uint64, error) {
	info, err := dm.file.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat database file: %w", err)
	}

	if info.Size()%int64(PageSize) != 0 {
		return 0, fmt.Errorf(
			"%w: size=%d, page_size=%d",
			ErrCorruptFileSize,
			info.Size(),
			PageSize,
		)
	}

	return uint64(info.Size() / int64(PageSize)), nil
}

func pageOffset(id PageID) (int64, error) {
	const maxInt64 = uint64(1<<63 - 1)

	maxPageID := (maxInt64 - uint64(PageSize-1)) / uint64(PageSize)

	if uint64(id) > maxPageID {
		return 0, fmt.Errorf(
			"%w: page ID cannot be represented as a file offset: %d",
			ErrPageOutOfRange,
			id,
		)
	}

	return int64(uint64(id) * uint64(PageSize)), nil
}
