package ne

import (
	"encoding/binary"
	"fmt"
)

// Standard Windows NE Resource Types (when high bit is set)
const (
	ResTypeCursor      = 0x8001
	ResTypeBitmap      = 0x8002
	ResTypeIcon        = 0x8003
	ResTypeMenu        = 0x8004
	ResTypeDialog      = 0x8005
	ResTypeString      = 0x8006
	ResTypeFontDir     = 0x8007
	ResTypeFont        = 0x8008
	ResTypeAccelerator = 0x8009
	ResTypeRCData      = 0x800A
	ResTypeGroupCursor = 0x800C
	ResTypeGroupIcon   = 0x800E
	ResTypeVersion     = 0x8010
)

// ResourceTypeName returns a friendly name for standard resource types.
func ResourceTypeName(typeID uint16) string {
	switch typeID {
	case ResTypeCursor:
		return "RT_CURSOR"
	case ResTypeBitmap:
		return "RT_BITMAP"
	case ResTypeIcon:
		return "RT_ICON"
	case ResTypeMenu:
		return "RT_MENU"
	case ResTypeDialog:
		return "RT_DIALOG"
	case ResTypeString:
		return "RT_STRING"
	case ResTypeFontDir:
		return "RT_FONTDIR"
	case ResTypeFont:
		return "RT_FONT"
	case ResTypeAccelerator:
		return "RT_ACCELERATOR"
	case ResTypeRCData:
		return "RT_RCDATA"
	case ResTypeGroupCursor:
		return "RT_GROUP_CURSOR"
	case ResTypeGroupIcon:
		return "RT_GROUP_ICON"
	case ResTypeVersion:
		return "RT_VERSION"
	default:
		if typeID&0x8000 != 0 {
			return fmt.Sprintf("TYPE_0x%04X", typeID)
		}
		return "NAMED_TYPE"
	}
}

// ResourceEntry represents an individual resource in the NE Resource Table.
type ResourceEntry struct {
	TypeID    uint16 // Resource type ID (e.g. 0x800A for RT_RCDATA)
	TypeName  string // Friendly type name
	ID        uint16 // Resource ID (e.g. 1, 4, etc.)
	RawID     uint16 // Raw rnID value from table
	IsInteger bool   // True if resource ID was integer (rnID & 0x8000 != 0)
	Name      string // Named resource string if not an integer ID
	Offset    uint32 // Absolute file offset in bytes
	Length    uint32 // Length in bytes
	RawOffset uint16 // Sector offset in alignment units
	RawLength uint16 // Sector length in alignment units
	Flags     uint16 // Resource attribute flags
}

// ResourceType groups resources sharing the same type.
type ResourceType struct {
	TypeID    uint16
	TypeName  string
	Resources []ResourceEntry
}

