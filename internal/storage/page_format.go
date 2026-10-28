package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

const (
	// PageFormatVersion identifies the current physical page format.
	PageFormatVersion uint16 = 1

	// Page header byte offsets.
	PageMagicOffset      = 0
	PageVersionOffset    = 4
	PageTypeOffset       = 6
	PageIDOffset         = 8
	PageGenerationOffset = 16
	PageChecksumOffset   = 24
	PageFlagsOffset      = 28
	PagePayloadOffset    = 32

	// PageHeaderSize is the number of bytes reserved for the page header.
	PageHeaderSize = PagePayloadOffset

	// PagePayloadSize is the number of bytes available after the header.
	PagePayloadSize = PageSize - PageHeaderSize
)

var (
	ErrInvalidPageMagic = errors.New(
		"invalid page magic",
	)

	ErrUnsupportedVersion = errors.New(
		"unsupported page format version",
	)

	ErrInvalidPageType = errors.New(
		"invalid page type",
	)

	ErrPageIDMismatch = errors.New(
		"page ID mismatch",
	)

	ErrPageChecksum = errors.New(
		"page checksum mismatch",
	)
)

// pageMagic identifies a page as belonging to GoRelix.
var pageMagic = [4]byte{'G', 'R', 'L', 'X'}

// PageType identifies the role of a physical page.
type PageType uint16

const (
	// PageTypeMetadata identifies the database metadata page.
	PageTypeMetadata PageType = 1

	// PageTypeData identifies an ordinary database data page.
	PageTypeData PageType = 2
)

// PageHeader describes the versioned metadata stored at the beginning
// of a GoRelix page.
//
// Fields are encoded explicitly. Go struct memory layout is never
// written directly to disk.
type PageHeader struct {
	Version    uint16
	Type       PageType
	ID         PageID
	Generation uint64
	Flags      uint32
}

// FormatPage initializes a zero-filled page with a valid header
// and checksum.
//
// The payload is initially zero-filled. The caller may subsequently
// modify the payload and call RefreshPageChecksum before writing it.
func FormatPage(
	id PageID,
	pageType PageType,
	generation uint64,
) (Page, error) {
	if !validPageType(pageType) {
		return Page{}, fmt.Errorf(
			"%w: %d",
			ErrInvalidPageType,
			pageType,
		)
	}

	var page Page

	// Write the GoRelix page identifier.
	copy(
		page[PageMagicOffset:PageVersionOffset],
		pageMagic[:],
	)

	// Write the page format version.
	binary.LittleEndian.PutUint16(
		page[PageVersionOffset:PageTypeOffset],
		PageFormatVersion,
	)

	// Write the page type.
	binary.LittleEndian.PutUint16(
		page[PageTypeOffset:PageIDOffset],
		uint16(pageType),
	)

	// Write the physical page ID.
	binary.LittleEndian.PutUint64(
		page[PageIDOffset:PageGenerationOffset],
		uint64(id),
	)

	// Write the page generation.
	binary.LittleEndian.PutUint64(
		page[PageGenerationOffset:PageChecksumOffset],
		generation,
	)

	// Initialize flags to zero.
	binary.LittleEndian.PutUint32(
		page[PageFlagsOffset:PagePayloadOffset],
		0,
	)

	// Calculate the checksum after initializing the page.
	setPageChecksum(&page)

	return page, nil
}

// DecodePageHeader validates a page's magic, version, type, physical ID,
// and checksum before returning its decoded header.
//
// expectedID must be the physical page ID used to read this page.
func DecodePageHeader(
	page Page,
	expectedID PageID,
) (PageHeader, error) {
	// Validate the page identifier.
	if string(page[PageMagicOffset:PageVersionOffset]) != string(pageMagic[:]) {
		return PageHeader{}, ErrInvalidPageMagic
	}

	// Validate the format version.
	version := binary.LittleEndian.Uint16(
		page[PageVersionOffset:PageTypeOffset],
	)

	if version != PageFormatVersion {
		return PageHeader{}, fmt.Errorf(
			"%w: got %d, supported %d",
			ErrUnsupportedVersion,
			version,
			PageFormatVersion,
		)
	}

	// Validate the checksum before trusting the remaining header fields.
	storedChecksum := binary.LittleEndian.Uint32(
		page[PageChecksumOffset:PageFlagsOffset],
	)

	calculatedChecksum := calculatePageChecksum(page)

	if storedChecksum != calculatedChecksum {
		return PageHeader{}, ErrPageChecksum
	}

	// Decode and validate the page type.
	pageType := PageType(
		binary.LittleEndian.Uint16(
			page[PageTypeOffset:PageIDOffset],
		),
	)

	if !validPageType(pageType) {
		return PageHeader{}, fmt.Errorf(
			"%w: %d",
			ErrInvalidPageType,
			pageType,
		)
	}

	// Decode and validate the physical page ID.
	id := PageID(
		binary.LittleEndian.Uint64(
			page[PageIDOffset:PageGenerationOffset],
		),
	)

	if id != expectedID {
		return PageHeader{}, fmt.Errorf(
			"%w: expected %d, got %d",
			ErrPageIDMismatch,
			expectedID,
			id,
		)
	}

	// Decode the remaining header fields.
	generation := binary.LittleEndian.Uint64(
		page[PageGenerationOffset:PageChecksumOffset],
	)

	flags := binary.LittleEndian.Uint32(
		page[PageFlagsOffset:PagePayloadOffset],
	)

	return PageHeader{
		Version:    version,
		Type:       pageType,
		ID:         id,
		Generation: generation,
		Flags:      flags,
	}, nil
}

// RefreshPageChecksum recalculates the checksum after a page is modified.
//
// Call this after modifying any header or payload bytes. It does not
// validate the page or write it to disk. DecodePageHeader performs
// validation, while DiskManager handles physical writes.
func RefreshPageChecksum(page *Page) {
	setPageChecksum(page)
}

// validPageType reports whether the supplied page type is supported.
func validPageType(pageType PageType) bool {
	switch pageType {
	case PageTypeMetadata, PageTypeData:
		return true
	default:
		return false
	}
}

// calculatePageChecksum calculates a CRC-32/IEEE checksum across the
// entire page, treating the checksum field itself as zero.
//
// This detects many forms of accidental corruption. It does not provide
// cryptographic integrity or make physical writes atomic.
func calculatePageChecksum(page Page) uint32 {
	// Work on a copy so calculating the checksum never modifies
	// the caller's page.
	copyOfPage := page

	// Clear the checksum field before calculation.
	for i := PageChecksumOffset; i < PageFlagsOffset; i++ {
		copyOfPage[i] = 0
	}

	return crc32.ChecksumIEEE(copyOfPage[:])
}

// setPageChecksum writes the calculated checksum into the page.
func setPageChecksum(page *Page) {
	checksum := calculatePageChecksum(*page)

	binary.LittleEndian.PutUint32(
		page[PageChecksumOffset:PageFlagsOffset],
		checksum,
	)
}
