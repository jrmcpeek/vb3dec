package pcode

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"

	"vb3dec/pkg/win1252"
)

type eventParamDef struct {
	Name string
	Type string
}

var eventParamDefs = map[string][]eventParamDef{
	"MouseDown": {
		{"Button", "Integer"},
		{"Shift", "Integer"},
		{"X", "Single"},
		{"Y", "Single"},
	},
	"MouseMove": {
		{"Button", "Integer"},
		{"Shift", "Integer"},
		{"X", "Single"},
		{"Y", "Single"},
	},
	"MouseUp": {
		{"Button", "Integer"},
		{"Shift", "Integer"},
		{"X", "Single"},
		{"Y", "Single"},
	},
	"KeyDown": {
		{"KeyCode", "Integer"},
		{"Shift", "Integer"},
	},
	"KeyUp": {
		{"KeyCode", "Integer"},
		{"Shift", "Integer"},
	},
	"KeyPress": {
		{"KeyAscii", "Integer"},
	},
	"Unload": {
		{"Cancel", "Integer"},
	},
	"QueryUnload": {
		{"Cancel", "Integer"},
		{"UnloadMode", "Integer"},
	},
	"DragDrop": {
		{"Source", "Control"},
		{"X", "Single"},
		{"Y", "Single"},
	},
	"DragOver": {
		{"Source", "Control"},
		{"X", "Single"},
		{"Y", "Single"},
		{"State", "Integer"},
	},
	"LinkOpen": {
		{"Cancel", "Integer"},
	},
	"LinkError": {
		{"LinkErr", "Integer"},
	},
}

// standardMethods maps VB3 standard control/form method IDs to their names.
var standardMethods = map[uint16]string{
	0:  "PrintForm",
	1:  "LinkSend",
	2:  "AddItem",
	3:  "RemoveItem",
	4:  "Refresh",
	5:  "LinkPoke",
	6:  "LinkRequest",
	7:  "LinkExecute",
	8:  "GetText",
	9:  "GetData",
	10: "SetText",
	11: "SetData",
	12: "Clear",
	13: "GetFormat",
	14: "Arrange",
	15: "Show",
	16: "Hide",
	17: "EndDoc",
	18: "NewPage",
	19: "SetFocus",
	20: "Drag",
	21: "Move",
	22: "ZOrder",
	23: "Close",
	24: "Delete",
}

var formProperties = map[uint16]string{
	0xC000: "Caption",
	0xC001: "Name",
	0xC002: "hWnd",
	0xC003: "BackColor",
	0xC004: "ForeColor",
	0xC005: "Left",
	0xC006: "Top",
	0xC007: "Width",
	0xC008: "Height",
	0xC009: "Enabled",
	0xC00A: "WindowState",
	0xC00B: "MousePointer",
	0xC00C: "FontName",
	0xC00D: "FontSize",
	0xC00E: "FontBold",
	0xC00F: "FontItalic",
	0xC010: "FontStrikethru",
	0xC011: "FontUnderline",
	0xC012: "hDC",
	0xC013: "CurrentX",
	0xC014: "CurrentY",
	0xC015: "ScaleLeft",
	0xC016: "ScaleTop",
	0xC017: "ScaleWidth",
	0xC018: "ScaleHeight",
	0xC019: "ScaleMode",
	0xC01A: "FontTransparent",
	0xC01B: "DrawStyle",
	0xC01C: "DrawWidth",
	0xC01D: "FillStyle",
	0xC01E: "FillColor",
	0xC01F: "DrawMode",
	0xC020: "AutoRedraw",
	0xC021: "Picture",
	0xC022: "BorderStyle",
	0xC023: "Icon",
	0xC024: "LinkTopic",
	0xC025: "LinkMode",
	0xC026: "MaxButton",
	0xC027: "MinButton",
	0xC028: "ControlBox",
	0xC029: "Image",
	0xC02E: "Visible",
	0xC02F: "Tag",
	0xC030: "MDIChild",
	0xC031: "KeyPreview",
	0xC032: "ClipControls",
	0xC033: "HelpContextID",
	0xC034: "ActiveControl",
	0xC035: "ClientLeft",
	0xC036: "ClientTop",
	0xC037: "ClientWidth",
	0xC038: "ClientHeight",
}

var comboBoxProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "Index",
	0xC002: "BackColor",
	0xC003: "ForeColor",
	0xC004: "Left",
	0xC005: "Top",
	0xC006: "Width",
	0xC007: "Height",
	0xC008: "Enabled",
	0xC009: "Visible",
	0xC00A: "MousePointer",
	0xC00B: "Text",
	0xC00C: "FontName",
	0xC00D: "FontBold",
	0xC00E: "FontItalic",
	0xC00F: "FontStrikethru",
	0xC010: "FontUnderline",
	0xC011: "FontSize",
	0xC012: "TabIndex",
	0xC013: "ListCount",
	0xC014: "ListIndex",
	0xC015: "List",
	0xC016: "Sorted",
	0xC017: "SelStart",
	0xC018: "SelLength",
	0xC019: "SelText",
	0xC01A: "Parent",
	0xC01B: "DragMode",
	0xC01C: "DragIcon",
	0xC01D: "TabStop",
	0xC01E: "Tag",
	0xC01F: "Style",
	0xC020: "hWnd",
	0xC021: "ItemData",
	0xC022: "NewIndex",
	0xC023: "HelpContextID",
}

var listBoxProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "Index",
	0xC002: "BackColor",
	0xC003: "ForeColor",
	0xC004: "Left",
	0xC005: "Top",
	0xC006: "Width",
	0xC007: "Height",
	0xC008: "Enabled",
	0xC009: "Visible",
	0xC00A: "MousePointer",
	0xC00B: "FontName",
	0xC00C: "FontSize",
	0xC00D: "FontBold",
	0xC00E: "FontItalic",
	0xC00F: "FontStrikethru",
	0xC010: "FontUnderline",
	0xC011: "TabIndex",
	0xC012: "ListCount",
	0xC013: "ListIndex",
	0xC014: "List",
	0xC015: "Sorted",
	0xC016: "Text",
	0xC017: "Parent",
	0xC018: "DragMode",
	0xC019: "DragIcon",
	0xC01A: "TabStop",
	0xC01B: "Tag",
	0xC01C: "Columns",
	0xC01D: "MultiSelect",
	0xC01E: "Selected",
	0xC01F: "SelCount",
	0xC020: "TopIndex",
	0xC021: "ItemData",
	0xC022: "NewIndex",
	0xC023: "HelpContextID",
	0xC024: "hWnd",
}

var dirListBoxProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "Index",
	0xC002: "BackColor",
	0xC003: "ForeColor",
	0xC004: "Left",
	0xC005: "Top",
	0xC006: "Width",
	0xC007: "Height",
	0xC008: "Enabled",
	0xC009: "Visible",
	0xC00A: "MousePointer",
	0xC00B: "TabIndex",
	0xC00C: "Path",
	0xC00D: "ListCount",
	0xC00E: "ListIndex",
	0xC00F: "List",
	0xC010: "FontName",
	0xC011: "FontSize",
	0xC012: "FontBold",
	0xC013: "FontItalic",
	0xC014: "FontStrikethru",
	0xC015: "FontUnderline",
	0xC016: "Parent",
	0xC017: "DragMode",
	0xC018: "DragIcon",
	0xC019: "TabStop",
	0xC01A: "Tag",
	0xC01B: "hWnd",
	0xC01C: "HelpContextID",
}

var fileListBoxProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "Index",
	0xC002: "BackColor",
	0xC003: "ForeColor",
	0xC004: "Left",
	0xC005: "Top",
	0xC006: "Width",
	0xC007: "Height",
	0xC008: "Enabled",
	0xC009: "Visible",
	0xC00A: "MousePointer",
	0xC00B: "TabIndex",
	0xC00C: "Path",
	0xC00D: "Pattern",
	0xC00E: "FileName",
	0xC00F: "Normal",
	0xC010: "ReadOnly",
	0xC011: "Archive",
	0xC012: "Hidden",
	0xC013: "System",
	0xC014: "ListCount",
	0xC015: "ListIndex",
	0xC016: "List",
	0xC017: "FontName",
	0xC018: "FontSize",
	0xC019: "FontBold",
	0xC01A: "FontItalic",
	0xC01B: "FontStrikethru",
	0xC01C: "FontUnderline",
	0xC01D: "Parent",
	0xC01E: "DragMode",
	0xC01F: "DragIcon",
	0xC020: "TabStop",
	0xC021: "Tag",
	0xC022: "hWnd",
	0xC023: "HelpContextID",
	0xC024: "MultiSelect",
	0xC025: "Selected",
	0xC026: "TopIndex",
}

var mmControlProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "BorderStyle",
	0xC002: "DragIcon",
	0xC003: "DragMode",
	0xC004: "Enabled",
	0xC005: "Height",
	0xC006: "Index",
	0xC007: "Left",
	0xC008: "MousePointer",
	0xC009: "Parent",
	0xC00A: "TabIndex",
	0xC00B: "TabStop",
	0xC00C: "Tag",
	0xC00D: "Top",
	0xC00E: "Visible",
	0xC00F: "Width",
	0xC010: "AutoEnable",
	0xC011: "UpdateInterval",
	0xC012: "Orientation",
	0xC013: "PlayVisible",
	0xC014: "PlayEnabled",
	0xC015: "PauseVisible",
	0xC016: "PauseEnabled",
	0xC017: "StopVisible",
	0xC018: "StopEnabled",
	0xC019: "BackVisible",
	0xC01A: "BackEnabled",
	0xC01B: "StepVisible",
	0xC01C: "StepEnabled",
	0xC01D: "EjectVisible",
	0xC01E: "EjectEnabled",
	0xC01F: "PrevVisible",
	0xC020: "PrevEnabled",
	0xC021: "NextVisible",
	0xC022: "NextEnabled",
	0xC023: "RecordVisible",
	0xC024: "RecordEnabled",
	0xC025: "Command",
	0xC026: "DeviceID",
	0xC027: "DeviceType",
	0xC028: "FileName",
	0xC029: "CanEject",
	0xC02A: "CanPlay",
	0xC02B: "CanRecord",
	0xC02C: "CanStep",
	0xC02D: "UsesWindows",
	0xC02E: "Start",
	0xC02F: "Length",
	0xC030: "Tracks",
	0xC031: "TimeFormat",
	0xC032: "Mode",
	0xC033: "Position",
	0xC034: "Silent",
	0xC035: "RecordMode",
	0xC036: "Notify",
	0xC037: "Wait",
	0xC038: "Shareable",
	0xC039: "From",
	0xC03A: "To",
	0xC03B: "Frames",
	0xC03C: "Track",
	0xC03D: "TrackLength",
	0xC03E: "TrackPosition",
	0xC03F: "Error",
	0xC040: "ErrorMessage",
	0xC041: "NotifyValue",
	0xC042: "NotifyMessage",
	0xC043: "hWndDisplay",
	0xC044: "(About)",
	0xC045: "HelpContextID",
	0xC046: "hWnd",
}

