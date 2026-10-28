package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDatabaseInitializesEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.db")

	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}

	metadata := db.Metadata()

	if metadata.AllocatedCount != 0 {
		t.Errorf("allocated count = %d, want 0", metadata.AllocatedCount)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	disk, err := OpenDiskManager(path)
	if err != nil {
		t.Fatalf("OpenDiskManager() error = %v", err)
	}
	defer disk.Close()

	count, err := disk.PageCount()
	if err != nil {
		t.Fatalf("PageCount() error = %v", err)
	}

	if count != 1 {
		t.Fatalf("page count = %d, want 1", count)
	}
}

func TestOpenDatabaseCanReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")

	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("initial OpenDatabase() error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("initial Close() error = %v", err)
	}

	reopened, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("reopening database failed: %v", err)
	}
	defer reopened.Close()

	if reopened.Metadata().AllocatedCount != 0 {
		t.Fatal("reopened database has unexpected allocated pages")
	}
}

func TestOpenDatabaseRejectsInvalidExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.db")

	// Create a nonempty file containing a validly sized but invalid page.
	disk, err := OpenDiskManager(path)
	if err != nil {
		t.Fatalf("OpenDiskManager() error = %v", err)
	}

	invalidPage := Page{}
	if _, err := disk.AppendPage(invalidPage); err != nil {
		disk.Close()
		t.Fatalf("AppendPage() error = %v", err)
	}

	if err := disk.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() before opening error = %v", err)
	}

	db, err := OpenDatabase(path)
	if err == nil {
		db.Close()
		t.Fatal("OpenDatabase() succeeded for invalid metadata")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after opening error = %v", err)
	}

	if string(before) != string(after) {
		t.Fatal("invalid database file was modified during failed open")
	}
}

func TestDatabaseCloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "close.db")

	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestOpenDatabaseRejectsInvalidFileSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-size.db")

	if err := os.WriteFile(path, []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := OpenDatabase(path)
	if !errors.Is(err, ErrCorruptFileSize) {
		t.Fatalf("error = %v, want ErrCorruptFileSize", err)
	}

	contents, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	if len(contents) != 3 {
		t.Fatalf("file size = %d, want 3; file may have been modified", len(contents))
	}
}
