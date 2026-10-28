package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// MetadataFormatVersion identifies the database metadata format.
	MetadataFormatVersion uint16 = 1

	// Database metadata is stored in physical page 0.
	MetadataPageID PageID = 0

	// Metadata payload layout.
	MetadataMagicOffset          = 0
	MetadataVersionOffset        = 4
	MetadataPageSizeOffset       = 6
	MetadataBitmapCapacityOffset = 10
	MetadataAllocatedCountOffset = 14
	MetadataReservedOffset       = 18
	MetadataHeaderSize           = 32

	// The bitmap follows the metadata header inside page 0's payload.
	MetadataBitmapOffset = MetadataHeaderSize

	// Each bitmap bit represents one allocatable physical page.
	MetadataBitmapBytes    = PagePayloadSize - MetadataBitmapOffset
	MetadataBitmapCapacity = MetadataBitmapBytes * 8

	// Page 0 is reserved for metadata.
	FirstAllocatablePageID PageID = 1

	// Maximum number of physical pages, including metadata page 0.
	MaxDatabasePages = MetadataBitmapCapacity + 1
)

var (
	ErrInvalidMetadataMagic       = errors.New("invalid database metadata magic")
	ErrUnsupportedMetadataVersion = errors.New("unsupported database metadata version")
	ErrInvalidMetadataPageSize    = errors.New("invalid database metadata page size")
	ErrInvalidMetadataCapacity    = errors.New("invalid database metadata bitmap capacity")
	ErrInvalidAllocatedPageCount  = errors.New("invalid allocated page count")
	ErrInvalidMetadataReserved    = errors.New("metadata reserved bytes must be zero")
	ErrInvalidMetadataBitmap      = errors.New("invalid database metadata bitmap")
)

// Metadata describes the database's physical storage allocation.
//
// Bitmap bit 0 represents physical page 1.
// Bitmap bit 1 represents physical page 2, and so on.
// Physical page 0 is reserved for metadata.
type Metadata struct {
	Version        uint16
	PageSize       uint32
	BitmapCapacity uint32
	AllocatedCount uint32
	Bitmap         []byte
}

var metadataMagic = [4]byte{'G', 'R', 'D', 'B'}

// NewMetadata returns an initialized metadata structure.
func NewMetadata() Metadata {
	return Metadata{
		Version:        MetadataFormatVersion,
		PageSize:       PageSize,
		BitmapCapacity: MetadataBitmapCapacity,
		AllocatedCount: 0,
		Bitmap:         make([]byte, MetadataBitmapBytes),
	}
}

// EncodeMetadata writes database metadata into a formatted metadata page.
//
// The page must be physical page 0 and have PageTypeMetadata.
func EncodeMetadata(metadata Metadata) (Page, error) {
	if metadata.Version != MetadataFormatVersion {
		return Page{}, fmt.Errorf(
			"%w: %d",
			ErrUnsupportedMetadataVersion,
			metadata.Version,
		)
	}

	if metadata.PageSize != PageSize {
		return Page{}, fmt.Errorf(
			"%w: got %d, want %d",
			ErrInvalidMetadataPageSize,
			metadata.PageSize,
			PageSize,
		)
	}

	if metadata.BitmapCapacity != MetadataBitmapCapacity {
		return Page{}, fmt.Errorf(
			"%w: got %d, want %d",
			ErrInvalidMetadataCapacity,
			metadata.BitmapCapacity,
			MetadataBitmapCapacity,
		)
	}

	if len(metadata.Bitmap) != MetadataBitmapBytes {
		return Page{}, fmt.Errorf(
			"%w: got %d bytes, want %d",
			ErrInvalidMetadataBitmap,
			len(metadata.Bitmap),
			MetadataBitmapBytes,
		)
	}

	if err := validateMetadataBitmap(metadata); err != nil {
		return Page{}, err
	}

	page, err := FormatPage(MetadataPageID, PageTypeMetadata, 0)
	if err != nil {
		return Page{}, err
	}

	payload := page[PagePayloadOffset:]

	copy(payload[MetadataMagicOffset:], metadataMagic[:])

	binary.LittleEndian.PutUint16(
		payload[MetadataVersionOffset:],
		metadata.Version,
	)
	binary.LittleEndian.PutUint32(
		payload[MetadataPageSizeOffset:],
		metadata.PageSize,
	)
	binary.LittleEndian.PutUint32(
		payload[MetadataBitmapCapacityOffset:],
		metadata.BitmapCapacity,
	)
	binary.LittleEndian.PutUint32(
		payload[MetadataAllocatedCountOffset:],
		metadata.AllocatedCount,
	)

	// Reserved bytes remain zero.
	copy(payload[MetadataBitmapOffset:], metadata.Bitmap)

	RefreshPageChecksum(&page)

	return page, nil
}