var optionButtonProperties = map[uint16]string{
	0xC000: "Caption",
	0xC001: "Name",
	0xC002: "Index",
	0xC003: "BackColor",
	0xC004: "ForeColor",
	0xC005: "Left",
	0xC006: "Top",
	0xC007: "Width",
	0xC008: "Height",
	0xC009: "Enabled",
	0xC00A: "Visible",
	0xC00B: "MousePointer",
	0xC00C: "FontName",
	0xC00D: "FontSize",
	0xC00E: "FontBold",
	0xC00F: "FontItalic",
	0xC010: "FontStrikethru",
	0xC011: "FontUnderline",
	0xC012: "TabIndex",
	0xC013: "Value",
	0xC014: "Parent",
	0xC015: "DragMode",
	0xC016: "DragIcon",
	0xC017: "TabStop",
	0xC018: "Tag",
	0xC019: "Alignment",
	0xC01A: "HelpContextID",
	0xC01B: "hWnd",
}

var checkBoxProperties = map[uint16]string{
	0xC000: "Caption",
	0xC001: "Name",
	0xC002: "Index",
	0xC003: "BackColor",
	0xC004: "ForeColor",
	0xC005: "Left",
	0xC006: "Top",
	0xC007: "Width",
	0xC008: "Height",
	0xC009: "Enabled",
	0xC00A: "Visible",
	0xC00B: "MousePointer",
	0xC00C: "FontName",
	0xC00D: "FontBold",
	0xC00E: "FontItalic",
	0xC00F: "FontStrikethru",
	0xC010: "FontUnderline",
	0xC011: "FontSize",
	0xC012: "TabIndex",
	0xC013: "Value",
	0xC014: "Parent",
	0xC015: "DragMode",
	0xC016: "DragIcon",
	0xC017: "TabStop",
	0xC018: "Tag",
	0xC019: "Alignment",
	0xC01A: "HelpContextID",
	0xC01B: "hWnd",
	0xC01C: "DataSource",
	0xC01D: "DataField",
	0xC01E: "DataChanged",
}

var commandButtonProperties = map[uint16]string{
	0xC000: "Caption",
	0xC001: "Name",
	0xC002: "Index",
	0xC003: "BackColor",
	0xC004: "Left",
	0xC005: "Top",
	0xC006: "Width",
	0xC007: "Height",
	0xC008: "Enabled",
	0xC009: "Visible",
	0xC00A: "MousePointer",
	0xC00B: "FontName",
	0xC00C: "FontSize",
	0xC00D: "FontBold",
	0xC00E: "FontItalic",
	0xC00F: "FontStrikethru",
	0xC010: "FontUnderline",
	0xC011: "TabIndex",
	0xC012: "Value",
	0xC013: "Default",
	0xC014: "Cancel",
	0xC015: "Parent",
	0xC016: "DragMode",
	0xC017: "DragIcon",
	0xC018: "TabStop",
	0xC019: "Tag",
	0xC01A: "hWnd",
	0xC01B: "HelpContextID",
}

var textBoxProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "Index",
	0xC002: "BackColor",
	0xC003: "ForeColor",
	0xC004: "Left",
	0xC005: "Top",
	0xC006: "Width",
	0xC007: "Height",
	0xC008: "Enabled",
	0xC009: "Visible",
	0xC00A: "MousePointer",
	0xC00B: "Text",
	0xC00C: "FontName",
	0xC00D: "FontSize",
	0xC00E: "FontBold",
	0xC00F: "FontItalic",
	0xC010: "FontStrikethru",
	0xC011: "FontUnderline",
	0xC012: "TabIndex",
	0xC013: "BorderStyle",
	0xC014: "LinkTopic",
	0xC015: "LinkItem",
	0xC016: "LinkMode",
	0xC017: "MultiLine",
	0xC018: "ScrollBars",
	0xC019: "SelStart",
	0xC01A: "SelLength",
	0xC01B: "SelText",
	0xC01C: "Parent",
	0xC01D: "DragMode",
	0xC01E: "DragIcon",
	0xC01F: "LinkTimeout",
	0xC020: "TabStop",
	0xC021: "Tag",
	0xC022: "PasswordChar",
	0xC023: "HideSelection",
	0xC024: "Alignment",
	0xC025: "MaxLength",
	0xC026: "HelpContextID",
	0xC027: "hWnd",
	0xC029: "DataSource",
	0xC02A: "DataField",
	0xC02B: "DataChanged",
}

var labelProperties = map[uint16]string{
	0xC000: "Caption",
	0xC001: "Name",
	0xC002: "Index",
	0xC003: "BackColor",
	0xC004: "ForeColor",
	0xC005: "Left",
	0xC006: "Top",
	0xC007: "Width",
	0xC008: "Height",
	0xC009: "Enabled",
	0xC00A: "Visible",
	0xC00B: "MousePointer",
	0xC00C: "FontName",
	0xC00D: "FontSize",
	0xC00E: "FontBold",
	0xC00F: "FontItalic",
	0xC010: "FontStrikethru",
	0xC011: "FontUnderline",
	0xC012: "TabIndex",
	0xC013: "BorderStyle",
	0xC014: "Alignment",
	0xC015: "LinkTopic",
	0xC016: "LinkItem",
	0xC017: "LinkMode",
	0xC018: "AutoSize",
	0xC019: "Parent",
	0xC01A: "DragMode",
	0xC01B: "DragIcon",
	0xC01C: "LinkTimeout",
	0xC01D: "Tag",
	0xC01E: "WordWrap",
	0xC01F: "BackStyle",
	0xC020: "DataSource",
	0xC021: "DataField",
	0xC022: "DataChanged",
}

var frameProperties = map[uint16]string{
	0xC000: "Caption",
	0xC001: "Name",
	0xC002: "Index",
	0xC003: "BackColor",
	0xC004: "ForeColor",
	0xC005: "Left",
	0xC006: "Top",
	0xC007: "Width",
	0xC008: "Height",
	0xC009: "Enabled",
	0xC00A: "Visible",
	0xC00B: "MousePointer",
	0xC00C: "FontName",
	0xC00D: "FontSize",
	0xC00E: "FontBold",
	0xC00F: "FontItalic",
	0xC010: "FontStrikethru",
	0xC011: "FontUnderline",
	0xC012: "TabIndex",
	0xC013: "Parent",
	0xC014: "DragMode",
	0xC015: "DragIcon",
	0xC016: "Tag",
	0xC017: "hWnd",
	0xC018: "ClipControls",
	0xC019: "HelpContextID",
}

var pictureBoxProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "BackColor",
	0xC002: "Index",
	0xC003: "Picture",
	0xC004: "ForeColor",
	0xC005: "Left",
	0xC006: "Top",
	0xC007: "Width",
	0xC008: "Height",
	0xC009: "Enabled",
	0xC00A: "Visible",
	0xC00B: "MousePointer",
	0xC00C: "FontName",
	0xC00D: "FontSize",
	0xC00E: "FontBold",
	0xC00F: "FontItalic",
	0xC010: "FontStrikethru",
	0xC011: "FontUnderline",
	0xC012: "TabIndex",
	0xC013: "hDC",
	0xC014: "CurrentX",
	0xC015: "CurrentY",
	0xC016: "ScaleLeft",
	0xC017: "ScaleTop",
	0xC018: "ScaleWidth",
	0xC019: "ScaleHeight",
	0xC01A: "ScaleMode",
	0xC01B: "FontTransparent",
	0xC01C: "DrawStyle",
	0xC01D: "DrawWidth",
	0xC01E: "FillStyle",
	0xC01F: "FillColor",
	0xC020: "DrawMode",
	0xC021: "AutoRedraw",
	0xC023: "AutoSize",
	0xC024: "BorderStyle",
	0xC025: "LinkTopic",
	0xC026: "LinkItem",
	0xC027: "LinkMode",
	0xC028: "Image",
	0xC029: "Parent",
	0xC02A: "DragMode",
	0xC02B: "DragIcon",
	0xC02C: "LinkTimeout",
	0xC02D: "TabStop",
	0xC02E: "Tag",
	0xC02F: "hWnd",
	0xC030: "ClipControls",
	0xC031: "HelpContextID",
	0xC032: "Align",
	0xC034: "DataSource",
	0xC035: "DataField",
	0xC036: "DataChanged",
}

var hScrollBarProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "Index",
	0xC002: "Left",
	0xC003: "Top",
	0xC004: "Width",
	0xC005: "Height",
	0xC006: "Enabled",
	0xC007: "Visible",
	0xC008: "MousePointer",
	0xC009: "TabIndex",
	0xC00A: "Min",
	0xC00B: "Max",
	0xC00C: "SmallChange",
	0xC00D: "LargeChange",
	0xC00E: "Value",
	0xC00F: "Parent",
	0xC010: "DragMode",
	0xC011: "DragIcon",
	0xC012: "TabStop",
	0xC013: "Tag",
	0xC014: "hWnd",
	0xC015: "HelpContextID",
}

var vScrollBarProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "Index",
	0xC002: "Left",
	0xC003: "Top",
	0xC004: "Width",
	0xC005: "Height",
	0xC006: "Enabled",
	0xC007: "Visible",
	0xC008: "MousePointer",
	0xC009: "TabIndex",
	0xC00A: "Min",
	0xC00B: "Max",
	0xC00C: "SmallChange",
	0xC00D: "LargeChange",
	0xC00E: "Value",
	0xC00F: "Parent",
	0xC010: "DragMode",
	0xC011: "DragIcon",
	0xC012: "TabStop",
	0xC013: "Tag",
	0xC014: "hWnd",
	0xC015: "HelpContextID",
}

var timerProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "Index",
	0xC002: "Enabled",
	0xC003: "Interval",
	0xC004: "Parent",
	0xC005: "Tag",
	0xC007: "Left",
	0xC008: "Top",
}

var imageProperties = map[uint16]string{
	0xC000: "Name",
	0xC001: "Index",
	0xC002: "Picture",
	0xC003: "Left",
	0xC004: "Top",
	0xC005: "Width",
	0xC006: "Height",
	0xC007: "Enabled",
	0xC008: "Visible",
	0xC009: "MousePointer",
	0xC00A: "Stretch",
	0xC00B: "Parent",
	0xC00C: "DragMode",
	0xC00D: "DragIcon",
	0xC00E: "Tag",
	0xC00F: "BorderStyle",
	0xC010: "DataSource",
	0xC011: "DataField",
	0xC012: "DataChanged",
}

// formatProperty resolves 16-bit property IDs into their Visual Basic property names,
// taking into account the specific control type when available.
func formatProperty(ctrlType string, propID uint16, isWrite bool, val string) string {
	switch ctrlType {
	case "Form", "MDIForm":
		if name, ok := formProperties[propID]; ok && name != "" {
			return name
		}
	case "ComboBox":
		if name, ok := comboBoxProperties[propID]; ok && name != "" {
			return name
		}
	case "ListBox":
		if name, ok := listBoxProperties[propID]; ok && name != "" {
			return name
		}
	case "DirListBox":
		if name, ok := dirListBoxProperties[propID]; ok && name != "" {
			return name
		}
	case "FileListBox":
		if name, ok := fileListBoxProperties[propID]; ok && name != "" {
			return name
		}
	case "MMControl":
		if name, ok := mmControlProperties[propID]; ok && name != "" {
			return name
		}
	case "OptionButton":
		if name, ok := optionButtonProperties[propID]; ok && name != "" {
			return name
		}
	case "CheckBox":
		if name, ok := checkBoxProperties[propID]; ok && name != "" {
			return name
		}
	case "CommandButton":
		if name, ok := commandButtonProperties[propID]; ok && name != "" {
			return name
		}
	case "TextBox":
		if name, ok := textBoxProperties[propID]; ok && name != "" {
			return name
		}
	case "Label":
		if name, ok := labelProperties[propID]; ok && name != "" {
			return name
		}
	case "Frame":
		if name, ok := frameProperties[propID]; ok && name != "" {
			return name
		}
	case "PictureBox":
		if name, ok := pictureBoxProperties[propID]; ok && name != "" {
			return name
		}
	case "HScrollBar":
		if name, ok := hScrollBarProperties[propID]; ok && name != "" {
			return name
		}
	case "VScrollBar":
		if name, ok := vScrollBarProperties[propID]; ok && name != "" {
			return name
		}
	case "Timer":
		if name, ok := timerProperties[propID]; ok && name != "" {
			return name
		}
	case "Image":
		if name, ok := imageProperties[propID]; ok && name != "" {
			return name
		}
	}

	// Generic / fallback mapping
	switch propID {
	case 0xC000:
		return "Caption"
	case 0xC002:
		if isWrite || val == "True" || val == "False" {
			return "Enabled"
		}
		return "hWnd"
	case 0xC003:
		return "BackColor"
	case 0xC004:
		return "ForeColor"
	case 0xC005:
		return "Left"
	case 0xC006:
		return "Top"
	case 0xC007:
		return "Width"
	case 0xC008:
		return "Enabled"
	case 0xC009:
		return "Visible"
	case 0xC00A:
		return "MousePointer"
	case 0xC00B:
		return "Text"
	case 0xC00C:
		return "FontName"
	case 0xC00D:
		return "FontBold"
	case 0xC00E:
		return "FontItalic"
	case 0xC00F:
		return "FontStrikethru"
	case 0xC010:
		return "FontUnderline"
	case 0xC011:
		return "FontSize"
	case 0xC012:
		return "TabIndex"
	case 0xC013:
		return "ListCount"
	case 0xC014:
		return "ListIndex"
	case 0xC015:
		return "List"
	case 0xC016:
		return "BorderStyle"
	case 0xC017:
		return "TabStop"
	case 0xC018:
		return "Tag"
	case 0xC025:
		return "Command"
	case 0xC028:
		return "DeviceType"
	case 0xC02A:
		return "FileName"
	case 0xC02E:
		return "Visible"
	case 0xC031:
		return "KeyPreview"
	case 0xC032:
		return "ClipControls"
	case 0xC036:
		return "Value"
	case 0xC037:
		return "Min"
	case 0xC038:
		return "Max"
	default:
		return fmt.Sprintf("Prop_%04X", propID)
	}
}

// UnhandledOpcodeWarning records an opcode that was not explicitly handled
// by the decompiler and fell through to the default handler.
type UnhandledOpcodeWarning struct {
	ModuleName string
	ProcName   string
	PC         int
	TokenID    uint16
	AltToken   uint16
	Keyword    string
	Case       uint16
	Params     []uint16
}

// Disassembler decodes P-code byte streams into valid Visual Basic 3 source code.
type Disassembler struct {
	table    *OpcodeTable
	proj     *Project
	Warnings []UnhandledOpcodeWarning
}

// NewDisassembler creates a new P-code disassembler instance.
func NewDisassembler(proj *Project) *Disassembler {
	return &Disassembler{
		table: GetOpcodeTable(),
		proj:  proj,
	}
}

// DisassembleProcedure decompiles a single procedure into clean VB3 source code.
func (d *Disassembler) DisassembleProcedure(proc *Procedure) (string, error) {
	if !proc.IsLocal {
		return d.formatDeclaration(proc), nil
	}
	if len(proc.Bytecode) == 0 {
		return d.formatEmptyProcedure(proc), nil
	}

	var mod *Module
	if d.proj != nil {
		for _, m := range d.proj.Modules {
			if m.Index == proc.ModuleIndex {
				mod = m
				break
			}
		}
		if mod == nil {
			for _, m := range d.proj.Modules {
				for _, p := range m.Procedures {
					if p == proc {
						mod = m
						break
					}
				}
				if mod != nil {
					break
				}
			}
		}
	}

	state := &procDisasmState{
		proc:      proc,
		mod:       mod,
		proj:      d.proj,
		table:     d.table,
		bc:        proc.Bytecode,
		vars:      make(map[uint16]string),
		varTypes:  make(map[uint16]string),
		arrayVars: make(map[uint16]bool),
	}

	code, err := state.disassemble()
	if len(state.warnings) > 0 {
		d.Warnings = append(d.Warnings, state.warnings...)
	}
	return code, err
}

