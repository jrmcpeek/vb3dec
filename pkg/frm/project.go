package frm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"vb3dec/pkg/ne"
)

// FormRef describes an embedded form discovered in the project structure.
type FormRef struct {
	Index      int    // 1-based index (1..N)
	ResourceID uint16 // Resource ID in RT_RCDATA (e.g. 4, 6, 8...)
	FileName   string // Original filename if preserved, or "FRM1.FRM"
	FormName   string // Original form name if preserved, or "frm1"
}

// ProjectInfo represents the project metadata and form directory from RT_RCDATA ID 1.
type ProjectInfo struct {
	Title        string
	StartupIndex int
	Forms        []FormRef
	CustomVBXs   []string
	VBGuard      bool // True if form names were stripped by VBGuard
}

// ParseVBProject parses the project directory from RT_RCDATA resource 1 in an NE executable.
func ParseVBProject(f *ne.File) (*ProjectInfo, error) {
	entry, err := f.FindResource(ne.ResTypeRCData, 1)
	if err != nil {
		return nil, fmt.Errorf("locating project structure (RT_RCDATA ID 1): %w", err)
	}

	data, err := f.ReadResourceData(entry)
	if err != nil {
		return nil, fmt.Errorf("reading project structure: %w", err)
	}

	if len(data) < 19 {
		return nil, fmt.Errorf("project structure too small (%d bytes)", len(data))
	}

	// Header: T1267 (19 bytes)
	// M1275 (2), M127E (2), StartupFormIdx (2), VBTitle (9), ProjTitleLen (2), HelpFileLen (2)
	startupFormIdx := int(int16(binary.LittleEndian.Uint16(data[4:6])))
	vbTitleRaw := bytes.TrimRight(data[6:15], "\x00")
	vbTitle := string(vbTitleRaw)

	projTitleLen := int(binary.LittleEndian.Uint16(data[15:17]))
	helpFileLen := int(binary.LittleEndian.Uint16(data[17:19]))

	pos := 19
	var projTitle string
	if projTitleLen > 0 && pos+projTitleLen <= len(data) {
		projTitle = strings.TrimRight(string(data[pos:pos+projTitleLen]), "\x00")
		pos += projTitleLen
	}
	if helpFileLen > 0 && pos+helpFileLen <= len(data) {
		pos += helpFileLen
	}

	if projTitle != "" {
		vbTitle = projTitle
	}

	var forms []FormRef
	var customVBXs []string
	vbGuardDetected := false
	formCounter := 1

	for pos+11 <= len(data) {
		recType := data[pos]
		if recType == 0 {
			break
		}

		nameLen := int(data[pos+1])
		resIDAssoc := binary.LittleEndian.Uint16(data[pos+2 : pos+4])
		// m2Curr := binary.LittleEndian.Uint16(data[pos+4 : pos+6])
		// m3Next := binary.LittleEndian.Uint16(data[pos+6 : pos+8])
		// m4Sub := data[pos+8]
		// m5Size := binary.LittleEndian.Uint16(data[pos+9 : pos+11])
		pos += 11

		var nameStr string
		if nameLen == 0xFF {
			if pos < len(data) {
				extraLen := int(data[pos])
				pos++
				if pos+extraLen <= len(data) {
					nameStr = strings.TrimRight(string(data[pos:pos+extraLen]), "\x00")
					pos += extraLen
				}
			}
		} else if nameLen > 0 && pos+nameLen <= len(data) {
			nameStr = strings.TrimRight(string(data[pos:pos+nameLen]), "\x00")
			pos += nameLen
		}

		// Read sub-items for Form and ControlType records
		if recType == 0x46 || recType == 0x58 {
			if pos+2 <= len(data) {
				subCount := int(int16(binary.LittleEndian.Uint16(data[pos : pos+2])))
				pos += 2
				if subCount > 0 && pos+subCount*4 <= len(data) {
					pos += subCount * 4
				}
			}
		}

		switch recType {
		case 0x43: // 'C' = Custom control (VBX)
			if nameStr != "" {
				customVBXs = append(customVBXs, nameStr)
			}
		case 0x46: // 'F' = Form
			var fileName string
			var formName string
			actualResID := resIDAssoc & 0x7FFF

			if nameStr == "" {
				vbGuardDetected = true
				fileName = fmt.Sprintf("FRM%d.FRM", formCounter)
				formName = fmt.Sprintf("frm%d", formCounter)
			} else {
				fileName = nameStr
				base := strings.TrimSuffix(nameStr, ".FRM")
				base = strings.TrimSuffix(base, ".frm")
				formName = strings.ToLower(base)
			}

			forms = append(forms, FormRef{
				Index:      formCounter,
				ResourceID: actualResID,
				FileName:   fileName,
				FormName:   formName,
			})
			formCounter++
		case 0x58: // 'X' = Control type
			// Handled above via subCount
		default:
			// Unknown record type, stop scanning
			break
		}
	}

	// Also check if odd RCData resources have offset 0 (confirming VBGuard)
	if !vbGuardDetected && len(forms) > 0 {
		for _, form := range forms {
			oddID := form.ResourceID + 1
			if res, err := f.FindResource(ne.ResTypeRCData, oddID); err == nil {
				if res.Offset == 0 {
					vbGuardDetected = true
					break
				}
			}
		}
	}

	return &ProjectInfo{
		Title:        vbTitle,
		StartupIndex: startupFormIdx,
		Forms:        forms,
		CustomVBXs:   customVBXs,
		VBGuard:      vbGuardDetected,
	}, nil
}
