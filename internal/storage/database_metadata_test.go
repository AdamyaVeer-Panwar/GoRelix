package storage

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestNewMetadata(t *testing.T) {
	metadata := NewMetadata()

	if metadata.Version != MetadataFormatVersion {
		t.Fatalf("version = %d, want %d",
			metadata.Version, MetadataFormatVersion)
	}

	if metadata.PageSize != PageSize {
		t.Fatalf("page size = %d, want %d",
			metadata.PageSize, PageSize)
	}

	if metadata.BitmapCapacity != MetadataBitmapCapacity {
		t.Fatalf("bitmap capacity = %d, want %d",
			metadata.BitmapCapacity, MetadataBitmapCapacity)
	}

	if metadata.AllocatedCount != 0 {
		t.Fatalf("allocated count = %d, want 0",
			metadata.AllocatedCount)
	}

	if len(metadata.Bitmap) != MetadataBitmapBytes {
		t.Fatalf("bitmap length = %d, want %d",
			len(metadata.Bitmap), MetadataBitmapBytes)
	}
}

func TestEncodeDecodeMetadata(t *testing.T) {
	metadata := NewMetadata()

	// Allocate physical pages 1 and 3.
	metadata.Bitmap[0] |= 1 << 0
	metadata.Bitmap[0] |= 1 << 2
	metadata.AllocatedCount = 2

	page, err := EncodeMetadata(metadata)
	if err != nil {
		t.Fatalf("EncodeMetadata() error = %v", err)
	}

	decoded, err := DecodeMetadata(page)
	if err != nil {
		t.Fatalf("DecodeMetadata() error = %v", err)
	}

	if decoded.Version != metadata.Version {
		t.Errorf("version = %d, want %d",
			decoded.Version, metadata.Version)
	}

	if decoded.PageSize != metadata.PageSize {
		t.Errorf("page size = %d, want %d",
			decoded.PageSize, metadata.PageSize)
	}

	if decoded.BitmapCapacity != metadata.BitmapCapacity {
		t.Errorf("bitmap capacity = %d, want %d",
			decoded.BitmapCapacity, metadata.BitmapCapacity)
	}

	if decoded.AllocatedCount != metadata.AllocatedCount {
		t.Errorf("allocated count = %d, want %d",
			decoded.AllocatedCount, metadata.AllocatedCount)
	}

	for i := range metadata.Bitmap {
		if decoded.Bitmap[i] != metadata.Bitmap[i] {
			t.Fatalf("bitmap differs at byte %d", i)
		}
	}
}

func TestEncodeMetadataRejectsUnsupportedVersion(t *testing.T) {
	metadata := NewMetadata()
	metadata.Version++

	_, err := EncodeMetadata(metadata)

	if !errors.Is(err, ErrUnsupportedMetadataVersion) {
		t.Fatalf("error = %v, want ErrUnsupportedMetadataVersion", err)
	}
}

func TestEncodeMetadataRejectsInvalidPageSize(t *testing.T) {
	metadata := NewMetadata()
	metadata.PageSize = 8192

	_, err := EncodeMetadata(metadata)

	if !errors.Is(err, ErrInvalidMetadataPageSize) {
		t.Fatalf("error = %v, want ErrInvalidMetadataPageSize", err)
	}
}

func TestEncodeMetadataRejectsInvalidBitmapLength(t *testing.T) {
	metadata := NewMetadata()
	metadata.Bitmap = metadata.Bitmap[:len(metadata.Bitmap)-1]

	_, err := EncodeMetadata(metadata)

	if !errors.Is(err, ErrInvalidMetadataBitmap) {
		t.Fatalf("error = %v, want ErrInvalidMetadataBitmap", err)
	}
}

func TestEncodeMetadataRejectsIncorrectAllocationCount(t *testing.T) {
	metadata := NewMetadata()
	metadata.Bitmap[0] = 1
	metadata.AllocatedCount = 0

	_, err := EncodeMetadata(metadata)

	if !errors.Is(err, ErrInvalidAllocatedPageCount) {
		t.Fatalf("error = %v, want ErrInvalidAllocatedPageCount", err)
	}
}

func TestDecodeMetadataRejectsCorruptedMagic(t *testing.T) {
	page, err := EncodeMetadata(NewMetadata())
	if err != nil {
		t.Fatal(err)
	}

	page[PagePayloadOffset+MetadataMagicOffset] ^= 0xFF
	RefreshPageChecksum(&page)

	_, err = DecodeMetadata(page)

	if !errors.Is(err, ErrInvalidMetadataMagic) {
		t.Fatalf("error = %v, want ErrInvalidMetadataMagic", err)
	}
}

func TestDecodeMetadataRejectsUnsupportedVersion(t *testing.T) {
	page, err := EncodeMetadata(NewMetadata())
	if err != nil {
		t.Fatal(err)
	}

	binary.LittleEndian.PutUint16(
		page[PagePayloadOffset+MetadataVersionOffset:],
		MetadataFormatVersion+1,
	)
	RefreshPageChecksum(&page)

	_, err = DecodeMetadata(page)

	if !errors.Is(err, ErrUnsupportedMetadataVersion) {
		t.Fatalf("error = %v, want ErrUnsupportedMetadataVersion", err)
	}
}

func TestDecodeMetadataRejectsIncorrectAllocationCount(t *testing.T) {
	page, err := EncodeMetadata(NewMetadata())
	if err != nil {
		t.Fatal(err)
	}

	binary.LittleEndian.PutUint32(
		page[PagePayloadOffset+MetadataAllocatedCountOffset:],
		1,
	)
	RefreshPageChecksum(&page)

	_, err = DecodeMetadata(page)

	if !errors.Is(err, ErrInvalidAllocatedPageCount) {
		t.Fatalf("error = %v, want ErrInvalidAllocatedPageCount", err)
	}
}

func TestDecodeMetadataRejectsNonzeroReservedBytes(t *testing.T) {
	page, err := EncodeMetadata(NewMetadata())
	if err != nil {
		t.Fatal(err)
	}

	page[PagePayloadOffset+MetadataReservedOffset] = 1
	RefreshPageChecksum(&page)

	_, err = DecodeMetadata(page)

	if !errors.Is(err, ErrInvalidMetadataReserved) {
		t.Fatalf("error = %v, want ErrInvalidMetadataReserved", err)
	}
}

func TestDecodeMetadataRejectsWrongPageType(t *testing.T) {
	page, err := FormatPage(0, PageTypeData, 0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = DecodeMetadata(page)

	if !errors.Is(err, ErrInvalidPageType) {
		t.Fatalf("error = %v, want ErrInvalidPageType", err)
	}
}

func TestDecodeMetadataRejectsCorruptedChecksum(t *testing.T) {
	page, err := EncodeMetadata(NewMetadata())
	if err != nil {
		t.Fatal(err)
	}

	page[PagePayloadOffset+MetadataMagicOffset] ^= 0xFF

	_, err = DecodeMetadata(page)

	if !errors.Is(err, ErrPageChecksum) {
		t.Fatalf("error = %v, want ErrPageChecksum", err)
	}
}

func TestMetadataBitmapCapacity(t *testing.T) {
	if MetadataBitmapCapacity != MetadataBitmapBytes*8 {
		t.Fatalf("capacity = %d, want %d",
			MetadataBitmapCapacity, MetadataBitmapBytes*8)
	}

	if MaxDatabasePages != MetadataBitmapCapacity+1 {
		t.Fatalf("maximum pages = %d, want %d",
			MaxDatabasePages, MetadataBitmapCapacity+1)
	}
}
