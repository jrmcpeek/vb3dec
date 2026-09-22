package frm

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vb3dec/pkg/ne"
)

// ExtractOptions configures form extraction behavior.
type ExtractOptions struct {
	// NamesDir optionally specifies a directory containing frmX.FRM.txt control name mapping files.
	NamesDir string

	// NameMaps optionally provides an in-memory map of formName -> (controlID -> controlName).
	NameMaps map[string]map[int]string
}

// ExtractedForm contains all outputs for an extracted form.
type ExtractedForm struct {
	Ref       FormRef
	Form      *Form
	TextFRM   string
	RawFRX    []byte
	RawStream []byte
	Assets    []*ExtractedAsset
}

// ExtractForms parses the project and extracts all forms from a 16-bit NE executable.
func ExtractForms(neFile *ne.File, opts ExtractOptions) ([]*ExtractedForm, *ProjectInfo, error) {
	proj, err := ParseVBProject(neFile)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing VB project directory: %w", err)
	}

	var results []*ExtractedForm

	for _, formRef := range proj.Forms {
		entry, err := neFile.FindResource(ne.ResTypeRCData, formRef.ResourceID)
		if err != nil {
			return nil, proj, fmt.Errorf("locating form resource ID %d: %w", formRef.ResourceID, err)
		}

		rawStream, err := neFile.ReadResourceData(entry)
		if err != nil {
			return nil, proj, fmt.Errorf("reading form resource ID %d: %w", formRef.ResourceID, err)
		}

		// Check for name maps
		var nameMap map[int]string
		if opts.NameMaps != nil {
			nameMap = opts.NameMaps[formRef.FormName]
		}
		if nameMap == nil && opts.NamesDir != "" {
			nameMap = loadNameMapFile(opts.NamesDir, formRef.FormName)
		}

		form, rawFRX, err := DecodeFormStream(rawStream, formRef, nameMap)
		if err != nil {
			return nil, proj, fmt.Errorf("decoding form %s (ID %d): %w", formRef.FormName, formRef.ResourceID, err)
		}

		textFRM := FormatTextFRM(form)

		var assets []*ExtractedAsset
		if len(rawFRX) > 0 {
			assets, _ = ExtractFRXAssets(rawFRX, formRef.FileName, form)
		}

		results = append(results, &ExtractedForm{
			Ref:       formRef,
			Form:      form,
			TextFRM:   textFRM,
			RawFRX:    rawFRX,
			RawStream: rawStream,
			Assets:    assets,
		})
	}

	return results, proj, nil
}

// SaveExtractedForms writes the extracted forms (.FRM, .FRX, and optionally .bin and assets) to outDir.
func SaveExtractedForms(forms []*ExtractedForm, outDir string, saveRaw bool) error {
	return SaveExtractedFormsWithAssets(forms, outDir, saveRaw, true, "")
}

// SaveExtractedFormsWithAssets writes extracted forms and allows configuring standalone asset output.
func SaveExtractedFormsWithAssets(forms []*ExtractedForm, outDir string, saveRaw bool, saveAssets bool, assetsDirOverride string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output directory %s: %w", outDir, err)
	}

	assetsDir := assetsDirOverride
	if assetsDir == "" {
		assetsDir = filepath.Join(outDir, "assets")
	}

	if saveAssets {
		if err := os.MkdirAll(assetsDir, 0755); err != nil {
			return fmt.Errorf("creating assets directory %s: %w", assetsDir, err)
		}
	}

	for _, ef := range forms {
		frmPath, err := safeOutputPath(outDir, ef.Ref.FileName)
		if err != nil {
			return fmt.Errorf("invalid form filename %q: %w", ef.Ref.FileName, err)
		}
		// Save .FRM text
		if err := os.WriteFile(frmPath, []byte(ef.TextFRM), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", frmPath, err)
		}

		// Save .FRX if assets exist
		if len(ef.RawFRX) > 0 {
			baseName := strings.TrimSuffix(ef.Ref.FileName, ".FRM")
			baseName = strings.TrimSuffix(baseName, ".frm")
			frxPath, err := safeOutputPath(outDir, baseName+".FRX")
			if err != nil {
				return fmt.Errorf("invalid FRX filename derived from %q: %w", ef.Ref.FileName, err)
			}
			if err := os.WriteFile(frxPath, ef.RawFRX, 0644); err != nil {
				return fmt.Errorf("writing %s: %w", frxPath, err)
			}
		}

		// Save raw binary stream if requested
		if saveRaw && len(ef.RawStream) > 0 {
			baseName := strings.TrimSuffix(ef.Ref.FileName, ".FRM")
			baseName = strings.TrimSuffix(baseName, ".frm")
			rawPath, err := safeOutputPath(outDir, baseName+".raw.bin")
			if err != nil {
				return fmt.Errorf("invalid raw filename derived from %q: %w", ef.Ref.FileName, err)
			}
			if err := os.WriteFile(rawPath, ef.RawStream, 0644); err != nil {
				return fmt.Errorf("writing %s: %w", rawPath, err)
			}
		}

		// Save standalone assets
		if saveAssets && len(ef.Assets) > 0 {
			for _, asset := range ef.Assets {
				assetPath, err := safeOutputPath(assetsDir, asset.FileName)
				if err != nil {
					return fmt.Errorf("invalid asset filename %q: %w", asset.FileName, err)
				}
				if err := os.WriteFile(assetPath, asset.Data, 0644); err != nil {
					return fmt.Errorf("writing standalone asset %s: %w", assetPath, err)
				}
			}
		}
	}

	return nil
}

// safeOutputPath resolves an archive-provided name beneath root.
func safeOutputPath(root, name string) (string, error) {
	if !filepath.IsLocal(name) {
		return "", fmt.Errorf("path is not local")
	}
	localized, err := filepath.Localize(filepath.ToSlash(name))
	if err != nil {
		return "", err
	}
	return filepath.Join(root, localized), nil
}

// loadNameMapFile attempts to load names from a file like frm1.FRM.txt
func loadNameMapFile(dir, formName string) map[int]string {
	candidates := []string{
		filepath.Join(dir, formName+".FRM.txt"),
		filepath.Join(dir, strings.ToLower(formName)+".frm.txt"),
		filepath.Join(dir, strings.ToUpper(formName)+".FRM.TXT"),
	}

	for _, cand := range candidates {
		f, err := os.Open(cand)
		if err != nil {
			continue
		}
		defer f.Close()

		nameMap := make(map[int]string)
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || line == "0" || strings.HasPrefix(line, "0 ") {
				continue
			}
			nameMap[lineNum] = line
			lineNum++
		}
		if len(nameMap) > 0 {
			return nameMap
		}
	}
	return nil
}
