package frm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"

	"vb3dec/pkg/win1252"
)

// DecodeFormStream decodes a raw binary form stream from an NE RCData resource.
func DecodeFormStream(stream []byte, formRef FormRef, nameMap map[int]string) (*Form, []byte, error) {
	if len(stream) < 9 {
		return nil, nil, fmt.Errorf("form stream too small (%d bytes)", len(stream))
	}

	if stream[0] != 0xFF || stream[1] != 0xCC || stream[2] != 0x2C {
		return nil, nil, fmt.Errorf("invalid form magic: expected FF CC 2C, got %02X %02X %02X",
			stream[0], stream[1], stream[2])
	}

	formNode := &ControlNode{
		ID:       0,
		Name:     formRef.FormName,
		TypeName: "Form",
	}

	form := &Form{
		Number:   formRef.Index,
		Name:     formRef.FormName,
		FileName: formRef.FileName,
		Root:     formNode,
	}

	pos := 9
	if pos+4 > len(stream) {
		return nil, nil, fmt.Errorf("form stream truncated at header")
	}

	// 1. Decode Form record
	formRecLen := int(binary.LittleEndian.Uint32(stream[pos:pos+4]) & 0x7FFFFFFF)
	if pos+formRecLen > len(stream) {
		return nil, nil, fmt.Errorf("form record exceeds stream size (%d > %d)", pos+formRecLen, len(stream))
	}
	formEndPos := pos + formRecLen
	p := pos + 5 // skip recLen (4) + ctrlID (1)

	// Form name if inline
	if p < formEndPos {
		fNameLen := int(stream[p])
		p++
		if fNameLen > 0 && p+fNameLen <= formEndPos {
			name := strings.TrimRight(string(stream[p:p+fNameLen]), "\x00")
			if name != "" {
				formNode.Name = name
				form.Name = name
			}
			p += fNameLen
		}
	}
	p++ // typeID (0x0D)

	// Pre-0xFF properties on Form (BorderStyle, ControlBox, MaxButton)
	borderStyle := -1
	controlBox := -1
	maxButton := -1

	for p < formEndPos && stream[p] != 0xFF {
		propID := stream[p]
		p++
		switch propID {
		case 0x22: // BorderStyle
			if p < formEndPos {
				borderStyle = int(stream[p])
				p++
			}
		case 0x26: // MaxButton
			if p < formEndPos {
				maxButton = int(int8(stream[p]))
				p++
			}
		case 0x28: // ControlBox
			if p < formEndPos {
				controlBox = int(int8(stream[p]))
				p++
			}
		default:
			p++
		}
	}

	if p < formEndPos && stream[p] == 0xFF {
		p++ // Skip 0xFF separator
	}

	var formProps []Property
	var formPictures [][]byte
	var formIcons [][]byte
	caption := ""
	linkTopic := ""
	var foreColor *uint32
	var clientLeft, clientTop, clientWidth, clientHeight int32
	hasClientBounds := false

	for p < formEndPos {
		if stream[p] == 0xFF {
			break // Reached event table
		}
		propID := stream[p]
		p++

		switch propID {
		case 0x00: // Caption
			if p < formEndPos {
				sLen := int(stream[p])
				p++
				if p+sLen <= formEndPos {
					caption = win1252.Decode(stream[p : p+sLen])
					p += sLen
				}
			}
		case 0x04: // ForeColor
			if p+4 <= formEndPos {
				fc := binary.LittleEndian.Uint32(stream[p : p+4])
				foreColor = &fc
				p += 4
			}
		case 0x05: // Coordinates
			if p+16 <= formEndPos {
				clientLeft = int32(binary.LittleEndian.Uint32(stream[p : p+4]))
				clientTop = int32(binary.LittleEndian.Uint32(stream[p+4 : p+8]))
				clientWidth = int32(binary.LittleEndian.Uint32(stream[p+8 : p+12]))
				clientHeight = int32(binary.LittleEndian.Uint32(stream[p+12 : p+16]))
				hasClientBounds = true
				p += 16
			}
		case 0x19: // ScaleMode
			p += 2
		case 0x42: // Flag byte
			p++
		case 0x21: // Picture
			if p+4 <= formEndPos {
				picLen := int(binary.LittleEndian.Uint32(stream[p : p+4]))
				p += 4
				if picLen > 0 && p+picLen <= formEndPos {
					picData := make([]byte, picLen)
					copy(picData, stream[p:p+picLen])
					formPictures = append(formPictures, picData)
					p += picLen
				}
			}
		case 0x23: // Icon
			if p+4 <= formEndPos {
				iconLen := binary.LittleEndian.Uint32(stream[p : p+4])
				p += 4
				if iconLen != 0xFFFFFFFF && int(iconLen) > 0 && p+int(iconLen) <= formEndPos {
					iconData := make([]byte, iconLen)
					copy(iconData, stream[p:p+int(iconLen)])
					formIcons = append(formIcons, iconData)
					p += int(iconLen)
				}
			}
		case 0x24: // LinkTopic
			if p < formEndPos {
				sLen := int(stream[p])
				p++
				if p+sLen <= formEndPos {
					linkTopic = win1252.Decode(stream[p : p+sLen])
					p += sLen
				}
			}
		case 0x25, 0x2E: // LinkMode, Visible
			p++
		case 0x35, 0x36, 0x37, 0x38: // ClientLeft, ClientTop, ClientWidth, ClientHeight
			p += 4
		default:
			// Unhandled property, safety break
			break
		}
	}

	// Calculate outer window bounds from client bounds based on BorderStyle
	var outerLeft, outerTop, outerWidth, outerHeight int32
	if hasClientBounds {
		if borderStyle == 3 {
			// Fixed Double: border 60 twips, titlebar 450 twips
			outerLeft = clientLeft - 60
			outerTop = clientTop - 450
			outerWidth = clientWidth + 120
			outerHeight = clientHeight + 510
		} else if borderStyle == 1 {
			// Fixed Single: border 15 twips, titlebar 450 twips
			outerLeft = clientLeft - 15
			outerTop = clientTop - 450
			outerWidth = clientWidth + 30
			outerHeight = clientHeight + 480
		} else {
			outerLeft = clientLeft
			outerTop = clientTop
			outerWidth = clientWidth
			outerHeight = clientHeight
		}
	}

	// Buffer for FRX assets
	var frxBuf bytes.Buffer

	// Build Form properties
	if borderStyle >= 0 {
		formProps = append(formProps, Property{
			Name:    "BorderStyle",
			Value:   fmt.Sprintf("%d", borderStyle),
			Comment: FormatEnumComment("Form", "BorderStyle", borderStyle),
		})
	}
	if caption != "" {
		formProps = append(formProps, Property{
			Name:  "Caption",
			Value: win1252.QuoteVBString(caption),
		})
	}
	if hasClientBounds {
		formProps = append(formProps, Property{
			Name:  "ClientHeight",
			Value: fmt.Sprintf("%d", clientHeight),
		})
		formProps = append(formProps, Property{
			Name:  "ClientLeft",
			Value: fmt.Sprintf("%d", clientLeft),
		})
		formProps = append(formProps, Property{
			Name:  "ClientTop",
			Value: fmt.Sprintf("%d", clientTop),
		})
		formProps = append(formProps, Property{
			Name:  "ClientWidth",
			Value: fmt.Sprintf("%d", clientWidth),
		})
	}
	if controlBox >= 0 {
		formProps = append(formProps, Property{
			Name:    "ControlBox",
			Value:   fmt.Sprintf("%d", controlBox),
			Comment: FormatEnumComment("Form", "ControlBox", controlBox),
		})
	}
	if foreColor != nil {
		formProps = append(formProps, Property{
			Name:  "ForeColor",
			Value: fmt.Sprintf("&H%08X&", *foreColor),
		})
	}
	if hasClientBounds {
		formProps = append(formProps, Property{
			Name:  "Height",
			Value: fmt.Sprintf("%d", outerHeight),
		})
	}

	// Icon
	baseFRXName := strings.TrimSuffix(form.FileName, ".FRM")
	baseFRXName = strings.TrimSuffix(baseFRXName, ".frm")

	for _, iconBytes := range formIcons {
		offset := uint32(frxBuf.Len())
		// Write 4-byte length prefix + data
		var lenBuf [4]byte
		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(iconBytes)))
		frxBuf.Write(lenBuf[:])
		frxBuf.Write(iconBytes)

		form.FRXAssets = append(form.FRXAssets, FRXAsset{
			PropName: "Icon",
			Data:     iconBytes,
			Offset:   offset,
		})
		formProps = append(formProps, Property{
			Name:  "Icon",
			Value: fmt.Sprintf("%s.FRX:%04X", baseFRXName, offset),
		})
	}

	if hasClientBounds {
		formProps = append(formProps, Property{
			Name:  "Left",
			Value: fmt.Sprintf("%d", outerLeft),
		})
	}
	if linkTopic != "" {
		formProps = append(formProps, Property{
			Name:  "LinkTopic",
			Value: win1252.QuoteVBString(linkTopic),
		})
	}
	if maxButton >= 0 {
		formProps = append(formProps, Property{
			Name:    "MaxButton",
			Value:   fmt.Sprintf("%d", maxButton),
			Comment: FormatEnumComment("Form", "MaxButton", maxButton),
		})
	}

	// Picture
	for _, picBytes := range formPictures {
		offset := uint32(frxBuf.Len())
		var lenBuf [4]byte
		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(picBytes)))
		frxBuf.Write(lenBuf[:])
		frxBuf.Write(picBytes)

		form.FRXAssets = append(form.FRXAssets, FRXAsset{
			PropName: "Picture",
			Data:     picBytes,
			Offset:   offset,
		})
		formProps = append(formProps, Property{
			Name:  "Picture",
			Value: fmt.Sprintf("%s.FRX:%04X", baseFRXName, offset),
		})
	}

	if hasClientBounds {
		formProps = append(formProps, Property{
			Name:  "ScaleHeight",
			Value: fmt.Sprintf("%d", clientHeight),
		})
		formProps = append(formProps, Property{
			Name:  "ScaleWidth",
			Value: fmt.Sprintf("%d", clientWidth),
		})
		formProps = append(formProps, Property{
			Name:  "Top",
			Value: fmt.Sprintf("%d", outerTop),
		})
		formProps = append(formProps, Property{
			Name:  "Width",
			Value: fmt.Sprintf("%d", outerWidth),
		})
	}

	formNode.Properties = formProps
	pos = formEndPos

	// 2. Decode Child Controls & Container Stack
	containerStack := []*ControlNode{formNode}
	var lastNode *ControlNode

	for pos < len(stream) {
		tag := stream[pos]
		pos++

		if tag == 4 {
			// End of Form
			break
		}
		if tag == 2 {
			// End of Container
			if len(containerStack) > 1 {
				containerStack = containerStack[:len(containerStack)-1]
			}
			continue
		}
		if tag == 5 {
			// Sub-container start (children belong to lastNode)
			if lastNode != nil {
				containerStack = append(containerStack, lastNode)
			}
			continue
		}
		if tag != 1 && tag != 3 {
			// Unknown tag, break
			break
		}

		if pos+4 > len(stream) {
			return nil, nil, fmt.Errorf("control record header truncated at offset %d", pos)
		}

		rawLen := binary.LittleEndian.Uint32(stream[pos : pos+4])
		hasFlags := (rawLen & 0x80000000) != 0
		recLen := int(rawLen & 0x7FFFFFFF)
		if recLen < 5 {
			return nil, nil, fmt.Errorf("control record at offset %d has invalid length %d", pos, recLen)
		}
		if pos+recLen > len(stream) {
			return nil, nil, fmt.Errorf("control record at offset %d exceeds stream size", pos)
		}
		ctrlID := int(stream[pos+4])
		ctrlEndPos := pos + recLen

		cp := pos + 5
		if hasFlags {
			cp += 2 // Control array index
		}
		if cp > ctrlEndPos {
			return nil, nil, fmt.Errorf("control record at offset %d is truncated before its name", pos)
		}

		var inlineName string
		if cp < ctrlEndPos {
			nLen := int(stream[cp])
			cp++
			if cp+nLen > ctrlEndPos {
				return nil, nil, fmt.Errorf("control record at offset %d has truncated name", pos)
			}
			if nLen > 0 {
				inlineName = strings.TrimRight(string(stream[cp:cp+nLen]), "\x00")
				cp += nLen
			}
		}

		if cp >= ctrlEndPos {
			return nil, nil, fmt.Errorf("control record at offset %d is missing its type", pos)
		}
		typeID := stream[cp]
		cp++
		var typeName string
		if typeID == 0xFF {
			if cp >= ctrlEndPos {
				return nil, nil, fmt.Errorf("control record at offset %d is missing its type name length", pos)
			}
			tLen := int(stream[cp])
			cp++
			if cp+tLen > ctrlEndPos {
				return nil, nil, fmt.Errorf("control record at offset %d has truncated type name", pos)
			}
			typeName = string(stream[cp : cp+tLen])
			cp += tLen
		} else {
			typeName = BuiltinControlTypeName(typeID)
		}

		// Control name resolution
		var ctrlName string
		if nameMap != nil && nameMap[ctrlID] != "" {
			ctrlName = nameMap[ctrlID]
		} else if inlineName != "" {
			ctrlName = inlineName
		} else {
			ctrlName = fmt.Sprintf("control%d", ctrlID)
		}

		node := &ControlNode{
			ID:       ctrlID,
			Name:     ctrlName,
			TypeName: typeName,
		}

		seenSeparator := false
		if cp < ctrlEndPos && stream[cp] == 0xFF {
			seenSeparator = true
			cp++
		}

		// Decode properties
		properties, err := decodeControlProperties(typeName, stream[cp:ctrlEndPos], baseFRXName, &frxBuf, form, seenSeparator)
		if err != nil {
			return nil, nil, fmt.Errorf("decode control %d: %w", ctrlID, err)
		}
		node.Properties = properties

		// Add to tree and flat list
		parent := containerStack[len(containerStack)-1]
		parent.Children = append(parent.Children, node)
		form.Controls = append(form.Controls, node)
		lastNode = node

		pos = ctrlEndPos
	}

	form.RawFRXData = frxBuf.Bytes()
	return form, form.RawFRXData, nil
}

