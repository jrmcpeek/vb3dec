// Package vbx reads Visual Basic 3 control models (the Control Development Kit
// MODEL and PROPINFO structures) from 16-bit NE modules.
//
// Every VB3 control class, whether built into VBRUN300.DLL or supplied by a
// custom .VBX library, is described by a MODEL whose npproplist is an ordered,
// NULL-terminated array of property references. A property ID in a compiled
// form stream is an index into that array, and the referenced property's data
// type determines how its value is serialized. Standard properties are
// referenced by the sentinel values ~0..~42 (0xFFFF..0xFFD5) and resolve
// through StdProps.
package vbx

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"vb3dec/pkg/ne"
)

// DataType is a CDK property data type (the PF_datatype bits of PROPINFO.fl).
type DataType uint8

// CDK property data types.
const (
	DTHSZ     DataType = 1  // String handle
	DTShort   DataType = 2  // 16-bit integer
	DTLong    DataType = 3  // 32-bit integer
	DTBool    DataType = 4  // Boolean
	DTColor   DataType = 5  // RGB color
	DTEnum    DataType = 6  // Enumerated value
	DTReal    DataType = 7  // Single-precision float
	DTXPos    DataType = 8  // Horizontal position in twips
	DTXSize   DataType = 9  // Width in twips
	DTYPos    DataType = 10 // Vertical position in twips
	DTYSize   DataType = 11 // Height in twips
	DTPicture DataType = 12 // Picture
)

// VB3-internal pseudo types used by standard properties that are never
// serialized as ordinary property values.
const (
	DTIndex  DataType = 61 // Control array Index
	DTName   DataType = 62 // Control Name (stored in the record header)
	DTParent DataType = 63 // Parent form reference
)

// Standard property indices (the n in the ~n sentinel) used by the
// serialization rules.
const (
	StdLeft     = 5  // Left; Left, Top, Width and Height are serialized together
	StdFontName = 13 // FontName; the font properties are serialized together
)

// Prop describes one entry of a control's property list.
type Prop struct {
	Name  string
	Flags uint32 // PROPINFO.fl
	Std   int    // Standard property index, or -1 for a control-specific property
}

// Type returns the property's data type.
func (p Prop) Type() DataType {
	return DataType(p.Flags & 0x7F)
}

// Event describes one entry of a control's event list.
type Event struct {
	Name    string
	Params  int    // EVENTINFO.cParms
	Profile string // Parameter declarations (EVENTINFO.npszParmProf), if provided
	Std     int    // Standard event index, or -1 for a control-specific event
}

// Model describes one registered control class.
type Model struct {
	Version    uint16  // MODEL.usVersion (VB_VERSION the model targets)
	DefCtlName string  // Default control name, e.g. "Command3D"
	ClassName  string  // Class name, which is the type name stored in form streams
	Props      []Prop  // Property list in property ID order
	Events     []Event // Event list in event slot order
}

// Model field offsets within the CDK MODEL structure.
const (
	modelDefCtlNameOff = 0x14
	modelClassNameOff  = 0x16
	modelPropListOff   = 0x1A
	modelEventListOff  = 0x1C
	modelMinSize       = 0x20
)

// maxVBVersion is the newest model version a VB3 form can reference.
const maxVBVersion = 0x0300

// ParseFile opens a VBX (or any NE module) and returns its VB3 control models.
func ParseFile(path string) ([]*Model, error) {
	f, err := ne.Open(path)
	if err != nil {
		return nil, err
	}
	return ParseModels(f)
}

// ParseModels returns the control models registered by an NE module. When a
// module registers several versions of the same class (for VB1, VB2 and VB3),
// only the newest version usable by VB3 is returned.
func ParseModels(f *ne.File) ([]*Model, error) {
	best := make(map[string]*Model)
	for i := range f.Segments {
		seg := &f.Segments[i]
		if seg.Flags&ne.SegFlagData == 0 || seg.FileLength == 0 {
			continue
		}
		data, err := f.ReadSegmentData(seg)
		if err != nil {
			return nil, fmt.Errorf("reading data segment %d: %w", seg.Index, err)
		}
		for _, m := range ScanSegment(data, StdProps, StdEvents, true) {
			if !isVBXModelVersion(m.Version) {
				continue
			}
			key := strings.ToLower(m.ClassName)
			if prev, ok := best[key]; !ok || m.Version > prev.Version {
				best[key] = m
			}
		}
	}
	models := make([]*Model, 0, len(best))
	for _, m := range best {
		models = append(models, m)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ClassName < models[j].ClassName })
	return models, nil
}

func isVBXModelVersion(v uint16) bool {
	return v == 0x0100 || v == 0x0200 || v == 0x0300
}