// parseResourceTable parses the NE Resource Table from file data.
func parseResourceTable(data []byte, neOffset uint32, hdr *Header) ([]ResourceType, error) {
	if hdr.ResourceTableOffset == hdr.ResidentNameTableOffset {
		return nil, nil
	}

	resTableOffset := neOffset + uint32(hdr.ResourceTableOffset)
	if int(resTableOffset)+2 > len(data) {
		return nil, fmt.Errorf("resource table offset 0x%X exceeds file size", resTableOffset)
	}

	alignShift := binary.LittleEndian.Uint16(data[resTableOffset : resTableOffset+2])
	resAlign := uint32(1) << alignShift

	readPascalString := func(offset uint32) string {
		if int(offset) >= len(data) {
			return ""
		}
		strLen := int(data[offset])
		if int(offset)+1+strLen > len(data) {
			return ""
		}
		return string(data[offset+1 : offset+1+uint32(strLen)])
	}

	pos := resTableOffset + 2
	var types []ResourceType

	for {
		if int(pos)+8 > len(data) {
			break
		}

		typeID := binary.LittleEndian.Uint16(data[pos : pos+2])
		if typeID == 0 {
			break // 0 indicates end of resource table type list
		}

		count := binary.LittleEndian.Uint16(data[pos+2 : pos+4])
		pos += 8 // skip typeID(2), count(2), reserved(4)

		var typeName string
		if typeID&0x8000 != 0 {
			typeName = ResourceTypeName(typeID)
		} else {
			typeName = readPascalString(resTableOffset + uint32(typeID))
			if typeName == "" {
				typeName = fmt.Sprintf("NAME_AT_0x%X", typeID)
			}
		}

		rt := ResourceType{
			TypeID:   typeID,
			TypeName: typeName,
		}

		for i := 0; i < int(count); i++ {
			if int(pos)+12 > len(data) {
				return nil, fmt.Errorf("resource entry %d of type 0x%04X exceeds file size", i, typeID)
			}

			rnOffset := binary.LittleEndian.Uint16(data[pos : pos+2])
			rnLength := binary.LittleEndian.Uint16(data[pos+2 : pos+4])
			rnFlags := binary.LittleEndian.Uint16(data[pos+4 : pos+6])
			rnID := binary.LittleEndian.Uint16(data[pos+6 : pos+8])
			pos += 12 // rnOffset(2), rnLength(2), rnFlags(2), rnID(2), rnHandle(4)

			var fileOffset uint32
			var fileLen uint32
			if rnOffset != 0 {
				fileOffset = uint32(rnOffset) * resAlign
				fileLen = uint32(rnLength) * resAlign
			}

			var id uint16
			var isInteger bool
			var name string

			if rnID&0x8000 != 0 {
				isInteger = true
				id = rnID & 0x7FFF
			} else {
				isInteger = false
				id = rnID
				name = readPascalString(resTableOffset + uint32(rnID))
			}

			rt.Resources = append(rt.Resources, ResourceEntry{
				TypeID:    typeID,
				TypeName:  typeName,
				ID:        id,
				RawID:     rnID,
				IsInteger: isInteger,
				Name:      name,
				Offset:    fileOffset,
				Length:    fileLen,
				RawOffset: rnOffset,
				RawLength: rnLength,
				Flags:     rnFlags,
			})
		}

		types = append(types, rt)
	}

	return types, nil
}

// FindResource locates a resource by type ID and resource ID.
// typeID may be specified with or without 0x8000 bit (e.g. 0x000A or 0x800A).
// resID is the integer resource ID (e.g. 1, 4, etc.).
func (f *File) FindResource(typeID, resID uint16) (*ResourceEntry, error) {
	normType := typeID
	if normType < 0x8000 {
		normType |= 0x8000
	}

	for _, rt := range f.Resources {
		if rt.TypeID == normType || rt.TypeID == typeID {
			for i := range rt.Resources {
				if rt.Resources[i].ID == resID || rt.Resources[i].RawID == resID {
					return &rt.Resources[i], nil
				}
			}
		}
	}
	return nil, fmt.Errorf("resource type 0x%04X ID %d not found", typeID, resID)
}

// ResourcesByType returns all resource entries matching the given type ID.
func (f *File) ResourcesByType(typeID uint16) []ResourceEntry {
	normType := typeID
	if normType < 0x8000 {
		normType |= 0x8000
	}

	var results []ResourceEntry
	for _, rt := range f.Resources {
		if rt.TypeID == normType || rt.TypeID == typeID {
			results = append(results, rt.Resources...)
		}
	}
	return results
}

// ReadResourceData returns the raw byte slice for a resource entry.
func (f *File) ReadResourceData(entry *ResourceEntry) ([]byte, error) {
	if entry.Offset == 0 {
		return nil, fmt.Errorf("resource (type 0x%04X, ID %d) has no file data (offset 0)", entry.TypeID, entry.ID)
	}
	end := entry.Offset + entry.Length
	if int(end) > len(f.data) {
		return nil, fmt.Errorf("resource data extends beyond EOF (%d > %d)", end, len(f.data))
	}
	return f.data[entry.Offset:end], nil
}
