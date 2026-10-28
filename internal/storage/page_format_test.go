package storage

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestFormatAndDecodePage(t *testing.T) {
	const (
		id         PageID   = 7
		generation uint64   = 3
		pageType   PageType = PageTypeData
	)

	page, err := FormatPage(id, pageType, generation)
	if err != nil {
		t.Fatalf("FormatPage failed: %v", err)
	}

	header, err := DecodePageHeader(page, id)
	if err != nil {
		t.Fatalf("DecodePageHeader failed: %v", err)
	}

	if header.ID != id {
		t.Errorf("expected ID %d, got %d", id, header.ID)
	}

	if header.Version != PageFormatVersion {
		t.Errorf(
			"expected version %d, got %d",
			PageFormatVersion,
			header.Version,
		)
	}

	if header.Type != pageType {
		t.Errorf(
			"expected page type %d, got %d",
			pageType,
			header.Type,
		)
	}

	if header.Generation != generation {
		t.Errorf(
			"expected generation %d, got %d",
			generation,
			header.Generation,
		)
	}

	if header.Flags != 0 {
		t.Errorf("expected flags 0, got %d", header.Flags)
	}

	for i, value := range page[PagePayloadOffset:] {
		if value != 0 {
			t.Fatalf(
				"expected zero-filled payload; byte %d contains %d",
				i,
				value,
			)
		}
	}
}

func TestFormatRejectsInvalidPageType(t *testing.T) {
	_, err := FormatPage(0, PageType(99), 0)

	if !errors.Is(err, ErrInvalidPageType) {
		t.Fatalf(
			"expected ErrInvalidPageType, got %v",
			err,
		)
	}
}

func TestDecodeRejectsInvalidMagic(t *testing.T) {
	page, err := FormatPage(0, PageTypeData, 0)
	if err != nil {
		t.Fatal(err)
	}

	page[PageMagicOffset] ^= 0xFF

	_, err = DecodePageHeader(page, 0)

	if !errors.Is(err, ErrInvalidPageMagic) {
		t.Fatalf(
			"expected ErrInvalidPageMagic, got %v",
			err,
		)
	}
}

func TestDecodeRejectsUnsupportedVersion(t *testing.T) {
	page, err := FormatPage(0, PageTypeData, 0)
	if err != nil {
		t.Fatal(err)
	}

	binary.LittleEndian.PutUint16(
		page[PageVersionOffset:PageTypeOffset],
		PageFormatVersion+1,
	)

	// Recalculate the checksum so the test isolates version validation.
	RefreshPageChecksum(&page)

	_, err = DecodePageHeader(page, 0)

	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf(
			"expected ErrUnsupportedVersion, got %v",
			err,
		)
	}
}

func TestDecodeRejectsInvalidPageType(t *testing.T) {
	page, err := FormatPage(0, PageTypeData, 0)
	if err != nil {
		t.Fatal(err)
	}

	binary.LittleEndian.PutUint16(
		page[PageTypeOffset:PageIDOffset],
		uint16(99),
	)

	// Recalculate the checksum to isolate page-type validation.
	RefreshPageChecksum(&page)

	_, err = DecodePageHeader(page, 0)

	if !errors.Is(err, ErrInvalidPageType) {
		t.Fatalf(
			"expected ErrInvalidPageType, got %v",
			err,
		)
	}
}

func TestDecodeRejectsPageIDMismatch(t *testing.T) {
	page, err := FormatPage(7, PageTypeData, 0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = DecodePageHeader(page, 8)

	if !errors.Is(err, ErrPageIDMismatch) {
		t.Fatalf(
			"expected ErrPageIDMismatch, got %v",
			err,
		)
	}
}

func TestDecodeRejectsCorruptedHeaderFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Page)
	}{
		{
			name: "page ID",
			mutate: func(page *Page) {
				page[PageIDOffset] ^= 0x01
			},
		},
		{
			name: "generation",
			mutate: func(page *Page) {
				page[PageGenerationOffset] ^= 0x01
			},
		},
		{
			name: "flags",
			mutate: func(page *Page) {
				page[PageFlagsOffset] ^= 0x01
			},
		},
		{
			name: "checksum",
			mutate: func(page *Page) {
				page[PageChecksumOffset] ^= 0x01
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := FormatPage(0, PageTypeData, 0)
			if err != nil {
				t.Fatal(err)
			}

			tt.mutate(&page)

			_, err = DecodePageHeader(page, 0)

			if !errors.Is(err, ErrPageChecksum) {
				t.Fatalf(
					"expected ErrPageChecksum, got %v",
					err,
				)
			}
		})
	}
}

func TestDecodeRejectsPayloadCorruption(t *testing.T) {
	page, err := FormatPage(0, PageTypeData, 0)
	if err != nil {
		t.Fatal(err)
	}

	page[PagePayloadOffset] ^= 0x01

	_, err = DecodePageHeader(page, 0)

	if !errors.Is(err, ErrPageChecksum) {
		t.Fatalf(
			"expected ErrPageChecksum, got %v",
			err,
		)
	}
}

func TestRefreshPageChecksumAfterPayloadChange(t *testing.T) {
	page, err := FormatPage(1, PageTypeData, 0)
	if err != nil {
		t.Fatalf("FormatPage failed: %v", err)
	}

	// Modify the payload without refreshing the checksum.
	page[PagePayloadOffset] = 42

	_, err = DecodePageHeader(page, 1)

	if !errors.Is(err, ErrPageChecksum) {
		t.Fatalf(
			"expected ErrPageChecksum before refresh, got %v",
			err,
		)
	}

	// Refresh the checksum after modifying the payload.
	RefreshPageChecksum(&page)

	header, err := DecodePageHeader(page, 1)
	if err != nil {
		t.Fatalf(
			"DecodePageHeader failed after refresh: %v",
			err,
		)
	}

	if header.ID != 1 {
		t.Errorf("expected page ID 1, got %d", header.ID)
	}

	if page[PagePayloadOffset] != 42 {
		t.Errorf("payload was not preserved")
	}
}

func TestRefreshPageChecksumDoesNotModifyPayload(t *testing.T) {
	page, err := FormatPage(1, PageTypeData, 0)
	if err != nil {
		t.Fatal(err)
	}

	page[PagePayloadOffset] = 10
	page[PagePayloadOffset+1] = 20

	before := page

	RefreshPageChecksum(&page)

	if page[PagePayloadOffset] != before[PagePayloadOffset] {
		t.Fatal("checksum refresh modified the first payload byte")
	}

	if page[PagePayloadOffset+1] != before[PagePayloadOffset+1] {
		t.Fatal("checksum refresh modified the second payload byte")
	}
}