// DisassembleModule decompiles all procedures in a module into full VB3 module source code.
func (d *Disassembler) DisassembleModule(mod *Module) (string, error) {
	var sb strings.Builder

	// Module header
	sb.WriteString(fmt.Sprintf("' %s.bas\n", mod.Name))
	sb.WriteString("Option Explicit\n\n")

	if len(mod.ModuleVars) > 0 {
		for _, mv := range mod.ModuleVars {
			sb.WriteString(mv)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Separate declarations from procedures
	var decls []*Procedure
	var locals []*Procedure

	for _, p := range mod.Procedures {
		if p.IsLocal {
			locals = append(locals, p)
		} else {
			decls = append(decls, p)
		}
	}

	// Emit API Declarations
	for _, p := range decls {
		declText := d.formatDeclaration(p)
		if declText != "" {
			sb.WriteString(declText)
			sb.WriteString("\n")
		}
	}
	if len(decls) > 0 {
		sb.WriteString("\n")
	}

	// Emit Local Procedures
	for i, p := range locals {
		code, err := d.DisassembleProcedure(p)
		if err != nil {
			sb.WriteString(fmt.Sprintf("' Error decompiling %s: %v\n", p.Name, err))
			continue
		}
		sb.WriteString(code)
		if i < len(locals)-1 {
			sb.WriteString("\n\n")
		}
	}

	return sb.String(), nil
}

// DisassembleFormCode decompiles declarations and procedures for appending directly below End in a .FRM file.
func (d *Disassembler) DisassembleFormCode(mod *Module) (string, error) {
	var sb strings.Builder
	sb.WriteString("\nOption Explicit\n\n")

	if len(mod.ModuleVars) > 0 {
		for _, mv := range mod.ModuleVars {
			sb.WriteString(mv)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	var decls []*Procedure
	var locals []*Procedure

	for _, p := range mod.Procedures {
		if p.IsLocal {
			locals = append(locals, p)
		} else {
			decls = append(decls, p)
		}
	}

	for _, p := range decls {
		declText := d.formatDeclaration(p)
		if declText != "" {
			sb.WriteString(declText)
			sb.WriteString("\n")
		}
	}
	if len(decls) > 0 {
		sb.WriteString("\n")
	}

	for i, p := range locals {
		code, err := d.DisassembleProcedure(p)
		if err != nil {
			sb.WriteString(fmt.Sprintf("' Error decompiling %s: %v\n", p.Name, err))
			continue
		}
		sb.WriteString(code)
		if i < len(locals)-1 {
			sb.WriteString("\n\n")
		}
	}

	return sb.String(), nil
}

func formatExternalParams(p *Procedure, alias string) string {
	switch alias {
	case "FindChildByClass", "FindChildByTitle":
		return "ByVal p1%, ByVal p2$"
	case "Findwindow":
		return "ByVal p1 As Any, ByVal p2 As Any"
	case "GetCurrentDirectory", "GetMenu", "GetMenuItemCount", "GetMenuItemID", "GetMenuString", "GetSubMenu", "ptGetStringFromAddress":
		return ""
	case "getnextwindow":
		return "ByVal p1%, ByVal p2%"
	case "GetParent", "getwindowtextlength":
		return "ByVal p1%"
	case "getwindowtext":
		return "ByVal p1%, ByVal p2$, ByVal p3%"
	case "SendMessage":
		if p.NameID == 0x0221 {
			return "ByVal p1%, ByVal p2%, ByVal p3%, ByVal p4$"
		}
		return "ByVal p1%, ByVal p2%, ByVal p3%, ByVal p4&"
	case "setwindowpos":
		return "ByVal p1%, ByVal p2%, ByVal p3%, ByVal p4%, ByVal p5%, ByVal p6%, ByVal p7%"
	default:
		cnt := int(p.PubOrPriv >> 9)
		var list []string
		for i := 1; i <= cnt; i++ {
			list = append(list, fmt.Sprintf("ByVal p%d%%", i))
		}
		return strings.Join(list, ", ")
	}
}

func (d *Disassembler) formatDeclaration(p *Procedure) string {
	procType := "Sub"
	retType := ""
	fnType := (p.SubOrFunc >> 8) & 7
	if fnType != 0 {
		procType = "Function"
		if dt, ok := DataTypes[fnType]; ok {
			retType = " As " + dt
		} else {
			retType = " As Variant"
		}
	}

	name := p.DeclaredName()

	lib := p.LibName
	if lib == "" {
		lib = "User"
	}
	alias := p.AliasName

	aliasClause := ""
	if alias != "" && alias != name {
		aliasClause = fmt.Sprintf(" Alias %q", alias)
	}

	params := formatExternalParams(p, alias)

	return fmt.Sprintf("Declare %s %s Lib %q%s (%s)%s", procType, name, lib, aliasClause, params, retType)
}

func (d *Disassembler) formatEmptyProcedure(p *Procedure) string {
	procType := "Sub"
	retType := ""
	if p.IsFunction() {
		procType = "Function"
		fnType := (p.SubOrFunc >> 8) & 7
		if dt, ok := DataTypes[fnType]; ok {
			retType = " As " + dt
		} else {
			retType = " As Variant"
		}
	}
	return fmt.Sprintf("%s %s ()%s\nEnd %s", procType, p.Name, retType, procType)
}

type procDisasmState struct {
	proc             *Procedure
	mod              *Module
	proj             *Project
	table            *OpcodeTable
	bc               []byte
	pc               int
	stack            []string
	lines            []string
	labels           map[int]string
	vars             map[uint16]string // offset -> variable name
	varTypes         map[uint16]string // offset -> variable type ("Integer", "Long", "String")
	retSlot          uint16            // Return value slot for functions
	hasRet           bool
	indent           int
	prevOpWas        bool
	ifCond           string                   // In-flight single-line If condition
	lastSingleLineIf bool                     // Tracks if the last emitted line was a single-line If
	singleLineElse   bool                     // In-flight single-line Else clause
	caseIndentStack  []bool                   // Tracks indentation inside Case blocks for nested Select Case
	arrayVars        map[uint16]bool          // offset -> true if variable is an array (e.g. ReDim'd)
	pendingAsType    string                   // In-flight "As <Type>" clause for ReDim
	warnings         []UnhandledOpcodeWarning // Warnings for unhandled opcodes
}

func (s *procDisasmState) getTargetControlType(target string) string {
	// If target has a dot (e.g. "frm7.control1")
	if idx := strings.Index(target, "."); idx >= 0 {
		formPart := target[:idx]
		ctrlPart := target[idx+1:]
		if s.proj != nil {
			for _, m := range s.proj.Modules {
				if strings.EqualFold(m.Name, formPart) {
					if m.ControlTypes != nil {
						if ct, ok := m.ControlTypes[ctrlPart]; ok {
							return ct
						}
					}
				}
			}
		}
		if s.mod != nil && (formPart == "Me" || strings.EqualFold(s.mod.Name, formPart)) {
			if s.mod.ControlTypes != nil {
				if ct, ok := s.mod.ControlTypes[ctrlPart]; ok {
					return ct
				}
			}
		}
	}

	if target == "Me" || (s.mod != nil && s.mod.IsForm && strings.EqualFold(target, s.mod.Name)) {
		return "Form"
	}
	if s.proj != nil {
		for _, m := range s.proj.Modules {
			if m.IsForm && strings.EqualFold(target, m.Name) {
				return "Form"
			}
		}
	}

	if s.mod != nil && s.mod.ControlTypes != nil {
		if ct, ok := s.mod.ControlTypes[target]; ok {
			return ct
		}
	}
	if s.proj != nil {
		for _, m := range s.proj.Modules {
			if m.ControlTypes != nil {
				if ct, ok := m.ControlTypes[target]; ok {
					return ct
				}
			}
		}
	}
	return ""
}

func (s *procDisasmState) resolveControlName(target string, ctrlIdx int) string {
	var targetMod *Module
	if s.proj != nil {
		for _, m := range s.proj.Modules {
			if strings.EqualFold(m.Name, target) {
				targetMod = m
				break
			}
		}
	}
	if targetMod == nil {
		targetMod = s.mod
	}
	if targetMod != nil && targetMod.ControlByIndex != nil {
		if name, ok := targetMod.ControlByIndex[ctrlIdx]; ok && name != "" {
			return name
		}
	}
	return fmt.Sprintf("control%d", ctrlIdx)
}

func (s *procDisasmState) formatPropertyForTarget(target string, propID uint16, isWrite bool, val string) string {
	ctrlType := s.getTargetControlType(target)
	return formatProperty(ctrlType, propID, isWrite, val)
}

func typeRank(t string) int {
	switch t {
	case "String":
		return 7
	case "Currency":
		return 6
	case "Double":
		return 5
	case "Single":
		return 4
	case "Long":
		return 3
	case "Integer":
		return 2
	case "Variant":
		return 1
	default:
		return 0
	}
}

func (s *procDisasmState) recordVarTypeByOffset(off uint16, typ string) {
	curr := s.varTypes[off]
	if typeRank(typ) > typeRank(curr) {
		s.varTypes[off] = typ
	}
}

func (s *procDisasmState) recordVarType(varName string, typ string) {
	clean := strings.TrimRight(varName, "$%&!#")
	for off, name := range s.vars {
		if name == clean {
			s.recordVarTypeByOffset(off, typ)
			return
		}
	}
}

func (s *procDisasmState) getVarTypeByName(varName string) (string, bool) {
	clean := strings.TrimRight(varName, "$%&!#")
	for off, name := range s.vars {
		if name == clean {
			t, ok := s.varTypes[off]
			return t, ok
		}
	}
	return "", false
}

func (s *procDisasmState) formatVarRef(offset uint16) string {
	name := s.getVarName(offset)
	if s.varTypes[offset] == "String" && (strings.HasPrefix(name, "p") || strings.HasPrefix(name, "l") || strings.HasPrefix(name, "gv")) {
		return name + "$"
	}
	return name
}

func (s *procDisasmState) push(expr string) {
	s.stack = append(s.stack, expr)
	s.prevOpWas = false
}

func (s *procDisasmState) pop() string {
	if len(s.stack) == 0 {
		return ""
	}
	val := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	return val
}

func (s *procDisasmState) emitLine(line string) {
	if line == "" {
		return
	}
	if s.singleLineElse {
		if len(s.lines) > 0 {
			s.lines[len(s.lines)-1] += " Else " + line
		} else {
			pad := strings.Repeat("    ", s.indent)
			s.lines = append(s.lines, pad+"Else "+line)
		}
		s.singleLineElse = false
		s.lastSingleLineIf = false
		return
	}
	s.lastSingleLineIf = false
	pad := strings.Repeat("    ", s.indent)
	s.lines = append(s.lines, pad+line)
}

func (s *procDisasmState) emitStatement(stmt string) {
	if stmt == "" {
		return
	}
	if s.ifCond != "" {
		s.emitLine(fmt.Sprintf("If %s Then %s", s.ifCond, stmt))
		s.ifCond = ""
		s.lastSingleLineIf = true
		return
	}
	s.emitLine(stmt)
}

func formatArgList(args []string) string {
	for len(args) > 0 && strings.HasPrefix(args[len(args)-1], "<default") {
		args = args[:len(args)-1]
	}
	res := make([]string, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "<default") {
			res[i] = ""
		} else {
			res[i] = a
		}
	}
	return strings.Join(res, ", ")
}

func (s *procDisasmState) getVarName(offset uint16) string {
	if s.hasRet && offset == s.retSlot {
		return s.proc.Name
	}
	if name, ok := s.vars[offset]; ok {
		return name
	}

	// In Form/code modules: check mod.ControlMap first!
	if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
		mod := s.proj.Modules[s.proc.ModuleIndex-1]
		if ctrlName, ok := mod.ControlMap[offset]; ok {
			s.vars[offset] = ctrlName
			return ctrlName
		}
	}

	var name string
	isEvent := strings.Contains(s.proc.Name, "_")
	if isEvent {
		name = fmt.Sprintf("l%04X", offset)
	} else if offset >= 0x0010 && offset < 0x0500 {
		var modBytes []byte
		if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
			modBytes = s.proj.Modules[s.proc.ModuleIndex-1].ModBytes
		}
		if len(modBytes) > 0 && int(offset)+2 <= len(modBytes) {
			bp := int16(binary.LittleEndian.Uint16(modBytes[offset : offset+2]))
			if bp >= 6 && bp%2 == 0 && bp <= 60 {
				name = fmt.Sprintf("p%04X", offset)
			} else {
				name = fmt.Sprintf("l%04X", offset)
			}
		} else if offset < 0x0040 {
			name = fmt.Sprintf("p%04X", offset)
		} else {
			name = fmt.Sprintf("l%04X", offset)
		}
	} else {
		name = fmt.Sprintf("gv%04X", offset)
	}

	s.vars[offset] = name
	return name
}

// First pass: scan jump targets and variable references
func (s *procDisasmState) scanPass() {
	s.labels = make(map[int]string)
	pc := 0
	bc := s.bc

	isFunc := s.proc.IsFunction()
	var varRefs []uint16
	var assignedVars []uint16
	tokenTypes := make(map[uint16]string)

	for pc+2 <= len(bc) {
		token := binary.LittleEndian.Uint16(bc[pc : pc+2])
		info, altToken := s.table.Lookup(token)
		pc += 2

		if info == nil {
			continue
		}

		if info.Case == 8 {
			if pc+6 <= len(bc) {
				totLen := int(binary.LittleEndian.Uint16(bc[pc : pc+2]))
				pc += totLen + 2
			}
			continue
		}

		if info.Case == 10 || info.Case == 11 || info.Case == 12 || info.Case == 14 {
			if pc+2 <= len(bc) {
				target := int(int16(binary.LittleEndian.Uint16(bc[pc : pc+2])))
				if target > 0 && target < len(bc) {
					s.labels[target] = fmt.Sprintf("L%04X", target)
				}
				pc += 2
			}
			continue
		}

		if info.Case == 13 {
			if pc+2 <= len(bc) {
				cnt := int(binary.LittleEndian.Uint16(bc[pc:pc+2])) / 2
				pc += 2
				for i := 0; i < cnt && pc+2 <= len(bc); i++ {
					target := int(binary.LittleEndian.Uint16(bc[pc : pc+2]))
					if target > 0 && target < len(bc) {
						s.labels[target] = fmt.Sprintf("L%04X", target)
					}
					pc += 2
				}
			}
			continue
		}

		typeCode := (altToken >> 10) & 0x07
		var params []uint16
		for i := 0; i < info.NumParams && pc+2 <= len(bc); i++ {
			pVal := binary.LittleEndian.Uint16(bc[pc : pc+2])
			pc += 2
			params = append(params, pVal)
		}

		if strings.HasPrefix(info.Keyword, "var") {
			var varSlot uint16
			if (info.Keyword == "var()" || info.Keyword == "var()=") && len(params) >= 2 {
				varSlot = params[1]
			} else if len(params) >= 1 {
				varSlot = params[0]
			}

			if varSlot != 0 {
				_, _, iToken2 := s.table.LookupControl(token)
				var tName string
				if iToken2 >= 1 && iToken2 <= 7 {
					tName = DataTypes[uint16(iToken2)]
				}
				if tName != "" {
					if typeRank(tName) > typeRank(tokenTypes[varSlot]) {
						tokenTypes[varSlot] = tName
					}
				}
				if typeCode == 7 || iToken2 == 7 {
					s.recordVarTypeByOffset(varSlot, "String")
				}
				if strings.HasPrefix(info.Keyword, "var=") && info.Keyword != "var()=" {
					assignedVars = append(assignedVars, varSlot)
				} else {
					varRefs = append(varRefs, varSlot)
				}
			}
		}
	}

	// Return slot determination:
	// In VB3 compiled code, if a function assigns a return value, the assignment target
	// is allocated at a specific BP offset: BP - 22 - sizeof(ReturnType).
	// Integer: -24 (0xFFE8)
	// Long/Single: -26 (0xFFE6)
	// Double/Currency: -30 (0xFFE2)
	// Variant: -38 (0xFFDA)
	// String: 1
	retSlotBpMap := map[uint16]int16{
		1: -24, // Integer
		2: -26, // Long
		3: -26, // Single
		4: -30, // Double
		5: -30, // Currency
		6: -38, // Variant
		7: 1,   // String
	}

	var modBytes []byte
	if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
		modBytes = s.proj.Modules[s.proc.ModuleIndex-1].ModBytes
	}

	if isFunc && len(modBytes) > 0 {
		fnType := (s.proc.SubOrFunc >> 8) & 7
		if expectedBp, ok := retSlotBpMap[fnType]; ok {
			for _, v := range assignedVars {
				if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
					mod := s.proj.Modules[s.proc.ModuleIndex-1]
					if _, ok := mod.ControlMap[v]; ok {
						continue
					}
				}
				if int(v)+2 <= len(modBytes) {
					bp := int16(binary.LittleEndian.Uint16(modBytes[v : v+2]))
					if bp == expectedBp {
						s.retSlot = v
						s.hasRet = true
						break
					}
				}
			}
		}
	} else if isFunc && len(assignedVars) > 0 {
		var validCandidates []uint16
		for _, v := range assignedVars {
			if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
				mod := s.proj.Modules[s.proc.ModuleIndex-1]
				if _, ok := mod.ControlMap[v]; ok {
					continue
				}
			}
			validCandidates = append(validCandidates, v)
		}
		if len(validCandidates) > 0 {
			minAssigned := validCandidates[0]
			for _, v := range validCandidates {
				if v < minAssigned {
					minAssigned = v
				}
			}
			s.retSlot = minAssigned
			s.hasRet = true
		}
	}

	// Classify parameter variables:
	isEvent := strings.Contains(s.proc.Name, "_")
	numParams := int(s.proc.PubOrPriv >> 9)
	var evParams []eventParamDef
	if isEvent {
		evSuffix := s.proc.Name[strings.LastIndex(s.proc.Name, "_")+1:]
		if defs, ok := eventParamDefs[evSuffix]; ok {
			evParams = defs
			numParams = len(defs)
		} else {
			numParams = 0
		}
	}

	allVars := append(varRefs, assignedVars...)
	sort.Slice(allVars, func(i, j int) bool { return allVars[i] < allVars[j] })

	paramCount := 0
	for _, v := range allVars {
		if s.hasRet && v == s.retSlot {
			continue
		}
		if _, exists := s.vars[v]; exists {
			continue
		}
		if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
			mod := s.proj.Modules[s.proc.ModuleIndex-1]
			if ctrlName, ok := mod.ControlMap[v]; ok {
				s.vars[v] = ctrlName
				continue
			}
		}
		if isEvent {
			if paramCount < len(evParams) && v >= 0x0010 && v < 0x0500 {
				s.vars[v] = evParams[paramCount].Name
				s.recordVarTypeByOffset(v, evParams[paramCount].Type)
				paramCount++
			} else {
				_ = s.getVarName(v)
				if tName, ok := tokenTypes[v]; ok {
					s.recordVarTypeByOffset(v, tName)
				}
			}
		} else {
			isParam := false
			if numParams > 0 && paramCount < numParams {
				if len(modBytes) > 0 && int(v)+2 <= len(modBytes) {
					bp := int16(binary.LittleEndian.Uint16(modBytes[v : v+2]))
					if bp >= 6 && bp%2 == 0 && bp <= 60 {
						isParam = true
					}
				} else if len(modBytes) == 0 {
					isParam = true
				}
			}

			if isParam && v >= 0x0010 && v < 0x0500 {
				s.vars[v] = fmt.Sprintf("p%04X", v)
				if tName, ok := tokenTypes[v]; ok {
					s.recordVarTypeByOffset(v, tName)
				}
				paramCount++
			} else {
				_ = s.getVarName(v)
				if tName, ok := tokenTypes[v]; ok {
					s.recordVarTypeByOffset(v, tName)
				}
			}
		}
	}

	if !isEvent && paramCount == 0 && numParams > 0 {
		targetParams := numParams
		if s.proc != nil && len(s.proc.ParamTypes) > 0 && len(s.proc.ParamTypes) < targetParams {
			targetParams = len(s.proc.ParamTypes)
		}
		candOff := uint16(0x0020)
		if s.hasRet && s.retSlot != 0 {
			candOff = s.retSlot + 2
		} else if len(allVars) > 0 {
			minOff := allVars[0]
			for _, v := range allVars {
				if v < minOff {
					minOff = v
				}
			}
			if minOff > uint16(targetParams*2) {
				candOff = minOff - uint16(targetParams*2)
			}
		}
		for paramCount < targetParams {
			for {
				if _, exists := s.vars[candOff]; !exists {
					break
				}
				candOff += 2
			}
			pName := fmt.Sprintf("p%04X", candOff)
			s.vars[candOff] = pName
			pType := "Variant"
			if s.proc != nil && s.proc.ParamTypes != nil && s.proc.ParamTypes[paramCount] != "" {
				pType = s.proc.ParamTypes[paramCount]
			}
			s.recordVarTypeByOffset(candOff, pType)
			paramCount++
			candOff += 2
		}
	}
}

