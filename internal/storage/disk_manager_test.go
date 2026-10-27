package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func openTestDiskManager(t *testing.T) *DiskManager {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")

	dm, err := OpenDiskManager(path)
	if err != nil {
		t.Fatalf("OpenDiskManager failed: %v", err)
	}

	t.Cleanup(func() {
		if err := dm.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	return dm
}

func TestOpenCreatesEmptyDatabase(t *testing.T) {
	dm := openTestDiskManager(t)

	count, err := dm.PageCount()
	if err != nil {
		t.Fatalf("PageCount failed: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected 0 pages, got %d", count)
	}
}

func TestAppendAndReadPage(t *testing.T) {
	dm := openTestDiskManager(t)

	var expected Page

	for i := range expected {
		expected[i] = byte(i % 251)
	}

	id, err := dm.AppendPage(expected)
	if err != nil {
		t.Fatalf("AppendPage failed: %v", err)
	}

	if id != 0 {
		t.Fatalf("expected first page ID 0, got %d", id)
	}

	actual, err := dm.ReadPage(id)
	if err != nil {
		t.Fatalf("ReadPage failed: %v", err)
	}

	if actual != expected {
		t.Fatal("read page differs from written page")
	}
}

func TestPagesPersistAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persistent.db")

	first, err := OpenDiskManager(path)
	if err != nil {
		t.Fatalf("initial open failed: %v", err)
	}

	var pageA, pageB Page

	for i := range pageA {
		pageA[i] = 0xAA
		pageB[i] = 0xBB
	}

	idA, err := first.AppendPage(pageA)
	if err != nil {
		t.Fatalf("append page A failed: %v", err)
	}

	idB, err := first.AppendPage(pageB)
	if err != nil {
		t.Fatalf("append page B failed: %v", err)
	}

	if idA != 0 || idB != 1 {
		t.Fatalf("expected page IDs 0 and 1, got %d and %d", idA, idB)
	}

	if err := first.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	second, err := OpenDiskManager(path)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer second.Close()

	actualA, err := second.ReadPage(idA)
	if err != nil {
		t.Fatalf("read page A after reopen failed: %v", err)
	}

	actualB, err := second.ReadPage(idB)
	if err != nil {
		t.Fatalf("read page B after reopen failed: %v", err)
	}

	if actualA != pageA {
		t.Fatal("page A changed after reopening")
	}

	if actualB != pageB {
		t.Fatal("page B changed after reopening")
	}

	count, err := second.PageCount()
	if err != nil {
		t.Fatalf("PageCount failed: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected 2 pages after reopening, got %d", count)
	}
}

func TestWritePageUpdatesExistingPage(t *testing.T) {
	dm := openTestDiskManager(t)

	var original Page
	var updated Page

	for i := range original {
		original[i] = 0x11
		updated[i] = 0x22
	}

	id, err := dm.AppendPage(original)
	if err != nil {
		t.Fatalf("AppendPage failed: %v", err)
	}

	if err := dm.WritePage(id, updated); err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}

	actual, err := dm.ReadPage(id)
	if err != nil {
		t.Fatalf("ReadPage failed: %v", err)
	}

	if actual != updated {
		t.Fatal("page contents were not updated")
	}
}

func TestRejectsOutOfRangePageIDs(t *testing.T) {
	dm := openTestDiskManager(t)

	_, err := dm.ReadPage(0)
	if !errors.Is(err, ErrPageOutOfRange) {
		t.Fatalf("expected ErrPageOutOfRange, got %v", err)
	}

	err = dm.WritePage(0, Page{})
	if !errors.Is(err, ErrPageOutOfRange) {
		t.Fatalf("expected ErrPageOutOfRange, got %v", err)
	}
}

func TestRejectsPartialPageFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.db")

	if err := os.WriteFile(path, []byte{0x01}, 0o600); err != nil {
		t.Fatalf("creating invalid test file failed: %v", err)
	}

	dm, err := OpenDiskManager(path)

	if dm != nil {
		dm.Close()
		t.Fatal("expected no DiskManager for a partial-page file")
	}

	if !errors.Is(err, ErrCorruptFileSize) {
		t.Fatalf("expected ErrCorruptFileSize, got %v", err)
	}
}

func TestRejectsEmptyPath(t *testing.T) {
	dm, err := OpenDiskManager("")

	if dm != nil {
		dm.Close()
		t.Fatal("expected no DiskManager for an empty path")
	}

	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("expected ErrInvalidPath, got %v", err)
	}
}

func TestOperationsRejectClosedManager(t *testing.T) {
	dm := openTestDiskManager(t)

	if err := dm.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := dm.PageCount(); !errors.Is(err, ErrClosed) {
		t.Fatalf("PageCount: expected ErrClosed, got %v", err)
	}

	if _, err := dm.ReadPage(0); !errors.Is(err, ErrClosed) {
		t.Fatalf("ReadPage: expected ErrClosed, got %v", err)
	}

	if err := dm.WritePage(0, Page{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("WritePage: expected ErrClosed, got %v", err)
	}

	if _, err := dm.AppendPage(Page{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("AppendPage: expected ErrClosed, got %v", err)
	}

	if err := dm.Sync(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Sync: expected ErrClosed, got %v", err)
	}

	if err := dm.Close(); err != nil {
		t.Fatalf("second Close should be safe: %v", err)
	}
}

func TestConcurrentAppendsReceiveDistinctPages(t *testing.T) {
	dm := openTestDiskManager(t)

	const workers = 16

	type result struct {
		id     PageID
		marker byte
		err    error
	}

	results := make(chan result, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		marker := byte(i + 1)

		wg.Add(1)

		go func(marker byte) {
			defer wg.Done()

			var page Page

			for j := range page {
				page[j] = marker
			}

			id, err := dm.AppendPage(page)

			results <- result{
				id:     id,
				marker: marker,
				err:    err,
			}
		}(marker)
	}

	wg.Wait()
	close(results)

	expected := make(map[PageID]byte, workers)

	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent append failed: %v", result.err)
		}

		if _, exists := expected[result.id]; exists {
			t.Fatalf("duplicate page ID allocated: %d", result.id)
		}

		expected[result.id] = result.marker
	}

	if len(expected) != workers {
		t.Fatalf("expected %d distinct pages, got %d", workers, len(expected))
	}

	for id, marker := range expected {
		actual, err := dm.ReadPage(id)
		if err != nil {
			t.Fatalf("reading page %d failed: %v", id, err)
		}

		for offset, value := range actual {
			if value != marker {
				t.Fatalf(
					"page %d differs at offset %d: expected %x, got %x",
					id,
					offset,
					marker,
					value,
				)
			}
		}
	}
}
