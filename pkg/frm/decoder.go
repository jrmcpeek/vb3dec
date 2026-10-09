package frm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"vb3dec/pkg/vbx"
	"vb3dec/pkg/win1252"
)

// DecodeFormStream decodes a raw binary form stream from an NE RCData resource.
// Custom (VBX) control properties are not decoded; see DecodeFormStreamWithModels.
func DecodeFormStream(stream []byte, formRef FormRef, nameMap map[int]string) (*Form, []byte, error) {
	return DecodeFormStreamWithModels(stream, formRef, nameMap, nil)
}

// DecodeFormStreamWithModels decodes a raw binary form stream, resolving custom
// control property lists through customModels (keyed by lower-case class name).
//
// Each property in the stream is a one-byte index into the control model's
// property list, followed by a payload whose layout is determined by the
// property's data type. Corrupt record framing is an error; a property that
// cannot be decoded only stops decoding of that control's properties and is
// reported in Form.Warnings.
func DecodeFormStreamWithModels(stream []byte, formRef FormRef, nameMap map[int]string, customModels map[string]*vbx.Model) (*Form, []byte, error) {
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

	baseFRXName := strings.TrimSuffix(form.FileName, ".FRM")
	baseFRXName = strings.TrimSuffix(baseFRXName, ".frm")

	// Buffer for FRX assets
	var frxBuf bytes.Buffer
	pd := &propDecoder{form: form, frxBuf: &frxBuf, baseFRXName: baseFRXName}

	var formValues []propValue
	if p < formEndPos {
		formModel := vbx.BuiltinModels["Form"]
		values, eventPos, err := walkProperties(formModel, stream[p:formEndPos], true, false)
		if err != nil {
			form.Warnings = append(form.Warnings, fmt.Sprintf("form %s: %v; remaining properties skipped", form.Name, err))
		}
		formValues = values
		formNode.Events = decodeEventTable(stream[p:formEndPos], eventPos, formModel)
	}
	formNode.Properties = pd.formProperties(formValues)
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
		if tag == 1 && lastNode != nil {
			// First child of the preceding control; tag 2 closes the level.
			containerStack = append(containerStack, lastNode)
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
		arrayIndex := 0
		if hasFlags {
			if cp+2 > ctrlEndPos {
				return nil, nil, fmt.Errorf("control record at offset %d is truncated before its array index", pos)
			}
			arrayIndex = int(binary.LittleEndian.Uint16(stream[cp : cp+2]))
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
		var model *vbx.Model
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
			model = customModels[strings.ToLower(typeName)]
		} else {
			typeName = BuiltinControlTypeName(typeID)
			model = builtinModel(typeID)
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
			ID:         ctrlID,
			Name:       ctrlName,
			TypeName:   typeName,
			IsArray:    hasFlags,
			ArrayIndex: arrayIndex,
		}

		// Decode properties
		if model == nil {
			if typeID == 0xFF {
				form.Warnings = append(form.Warnings, fmt.Sprintf("control %s (%s): custom control model not available; properties not decoded", ctrlName, typeName))
			} else {
				form.Warnings = append(form.Warnings, fmt.Sprintf("control %s (%s): no property table for control type 0x%02X; properties not decoded", ctrlName, typeName, typeID))
			}
		} else {
			values, eventPos, err := walkProperties(model, stream[cp:ctrlEndPos], false, model == vbx.BuiltinModels["Combo"])
			if err != nil {
				form.Warnings = append(form.Warnings, fmt.Sprintf("control %s (%s): %v; remaining properties skipped", ctrlName, typeName, err))
			}
			node.Properties = pd.controlProperties(typeName, values)
			node.Events = decodeEventTable(stream[cp:ctrlEndPos], eventPos, model)
		}

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

// propValue is one serialized property: its definition and raw payload.
type propValue struct {
	prop vbx.Prop
	data []byte
}

// comboStyleDropdownList is the ComboBox Style value whose Text is not serialized.
const comboStyleDropdownList = 2

// walkProperties splits a control record's property area into property values.
// The area holds optional pre-window-creation properties, a 0xFF separator,
// the remaining properties, and finally the event table, which starts with a
// second 0xFF. It returns the values and the event table's offset in blob, or
// -1 if the walk did not reach it. Values decoded before an error are
// returned with it.
func walkProperties(model *vbx.Model, blob []byte, isForm bool, isCombo bool) ([]propValue, int, error) {
	var values []propValue
	seenSeparator := false
	comboStyle := -1
	p := 0
	for p < len(blob) {
		if blob[p] == 0xFF {
			if !seenSeparator {
				seenSeparator = true
				p++
				continue
			}
			return values, p, nil
		}
		id := int(blob[p])
		p++
		if id >= len(model.Props) {
			return values, -1, fmt.Errorf("property ID 0x%02X is outside the %d-entry property list", id, len(model.Props))
		}
		prop := model.Props[id]
		if isCombo && prop.Std == stdText && comboStyle == comboStyleDropdownList {
			values = append(values, propValue{prop: prop})
			continue
		}
		n, err := payloadSize(prop, blob[p:], isForm)
		if err != nil {
			return values, -1, fmt.Errorf("property %s (0x%02X): %w", prop.Name, id, err)
		}
		if isCombo && prop.Name == "Style" && n == 1 {
			comboStyle = int(blob[p])
		}
		values = append(values, propValue{prop: prop, data: blob[p : p+n]})
		p += n
	}
	return values, -1, nil
}

// decodeEventTable reads the event table at pos (0xFF, slot count, one word
// per slot) and returns the bound slots. A slot word with bit 0 set refers to
// the descriptor of the event procedure. When the property walk did not reach
// the table (pos < 0), the table is located from the end of the record.
func decodeEventTable(blob []byte, pos int, model *vbx.Model) []EventBinding {
	if pos < 0 {
		pos = findEventTable(blob)
	}
	if pos < 0 || pos+2 > len(blob) || blob[pos] != 0xFF {
		return nil
	}
	count := int(blob[pos+1])
	var bindings []EventBinding
	for slot := 0; slot < count && pos+4+slot*2 <= len(blob); slot++ {
		w := binary.LittleEndian.Uint16(blob[pos+2+slot*2:])
		if w == 0 || w&1 == 0 {
			continue
		}
		b := EventBinding{Slot: slot, ProcRef: w &^ 1}
		if model != nil && slot < len(model.Events) {
			b.Event = model.Events[slot]
		}
		bindings = append(bindings, b)
	}
	return bindings
}

// findEventTable locates an event table that ends the record, allowing a few
// trailing bytes.
func findEventTable(blob []byte) int {
	for p := len(blob) - 2; p >= 0; p-- {
		if blob[p] != 0xFF {
			continue
		}
		count := int(blob[p+1])
		if rem := len(blob) - (p + 2 + count*2); count > 0 && rem >= 0 && rem <= 4 {
			return p
		}
	}
	return -1
}

// stdText is the standard Text property index.
const stdText = 27

// payloadSize returns the serialized size of a property value at the start of rest.
func payloadSize(prop vbx.Prop, rest []byte, isForm bool) (int, error) {
	var n int
	switch {
	case prop.Std == vbx.StdLeft:
		// Left, Top, Width and Height are stored together: 16-bit twips for
		// controls, 32-bit twips for forms.
		n = 8
		if isForm {
			n = 16
		}
	case prop.Std == vbx.StdFontName:
		// Font block: name, size (Single), and attribute flags.
		if len(rest) < 1 {
			return 0, fmt.Errorf("missing font name length")
		}
		n = 1 + int(rest[0]) + 4 + 1
	case prop.Name == "ScaleMode" && prop.Type() == vbx.DTEnum:
		// The scale mode word is followed by a flags word.
		n = 4
	default:
		switch prop.Type() {
		case vbx.DTHSZ:
			if len(rest) < 1 {
				return 0, fmt.Errorf("missing length byte")
			}
			n = 1 + int(rest[0])
		case vbx.DTShort, vbx.DTIndex:
			n = 2
		case vbx.DTLong, vbx.DTColor, vbx.DTReal, vbx.DTXPos, vbx.DTXSize, vbx.DTYPos, vbx.DTYSize:
			n = 4
		case vbx.DTBool, vbx.DTEnum:
			n = 1
		case vbx.DTPicture:
			if len(rest) < 4 {
				return 0, fmt.Errorf("missing picture length")
			}
			n = 4
			if l := binary.LittleEndian.Uint32(rest[:4]); l != 0xFFFFFFFF {
				n += int(l)
			}
		default:
			return 0, fmt.Errorf("unsupported data type %d", prop.Type())
		}
	}
	if n > len(rest) {
		return 0, fmt.Errorf("value needs %d bytes, %d remain", n, len(rest))
	}
	return n, nil
}

// propDecoder converts property values into .FRM text properties and
// accumulates picture payloads into the form's .FRX buffer.
type propDecoder struct {
	form        *Form
	frxBuf      *bytes.Buffer
	baseFRXName string
}

// formProperties builds the Form block properties. The form's Left..Height
// block holds the client rectangle; the outer window bounds written to the
// .FRM are derived from it and the border style.
func (pd *propDecoder) formProperties(values []propValue) []Property {
	borderStyle := -1
	var clientLeft, clientTop, clientWidth, clientHeight int32
	hasClientBounds := false
	var pictures, icons [][]byte
	var props []Property

	for _, v := range values {
		switch {
		case v.prop.Std == vbx.StdLeft:
			clientLeft = int32(binary.LittleEndian.Uint32(v.data[0:4]))
			clientTop = int32(binary.LittleEndian.Uint32(v.data[4:8]))
			clientWidth = int32(binary.LittleEndian.Uint32(v.data[8:12]))
			clientHeight = int32(binary.LittleEndian.Uint32(v.data[12:16]))
			hasClientBounds = true
		case strings.HasPrefix(v.prop.Name, "Client"):
			// Duplicates the client rectangle stored in the Left..Height block.
		case v.prop.Name == "Picture":
			if payload, ok := picturePayload(v.data); ok {
				pictures = append(pictures, payload)
			}
		case v.prop.Name == "Icon":
			if payload, ok := picturePayload(v.data); ok {
				icons = append(icons, payload)
			}
		default:
			if v.prop.Name == "BorderStyle" && len(v.data) == 1 {
				borderStyle = int(v.data[0])
			}
			props = append(props, pd.formatValue("Form", v)...)
		}
	}

	if hasClientBounds {
		// Calculate outer window bounds from client bounds based on BorderStyle
		outerLeft, outerTop, outerWidth, outerHeight := clientLeft, clientTop, clientWidth, clientHeight
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
		}
		props = append(props,
			Property{Name: "ClientHeight", Value: fmt.Sprintf("%d", clientHeight)},
			Property{Name: "ClientLeft", Value: fmt.Sprintf("%d", clientLeft)},
			Property{Name: "ClientTop", Value: fmt.Sprintf("%d", clientTop)},
			Property{Name: "ClientWidth", Value: fmt.Sprintf("%d", clientWidth)},
			Property{Name: "Height", Value: fmt.Sprintf("%d", outerHeight)},
			Property{Name: "Left", Value: fmt.Sprintf("%d", outerLeft)},
			Property{Name: "ScaleHeight", Value: fmt.Sprintf("%d", clientHeight)},
			Property{Name: "ScaleWidth", Value: fmt.Sprintf("%d", clientWidth)},
			Property{Name: "Top", Value: fmt.Sprintf("%d", outerTop)},
			Property{Name: "Width", Value: fmt.Sprintf("%d", outerWidth)},
		)
	}

	// Icons precede pictures in the .FRX.
	for _, icon := range icons {
		props = append(props, pd.addFRXAsset("Icon", icon))
	}
	for _, pic := range pictures {
		props = append(props, pd.addFRXAsset("Picture", pic))
	}

	sortProperties(props)
	return props
}

// controlProperties builds a child control's properties.
func (pd *propDecoder) controlProperties(typeName string, values []propValue) []Property {
	var props []Property
	for _, v := range values {
		props = append(props, pd.formatValue(typeName, v)...)
	}
	sortProperties(props)
	return props
}

// formatValue renders one property value as .FRM text properties.
func (pd *propDecoder) formatValue(typeName string, v propValue) []Property {
	prop, data := v.prop, v.data
	if !isPropertyName(prop.Name) {
		return nil
	}
	switch {
	case prop.Std == vbx.StdLeft:
		l := int16(binary.LittleEndian.Uint16(data[0:2]))
		t := int16(binary.LittleEndian.Uint16(data[2:4]))
		w := int16(binary.LittleEndian.Uint16(data[4:6]))
		h := int16(binary.LittleEndian.Uint16(data[6:8]))
		return []Property{
			{Name: "Height", Value: fmt.Sprintf("%d", h)},
			{Name: "Left", Value: fmt.Sprintf("%d", l)},
			{Name: "Top", Value: fmt.Sprintf("%d", t)},
			{Name: "Width", Value: fmt.Sprintf("%d", w)},
		}
	case prop.Std == vbx.StdFontName:
		var props []Property
		if _, err := decodeFont(data, 0, &props); err != nil {
			pd.form.Warnings = append(pd.form.Warnings, fmt.Sprintf("%s font: %v", typeName, err))
		}
		return props
	case prop.Name == "ScaleMode" && prop.Type() == vbx.DTEnum:
		mode := int(int16(binary.LittleEndian.Uint16(data[0:2])))
		if mode == 1 {
			return nil // Twips (default)
		}
		return []Property{{Name: "ScaleMode", Value: fmt.Sprintf("%d", mode)}}
	}

	switch prop.Type() {
	case vbx.DTHSZ:
		if len(data) == 0 {
			return nil // Not serialized (dropdown-list ComboBox Text)
		}
		return []Property{{Name: prop.Name, Value: win1252.QuoteVBString(win1252.Decode(data[1:]))}}
	case vbx.DTShort, vbx.DTIndex:
		return []Property{{Name: prop.Name, Value: fmt.Sprintf("%d", int16(binary.LittleEndian.Uint16(data)))}}
	case vbx.DTLong, vbx.DTXPos, vbx.DTXSize, vbx.DTYPos, vbx.DTYSize:
		return []Property{{Name: prop.Name, Value: fmt.Sprintf("%d", int32(binary.LittleEndian.Uint32(data)))}}
	case vbx.DTColor:
		return []Property{{Name: prop.Name, Value: fmt.Sprintf("&H%08X&", binary.LittleEndian.Uint32(data))}}
	case vbx.DTReal:
		return []Property{{Name: prop.Name, Value: formatSingle(math.Float32frombits(binary.LittleEndian.Uint32(data)))}}
	case vbx.DTBool:
		val := int(int8(data[0]))
		comment := FormatEnumComment(typeName, prop.Name, val)
		if comment == "" {
			if val == 0 {
				comment = "False"
			} else {
				comment = "True"
			}
		}
		return []Property{{Name: prop.Name, Value: fmt.Sprintf("%d", val), Comment: comment}}
	case vbx.DTEnum:
		val := int(data[0])
		return []Property{{Name: prop.Name, Value: fmt.Sprintf("%d", val), Comment: FormatEnumComment(typeName, prop.Name, val)}}
	case vbx.DTPicture:
		if payload, ok := picturePayload(data); ok {
			return []Property{pd.addFRXAsset(prop.Name, payload)}
		}
	}
	return nil
}

// picturePayload returns the picture data of a DT_PICTURE value, if any.
func picturePayload(data []byte) ([]byte, bool) {
	if len(data) <= 4 {
		return nil, false
	}
	return data[4:], true
}

// addFRXAsset appends a length-prefixed payload to the .FRX buffer and returns
// the property referencing it.
func (pd *propDecoder) addFRXAsset(propName string, payload []byte) Property {
	offset := uint32(pd.frxBuf.Len())
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	pd.frxBuf.Write(lenBuf[:])
	pd.frxBuf.Write(payload)

	data := make([]byte, len(payload))
	copy(data, payload)
	pd.form.FRXAssets = append(pd.form.FRXAssets, FRXAsset{
		PropName: propName,
		Data:     data,
		Offset:   offset,
	})
	return Property{
		Name:  propName,
		Value: fmt.Sprintf("%s.FRX:%04X", pd.baseFRXName, offset),
	}
}

// isPropertyName reports whether name can appear in .FRM text; property lists
// also contain placeholders such as " " and "(About)".
func isPropertyName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		isAlpha := c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
		if !isAlpha && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// formatSingle formats a Single the way VB writes numeric literals.
func formatSingle(f float32) string {
	return strconv.FormatFloat(float64(f), 'g', -1, 32)
}

// sortProperties sorts properties alphabetically by name, matching standard VB3 .FRM output.
func sortProperties(props []Property) {
	sort.SliceStable(props, func(i, j int) bool {
		return props[i].Name < props[j].Name
	})
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