func (s *procDisasmState) disassemble() (string, error) {
	s.scanPass()

	s.pc = 0
	s.indent = 1
	bc := s.bc

	for s.pc < len(bc) {
		startPC := s.pc

		if lbl, ok := s.labels[startPC]; ok {
			s.lines = append(s.lines, lbl+":")
		}

		if s.pc+2 > len(bc) {
			break
		}

		token := binary.LittleEndian.Uint16(bc[s.pc : s.pc+2])
		info, altToken := s.table.Lookup(token)
		s.pc += 2

		if info == nil {
			s.emitLine(fmt.Sprintf("' <UNKNOWN_OPCODE: 0x%04X @ offset 0x%04X>", token, startPC))
			continue
		}

		if info.Case == 8 {
			if s.pc+6 <= len(bc) {
				totLen := int(binary.LittleEndian.Uint16(bc[s.pc : s.pc+2]))
				s.pc += 4 // totLen (2) + dummy (2)
				strLen := 0
				if totLen > 2 && s.pc+2 <= len(bc) {
					strLen = int(binary.LittleEndian.Uint16(bc[s.pc : s.pc+2]))
					s.pc += 2
				}

				var rawBytes []byte
				if s.pc+strLen <= len(bc) {
					rawBytes = bc[s.pc : s.pc+strLen]
					s.pc += strLen
				}
				rem := totLen - strLen - 4
				if rem > 0 && s.pc+rem <= len(bc) {
					s.pc += rem
				}

				s.push(win1252.FormatVBString(rawBytes))
			}
			continue
		}

		if info.Case == 13 {
			if s.pc+2 <= len(bc) {
				cnt := int(binary.LittleEndian.Uint16(bc[s.pc:s.pc+2])) / 2
				s.pc += 2
				s.pc += cnt * 2
			}
			continue
		}

		var params []uint16
		for i := 0; i < info.NumParams && s.pc+2 <= len(bc); i++ {
			params = append(params, binary.LittleEndian.Uint16(bc[s.pc:s.pc+2]))
			s.pc += 2
		}

		s.decodeInstruction(info, altToken, params, startPC)
		if info.Case == 4 || info.Keyword == "eos" {
			break
		}
	}

	return s.renderProcedure(), nil
}

