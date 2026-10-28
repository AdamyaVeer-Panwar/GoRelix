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
