package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vb3dec/pkg/frm"
	"vb3dec/pkg/ne"
	"vb3dec/pkg/pcode"
	"vb3dec/pkg/win1252"
)

func main() {
	hadErrors := false
	formsFlag := flag.Bool("forms", true, "Extract and decompile form resources (.FRM / .FRX)")
	codeFlag := flag.Bool("code", true, "Disassemble and decompile P-code procedures (.BAS)")
	outDirFlag := flag.String("out", "./out_decompiled", "Output directory for extracted files")
	namesDirFlag := flag.String("names", "", "Optional directory containing external name mapping files (frmX.FRM.txt)")
	rawFlag := flag.Bool("raw", false, "Also dump raw binary form streams (.raw.bin)")
	assetsFlag := flag.Bool("assets", true, "Extract standalone resource files (.ico, .bmp, .gif, etc.) from .FRX containers")
	assetsDirFlag := flag.String("assets-dir", "", "Custom directory for standalone assets (default: <out>/assets)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Visual Basic 3 Decompiler (vb3dec)\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <path-to-vb3-exe> [output-dir]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	targetExe := filepath.Join("test_input", "FF.EXE")
	if flag.NArg() > 0 {
		targetExe = flag.Arg(0)
	}

	outDir := *outDirFlag
	if flag.NArg() > 1 {
		outDir = flag.Arg(1)
	}

	fmt.Printf("Visual Basic 3 Decompiler (vb3dec)\n")
	fmt.Printf("Target: %s\n", targetExe)
	fmt.Printf("Output: %s\n\n", outDir)

	f, err := ne.Open(targetExe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening NE file: %v\n", err)
		os.Exit(1)
	}

	// Parse project directory
	proj, err := frm.ParseVBProject(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading VB project structure: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Project Info:\n")
	fmt.Printf("  Title:           %s\n", proj.Title)
	if proj.StartupIndex < 0 {
		fmt.Printf("  Startup:         Sub Main\n")
	} else {
		fmt.Printf("  Startup Form:    Index %d\n", proj.StartupIndex)
	}
	fmt.Printf("  VBGuard Status:  %s\n", formatVBGuardStatus(proj.VBGuard))
	if len(proj.CustomVBXs) > 0 {
		fmt.Printf("  Custom VBXs:     %v\n", proj.CustomVBXs)
	}
	fmt.Printf("  Forms Found:     %d\n\n", len(proj.Forms))

	var extracted []*frm.ExtractedForm
	if *formsFlag {
		opts := frm.ExtractOptions{
			NamesDir: *namesDirFlag,
		}

		var err error
		extracted, _, err = frm.ExtractForms(f, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error extracting forms: %v\n", err)
			os.Exit(1)
		}

		if err := frm.SaveExtractedFormsWithAssets(extracted, outDir, *rawFlag, *assetsFlag, *assetsDirFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving forms to %s: %v\n", outDir, err)
			os.Exit(1)
		}

		fmt.Printf("Extracted Forms (%s):\n", outDir)
		fmt.Printf("%-8s %-10s %-8s %-10s %-12s %-18s\n", "Form", "File", "Res ID", "Controls", "FRX Size", "Standalone Assets")
		fmt.Println("-------------------------------------------------------------------------")

		totalControls := 0
		totalAssets := 0
		for _, ef := range extracted {
			ctrlCount := len(ef.Form.Controls)
			totalControls += ctrlCount
			totalAssets += len(ef.Assets)

			frxSizeStr := "-"
			if len(ef.RawFRX) > 0 {
				frxSizeStr = fmt.Sprintf("%d bytes", len(ef.RawFRX))
			}

			assetsStr := "-"
			if len(ef.Assets) > 0 {
				assetsStr = summarizeAssets(ef.Assets)
			}

			fmt.Printf("%-8s %-10s %-8d %-10d %-12s %-18s\n",
				ef.Form.Name,
				ef.Ref.FileName,
				ef.Ref.ResourceID,
				ctrlCount,
				frxSizeStr,
				assetsStr,
			)
		}
		fmt.Println("-------------------------------------------------------------------------")
		fmt.Printf("Total: %d forms, %d controls decompiled.\n", len(extracted), totalControls)
		if *assetsFlag && totalAssets > 0 {
			targetAssetsDir := *assetsDirFlag
			if targetAssetsDir == "" {
				targetAssetsDir = filepath.Join(outDir, "assets")
			}
			fmt.Printf("Extracted %d standalone media asset files into: %s\n", totalAssets, targetAssetsDir)
		}
	}

	if *codeFlag {
		pcodeProj, err := pcode.ParseProject(f, frm.ExtractOptions{
			NamesDir: *namesDirFlag,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing P-code structures: %v\n", err)
			os.Exit(1)
		} else {
			d := pcode.NewDisassembler(pcodeProj)
			if err := os.MkdirAll(outDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating output directory %s: %v\n", outDir, err)
				os.Exit(1)
			}

			// 1. Emit Globals.bas if global variables exist
			if len(pcodeProj.GlobalVars) > 0 {
				var sb strings.Builder
				sb.WriteString("' Globals.bas - Project Global Definitions\n")
				sb.WriteString("Option Explicit\n\n")
				for _, gv := range pcodeProj.GlobalVars {
					sb.WriteString(gv)
					sb.WriteString("\n")
				}
				globalsPath := filepath.Join(outDir, "Globals.bas")
				if err := os.WriteFile(globalsPath, []byte(sb.String()), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", globalsPath, err)
					hadErrors = true
				}
			}

			fmt.Printf("\nDecompiled Modules (%s):\n", outDir)
			fmt.Printf("%-10s %-8s %-12s %-12s %-15s\n", "Module", "Type", "Local Procs", "API Decls", "Output File")
			fmt.Println("-------------------------------------------------------------")

			totalLocal := 0
			totalDecl := 0
			for _, mod := range pcodeProj.Modules {
				code, err := d.DisassembleModule(mod)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error decompiling module %s: %v\n", mod.Name, err)
					hadErrors = true
					continue
				}

				fileName := mod.Name + ".bas"
				outPath := filepath.Join(outDir, fileName)
				if err := os.WriteFile(outPath, []byte(code), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", outPath, err)
					hadErrors = true
					continue
				}

				// If this is a form, also append code to the .FRM file (below End)
				if mod.IsForm {
					frmFileName := strings.ToUpper(mod.Name) + ".FRM"
					frmPath := filepath.Join(outDir, frmFileName)
					if data, err := os.ReadFile(frmPath); err == nil {
						if !strings.Contains(string(data), "Sub ") && !strings.Contains(string(data), "Function ") {
							formCode, err := d.DisassembleFormCode(mod)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error decompiling form code %s: %v\n", mod.Name, err)
								hadErrors = true
							} else if err := os.WriteFile(frmPath, append(data, []byte(formCode)...), 0644); err != nil {
								fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", frmPath, err)
								hadErrors = true
							}
						}
					} else if !os.IsNotExist(err) {
						fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", frmPath, err)
						hadErrors = true
					}
				}

				localCount := 0
				declCount := 0
				for _, p := range mod.Procedures {
					if p.IsLocal {
						localCount++
					} else {
						declCount++
					}
				}
				totalLocal += localCount
				totalDecl += declCount

				modType := "Form"
				if !mod.IsForm {
					modType = "Code"
				}

				fmt.Printf("%-10s %-8s %-12d %-12d %-15s\n",
					mod.Name,
					modType,
					localCount,
					declCount,
					fileName,
				)
			}
			fmt.Println("-------------------------------------------------------------")
			fmt.Printf("Total: %d modules, %d procedures (%d local bytecode, %d API declarations).\n\n",
				len(pcodeProj.Modules), totalLocal+totalDecl, totalLocal, totalDecl)

			// Generate Project .MAK file
			baseExe := filepath.Base(targetExe)
			projName := strings.TrimSuffix(baseExe, filepath.Ext(baseExe))
			makFileName := projName + ".MAK"
			makPath := filepath.Join(outDir, makFileName)

			var makLines []string
			if len(extracted) > 0 {
				for _, ef := range extracted {
					makLines = append(makLines, ef.Ref.FileName)
				}
			} else {
				for _, fRef := range proj.Forms {
					makLines = append(makLines, fRef.FileName)
				}
			}
			if len(pcodeProj.GlobalVars) > 0 {
				makLines = append(makLines, "Globals.bas")
			}
			for _, mod := range pcodeProj.Modules {
				if !mod.IsForm {
					makLines = append(makLines, mod.Name+".bas")
				}
			}
			for _, vbx := range proj.CustomVBXs {
				makLines = append(makLines, filepath.Base(vbx))
			}
			makLines = append(makLines,
				"ProjWinSize=152,402,248,215",
				"ProjWinShow=1",
			)
			if proj.StartupIndex >= 0 && proj.StartupIndex < len(proj.Forms) {
				makLines = append(makLines, "IconForm="+win1252.QuoteVBString(proj.Forms[proj.StartupIndex].FormName))
			} else {
				makLines = append(makLines, `IconForm="frm1"`)
			}
			if proj.Title != "" {
				makLines = append(makLines, "Title="+win1252.QuoteVBString(proj.Title))
			} else {
				makLines = append(makLines, "Title="+win1252.QuoteVBString(projName))
			}
			makLines = append(makLines, "ExeName="+win1252.QuoteVBString(baseExe))

			makContent := strings.Join(makLines, "\r\n") + "\r\n"
			if err := os.WriteFile(makPath, []byte(makContent), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", makPath, err)
				hadErrors = true
			} else {
				fmt.Printf("Generated Visual Basic 3 project file: %s\n\n", makPath)
			}
		}
	}

	// Copy companion dependencies (.dll, .vbx) from the target EXE directory to outDir
	exeDir := filepath.Dir(targetExe)
	absExeDir, err1 := filepath.Abs(exeDir)
	absOutDir, err2 := filepath.Abs(outDir)
	if err1 == nil && err2 == nil && !strings.EqualFold(absExeDir, absOutDir) {
		if entries, err := os.ReadDir(exeDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				ext := strings.ToLower(filepath.Ext(name))
				if ext == ".dll" || ext == ".vbx" {
					srcPath := filepath.Join(exeDir, name)
					dstPath := filepath.Join(outDir, name)
					if data, err := os.ReadFile(srcPath); err == nil {
						if err := os.WriteFile(dstPath, data, 0644); err == nil {
							fmt.Printf("Copied companion dependency: %s -> %s\n", name, dstPath)
						} else {
							fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", dstPath, err)
							hadErrors = true
						}
					} else {
						fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", srcPath, err)
						hadErrors = true
					}
				}
			}
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading companion dependencies from %s: %v\n", exeDir, err)
			hadErrors = true
		}
	}
	if hadErrors {
		os.Exit(1)
	}
}

func summarizeAssets(assets []*frm.ExtractedAsset) string {
	counts := make(map[string]int)
	for _, a := range assets {
		counts[a.Format]++
	}
	var parts []string
	for ext, count := range counts {
		parts = append(parts, fmt.Sprintf("%d .%s", count, ext))
	}
	return fmt.Sprintf("%d (%s)", len(assets), joinStrings(parts, ", "))
}

func joinStrings(strs []string, sep string) string {
	res := ""
	for i, s := range strs {
		if i > 0 {
			res += sep
		}
		res += s
	}
	return res
}

func formatVBGuardStatus(vbGuard bool) string {
	if vbGuard {
		return "Protected (Control & form names stripped at compile/link time)"
	}
	return "Unprotected (Standard form names preserved)"
}
