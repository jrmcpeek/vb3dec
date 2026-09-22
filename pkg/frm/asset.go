package frm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// ExtractedAsset represents a standalone file extracted from an .FRX binary stream.
type ExtractedAsset struct {
	FormFileName string // e.g. "FRM1.FRM"
	ControlName  string // e.g. "control34" or "frm1"
	PropertyName string // e.g. "Picture", "Icon"
	Offset       uint32 // Byte offset in the FRX file
	Length       uint32 // Length of payload in bytes
	Format       string // File extension: "bmp", "ico", "gif", "cur", "wmf", "emf", "jpg", "png", "wav", "mid", "bin"
	MimeType     string // MIME descriptor, e.g. "image/bmp"
	FileName     string // Generated standalone filename, e.g. "FRM1_control34_Picture_15B44.bmp"
	Data         []byte // Raw file payload (excluding 4-byte FRX length prefix)
}

// DetectAssetFormat examines the raw payload bytes and identifies the media format.
func DetectAssetFormat(payload []byte) (ext string, mime string) {
	if len(payload) < 4 {
		return "bin", "application/octet-stream"
	}

	// 1. Windows Bitmap (.bmp): Starts with "BM" (0x42, 0x4D)
	if payload[0] == 0x42 && payload[1] == 0x4D {
		return "bmp", "image/bmp"
	}

	// 2. GIF Image (.gif): Starts with "GIF87a" or "GIF89a"
	if len(payload) >= 6 && (string(payload[:6]) == "GIF87a" || string(payload[:6]) == "GIF89a") {
		return "gif", "image/gif"
	}

	// 3. PNG Image (.png): 89 50 4E 47 0D 0A 1A 0A
	if len(payload) >= 8 && bytes.Equal(payload[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "png", "image/png"
	}

	// 4. JPEG Image (.jpg): FF D8 FF
	if len(payload) >= 3 && payload[0] == 0xFF && payload[1] == 0xD8 && payload[2] == 0xFF {
		return "jpg", "image/jpeg"
	}

	// 5. Windows Icon (.ico) or Cursor (.cur):
	// Reserved (2 bytes = 0x0000), Type (2 bytes: 1 = ICO, 2 = CUR)
	if payload[0] == 0x00 && payload[1] == 0x00 {
		resType := binary.LittleEndian.Uint16(payload[2:4])
		if resType == 1 {
			return "ico", "image/x-icon"
		}
		if resType == 2 {
			return "cur", "image/x-icon"
		}
	}

	// 6. Windows Metafile (.wmf)
	// APM (Aldus Placeable Metafile): D7 CD C6 9A
	if payload[0] == 0xD7 && payload[1] == 0xCD && payload[2] == 0xC6 && payload[3] == 0x9A {
		return "wmf", "image/x-wmf"
	}
	// Standard Windows Metafile: 01 00 09 00
	if payload[0] == 0x01 && payload[1] == 0x00 && payload[2] == 0x09 && payload[3] == 0x00 {
		return "wmf", "image/x-wmf"
	}

	// 7. Enhanced Metafile (.emf): 01 00 00 00, with " EMF" at offset 40
	if len(payload) >= 44 && binary.LittleEndian.Uint32(payload[:4]) == 1 {
		if string(payload[40:44]) == " EMF" || string(payload[40:44]) == "EMF " {
			return "emf", "image/x-emf"
		}
	}

	// 8. RIFF Audio (.wav): Starts with "RIFF" and contains "WAVE" at offset 8
	if len(payload) >= 12 && string(payload[:4]) == "RIFF" && string(payload[8:12]) == "WAVE" {
		return "wav", "audio/wav"
	}

	// 9. MIDI Music (.mid): Starts with "MThd"
	if string(payload[:4]) == "MThd" {
		return "mid", "audio/midi"
	}

	return "bin", "application/octet-stream"
}

type assetRef struct {
	ControlName  string
	PropertyName string
}

// ExtractFRXAssets sequentially walks an .FRX byte stream, extracts each standalone asset payload,
// correlates it with the form's property references, and determines the appropriate file format and name.
func ExtractFRXAssets(rawFRX []byte, formFileName string, form *Form) ([]*ExtractedAsset, error) {
	if len(rawFRX) == 0 {
		return nil, nil
	}

	baseName := strings.TrimSuffix(filepath.Base(strings.ReplaceAll(formFileName, `\`, `/`)), filepath.Ext(formFileName))
	baseName = sanitizeAssetComponent(baseName)

	// Build mapping from hex offset to control/property reference
	refMap := make(map[uint32][]assetRef)

	addRef := func(ctrlName, propName, val string) {
		colonIdx := strings.LastIndex(val, ":")
		if colonIdx == -1 {
			return
		}
		hexStr := val[colonIdx+1:]
		offset64, err := strconv.ParseUint(hexStr, 16, 32)
		if err != nil {
			return
		}
		offset := uint32(offset64)
		refMap[offset] = append(refMap[offset], assetRef{
			ControlName:  ctrlName,
			PropertyName: propName,
		})
	}

	// Collect from Root form properties
	if form != nil && form.Root != nil {
		for _, prop := range form.Root.Properties {
			if strings.Contains(prop.Value, ".FRX:") || strings.Contains(prop.Value, ".frx:") {
				addRef(form.Name, prop.Name, prop.Value)
			}
		}
	}

	// Collect from all controls
	if form != nil {
		for _, ctrl := range form.Controls {
			for _, prop := range ctrl.Properties {
				if strings.Contains(prop.Value, ".FRX:") || strings.Contains(prop.Value, ".frx:") {
					addRef(ctrl.Name, prop.Name, prop.Value)
				}
			}
		}
	}

	var assets []*ExtractedAsset
	pos := 0

	for pos+4 <= len(rawFRX) {
		offset := uint32(pos)
		length := binary.LittleEndian.Uint32(rawFRX[pos : pos+4])
		pos += 4

		if pos+int(length) > len(rawFRX) {
			// Length exceeds remaining stream; treat remainder as payload or stop
			rem := len(rawFRX) - pos
			if rem > 0 {
				payload := rawFRX[pos:]
				ext, mime := DetectAssetFormat(payload)
				ctrlName := "unknown"
				propName := "data"
				if refs, ok := refMap[offset]; ok && len(refs) > 0 {
					ctrlName = refs[0].ControlName
					propName = refs[0].PropertyName
				}
				fileName := fmt.Sprintf("%s_%s_%s_%04X.%s", baseName, sanitizeAssetComponent(ctrlName), sanitizeAssetComponent(propName), offset, ext)
				assets = append(assets, &ExtractedAsset{
					FormFileName: formFileName,
					ControlName:  ctrlName,
					PropertyName: propName,
					Offset:       offset,
					Length:       uint32(rem),
					Format:       ext,
					MimeType:     mime,
					FileName:     fileName,
					Data:         payload,
				})
			}
			break
		}

		payload := rawFRX[pos : pos+int(length)]
		pos += int(length)

		ext, mime := DetectAssetFormat(payload)

		ctrlName := "asset"
		propName := "item"
		if refs, ok := refMap[offset]; ok && len(refs) > 0 {
			ctrlName = refs[0].ControlName
			propName = refs[0].PropertyName
		}

		fileName := fmt.Sprintf("%s_%s_%s_%04X.%s", baseName, sanitizeAssetComponent(ctrlName), sanitizeAssetComponent(propName), offset, ext)

		assets = append(assets, &ExtractedAsset{
			FormFileName: formFileName,
			ControlName:  ctrlName,
			PropertyName: propName,
			Offset:       offset,
			Length:       length,
			Format:       ext,
			MimeType:     mime,
			FileName:     fileName,
			Data:         payload,
		})
	}

	return assets, nil
}

func sanitizeAssetComponent(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>|`, r) {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	result := strings.TrimSpace(b.String())
	if result == "" || result == "." || result == ".." {
		return "asset"
	}
	return result
}