func (s *procDisasmState) decodeInstruction(info *OpcodeInfo, altToken uint16, params []uint16, pc int) {
	kw := info.Keyword

	switch {
	case info.Case == 5 || kw == "nl":
		s.lastSingleLineIf = false
		if len(s.stack) > 0 {
			stmt := s.pop()
			if stmt != "" && !strings.HasPrefix(stmt, "'") && stmt != "End" && stmt != "End " {
				s.emitStatement(stmt)
			}
			s.stack = s.stack[:0]
		} else if s.ifCond != "" {
			s.emitLine(fmt.Sprintf("If %s Then", s.ifCond))
			s.indent++
			s.ifCond = ""
		}

	case info.Case == 4 || kw == "eos":
		s.lastSingleLineIf = false
		if len(s.stack) > 0 {
			stmt := s.pop()
			if stmt != "" && !strings.HasPrefix(stmt, "'") && stmt != "End" && stmt != "End " {
				s.emitStatement(stmt)
			}
			s.stack = s.stack[:0]
		} else if s.ifCond != "" {
			s.emitLine(fmt.Sprintf("If %s Then", s.ifCond))
			s.indent++
			s.ifCond = ""
		}

	case kw == "pop.var=":
		var propID uint16
		if len(params) > 0 {
			propID = params[0]
		}
		target := s.pop()
		val := s.pop()
		if target == "" {
			target = "Me"
		}
		if propID&0xC000 == 0x8000 {
			ctrlIdx := int(propID & 0x3FFF)
			ctrlName := s.resolveControlName(target, ctrlIdx)
			targetExpr := ctrlName
			if target != "" && target != "Me" {
				targetExpr = fmt.Sprintf("%s.%s", target, ctrlName)
			}
			stmt := fmt.Sprintf("%s = %s", targetExpr, val)
			s.emitStatement(stmt)
			break
		}
		propName := s.formatPropertyForTarget(target, propID, true, val)
		stmt := fmt.Sprintf("%s.%s = %s", target, propName, val)
		s.emitStatement(stmt)

	case kw == "pop.var":
		var propID uint16
		if len(params) > 0 {
			propID = params[0]
		}
		target := s.pop()
		if propID&0xC000 == 0x8000 {
			ctrlIdx := int(propID & 0x3FFF)
			ctrlName := s.resolveControlName(target, ctrlIdx)
			if target == "" || target == "Me" {
				s.push(ctrlName)
			} else {
				s.push(fmt.Sprintf("%s.%s", target, ctrlName))
			}
			break
		}
		if target == "" {
			target = "Me"
		}
		propName := s.formatPropertyForTarget(target, propID, false, "")
		s.push(fmt.Sprintf("%s.%s", target, propName))

	case kw == "pop.var()=":
		numArgs := 1
		propID := uint16(0)
		if len(params) > 0 {
			numArgs = int(params[0])
		}
		if len(params) > 1 {
			propID = params[1]
		}
		target := s.pop()
		var args []string
		for i := 0; i < numArgs && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		val := s.pop()
		if target == "" {
			target = "Me"
		}
		if propID&0xC000 == 0x8000 {
			ctrlIdx := int(propID & 0x3FFF)
			ctrlName := s.resolveControlName(target, ctrlIdx)
			targetExpr := ctrlName
			if target != "" && target != "Me" {
				targetExpr = fmt.Sprintf("%s.%s", target, ctrlName)
			}
			stmt := fmt.Sprintf("%s(%s) = %s", targetExpr, strings.Join(args, ", "), val)
			s.emitStatement(stmt)
			break
		}
		propName := s.formatPropertyForTarget(target, propID, true, val)
		stmt := fmt.Sprintf("%s.%s(%s) = %s", target, propName, strings.Join(args, ", "), val)
		s.emitStatement(stmt)

	case kw == "pop.var()":
		numArgs := 1
		propID := uint16(0)
		if len(params) > 0 {
			numArgs = int(params[0])
		}
		if len(params) > 1 {
			propID = params[1]
		}
		target := s.pop()
		var args []string
		for i := 0; i < numArgs && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		if propID&0xC000 == 0x8000 {
			ctrlIdx := int(propID & 0x3FFF)
			ctrlName := s.resolveControlName(target, ctrlIdx)
			targetExpr := ctrlName
			if target != "" && target != "Me" {
				targetExpr = fmt.Sprintf("%s.%s", target, ctrlName)
			}
			s.push(fmt.Sprintf("%s(%s)", targetExpr, strings.Join(args, ", ")))
			break
		}
		if target == "" {
			target = "Me"
		}
		propName := s.formatPropertyForTarget(target, propID, false, "")
		s.push(fmt.Sprintf("%s.%s(%s)", target, propName, strings.Join(args, ", ")))

	case kw == "met":
		methodID := uint16(0)
		argCount := 0
		if len(params) > 0 {
			methodID = params[0]
		}
		if len(params) > 1 {
			argCount = int(params[1])
		}
		var args []string
		for i := 0; i < argCount && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		target := s.pop()
		if target == "" {
			target = "Me"
		}
		normID := methodID
		if normID >= 0xC000 {
			normID = normID & 0x0FFF
		}
		methodName, ok := standardMethods[normID]
		if !ok {
			methodName, ok = standardMethods[methodID]
		}
		if !ok {
			methodName = fmt.Sprintf("Method_%d", methodID)
		}

		isFunction := methodID >= 0xC000 || normID == 8 || normID == 9 || normID == 13 || normID == 6
		var callExpr string
		formattedArgs := formatArgList(args)
		if isFunction {
			if formattedArgs != "" {
				callExpr = fmt.Sprintf("%s.%s(%s)", target, methodName, formattedArgs)
			} else {
				callExpr = fmt.Sprintf("%s.%s()", target, methodName)
			}
		} else {
			if formattedArgs != "" {
				callExpr = fmt.Sprintf("%s.%s %s", target, methodName, formattedArgs)
			} else {
				callExpr = fmt.Sprintf("%s.%s", target, methodName)
			}
		}
		s.push(callExpr)

	case kw == "eom":
		if len(s.stack) > 0 {
			stmt := s.pop()
			s.emitStatement(stmt)
		}

	case kw == "End " || kw == "End":
		if s.ifCond == "" && s.pc+2 <= len(s.bc) {
			nextTok := binary.LittleEndian.Uint16(s.bc[s.pc : s.pc+2])
			nextInfo, _ := s.table.Lookup(nextTok)
			if nextInfo != nil && (nextInfo.Case == 4 || nextInfo.Keyword == "eos") {
				return
			}
		}
		s.emitStatement("End")

	case strings.HasPrefix(kw, "var()="):
		numDims := 1
		var varOff uint16
		if len(params) >= 2 {
			numDims = int(params[0])
			varOff = params[1]
		} else if len(params) == 1 {
			varOff = params[0]
		}
		if numDims < 1 {
			numDims = 1
		}
		indices := make([]string, numDims)
		for d := numDims - 1; d >= 0; d-- {
			idx := s.pop()
			indices[d] = idx
		}
		val := s.pop()
		varName := strings.TrimRight(s.getVarName(varOff), "$")
		stmt := fmt.Sprintf("%s(%s) = %s", varName, strings.Join(indices, ", "), val)
		s.emitStatement(stmt)

	case kw == "As":
		if len(params) > 0 {
			if dt, ok := DataTypes[params[0]]; ok {
				s.pendingAsType = dt
			}
		}

	case strings.HasPrefix(kw, "var()"):
		numBounds := 1
		var varOff uint16
		if len(params) >= 2 {
			numBounds = int(params[0])
			varOff = params[1]
		} else if len(params) == 1 {
			varOff = params[0]
		}

		isReDim := false
		if s.pc+2 <= len(s.bc) {
			nextTok := binary.LittleEndian.Uint16(s.bc[s.pc : s.pc+2])
			nextInfo, _ := s.table.Lookup(nextTok)
			if nextInfo != nil && (nextInfo.Keyword == "ReDim" || nextInfo.Keyword == "ReDim Preserve") {
				isReDim = true
			}
		}

		if isReDim {
			numDims := numBounds / 2
			if numDims < 1 {
				numDims = 1
			}
			dims := make([]string, numDims)
			for d := numDims - 1; d >= 0; d-- {
				upper := s.pop()
				lower := ""
				if len(s.stack) > 0 {
					lower = s.pop()
				}
				if lower != "" {
					dims[d] = fmt.Sprintf("%s To %s", lower, upper)
				} else {
					dims[d] = upper
				}
			}
			varName := strings.TrimRight(s.getVarName(varOff), "$")
			s.arrayVars[varOff] = true
			if s.pendingAsType != "" {
				s.recordVarTypeByOffset(varOff, s.pendingAsType)
			}
			s.push(fmt.Sprintf("%s(%s)", varName, strings.Join(dims, ", ")))
		} else {
			numDims := numBounds
			if numDims < 0 {
				numDims = 0
			}
			indices := make([]string, numDims)
			for d := numDims - 1; d >= 0; d-- {
				idx := s.pop()
				indices[d] = idx
			}
			varName := strings.TrimRight(s.getVarName(varOff), "$")
			s.push(fmt.Sprintf("%s(%s)", varName, strings.Join(indices, ", ")))
		}

	case kw == "ReDim" || kw == "ReDim Preserve":
		target := s.pop()
		stmt := fmt.Sprintf("%s %s", kw, target)
		if s.pendingAsType != "" {
			stmt = fmt.Sprintf("%s %s As %s", kw, target, s.pendingAsType)
			s.pendingAsType = ""
		}
		s.emitStatement(stmt)

	case strings.HasPrefix(kw, "var="):
		var off uint16
		if len(params) > 0 {
			off = params[0]
		}
		val := s.pop()
		if val == "" {
			val = "0"
		}
		if strings.HasPrefix(val, "Len(") || strings.HasPrefix(val, "Asc(") || strings.HasPrefix(val, "MsgBox") {
			s.recordVarTypeByOffset(off, "Integer")
		} else if strings.HasPrefix(val, `"`) || (strings.HasSuffix(val, "$") && !strings.Contains(val, " ")) {
			s.recordVarTypeByOffset(off, "String")
		} else if strings.HasPrefix(val, "Mid$(") || strings.HasPrefix(val, "Left$(") || strings.HasPrefix(val, "Right$(") || strings.HasPrefix(val, "Chr$(") || strings.HasPrefix(val, "LCase$(") || strings.HasPrefix(val, "UCase$(") || strings.HasPrefix(val, "Str$(") || strings.HasPrefix(val, "String$(") || strings.HasPrefix(val, "Space$(") || strings.HasPrefix(val, "Format$(") || strings.HasPrefix(val, "Trim$(") || strings.HasPrefix(val, "Hex$(") || strings.HasPrefix(val, "Oct$(") {
			s.recordVarTypeByOffset(off, "String")
		} else if _, err := strconv.Atoi(val); err == nil {
			s.recordVarTypeByOffset(off, "Integer")
		}

		rawName := s.getVarName(off)
		cleanTarget := strings.TrimRight(rawName, "$%&!#")
		if cleanTarget == s.proc.Name {
			fnType := (s.proc.SubOrFunc >> 8) & 7
			if dt, ok := DataTypes[fnType]; ok && dt != "" && dt != "Variant" {
				cleanVal := strings.TrimRight(val, "$%&!#")
				for o, name := range s.vars {
					if name == cleanVal {
						s.varTypes[o] = dt
						break
					}
				}
			}
		}
		varName := s.formatVarRef(off)
		s.emitStatement(fmt.Sprintf("%s = %s", varName, val))

	case strings.HasPrefix(kw, "var"):
		varName := "var"
		if len(params) > 0 {
			varName = s.formatVarRef(params[0])
		}
		s.push(varName)

	case kw == "pop.":
		// Object / property pop token in method dispatch (no-op on expression stack)

	case kw == "exe":
		// VM execution / frame setup / method dispatch glue (no-op on expression stack)

	case kw == "call":
		argCount := 0
		var targetProc *Procedure
		if len(params) >= 2 {
			argCount = int(params[0])
			procPtr := params[1]
			if s.proj != nil {
				targetProc = s.proj.ProcByPtr[procPtr]
			}
		} else if len(params) == 1 {
			argCount = int(params[0])
		}

		var args []string
		for i := 0; i < argCount && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}

		targetName := ""
		if targetProc != nil {
			targetName = targetProc.DeclaredName()
		} else if len(s.stack) > 0 {
			targetName = s.pop()
		}

		if targetName != "" {
			line := targetName
			formattedArgs := formatArgList(args)
			if formattedArgs != "" {
				line = fmt.Sprintf("%s %s", targetName, formattedArgs)
			}
			s.emitStatement(line)
		}

	case kw == "c%":
		val := int(int16(altToken&0xFC00) >> 10)
		if len(params) > 0 {
			val = int(int16(params[0]))
		}
		s.push(fmt.Sprintf("%d", val))

	case kw == "c&":
		var val int32
		if len(params) >= 2 {
			val = int32(uint32(params[0]) | (uint32(params[1]) << 16))
		} else {
			val = int32(int16(altToken&0xFC00) >> 10)
		}
		s.push(fmt.Sprintf("%d&", val))

	case kw == "c!":
		var bits uint32
		if len(params) >= 2 {
			bits = uint32(params[0]) | (uint32(params[1]) << 16)
		}
		fVal := math.Float32frombits(bits)
		s.push(fmt.Sprintf("%g", fVal))

	case kw == "c#":
		var bits uint64
		if len(params) >= 4 {
			bits = uint64(params[0]) | (uint64(params[1]) << 16) |
				(uint64(params[2]) << 32) | (uint64(params[3]) << 48)
		}
		dVal := math.Float64frombits(bits)
		s.push(fmt.Sprintf("%g", dVal))

	case kw == "&Hc&":
		var val uint32
		if len(params) >= 2 {
			val = uint32(params[0]) | (uint32(params[1]) << 16)
		} else if len(params) == 1 {
			val = uint32(params[0])
		}
		s.push(fmt.Sprintf("&H%X&", val))

	case kw == "&Hc%":
		var val uint16
		if len(params) > 0 {
			val = params[0]
		}
		s.push(fmt.Sprintf("&H%X", val))

	case kw == "+" || kw == "*" || kw == "/" || kw == "\\" || kw == "^" || kw == "Mod" || kw == "&" || kw == "Like" || kw == "Is":
		r := s.pop()
		l := s.pop()
		if l == "" {
			l = "0"
		}
		if r == "" {
			r = "0"
		}
		s.push(fmt.Sprintf("%s %s %s", l, kw, r))

	case kw == "-":
		// Unary minus vs Binary subtraction
		// AltToken 0x00F9 is specifically unary negation.
		if (altToken&0x01FF) == 0x00F9 || len(s.stack) == 1 || s.prevOpWas {
			val := s.pop()
			if val == "" {
				val = "0"
			}
			s.push(fmt.Sprintf("-%s", val))
		} else if len(s.stack) >= 2 {
			r := s.pop()
			l := s.pop()
			s.push(fmt.Sprintf("%s - %s", l, r))
		}

	case kw == "=" || kw == "<>" || kw == "<" || kw == "<=" || kw == ">" || kw == ">=":
		r := s.pop()
		l := s.pop()
		for _, op := range []string{l, r} {
			cleanOp := strings.TrimRight(op, "$%&!#")
			if num, err := strconv.Atoi(cleanOp); err == nil {
				if (num >= 32765 && (kw == ">" || kw == ">=")) || num > 32767 || num < -32768 {
					other := l
					if op == l {
						other = r
					}
					s.recordVarType(other, "Long")
				}
			}
		}
		s.push(fmt.Sprintf("%s %s %s", l, kw, r))

	case kw == "And" || kw == "Or" || kw == "Xor" || kw == "Eqv" || kw == "Imp":
		r := s.pop()
		l := s.pop()
		s.push(fmt.Sprintf("%s %s %s", l, kw, r))

	case kw == "Not":
		arg := s.pop()
		s.push(fmt.Sprintf("Not(%s)", arg))

	case kw == "Len":
		arg := s.pop()
		s.recordVarType(arg, "String")
		s.push(fmt.Sprintf("Len(%s)", arg))

	case kw == "Asc":
		arg := s.pop()
		s.recordVarType(arg, "String")
		s.push(fmt.Sprintf("Asc(%s)", arg))

	case info.TokenID == 238 || (info.Case == 2 && kw == "" && info.Flags == 8 && info.BitForward2 == 1):
		arg := s.pop()
		cleanArg := strings.TrimSpace(arg)
		if strings.HasPrefix(cleanArg, "(") && strings.HasSuffix(cleanArg, ")") {
			s.push(cleanArg)
		} else if cleanArg != "" {
			s.push(fmt.Sprintf("(%s)", cleanArg))
		}

	case kw == "Chr$" || kw == "Space$" || kw == "Hex$" || kw == "Oct$" ||
		kw == "Trim$" || kw == "LTrim$" || kw == "RTrim$" ||
		kw == "Rnd" || kw == "Int" || kw == "Fix" ||
		kw == "Abs" || kw == "Sgn" || kw == "Sqr" || kw == "Sin" || kw == "Cos" ||
		kw == "Tan" || kw == "LCase$" || kw == "UCase$" || kw == "Str$" || kw == "Val" ||
		kw == "DateValue" || kw == "TimeValue" || kw == "Day" || kw == "Month" || kw == "Year" ||
		kw == "Weekday" || kw == "Hour" || kw == "Minute" || kw == "Second" ||
		kw == "IsEmpty" || kw == "IsNull" || kw == "IsNumeric" || kw == "VarType" || kw == "Error$":
		if info.BitForward2 == 0 {
			s.push(kw)
		} else {
			arg := s.pop()
			s.push(fmt.Sprintf("%s(%s)", kw, arg))
		}

	case kw == "Mid$":
		// The expression forms have two or three arguments: Mid$(text, start)
		// and Mid$(text, start, length). The assignment forms are separate
		// opcodes (Flags == 1) and must not be decoded as a three-argument call.
		argCount := int(info.BitForward2)
		if info.Flags != 8 {
			// Assignment forms encode the destination expression and RHS in the
			// same argument vector; the RHS is pushed last.
			args := make([]string, 0, argCount)
			for i := 0; i < argCount && len(s.stack) > 0; i++ {
				args = append([]string{s.pop()}, args...)
			}
			if len(args) >= 3 {
				s.emitStatement(fmt.Sprintf("Mid$(%s) = %s", strings.Join(args[:len(args)-1], ", "), args[len(args)-1]))
			} else if kw != "" {
				s.push(kw)
			}
			break
		}
		if argCount < 2 || argCount > 3 {
			if kw != "" {
				s.push(kw)
			}
			break
		}
		args := make([]string, 0, argCount)
		for i := 0; i < argCount && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		if len(args) > 0 {
			s.recordVarType(args[0], "String")
		}
		s.push(fmt.Sprintf("Mid$(%s)", strings.Join(args, ", ")))

	case kw == "Left$" || kw == "Right$" || kw == "InStr":
		argCount := int(info.BitForward2)
		if argCount < 2 {
			argCount = 2
		}
		args := make([]string, 0, argCount)
		for i := 0; i < argCount && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		if kw == "InStr" {
			textIndex := 0
			if len(args) >= 3 {
				textIndex = 1
			}
			if textIndex < len(args) {
				s.recordVarType(args[textIndex], "String")
			}
		} else if len(args) > 0 {
			s.recordVarType(args[0], "String")
		}
		s.push(fmt.Sprintf("%s(%s)", kw, strings.Join(args, ", ")))

	case kw == "String$":
		charArg := s.pop()
		cntArg := s.pop()
		s.push(fmt.Sprintf("String$(%s, %s)", cntArg, charArg))

	case kw == "True" || kw == "False" || kw == "Null":
		s.push(kw)

	case strings.HasPrefix(kw, "<default"):
		s.push(kw)

	case kw == "If ":
		cleanCond := strings.TrimSpace(s.pop())

		target := ""
		if len(params) > 0 {
			lbl, ok := s.labels[int(params[0])]
			if ok {
				target = lbl
			}
		}

		if target != "" {
			s.emitLine(fmt.Sprintf("If %s Then GoTo %s", cleanCond, target))
		} else {
			s.ifCond = cleanCond
		}

	case kw == "For ":
		if info.TokenID == 62 {
			toVal := s.pop()
			startVal := s.pop()
			loopVar := s.pop()
			s.emitLine(fmt.Sprintf("For %s = %s To %s", loopVar, startVal, toVal))
		} else {
			step := s.pop()
			toVal := s.pop()
			startVal := s.pop()
			loopVar := s.pop()
			if step == "1" || step == "" {
				s.emitLine(fmt.Sprintf("For %s = %s To %s Step 1", loopVar, startVal, toVal))
			} else {
				s.emitLine(fmt.Sprintf("For %s = %s To %s Step %s", loopVar, startVal, toVal, step))
			}
		}
		s.indent++

	case kw == "Next":
		if s.indent > 1 {
			s.indent--
		}
		varName := s.pop()
		if varName != "" {
			s.emitLine(fmt.Sprintf("Next %s", varName))
		} else {
			s.emitLine("Next")
		}

	case kw == "Exit Sub" || kw == "Exit Function" || kw == "Exit For" || kw == "Exit Do" || strings.TrimSpace(kw) == "Exit":
		exitCmd := kw
		if strings.TrimSpace(kw) == "Exit" {
			if s.proc.IsFunction() {
				exitCmd = "Exit Function"
			} else {
				exitCmd = "Exit Sub"
			}
		}
		s.emitStatement(exitCmd)

	case kw == "GoTo " || kw == "GoTo":
		target := ""
		if len(params) > 0 {
			target = fmt.Sprintf("L%04X", params[0])
		}
		s.emitStatement(fmt.Sprintf("GoTo %s", target))

	case kw == "GoSub " || kw == "GoSub":
		target := ""
		if len(params) > 0 {
			target = fmt.Sprintf("L%04X", params[0])
		}
		s.emitLine(fmt.Sprintf("GoSub %s", target))

	case kw == "Return":
		s.emitLine("Return")

	case strings.TrimSpace(kw) == "Do":
		s.emitLine("Do")
		s.indent++

	case strings.HasPrefix(kw, "Do While"):
		cond := strings.TrimSpace(s.pop())
		s.emitLine(fmt.Sprintf("Do While %s", cond))
		s.indent++

	case strings.HasPrefix(kw, "Do Until"):
		cond := strings.TrimSpace(s.pop())
		s.emitLine(fmt.Sprintf("Do Until %s", cond))
		s.indent++

	case strings.HasPrefix(kw, "While"):
		cond := strings.TrimSpace(s.pop())
		s.emitLine(fmt.Sprintf("While %s", cond))
		s.indent++

	case strings.TrimSpace(kw) == "Loop":
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine("Loop")

	case strings.HasPrefix(kw, "Loop While"):
		if s.indent > 1 {
			s.indent--
		}
		cond := strings.TrimSpace(s.pop())
		s.emitLine(fmt.Sprintf("Loop While %s", cond))

	case strings.HasPrefix(kw, "Loop Until"):
		if s.indent > 1 {
			s.indent--
		}
		cond := strings.TrimSpace(s.pop())
		s.emitLine(fmt.Sprintf("Loop Until %s", cond))

	case strings.TrimSpace(kw) == "Wend":
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine("Wend")

	case strings.HasPrefix(kw, "Select Case"):
		expr := s.pop()
		s.emitLine(fmt.Sprintf("Select Case %s", expr))
		s.indent++
		s.caseIndentStack = append(s.caseIndentStack, false)

	case strings.TrimSpace(kw) == "Case":
		val := s.pop()
		if len(s.caseIndentStack) > 0 && s.caseIndentStack[len(s.caseIndentStack)-1] {
			s.indent--
		}
		s.emitLine(fmt.Sprintf("Case %s", val))
		s.indent++
		if len(s.caseIndentStack) > 0 {
			s.caseIndentStack[len(s.caseIndentStack)-1] = true
		}

	case strings.TrimSpace(kw) == "Case Else":
		if len(s.caseIndentStack) > 0 && s.caseIndentStack[len(s.caseIndentStack)-1] {
			s.indent--
		}
		s.emitLine("Case Else")
		s.indent++
		if len(s.caseIndentStack) > 0 {
			s.caseIndentStack[len(s.caseIndentStack)-1] = true
		}

	case strings.TrimSpace(kw) == "End Select":
		if len(s.caseIndentStack) > 0 {
			wasIndented := s.caseIndentStack[len(s.caseIndentStack)-1]
			s.caseIndentStack = s.caseIndentStack[:len(s.caseIndentStack)-1]
			if wasIndented {
				s.indent--
			}
		}
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine("End Select")

	case kw == "Unload " || kw == "Unload":
		target := s.pop()
		if target == "" && s.proc != nil && s.proc.ModuleIndex > 1 && s.proj != nil && s.proc.ModuleIndex <= len(s.proj.Modules) {
			target = s.proj.Modules[s.proc.ModuleIndex-1].Name
		}
		if target != "" {
			s.emitStatement(fmt.Sprintf("Unload %s", target))
		} else {
			s.emitStatement("Unload")
		}

	case kw == "Load " || kw == "Load":
		target := s.pop()
		s.emitStatement(fmt.Sprintf("Load %s", target))

	case kw == "Randomize":
		if info.BitForward2 > 0 && len(s.stack) > 0 {
			seed := s.pop()
			s.emitStatement(fmt.Sprintf("Randomize %s", seed))
		} else {
			s.emitStatement("Randomize")
		}

	case kw == "DoEvents":
		s.emitStatement("DoEvents")

	case kw == "Beep":
		s.emitStatement("Beep")

	case kw == "Timer":
		s.push("Timer")

	case kw == "Print":
		arg := s.pop()
		s.emitStatement(fmt.Sprintf("Print %s", arg))

	case kw == "C<typ>":
		// Coercion token (preserves top of stack)

	case strings.TrimSpace(kw) == "End If":
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine("End If")

	case strings.TrimSpace(kw) == "Else":
		if s.lastSingleLineIf {
			s.singleLineElse = true
			break
		}
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine("Else")
		s.indent++

	case strings.HasPrefix(kw, "ElseIf"):
		cond := strings.TrimSpace(s.pop())
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine(fmt.Sprintf("ElseIf %s Then", cond))
		s.indent++

	case kw == "MsgBox":
		argCount := len(s.stack)
		if argCount > 3 {
			argCount = 3
		}
		var args []string
		for i := 0; i < argCount; i++ {
			args = append([]string{s.pop()}, args...)
		}
		formattedArgs := formatArgList(args)
		if info.Case == 2 {
			if formattedArgs != "" {
				s.push(fmt.Sprintf("MsgBox(%s)", formattedArgs))
			} else {
				s.push("MsgBox()")
			}
		} else {
			line := "MsgBox"
			if formattedArgs != "" {
				line = fmt.Sprintf("MsgBox %s", formattedArgs)
			}
			s.emitStatement(line)
		}

	case info.Flags == 8 && info.BitForward2 > 0 && kw != "":
		argCount := info.BitForward2
		var args []string
		for i := 0; i < argCount && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		s.push(fmt.Sprintf("%s(%s)", kw, formatArgList(args)))

	default:
		if kw != "" && kw != "?" {
			s.push(kw)
		}
		modName := ""
		if s.mod != nil {
			modName = s.mod.Name
		}
		warn := UnhandledOpcodeWarning{
			ModuleName: modName,
			ProcName:   s.proc.Name,
			PC:         pc,
			TokenID:    info.TokenID,
			AltToken:   altToken,
			Keyword:    kw,
			Case:       info.Case,
			Params:     params,
		}
		s.warnings = append(s.warnings, warn)
		log.Printf("[WARN] %s.%s @ 0x%04X: unhandled opcode token=%d (0x%04X) alt=0x%04X kw=%q case=%d params=%v",
			modName, s.proc.Name, pc, info.TokenID, info.TokenID, altToken, kw, info.Case, params)
	}
}

