package ne_test

import (
	"path/filepath"
	"testing"
	"vb3dec/pkg/ne"
)

func TestParseFFExe(t *testing.T) {
	path := filepath.Join("..", "..", "test_input", "FF.EXE")
	f, err := ne.Open(path)
	if err != nil {
		t.Fatalf("Failed to open FF.EXE: %v", err)
	}

	if string(f.Header.Magic[:]) != "NE" {
		t.Fatalf("Expected magic NE, got %s", string(f.Header.Magic[:]))
	}

	if f.Header.SegmentCount != 17 {
		t.Errorf("Expected 17 segments, got %d", f.Header.SegmentCount)
	}

	if f.SectorSize != 256 {
		t.Errorf("Expected sector size 256, got %d", f.SectorSize)
	}

	if len(f.Segments) != 17 {
		t.Fatalf("Expected 17 segments parsed, got %d", len(f.Segments))
	}

	// Segment 1 verification
	seg1 := f.Segments[0]
	if seg1.Index != 1 {
		t.Errorf("Expected seg 1 index 1, got %d", seg1.Index)
	}
	if seg1.FileOffset != 0x900 {
		t.Errorf("Expected seg 1 offset 0x900, got 0x%X", seg1.FileOffset)
	}
	if seg1.FileLength != 25 {
		t.Errorf("Expected seg 1 length 25, got %d", seg1.FileLength)
	}

	// Segment 2 verification (unallocated data segment)
	seg2 := f.Segments[1]
	if seg2.FileOffset != 0 || seg2.FileLength != 0 {
		t.Errorf("Expected seg 2 to have offset 0 and length 0, got offset 0x%X, length %d", seg2.FileOffset, seg2.FileLength)
	}
	if seg2.Flags&ne.SegFlagData == 0 {
		t.Errorf("Expected seg 2 to be DATA")
	}

	// SegmentAt lookup
	sAt := f.SegmentAt(0x00004BD6)
	if sAt == nil || sAt.Index != 4 {
		t.Errorf("Expected offset 0x4BD6 in Segment 4, got %v", sAt)
	}

	sAt = f.SegmentAt(0x10) // in MZ header
	if sAt != nil {
		t.Errorf("Expected offset 0x10 to not be in any segment, got Segment %d", sAt.Index)
	}
}

func TestParseFFDll(t *testing.T) {
	path := filepath.Join("..", "..", "test_input", "FF.DLL")
	f, err := ne.Open(path)
	if err != nil {
		t.Fatalf("Failed to open FF.DLL: %v", err)
	}

	if string(f.Header.Magic[:]) != "NE" {
		t.Fatalf("Expected magic NE, got %s", string(f.Header.Magic[:]))
	}

	if f.Header.SegmentCount != 2 {
		t.Errorf("Expected 2 segments, got %d", f.Header.SegmentCount)
	}

	if f.SectorSize != 16 {
		t.Errorf("Expected sector size 16, got %d", f.SectorSize)
	}

	seg1 := f.Segments[0]
	if seg1.FileOffset != 0x1E0 || seg1.FileLength != 2200 {
		t.Errorf("Expected seg 1 offset 0x1E0, len 2200, got offset 0x%X, len %d", seg1.FileOffset, seg1.FileLength)
	}

	seg2 := f.Segments[1]
	if seg2.FileOffset != 0xB80 || seg2.FileLength != 344 {
		t.Errorf("Expected seg 2 offset 0xB80, len 344, got offset 0x%X, len %d", seg2.FileOffset, seg2.FileLength)
	}
}