// ScanSegment searches a data segment for MODEL structures. A candidate is
// accepted when its default control name is an identifier and its property
// list is a non-empty, NULL-terminated array of valid property references.
// requireClass additionally requires the class name to be an identifier;
// VBRUN300.DLL's built-in models store a class atom there instead. Standard
// property and event references resolve through std and stdEvents.
func ScanSegment(seg []byte, std []Prop, stdEvents []Event, requireClass bool) []*Model {
	var models []*Model
	for m := 0; m+modelMinSize <= len(seg); m++ {
		defName, ok := identifierAt(seg, word(seg, m+modelDefCtlNameOff))
		if !ok {
			continue
		}
		className, ok := identifierAt(seg, word(seg, m+modelClassNameOff))
		if !ok {
			if requireClass {
				continue
			}
			className = ""
		}
		props, ok := propList(seg, word(seg, m+modelPropListOff), std)
		if !ok || len(props) == 0 {
			continue
		}
		events, _ := eventList(seg, word(seg, m+modelEventListOff), stdEvents)
		models = append(models, &Model{
			Version:    word(seg, m),
			DefCtlName: defName,
			ClassName:  className,
			Props:      props,
			Events:     events,
		})
	}
	return models
}

// propList resolves a NULL-terminated array of property references.
func propList(seg []byte, off uint16, std []Prop) ([]Prop, bool) {
	var props []Prop
	for pos := int(off); pos+2 <= len(seg); pos += 2 {
		ref := word(seg, pos)
		if ref == 0 {
			return props, true
		}
		if idx := int(0xFFFF - ref); idx < len(std) {
			props = append(props, std[idx])
			continue
		}
		p, ok := PropInfoAt(seg, ref)
		if !ok {
			return nil, false
		}
		props = append(props, p)
		if len(props) > 255 {
			return nil, false
		}
	}
	return nil, false
}

// eventList resolves a NULL-terminated array of event references.
func eventList(seg []byte, off uint16, std []Event) ([]Event, bool) {
	var events []Event
	for pos := int(off); off != 0 && pos+2 <= len(seg); pos += 2 {
		ref := word(seg, pos)
		if ref == 0 {
			return events, true
		}
		if idx := int(0xFFFF - ref); idx < len(std) {
			events = append(events, std[idx])
			continue
		}
		e, ok := EventInfoAt(seg, ref)
		if !ok || len(events) > 255 {
			return nil, false
		}
		events = append(events, e)
	}
	return nil, false
}

// EventInfoAt decodes an EVENTINFO structure: the name pointer, parameter
// count, parameter size in words, parameter type list and, for custom
// controls, the parameter profile string.
func EventInfoAt(seg []byte, off uint16) (Event, bool) {
	if int(off)+10 > len(seg) {
		return Event{}, false
	}
	name, ok := identifierAt(seg, word(seg, int(off)))
	if !ok {
		return Event{}, false
	}
	e := Event{Name: name, Params: int(word(seg, int(off)+2)), Std: -1}
	if e.Params > 16 {
		return Event{}, false
	}
	if e.Params > 0 {
		if profile, ok := cStringAt(seg, word(seg, int(off)+8)); ok && strings.Contains(profile, " As ") {
			e.Profile = profile
		}
	}
	return e, true
}

// PropInfoAt decodes the leading fields of a PROPINFO structure: the near
// pointer to the property name followed by the 32-bit flags.
func PropInfoAt(seg []byte, off uint16) (Prop, bool) {
	if int(off)+6 > len(seg) {
		return Prop{}, false
	}
	name, ok := cStringAt(seg, word(seg, int(off)))
	if !ok {
		return Prop{}, false
	}
	return Prop{
		Name:  name,
		Flags: binary.LittleEndian.Uint32(seg[off+2 : off+6]),
		Std:   -1,
	}, true
}

func word(seg []byte, off int) uint16 {
	if off < 0 || off+2 > len(seg) {
		return 0
	}
	return binary.LittleEndian.Uint16(seg[off : off+2])
}

// cStringAt returns the printable NUL-terminated string at off.
func cStringAt(seg []byte, off uint16) (string, bool) {
	const maxLen = 64
	start := int(off)
	if start == 0 || start >= len(seg) {
		return "", false
	}
	for i := start; i < len(seg) && i-start <= maxLen; i++ {
		c := seg[i]
		if c == 0 {
			if i == start {
				return "", false
			}
			return string(seg[start:i]), true
		}
		if c < 0x20 || c > 0x7E {
			return "", false
		}
	}
	return "", false
}

// identifierAt returns the NUL-terminated Basic identifier at off.
func identifierAt(seg []byte, off uint16) (string, bool) {
	s, ok := cStringAt(seg, off)
	if !ok {
		return "", false
	}
	for i, c := range s {
		isAlpha := c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
		if !isAlpha && (i == 0 || c < '0' || c > '9') {
			return "", false
		}
	}
	return s, true
}
