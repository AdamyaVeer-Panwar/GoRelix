package storage

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAllocatePageReturnsSequentialIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	defer db.Close()

	for i := uint64(1); i <= 3; i++ {
		id, err := db.AllocatePage()
		if err != nil {
			t.Fatalf("AllocatePage() error = %v", err)
		}

		if uint64(id) != i {
			t.Fatalf("AllocatePage() ID = %d, want %d", id, i)
		}
	}

	metadata := db.Metadata()
	if metadata.AllocatedCount != 3 {
		t.Fatalf(
			"AllocatedCount = %d, want 3",
			metadata.AllocatedCount,
		)
	}
}

func TestAllocatePageCreatesValidDataPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	defer db.Close()

	id, err := db.AllocatePage()
	if err != nil {
		t.Fatalf("AllocatePage() error = %v", err)
	}

	page, err := db.disk.ReadPage(id)
	if err != nil {
		t.Fatalf("ReadPage() error = %v", err)
	}

	header, err := DecodePageHeader(page, id)
	if err != nil {
		t.Fatalf("DecodePageHeader() error = %v", err)
	}

	if header.Type != PageTypeData {
		t.Fatalf("page type = %d, want %d", header.Type, PageTypeData)
	}

	if header.ID != id {
		t.Fatalf("page ID = %d, want %d", header.ID, id)
	}
}

func TestAllocatePagePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}

	firstID, err := db.AllocatePage()
	if err != nil {
		t.Fatalf("first AllocatePage() error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db, err = OpenDatabase(path)
	if err != nil {
		t.Fatalf("reopen database error = %v", err)
	}
	defer db.Close()

	metadata := db.Metadata()
	if metadata.AllocatedCount != 1 {
		t.Fatalf(
			"AllocatedCount after reopen = %d, want 1",
			metadata.AllocatedCount,
		)
	}

	secondID, err := db.AllocatePage()
	if err != nil {
		t.Fatalf("second AllocatePage() error = %v", err)
	}

	if secondID != firstID+1 {
		t.Fatalf("second page ID = %d, want %d", secondID, firstID+1)
	}
}

func TestAllocatePageRejectsClosedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err = db.AllocatePage()
	if !errors.Is(err, ErrDatabaseClosed) {
		t.Fatalf("AllocatePage() error = %v, want ErrDatabaseClosed", err)
	}
}

func TestAllocatePageRejectsFailedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	defer db.Close()

	db.mu.Lock()
	db.failed = true
	db.mu.Unlock()

	_, err = db.AllocatePage()
	if !errors.Is(err, ErrDatabaseFailed) {
		t.Fatalf(
			"AllocatePage() error = %v, want ErrDatabaseFailed",
			err,
		)
	}
}

func TestAllocatePageAtCapacityBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boundary.db")

	dm, err := OpenDiskManager(path)
	if err != nil {
		t.Fatalf("OpenDiskManager() error = %v", err)
	}
	defer dm.Close()

	// Simulate a database with all but one data page allocated.
	metadata := NewMetadata()
	for i := range metadata.Bitmap {
		metadata.Bitmap[i] = 0xFF
	}

	// Clear the final allocation bit.
	lastByte := len(metadata.Bitmap) - 1
	metadata.Bitmap[lastByte] &^= 1 << 7
	metadata.AllocatedCount = metadata.BitmapCapacity - 1

	// Create the logical physical size:
	// metadata page + all currently allocated data pages.
	physicalPageCount := uint64(metadata.AllocatedCount) + 1
	fileSize := int64(physicalPageCount) * int64(PageSize)

	if err := dm.file.Truncate(fileSize); err != nil {
		t.Fatalf("Truncate() error = %v", err)
	}

	metadataPage, err := EncodeMetadata(metadata)
	if err != nil {
		t.Fatalf("EncodeMetadata() error = %v", err)
	}

	if err := dm.WritePage(MetadataPageID, metadataPage); err != nil {
		t.Fatalf("WritePage() error = %v", err)
	}

	if err := dm.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	db := &Database{
		disk:     dm,
		metadata: metadata,
	}

	// Allocate the final available data page.
	id, err := db.AllocatePage()
	if err != nil {
		t.Fatalf("final AllocatePage() error = %v", err)
	}

	if uint64(id) != MaxDatabasePages-1 {
		t.Fatalf("final page ID = %d, want %d", id, MaxDatabasePages-1)
	}

	// Capture state after the final successful allocation.
	beforeMetadata := db.Metadata()
	beforePageCount, err := dm.PageCount()
	if err != nil {
		t.Fatalf("PageCount() error = %v", err)
	}

	// One more allocation must fail.
	_, err = db.AllocatePage()
	if !errors.Is(err, ErrDatabaseCapacityExceeded) {
		t.Fatalf(
			"AllocatePage() error = %v, want ErrDatabaseCapacityExceeded",
			err,
		)
	}

	// Verify that the failed allocation changed nothing.
	afterMetadata := db.Metadata()
	afterPageCount, err := dm.PageCount()
	if err != nil {
		t.Fatalf("PageCount() after rejection error = %v", err)
	}

	if afterMetadata.AllocatedCount != beforeMetadata.AllocatedCount {
		t.Fatalf(
			"AllocatedCount changed: got %d, want %d",
			afterMetadata.AllocatedCount,
			beforeMetadata.AllocatedCount,
		)
	}

	if afterPageCount != beforePageCount {
		t.Fatalf(
			"physical page count changed: got %d, want %d",
			afterPageCount,
			beforePageCount,
		)
	}

	if string(afterMetadata.Bitmap) != string(beforeMetadata.Bitmap) {
		t.Fatal("allocation bitmap changed after rejected allocation")
	}
}
