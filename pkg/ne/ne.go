package ne

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Magic numbers for MZ and NE headers.
const (
	DOSMagic = 0x5A4D // "MZ"
	NEMagic  = 0x454E // "NE"
)

// Segment Flag masks.
const (
	SegFlagData        = 0x0001 // 0 = CODE, 1 = DATA
	SegFlagAllocated   = 0x0002 // Allocated
	SegFlagLoaded      = 0x0004 // Loaded
	SegFlagIterated    = 0x0008 // Iterated
	SegFlagMoveable    = 0x0010 // 0 = FIXED, 1 = MOVEABLE
	SegFlagPure        = 0x0020 // Pure / Shareable
	SegFlagPreload     = 0x0040 // 0 = LOADONCALL, 1 = PRELOAD
	SegFlagExecuteOnly = 0x0080 // Execute-only (code segment) or Read-only (data segment)
	SegFlagRelocInfo   = 0x0100 // Segment has relocation records following data
	SegFlagDebugInfo   = 0x0200 // Contains debugging information
	SegFlagDPL         = 0x0C00 // 286 privilege level (DPL)
	SegFlagDiscardable = 0x1000 // Discardable
)

// Header represents the 64-byte New Executable (NE) header.
type Header struct {
	Magic                   [2]byte // 0x00: 'N', 'E'
	LinkerMajor             byte    // 0x02: Linker major version
	LinkerMinor             byte    // 0x03: Linker minor version
	EntryTableOffset        uint16  // 0x04: Offset of Entry Table relative to NE header
	EntryTableLength        uint16  // 0x06: Length of Entry Table in bytes
	CRC32                   uint32  // 0x08: File load-time CRC
	ModuleFlags             uint16  // 0x0C: Module flags
	AutoDataSegIndex        uint16  // 0x0E: Segment number of automatic data segment
	InitHeapSize            uint16  // 0x10: Initial local heap size
	InitStackSize           uint16  // 0x12: Initial stack size
	EntryIP                 uint16  // 0x14: Initial IP (entry point)
	EntryCS                 uint16  // 0x16: Initial CS (entry point segment index, 1-based)
	InitSP                  uint16  // 0x18: Initial SP
	InitSS                  uint16  // 0x1A: Initial SS (segment index, 1-based)
	SegmentCount            uint16  // 0x1C: Number of segments in Segment Table
	ModuleRefCount          uint16  // 0x1E: Number of referenced modules
	NonResNameTableLength   uint16  // 0x20: Length of non-resident names table in bytes
	SegmentTableOffset      uint16  // 0x22: Offset of Segment Table relative to NE header
	ResourceTableOffset     uint16  // 0x24: Offset of Resource Table relative to NE header
	ResidentNameTableOffset uint16  // 0x26: Offset of Resident Names Table relative to NE header
	ModuleRefTableOffset    uint16  // 0x28: Offset of Module Reference Table relative to NE header
	ImportedNamesOffset     uint16  // 0x2A: Offset of Imported Names Table relative to NE header
	NonResNameTableOffset   uint32  // 0x2C: File offset of Non-Resident Names Table
	MovableEntryCount       uint16  // 0x30: Number of movable entry points
	AlignShiftCount         uint16  // 0x32: Alignment shift count (sector size = 1 << AlignShiftCount)
	ResourceCount           uint16  // 0x34: Number of resource entries
	TargetOS                byte    // 0x36: Target operating system
	OSFlags                 byte    // 0x37: Other OS flags
	FastLoadOffset          uint16  // 0x38: Fastload area offset (sectors)
	FastLoadLength          uint16  // 0x39: Fastload area length (sectors)
	Reserved                uint16  // 0x3C: Reserved
	ExpectedWindowsVersion  uint16  // 0x3E: Expected Windows version (minor.major)
}