// DecodeMetadata validates and decodes database metadata from page 0.
func DecodeMetadata(page Page) (Metadata, error) {
	pageHeader, err := DecodePageHeader(page, MetadataPageID)
	if err != nil {
		return Metadata{}, err
	}

	if pageHeader.Type != PageTypeMetadata {
		return Metadata{}, fmt.Errorf(
			"%w: expected metadata page, got %d",
			ErrInvalidPageType,
			pageHeader.Type,
		)
	}

	payload := page[PagePayloadOffset:]

	if string(payload[MetadataMagicOffset:MetadataVersionOffset]) != string(metadataMagic[:]) {
		return Metadata{}, ErrInvalidMetadataMagic
	}

	version := binary.LittleEndian.Uint16(
		payload[MetadataVersionOffset:],
	)
	if version != MetadataFormatVersion {
		return Metadata{}, fmt.Errorf(
			"%w: %d",
			ErrUnsupportedMetadataVersion,
			version,
		)
	}

	pageSize := binary.LittleEndian.Uint32(
		payload[MetadataPageSizeOffset:],
	)
	if pageSize != PageSize {
		return Metadata{}, fmt.Errorf(
			"%w: got %d, want %d",
			ErrInvalidMetadataPageSize,
			pageSize,
			PageSize,
		)
	}

	capacity := binary.LittleEndian.Uint32(
		payload[MetadataBitmapCapacityOffset:],
	)
	if capacity != MetadataBitmapCapacity {
		return Metadata{}, fmt.Errorf(
			"%w: got %d, want %d",
			ErrInvalidMetadataCapacity,
			capacity,
			MetadataBitmapCapacity,
		)
	}

	allocatedCount := binary.LittleEndian.Uint32(
		payload[MetadataAllocatedCountOffset:],
	)
	if allocatedCount > capacity {
		return Metadata{}, fmt.Errorf(
			"%w: got %d, capacity %d",
			ErrInvalidAllocatedPageCount,
			allocatedCount,
			capacity,
		)
	}

	for _, value := range payload[MetadataReservedOffset:MetadataBitmapOffset] {
		if value != 0 {
			return Metadata{}, ErrInvalidMetadataReserved
		}
	}

	bitmap := append(
		[]byte(nil),
		payload[MetadataBitmapOffset:MetadataBitmapOffset+MetadataBitmapBytes]...,
	)

	metadata := Metadata{
		Version:        version,
		PageSize:       pageSize,
		BitmapCapacity: capacity,
		AllocatedCount: allocatedCount,
		Bitmap:         bitmap,
	}

	if err := validateMetadataBitmap(metadata); err != nil {
		return Metadata{}, err
	}

	return metadata, nil
}

// validateMetadataBitmap ensures the allocation count matches the bitmap.
func validateMetadataBitmap(metadata Metadata) error {
	if len(metadata.Bitmap) != MetadataBitmapBytes {
		return fmt.Errorf(
			"%w: got %d bytes, want %d",
			ErrInvalidMetadataBitmap,
			len(metadata.Bitmap),
			MetadataBitmapBytes,
		)
	}

	var allocated uint32

	for _, value := range metadata.Bitmap {
		for bits := value; bits != 0; bits &= bits - 1 {
			allocated++
		}
	}

	if allocated != metadata.AllocatedCount {
		return fmt.Errorf(
			"%w: bitmap contains %d allocated pages, metadata says %d",
			ErrInvalidAllocatedPageCount,
			allocated,
			metadata.AllocatedCount,
		)
	}

	return nil
}
