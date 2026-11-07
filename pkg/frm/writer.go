package frm

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"vb3dec/pkg/win1252"
)

// TextEncoding selects the character encoding of the generated text files
// (.FRM, .BAS, .MAK).
type TextEncoding int

const (
	// EncodingWindows1252 is the ANSI code page the VB3 IDE reads, and the default.
	EncodingWindows1252 TextEncoding = iota
	// EncodingUTF8 is for reading and diffing the output with modern tools.
	// VB3 shows its non-ASCII characters as mojibake, so a project written
	// this way is not for loading into VB3.
	EncodingUTF8
)

// ParseTextEncoding parses an encoding name: "windows-1252" (or "cp1252")
// or "utf-8" (or "utf8"), ignoring case.
func ParseTextEncoding(name string) (TextEncoding, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "windows-1252", "cp1252":
		return EncodingWindows1252, nil
	case "utf-8", "utf8":
		return EncodingUTF8, nil
	}
	return 0, fmt.Errorf("unknown text encoding %q (want windows-1252 or utf-8)", name)
}

func (e TextEncoding) String() string {
	if e == EncodingUTF8 {
		return "utf-8"
	}
	return "windows-1252"
}

// EncodeText converts text to the CRLF line endings of VB3 text files and to
// the given encoding. Text that Windows-1252 cannot represent is an error
// that names its line.
func EncodeText(text string, enc TextEncoding) ([]byte, error) {
	text = ToCRLF(text)
	if enc == EncodingUTF8 {
		return []byte(text), nil
	}
	data, err := win1252.Encode(text)
	if err != nil {
		var ue *win1252.UnrepresentableError
		if errors.As(err, &ue) {
			return nil, fmt.Errorf("line %d: %w", strings.Count(text[:ue.Offset], "\n")+1, err)
		}
		return nil, err
	}
	return data, nil
}

// WriteTextFile writes text to path with CRLF line endings in the given encoding.
func WriteTextFile(path, text string, enc TextEncoding) error {
	data, err := EncodeText(text, enc)
	if err != nil {
		return fmt.Errorf("encoding %s as %s: %w", path, enc, err)
	}
	return os.WriteFile(path, data, 0644)
}

// FormatTextFRM converts a decoded Form structure into standard Visual Basic ASCII .FRM format.
func FormatTextFRM(form *Form) string {
	var sb strings.Builder

	// Standard VB header
	sb.WriteString("VERSION 2.00\r\n")

	// Write Form root
	formatNode(&sb, form.Root, 0)

	return sb.String()
}

func formatNode(sb *strings.Builder, node *ControlNode, depth int) {
	indent := strings.Repeat("   ", depth)
	propIndent := strings.Repeat("   ", depth+1)

	// Block header: e.g. "Begin Form frm1 " or "   Begin CommandButton control1 "
	sb.WriteString(fmt.Sprintf("%sBegin %s %s \r\n", indent, node.TypeName, node.Name))

	// Properties
	for _, prop := range node.Properties {
		valStr := prop.Value
		if prop.Comment != "" {
			// Spacing before comment: match VB convention
			if len(prop.Value) == 1 {
				valStr = fmt.Sprintf("%-4s'%s", prop.Value, prop.Comment)
			} else {
				valStr = fmt.Sprintf("%s  '%s", prop.Value, prop.Comment)
			}
		}

		sb.WriteString(fmt.Sprintf("%s%-16s=   %s\r\n", propIndent, prop.Name, valStr))
	}

	// Child controls (e.g. in container controls or Form)
	for _, child := range node.Children {
		formatNode(sb, child, depth+1)
	}

	// Block close: "End\r\n" or "   End\r\n"
	sb.WriteString(fmt.Sprintf("%sEnd\r\n", indent))
}

// ToCRLF converts text to the CRLF line endings of VB3 text files (.FRM,
// .BAS, .MAK), leaving existing CRLF line endings unchanged.
func ToCRLF(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
}

// HasCode reports whether a text .FRM file has code after its form block,
// which closes with the first unindented "End" line. Property values cannot
// span lines, so text inside the form block never ends it.
func HasCode(frmText string) bool {
	lines := strings.Split(frmText, "\n")
	for i, line := range lines {
		if strings.TrimRight(line, "\r") != "End" {
			continue
		}
		for _, rest := range lines[i+1:] {
			if strings.TrimSpace(rest) != "" {
				return true
			}
		}
		return false
	}
	return false
}