func decodeControlProperties(typeName string, blob []byte, baseFRXName string, frxBuf *bytes.Buffer, form *Form, initialSeenSeparator bool) ([]Property, error) {
	var props []Property
	p := 0
	seenSeparator := initialSeenSeparator

	for p < len(blob) {
		if blob[p] == 0xFF {
			if !seenSeparator {
				seenSeparator = true
				p++
				continue
			}
			// Event table start
			break
		}
		propID := blob[p]
		p++

		switch typeName {
		case "Label":
			switch propID {
			case 0x00: // Caption
				value, err := readLengthPrefixedBytes(blob, &p)
				if err != nil {
					return nil, fmt.Errorf("Label caption: %w", err)
				}
				props = append(props, Property{Name: "Caption", Value: win1252.QuoteVBString(win1252.Decode(value))})
			case 0x03: // BackColor
				if p+4 <= len(blob) {
					c := binary.LittleEndian.Uint32(blob[p : p+4])
					props = append(props, Property{
						Name:  "BackColor",
						Value: fmt.Sprintf("&H%08X&", c),
					})
					p += 4
				}
			case 0x04: // ForeColor
				if p+4 <= len(blob) {
					c := binary.LittleEndian.Uint32(blob[p : p+4])
					props = append(props, Property{
						Name:  "ForeColor",
						Value: fmt.Sprintf("&H%08X&", c),
					})
					p += 4
				}
			case 0x05: // Coordinates
				if p+8 <= len(blob) {
					l := int16(binary.LittleEndian.Uint16(blob[p : p+2]))
					t := int16(binary.LittleEndian.Uint16(blob[p+2 : p+4]))
					w := int16(binary.LittleEndian.Uint16(blob[p+4 : p+6]))
					h := int16(binary.LittleEndian.Uint16(blob[p+6 : p+8]))
					props = append(props,
						Property{Name: "Height", Value: fmt.Sprintf("%d", h)},
						Property{Name: "Left", Value: fmt.Sprintf("%d", l)},
						Property{Name: "Top", Value: fmt.Sprintf("%d", t)},
						Property{Name: "Width", Value: fmt.Sprintf("%d", w)},
					)
					p += 8
				}
			case 0x0C: // Font
				var err error
				p, err = decodeFont(blob, p, &props)
				if err != nil {
					return nil, fmt.Errorf("Label font: %w", err)
				}
			case 0x12: // TabIndex
				if p+2 <= len(blob) {
					idx := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "TabIndex",
						Value: fmt.Sprintf("%d", idx),
					})
					p += 2
				}
			case 0x13: // BorderStyle
				if p < len(blob) {
					val := int(blob[p])
					props = append(props, Property{
						Name:    "BorderStyle",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("Label", "BorderStyle", val),
					})
					p++
				}
			case 0x14: // Alignment
				if p < len(blob) {
					val := int(blob[p])
					props = append(props, Property{
						Name:    "Alignment",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("Label", "Alignment", val),
					})
					p++
				}
			case 0x1F: // BackStyle
				if p < len(blob) {
					val := int(blob[p])
					props = append(props, Property{
						Name:    "BackStyle",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("Label", "BackStyle", val),
					})
					p++
				}
			default:
				p++
			}

		case "CommandButton":
			switch propID {
			case 0x00: // Caption
				value, err := readLengthPrefixedBytes(blob, &p)
				if err != nil {
					return nil, fmt.Errorf("CommandButton caption: %w", err)
				}
				props = append(props, Property{Name: "Caption", Value: win1252.QuoteVBString(win1252.Decode(value))})
			case 0x04: // Coordinates
				if p+8 <= len(blob) {
					l := int16(binary.LittleEndian.Uint16(blob[p : p+2]))
					t := int16(binary.LittleEndian.Uint16(blob[p+2 : p+4]))
					w := int16(binary.LittleEndian.Uint16(blob[p+4 : p+6]))
					h := int16(binary.LittleEndian.Uint16(blob[p+6 : p+8]))
					props = append(props,
						Property{Name: "Height", Value: fmt.Sprintf("%d", h)},
						Property{Name: "Left", Value: fmt.Sprintf("%d", l)},
						Property{Name: "Top", Value: fmt.Sprintf("%d", t)},
						Property{Name: "Width", Value: fmt.Sprintf("%d", w)},
					)
					p += 8
				}
			case 0x09: // Visible
				if p < len(blob) {
					val := int(int8(blob[p]))
					props = append(props, Property{
						Name:    "Visible",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("CommandButton", "Visible", val),
					})
					p++
				}
			case 0x0C: // Font
				var err error
				p, err = decodeFont(blob, p, &props)
				if err != nil {
					return nil, fmt.Errorf("CommandButton font: %w", err)
				}
			case 0x11: // TabIndex
				if p+2 <= len(blob) {
					idx := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "TabIndex",
						Value: fmt.Sprintf("%d", idx),
					})
					p += 2
				}
			default:
				p++
			}

		case "TextBox":
			switch propID {
			case 0x00, 0x0B: // Text
				value, err := readLengthPrefixedBytes(blob, &p)
				if err != nil {
					return nil, fmt.Errorf("TextBox text: %w", err)
				}
				props = append(props, Property{Name: "Text", Value: win1252.QuoteVBString(win1252.Decode(value))})
			case 0x04: // Coordinates
				if p+8 <= len(blob) {
					l := int16(binary.LittleEndian.Uint16(blob[p : p+2]))
					t := int16(binary.LittleEndian.Uint16(blob[p+2 : p+4]))
					w := int16(binary.LittleEndian.Uint16(blob[p+4 : p+6]))
					h := int16(binary.LittleEndian.Uint16(blob[p+6 : p+8]))
					props = append(props,
						Property{Name: "Height", Value: fmt.Sprintf("%d", h)},
						Property{Name: "Left", Value: fmt.Sprintf("%d", l)},
						Property{Name: "Top", Value: fmt.Sprintf("%d", t)},
						Property{Name: "Width", Value: fmt.Sprintf("%d", w)},
					)
					p += 8
				}
			case 0x09: // Visible
				if p < len(blob) {
					val := int(int8(blob[p]))
					props = append(props, Property{
						Name:    "Visible",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("TextBox", "Visible", val),
					})
					p++
				}
			case 0x0C: // Font
				var err error
				p, err = decodeFont(blob, p, &props)
				if err != nil {
					return nil, fmt.Errorf("TextBox font: %w", err)
				}
			case 0x11, 0x12: // TabIndex
				if p+2 <= len(blob) {
					idx := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "TabIndex",
						Value: fmt.Sprintf("%d", idx),
					})
					p += 2
				}
			case 0x13, 0x17: // MultiLine
				if p < len(blob) {
					val := int(int8(blob[p]))
					props = append(props, Property{
						Name:    "MultiLine",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("TextBox", "MultiLine", val),
					})
					p++
				}
			case 0x18: // TabStop
				if p < len(blob) {
					val := int(int8(blob[p]))
					props = append(props, Property{
						Name:    "TabStop",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("TextBox", "TabStop", val),
					})
					p++
				}
			default:
				p++
			}

		case "ComboBox":
			switch propID {
			case 0x04: // Coordinates
				if p+8 <= len(blob) {
					l := int16(binary.LittleEndian.Uint16(blob[p : p+2]))
					t := int16(binary.LittleEndian.Uint16(blob[p+2 : p+4]))
					w := int16(binary.LittleEndian.Uint16(blob[p+4 : p+6]))
					h := int16(binary.LittleEndian.Uint16(blob[p+6 : p+8]))
					props = append(props,
						Property{Name: "Height", Value: fmt.Sprintf("%d", h)},
						Property{Name: "Left", Value: fmt.Sprintf("%d", l)},
						Property{Name: "Top", Value: fmt.Sprintf("%d", t)},
						Property{Name: "Width", Value: fmt.Sprintf("%d", w)},
					)
					p += 8
				}
			case 0x0C: // Font
				var err error
				p, err = decodeFont(blob, p, &props)
				if err != nil {
					return nil, fmt.Errorf("ComboBox font: %w", err)
				}
			case 0x11, 0x12: // TabIndex
				if p+2 <= len(blob) {
					idx := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "TabIndex",
						Value: fmt.Sprintf("%d", idx),
					})
					p += 2
				}
			case 0x14, 0x1F: // Style
				if p < len(blob) {
					val := int(blob[p])
					props = append(props, Property{
						Name:    "Style",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("ComboBox", "Style", val),
					})
					p++
				}
			case 0x18, 0x1D: // TabStop
				if p < len(blob) {
					val := int(int8(blob[p]))
					props = append(props, Property{
						Name:    "TabStop",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("ComboBox", "TabStop", val),
					})
					p++
				}
			default:
				p++
			}

		case "ListBox":
			switch propID {
			case 0x04: // Coordinates
				if p+8 <= len(blob) {
					l := int16(binary.LittleEndian.Uint16(blob[p : p+2]))
					t := int16(binary.LittleEndian.Uint16(blob[p+2 : p+4]))
					w := int16(binary.LittleEndian.Uint16(blob[p+4 : p+6]))
					h := int16(binary.LittleEndian.Uint16(blob[p+6 : p+8]))
					props = append(props,
						Property{Name: "Height", Value: fmt.Sprintf("%d", h)},
						Property{Name: "Left", Value: fmt.Sprintf("%d", l)},
						Property{Name: "Top", Value: fmt.Sprintf("%d", t)},
						Property{Name: "Width", Value: fmt.Sprintf("%d", w)},
					)
					p += 8
				}
			case 0x0C: // Font
				var err error
				p, err = decodeFont(blob, p, &props)
				if err != nil {
					return nil, fmt.Errorf("ListBox font: %w", err)
				}
			case 0x11: // TabIndex
				if p+2 <= len(blob) {
					idx := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "TabIndex",
						Value: fmt.Sprintf("%d", idx),
					})
					p += 2
				}
			default:
				p++
			}

		case "Timer":
			switch propID {
			case 0x02: // Enabled
				if p < len(blob) {
					val := int(int8(blob[p]))
					props = append(props, Property{
						Name:    "Enabled",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("Timer", "Enabled", val),
					})
					p++
				}
			case 0x03: // Interval
				if p+4 <= len(blob) {
					val := binary.LittleEndian.Uint32(blob[p : p+4])
					props = append(props, Property{
						Name:  "Interval",
						Value: fmt.Sprintf("%d", val),
					})
					p += 4
				}
			case 0x07: // Left
				if p+4 <= len(blob) {
					val := binary.LittleEndian.Uint32(blob[p : p+4])
					props = append(props, Property{
						Name:  "Left",
						Value: fmt.Sprintf("%d", val),
					})
					p += 4
				}
			case 0x08: // Top
				if p+4 <= len(blob) {
					val := binary.LittleEndian.Uint32(blob[p : p+4])
					props = append(props, Property{
						Name:  "Top",
						Value: fmt.Sprintf("%d", val),
					})
					p += 4
				}
			default:
				p++
			}

		case "Image":
			switch propID {
			case 0x02: // Picture
				if p+4 <= len(blob) {
					picLen := int(binary.LittleEndian.Uint32(blob[p : p+4]))
					p += 4
					if picLen > 0 && p+picLen <= len(blob) {
						offset := uint32(frxBuf.Len())
						var lenBuf [4]byte
						binary.LittleEndian.PutUint32(lenBuf[:], uint32(picLen))
						frxBuf.Write(lenBuf[:])
						frxBuf.Write(blob[p : p+picLen])

						form.FRXAssets = append(form.FRXAssets, FRXAsset{
							PropName: "Picture",
							Data:     blob[p : p+picLen],
							Offset:   offset,
						})
						props = append(props, Property{
							Name:  "Picture",
							Value: fmt.Sprintf("%s.FRX:%04X", baseFRXName, offset),
						})
						p += picLen
					}
				}
			case 0x03, 0x04: // Coordinates
				if p+8 <= len(blob) {
					l := int16(binary.LittleEndian.Uint16(blob[p : p+2]))
					t := int16(binary.LittleEndian.Uint16(blob[p+2 : p+4]))
					w := int16(binary.LittleEndian.Uint16(blob[p+4 : p+6]))
					h := int16(binary.LittleEndian.Uint16(blob[p+6 : p+8]))
					props = append(props,
						Property{Name: "Height", Value: fmt.Sprintf("%d", h)},
						Property{Name: "Left", Value: fmt.Sprintf("%d", l)},
						Property{Name: "Top", Value: fmt.Sprintf("%d", t)},
						Property{Name: "Width", Value: fmt.Sprintf("%d", w)},
					)
					p += 8
				}
			case 0x08, 0x09: // Visible
				if p < len(blob) {
					val := int(int8(blob[p]))
					props = append(props, Property{
						Name:    "Visible",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("Image", "Visible", val),
					})
					p++
				}
			default:
				p++
			}

		case "OptionButton":
			switch propID {
			case 0x00: // Caption
				if p < len(blob) {
					sLen := int(blob[p])
					p++
					if p+sLen <= len(blob) {
						props = append(props, Property{
							Name:  "Caption",
							Value: win1252.QuoteVBString(win1252.Decode(blob[p : p+sLen])),
						})
						p += sLen
					}
				}
			case 0x03: // BackColor
				if p+4 <= len(blob) {
					c := binary.LittleEndian.Uint32(blob[p : p+4])
					props = append(props, Property{
						Name:  "BackColor",
						Value: fmt.Sprintf("&H%08X&", c),
					})
					p += 4
				}
			case 0x04, 0x05: // Coordinates
				if p+8 <= len(blob) {
					l := int16(binary.LittleEndian.Uint16(blob[p : p+2]))
					t := int16(binary.LittleEndian.Uint16(blob[p+2 : p+4]))
					w := int16(binary.LittleEndian.Uint16(blob[p+4 : p+6]))
					h := int16(binary.LittleEndian.Uint16(blob[p+6 : p+8]))
					props = append(props,
						Property{Name: "Height", Value: fmt.Sprintf("%d", h)},
						Property{Name: "Left", Value: fmt.Sprintf("%d", l)},
						Property{Name: "Top", Value: fmt.Sprintf("%d", t)},
						Property{Name: "Width", Value: fmt.Sprintf("%d", w)},
					)
					p += 8
				}
			case 0x11: // TabIndex
				if p+2 <= len(blob) {
					idx := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "TabIndex",
						Value: fmt.Sprintf("%d", idx),
					})
					p += 2
				}
			default:
				p++
			}

		case "FileListBox":
			switch propID {
			case 0x04: // Coordinates
				if p+8 <= len(blob) {
					l := int16(binary.LittleEndian.Uint16(blob[p : p+2]))
					t := int16(binary.LittleEndian.Uint16(blob[p+2 : p+4]))
					w := int16(binary.LittleEndian.Uint16(blob[p+4 : p+6]))
					h := int16(binary.LittleEndian.Uint16(blob[p+6 : p+8]))
					props = append(props,
						Property{Name: "Height", Value: fmt.Sprintf("%d", h)},
						Property{Name: "Left", Value: fmt.Sprintf("%d", l)},
						Property{Name: "Top", Value: fmt.Sprintf("%d", t)},
						Property{Name: "Width", Value: fmt.Sprintf("%d", w)},
					)
					p += 8
				}
			case 0x09: // Visible
				if p < len(blob) {
					val := int(int8(blob[p]))
					props = append(props, Property{
						Name:    "Visible",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("FileListBox", "Visible", val),
					})
					p++
				}
			case 0x0B, 0x11: // TabIndex
				if p+2 <= len(blob) {
					idx := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "TabIndex",
						Value: fmt.Sprintf("%d", idx),
					})
					p += 2
				}
			default:
				p++
			}

		case "HScrollBar":
			switch propID {
			case 0x02, 0x04: // Coordinates
				if p+8 <= len(blob) {
					l := int16(binary.LittleEndian.Uint16(blob[p : p+2]))
					t := int16(binary.LittleEndian.Uint16(blob[p+2 : p+4]))
					w := int16(binary.LittleEndian.Uint16(blob[p+4 : p+6]))
					h := int16(binary.LittleEndian.Uint16(blob[p+6 : p+8]))
					props = append(props,
						Property{Name: "Height", Value: fmt.Sprintf("%d", h)},
						Property{Name: "Left", Value: fmt.Sprintf("%d", l)},
						Property{Name: "Top", Value: fmt.Sprintf("%d", t)},
						Property{Name: "Width", Value: fmt.Sprintf("%d", w)},
					)
					p += 8
				}
			case 0x11: // TabIndex
				if p+2 <= len(blob) {
					idx := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "TabIndex",
						Value: fmt.Sprintf("%d", idx),
					})
					p += 2
				}
			case 0x12: // Value
				if p+2 <= len(blob) {
					val := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "Value",
						Value: fmt.Sprintf("%d", val),
					})
					p += 2
				}
			case 0x13: // Min
				if p+2 <= len(blob) {
					val := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "Min",
						Value: fmt.Sprintf("%d", val),
					})
					p += 2
				}
			case 0x14: // Max
				if p+2 <= len(blob) {
					val := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "Max",
						Value: fmt.Sprintf("%d", val),
					})
					p += 2
				}
			default:
				p++
			}

		case "MMControl":
			switch propID {
			case 0x04, 0x07: // Coordinates
				if p+8 <= len(blob) {
					l := int16(binary.LittleEndian.Uint16(blob[p : p+2]))
					t := int16(binary.LittleEndian.Uint16(blob[p+2 : p+4]))
					w := int16(binary.LittleEndian.Uint16(blob[p+4 : p+6]))
					h := int16(binary.LittleEndian.Uint16(blob[p+6 : p+8]))
					props = append(props,
						Property{Name: "Height", Value: fmt.Sprintf("%d", h)},
						Property{Name: "Left", Value: fmt.Sprintf("%d", l)},
						Property{Name: "Top", Value: fmt.Sprintf("%d", t)},
						Property{Name: "Width", Value: fmt.Sprintf("%d", w)},
					)
					p += 8
				}
			case 0x09: // Visible
				if p < len(blob) {
					val := int(int8(blob[p]))
					props = append(props, Property{
						Name:    "Visible",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("MMControl", "Visible", val),
					})
					p++
				}
			case 0x11: // TabIndex
				if p+2 <= len(blob) {
					idx := binary.LittleEndian.Uint16(blob[p : p+2])
					props = append(props, Property{
						Name:  "TabIndex",
						Value: fmt.Sprintf("%d", idx),
					})
					p += 2
				}
			case 0x18: // TabStop
				if p < len(blob) {
					val := int(int8(blob[p]))
					props = append(props, Property{
						Name:    "TabStop",
						Value:   fmt.Sprintf("%d", val),
						Comment: FormatEnumComment("MMControl", "TabStop", val),
					})
					p++
				}
			default:
				p++
			}

		default:
			p++
		}
	}

	// Sort properties alphabetically by name, matching standard VB3 .FRM output
	sort.Slice(props, func(i, j int) bool {
		return props[i].Name < props[j].Name
	})

	return props, nil
}