// rawNEHeader matches the exact 64-byte layout on disk.
type rawNEHeader struct {
	Magic                   [2]byte
	LinkerMajor             byte
	LinkerMinor             byte
	EntryTableOffset        uint16
	EntryTableLength        uint16
	CRC32                   uint32
	ModuleFlags             uint16
	AutoDataSegIndex        uint16
	InitHeapSize            uint16
	InitStackSize           uint16
	EntryIP                 uint16
	EntryCS                 uint16
	InitSP                  uint16
	InitSS                  uint16
	SegmentCount            uint16
	ModuleRefCount          uint16
	NonResNameTableLength   uint16
	SegmentTableOffset      uint16
	ResourceTableOffset     uint16
	ResidentNameTableOffset uint16
	ModuleRefTableOffset    uint16
	ImportedNamesOffset     uint16
	NonResNameTableOffset   uint32
	MovableEntryCount       uint16
	AlignShiftCount         uint16
	ResourceCount           uint16
	TargetOS                byte
	OSFlags                 byte
	FastLoadOffset          uint16
	FastLoadLength          uint16
	Reserved                uint16
	ExpectedWindowsVersion  uint16
}

// rawSegmentEntry matches the 8-byte entry in the NE Segment Table.
type rawSegmentEntry struct {
	SectorOffset uint16
	Length       uint16
	Flags        uint16
	MinAlloc     uint16
}

// Segment represents an entry in the Segment Table with resolved file offset and length.
type Segment struct {
	Index        int    // 1-based segment index
	SectorOffset uint16 // Sector offset in file (0 if segment has no file data)
	Length       uint16 // Stored length in file (0 means 65536 if SectorOffset != 0)
	Flags        uint16 // Segment attribute flags
	MinAlloc     uint16 // Minimum allocation size (0 means 65536)

	// Computed fields:
	FileOffset   uint32 // Absolute byte offset in the file
	FileLength   uint32 // Actual byte length in the file
	MinAllocSize uint32 // Minimum allocation size in bytes
}

// FlagString returns a human-readable list of attributes for the segment.
func (s *Segment) FlagString() string {
	var parts []string

	if s.Flags&SegFlagData != 0 {
		parts = append(parts, "DATA")
	} else {
		parts = append(parts, "CODE")
	}

	if s.Flags&SegFlagMoveable != 0 {
		parts = append(parts, "MOVEABLE")
	} else {
		parts = append(parts, "FIXED")
	}

	if s.Flags&SegFlagPure != 0 {
		parts = append(parts, "PURE")
	}

	if s.Flags&SegFlagPreload != 0 {
		parts = append(parts, "PRELOAD")
	} else {
		parts = append(parts, "LOADONCALL")
	}

	if s.Flags&SegFlagExecuteOnly != 0 {
		if s.Flags&SegFlagData != 0 {
			parts = append(parts, "READONLY")
		} else {
			parts = append(parts, "EXECUTEONLY")
		}
	}

	if s.Flags&SegFlagRelocInfo != 0 {
		parts = append(parts, "RELOCINFO")
	}

	if s.Flags&SegFlagDiscardable != 0 {
		parts = append(parts, "DISCARDABLE")
	}

	return strings.Join(parts, ", ")
}

// Contains returns true if the specified file offset falls within this segment.
func (s *Segment) Contains(fileOffset uint32) bool {
	if s.FileOffset == 0 && s.FileLength == 0 {
		return false
	}
	return fileOffset >= s.FileOffset && fileOffset < (s.FileOffset+s.FileLength)
}

// File represents a parsed 16-bit New Executable file.
type File struct {
	NEOffset   uint32
	SectorSize uint32
	Header     Header
	Segments   []Segment
	Resources  []ResourceType
	data       []byte
}

// Open reads and parses a New Executable from the given file path.
func Open(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	return Parse(data)
}

