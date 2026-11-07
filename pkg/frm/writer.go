package frm

import (
	"fmt"
	"strings"
)

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