func (s *procDisasmState) renderProcedure() string {
	var sb strings.Builder

	isFunc := s.proc.IsFunction()
	procType := "Sub"
	retType := ""

	if isFunc {
		procType = "Function"
		fnType := (s.proc.SubOrFunc >> 8) & 7
		if dt, ok := DataTypes[fnType]; ok {
			retType = " As " + dt
		} else {
			retType = " As Variant"
		}
	}

	var paramList []string
	var localVars []string

	var sortedOffsets []uint16
	for off := range s.vars {
		sortedOffsets = append(sortedOffsets, off)
	}
	sort.Slice(sortedOffsets, func(i, j int) bool { return sortedOffsets[i] < sortedOffsets[j] })

	isEvent := strings.Contains(s.proc.Name, "_")
	if isEvent {
		evSuffix := s.proc.Name[strings.LastIndex(s.proc.Name, "_")+1:]
		if defs, ok := eventParamDefs[evSuffix]; ok {
			for _, ep := range defs {
				paramList = append(paramList, fmt.Sprintf("%s As %s", ep.Name, ep.Type))
			}
		}
	}

	paramIdx := 0
	for _, off := range sortedOffsets {
		vName := s.vars[off]
		if s.hasRet && vName == s.proc.Name {
			continue
		}
		tName := s.varTypes[off]
		if strings.HasPrefix(vName, "p") {
			if !isEvent {
				if tName == "" {
					tName = "Variant"
				}
				arrSuffix := ""
				if s.arrayVars[off] {
					arrSuffix = "()"
				}
				byValPrefix := ""
				if s.proc != nil && s.proc.ParamByVal != nil && s.proc.ParamByVal[paramIdx] {
					byValPrefix = "ByVal "
				}
				paramList = append(paramList, fmt.Sprintf("%s%s%s As %s", byValPrefix, vName, arrSuffix, tName))
				paramIdx++
			}
		} else if strings.HasPrefix(vName, "l") {
			declKw := "Dim"
			if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
				modBytes := s.proj.Modules[s.proc.ModuleIndex-1].ModBytes
				if len(modBytes) > 0 && int(off)+2 <= len(modBytes) {
					bp := int16(binary.LittleEndian.Uint16(modBytes[off : off+2]))
					if bp == 0 {
						declKw = "Static"
					}
				}
			}
			typeStr := ""
			if tName != "" && tName != "Variant" {
				typeStr = " As " + tName
			}
			arrSuffix := ""
			if s.arrayVars[off] {
				arrSuffix = "()"
			}
			localVars = append(localVars, fmt.Sprintf("    %s %s%s%s", declKw, vName, arrSuffix, typeStr))
		}
	}

	paramsStr := strings.Join(paramList, ", ")
	sb.WriteString(fmt.Sprintf("%s %s (%s)%s\n", procType, s.proc.Name, paramsStr, retType))

	for _, lv := range localVars {
		sb.WriteString(lv)
		sb.WriteString("\n")
	}

	for _, line := range s.lines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("End %s", procType))

	return sb.String()
}