// Parse parses a New Executable from byte slice data.
func Parse(data []byte) (*File, error) {
	if len(data) < 0x40 {
		return nil, errors.New("file too small for DOS header")
	}

	// Verify DOS MZ header
	if data[0] != 'M' || data[1] != 'Z' {
		return nil, errors.New("invalid DOS signature (expected MZ)")
	}

	// Offset to NE header is at offset 0x3C in DOS header
	neOffset := binary.LittleEndian.Uint32(data[0x3C:0x40])
	if int(neOffset)+64 > len(data) {
		return nil, fmt.Errorf("NE header offset 0x%X is beyond file boundary", neOffset)
	}

	// Verify NE header magic
	if data[neOffset] != 'N' || data[neOffset+1] != 'E' {
		return nil, fmt.Errorf("invalid NE signature at 0x%X (expected NE, got %c%c)",
			neOffset, data[neOffset], data[neOffset+1])
	}

	var raw rawNEHeader
	if err := binary.Read(bytes.NewReader(data[neOffset:neOffset+64]), binary.LittleEndian, &raw); err != nil {
		return nil, fmt.Errorf("reading NE header: %w", err)
	}

	hdr := Header(raw)

	// Sector size calculation: 1 << AlignShiftCount (default to 512 if 0)
	sectorSize := uint32(1) << hdr.AlignShiftCount
	if hdr.AlignShiftCount == 0 {
		sectorSize = 512
	}

	// Segment table offset is relative to the start of NE header
	segTableOffset := neOffset + uint32(hdr.SegmentTableOffset)
	segTableSize := int(hdr.SegmentCount) * 8
	if int(segTableOffset)+segTableSize > len(data) {
		return nil, fmt.Errorf("segment table at 0x%X (size %d) exceeds file size", segTableOffset, segTableSize)
	}

	segments := make([]Segment, hdr.SegmentCount)
	for i := 0; i < int(hdr.SegmentCount); i++ {
		entryOffset := segTableOffset + uint32(i*8)
		var entry rawSegmentEntry
		if err := binary.Read(bytes.NewReader(data[entryOffset:entryOffset+8]), binary.LittleEndian, &entry); err != nil {
			return nil, fmt.Errorf("reading segment %d entry: %w", i+1, err)
		}

		var fileOffset uint32
		if entry.SectorOffset != 0 {
			fileOffset = uint32(entry.SectorOffset) * sectorSize
		}

		var fileLen uint32
		if entry.Length != 0 {
			fileLen = uint32(entry.Length)
		} else if entry.SectorOffset != 0 {
			// Length of 0 with non-zero sector offset means 64KB (65536 bytes)
			fileLen = 65536
		}

		var minAlloc uint32
		if entry.MinAlloc != 0 {
			minAlloc = uint32(entry.MinAlloc)
		} else {
			minAlloc = 65536
		}

		segments[i] = Segment{
			Index:        i + 1,
			SectorOffset: entry.SectorOffset,
			Length:       entry.Length,
			Flags:        entry.Flags,
			MinAlloc:     entry.MinAlloc,
			FileOffset:   fileOffset,
			FileLength:   fileLen,
			MinAllocSize: minAlloc,
		}
	}

	resources, err := parseResourceTable(data, neOffset, &hdr)
	if err != nil {
		return nil, fmt.Errorf("parsing resource table: %w", err)
	}

	return &File{
		NEOffset:   neOffset,
		SectorSize: sectorSize,
		Header:     hdr,
		Segments:   segments,
		Resources:  resources,
		data:       data,
	}, nil
}

// SegmentAt returns the segment containing the given file offset, or nil if none.
func (f *File) SegmentAt(fileOffset uint32) *Segment {
	for i := range f.Segments {
		if f.Segments[i].Contains(fileOffset) {
			return &f.Segments[i]
		}
	}
	return nil
}

// Bytes returns the raw underlying file bytes.
func (f *File) Bytes() []byte {
	return f.data
}

// ReadSegmentData returns the slice of bytes corresponding to the given segment.
func (f *File) ReadSegmentData(seg *Segment) ([]byte, error) {
	if seg.FileOffset == 0 && seg.FileLength == 0 {
		return nil, errors.New("segment has no file data")
	}
	end := seg.FileOffset + seg.FileLength
	if int(end) > len(f.data) {
		return nil, fmt.Errorf("segment %d data extends beyond EOF (%d > %d)", seg.Index, end, len(f.data))
	}
	return f.data[seg.FileOffset:end], nil
}