func readLengthPrefixedBytes(blob []byte, p *int) ([]byte, error) {
	if *p >= len(blob) {
		return nil, fmt.Errorf("missing length byte at offset %d", *p)
	}
	n := int(blob[*p])
	(*p)++
	if n > len(blob)-*p {
		return nil, fmt.Errorf("length %d at offset %d exceeds remaining data", n, *p-1)
	}
	value := blob[*p : *p+n]
	*p += n
	return value, nil
}

func decodeFont(blob []byte, p int, props *[]Property) (int, error) {
	if p >= len(blob) {
		return p, fmt.Errorf("missing font name length at offset %d", p)
	}
	sLen := int(blob[p])
	p++
	if p+sLen+5 > len(blob) {
		return p, fmt.Errorf("truncated font data at offset %d", p)
	}
	fontName := win1252.Decode(blob[p : p+sLen])
	p += sLen

	bits := binary.LittleEndian.Uint32(blob[p : p+4])
	p += 4
	fontSize := math.Float32frombits(bits)

	flags := blob[p]
	p++

	bold := (flags & 1) != 0
	italic := (flags & 2) != 0
	underline := (flags & 4) != 0
	strikethru := (flags & 8) != 0

	boldVal := 0
	if bold {
		boldVal = -1
	}
	italicVal := 0
	if italic {
		italicVal = -1
	}
	underlineVal := 0
	if underline {
		underlineVal = -1
	}
	strikethruVal := 0
	if strikethru {
		strikethruVal = -1
	}

	// Format font size string
	var sizeStr string
	if fontSize == float32(int(fontSize)) {
		sizeStr = fmt.Sprintf("%d", int(fontSize))
	} else {
		sizeStr = fmt.Sprintf("%.2f", fontSize)
		sizeStr = strings.TrimRight(sizeStr, "0")
	}

	*props = append(*props,
		Property{
			Name:    "FontBold",
			Value:   fmt.Sprintf("%d", boldVal),
			Comment: FormatEnumComment("", "FontBold", boldVal),
		},
		Property{
			Name:    "FontItalic",
			Value:   fmt.Sprintf("%d", italicVal),
			Comment: FormatEnumComment("", "FontItalic", italicVal),
		},
		Property{
			Name:  "FontName",
			Value: win1252.QuoteVBString(fontName),
		},
		Property{
			Name:  "FontSize",
			Value: sizeStr,
		},
		Property{
			Name:    "FontStrikethru",
			Value:   fmt.Sprintf("%d", strikethruVal),
			Comment: FormatEnumComment("", "FontStrikethru", strikethruVal),
		},
		Property{
			Name:    "FontUnderline",
			Value:   fmt.Sprintf("%d", underlineVal),
			Comment: FormatEnumComment("", "FontUnderline", underlineVal),
		},
	)

	return p, nil
}
