package frm

import (
	"fmt"
)

// Control types in VB3
const (
	CtrlPictureBox    = "PictureBox"
	CtrlLabel         = "Label"
	CtrlTextBox       = "TextBox"
	CtrlFrame         = "Frame"
	CtrlCommandButton = "CommandButton"
	CtrlCheckBox      = "CheckBox"
	CtrlOptionButton  = "OptionButton"
	CtrlComboBox      = "ComboBox"
	CtrlListBox       = "ListBox"
	CtrlHScrollBar    = "HScrollBar"
	CtrlVScrollBar    = "VScrollBar"
	CtrlTimer         = "Timer"
	CtrlPrinter       = "Printer"
	CtrlForm          = "Form"
	CtrlDriveListBox  = "DriveListBox"
	CtrlDirListBox    = "DirListBox"
	CtrlFileListBox   = "FileListBox"
	CtrlMenu          = "Menu"
	CtrlMDIForm       = "MDIForm"
	CtrlImage         = "Image"
)

// BuiltinControlTypeName maps a 1-byte control type ID to its standard VB3 control type name.
func BuiltinControlTypeName(typeID byte) string {
	switch typeID {
	case 0x00:
		return CtrlPictureBox
	case 0x01:
		return CtrlLabel
	case 0x02:
		return CtrlTextBox
	case 0x03:
		return CtrlFrame
	case 0x04:
		return CtrlCommandButton
	case 0x05:
		return CtrlCheckBox
	case 0x06:
		return CtrlOptionButton
	case 0x07:
		return CtrlComboBox
	case 0x08:
		return CtrlListBox
	case 0x09:
		return CtrlHScrollBar
	case 0x0A:
		return CtrlVScrollBar
	case 0x0B:
		return CtrlTimer
	case 0x0C:
		return CtrlPrinter
	case 0x0D:
		return CtrlForm
	case 0x10:
		return CtrlDriveListBox
	case 0x11:
		return CtrlDirListBox
	case 0x12:
		return CtrlFileListBox
	case 0x13:
		return CtrlMenu
	case 0x14:
		return CtrlMDIForm
	case 0x18:
		return CtrlImage
	default:
		return fmt.Sprintf("UnknownControl_0x%02X", typeID)
	}
}

// Property represents a single visual or behavioral property of a form or control.
type Property struct {
	Name    string
	Value   string
	Comment string
}

// ControlNode represents a control or form in the visual hierarchy.
type ControlNode struct {
	ID         int            // 1-based control ID (0 for form)
	Name       string         // e.g. "frm1", "control1"
	TypeName   string         // e.g. "CommandButton", "Label", "MMControl"
	Properties []Property     // Ordered property assignments
	Children   []*ControlNode // Nested controls (for containers like Frame, PictureBox)
}

// FRXAsset represents an embedded icon or picture extracted from a form stream.
type FRXAsset struct {
	PropName string
	Data     []byte
	Offset   uint32
}

// Form represents a fully decoded Visual Basic form.
type Form struct {
	Number     int            // 1-based form index
	Name       string         // e.g. "frm1"
	FileName   string         // e.g. "FRM1.FRM"
	Root       *ControlNode   // The Form node itself
	Controls   []*ControlNode // Flat list of all child controls in stream order
	FRXAssets  []FRXAsset     // Graphic assets belonging to this form
	RawFRXData []byte         // Complete binary .FRX buffer
}

// FormatEnumComment provides standard VB3 comments for enum properties.
func FormatEnumComment(ctrlType, propName string, val int) string {
	switch propName {
	case "BorderStyle":
		if ctrlType == "Form" {
			switch val {
			case 0:
				return "None"
			case 1:
				return "Fixed Single"
			case 2:
				return "Sizable"
			case 3:
				return "Fixed Double"
			}
		} else {
			switch val {
			case 0:
				return "None"
			case 1:
				return "Fixed Single"
			}
		}
	case "BackStyle":
		switch val {
		case 0:
			return "Transparent"
		case 1:
			return "Opaque"
		}
	case "Alignment":
		switch val {
		case 0:
			return "Left Justify"
		case 1:
			return "Right Justify"
		case 2:
			return "Center"
		}
	case "Style":
		if ctrlType == "ComboBox" {
			switch val {
			case 0:
				return "Dropdown Combo"
			case 1:
				return "Simple Combo"
			case 2:
				return "Dropdown List"
			}
		}
	case "ControlBox", "MaxButton", "MinButton", "Enabled", "Visible", "TabStop", "MultiLine":
		if val == 0 {
			return "False"
		} else if val == -1 || val == 1 {
			return "True"
		}
	case "FontBold", "FontItalic", "FontUnderline", "FontStrikethru":
		if val == 0 {
			return "False"
		} else if val == -1 || val == 1 {
			return "True"
		}
	}
	return ""
}
