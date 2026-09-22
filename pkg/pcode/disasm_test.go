package pcode_test

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"vb3dec/pkg/frm"
	"vb3dec/pkg/ne"
	"vb3dec/pkg/pcode"
)

func getTestProject(t *testing.T) (*ne.File, *pcode.Project) {
	t.Helper()
	exePath := filepath.Join("..", "..", "test_input", "FF.EXE")
	f, err := ne.Open(exePath)
	if err != nil {
		t.Fatalf("Failed to open test NE executable %s: %v", exePath, err)
	}

	proj, err := pcode.ParseProject(f, frm.ExtractOptions{})
	if err != nil {
		t.Fatalf("Failed to parse VB3 project: %v", err)
	}
	return f, proj
}

func TestFormInstanceMapping(t *testing.T) {
	_, proj := getTestProject(t)

	var frm5 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm5" {
			frm5 = m
			break
		}
	}
	if frm5 == nil {
		t.Fatal("frm5 module not found")
	}

	// Verify dynamically discovered form references without hardcoding
	if frm5.ControlMap[88] != "frm7" {
		t.Errorf("Expected frm5 offset 88 -> frm7, got %q", frm5.ControlMap[88])
	}
	if frm5.ControlMap[92] != "frm5" {
		t.Errorf("Expected frm5 offset 92 -> frm5, got %q", frm5.ControlMap[92])
	}
	if frm5.ControlMap[96] != "frm1" {
		t.Errorf("Expected frm5 offset 96 -> frm1, got %q", frm5.ControlMap[96])
	}
}

func TestModule1TypingAndSigils(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	mod1 := proj.Modules[0]
	tbl := pcode.GetOpcodeTable()
	for _, p := range mod1.Procedures {
		if p.Name == "fn0093" {
			bc := p.Bytecode
			for pc := 0; pc+2 <= len(bc); {
				tok := binary.LittleEndian.Uint16(bc[pc : pc+2])
				curPc := pc
				pc += 2
				info, alt := tbl.Lookup(tok)
				if info == nil {
					t.Logf("[%04X] unknown token 0x%04X", curPc, tok)
					continue
				}
				if info.Case == 8 {
					totLen := int(binary.LittleEndian.Uint16(bc[pc : pc+2]))
					s := string(bc[pc+2 : pc+2+totLen])
					pc += totLen + 2
					t.Logf("[%04X] str %q", curPc, s)
					continue
				}
				if info.Case == 10 || info.Case == 11 || info.Case == 12 || info.Case == 14 {
					target := binary.LittleEndian.Uint16(bc[pc : pc+2])
					pc += 2
					t.Logf("[%04X] %s L%04X", curPc, info.Keyword, target)
					continue
				}
				var params []uint16
				for i := 0; i < info.NumParams && pc+2 <= len(bc); i++ {
					params = append(params, binary.LittleEndian.Uint16(bc[pc:pc+2]))
					pc += 2
				}
				t.Logf("[%04X] tok=0x%04X alt=0x%04X kw=%q params=%v", curPc, tok, alt, info.Keyword, params)
			}
		}
	}
	code, err := d.DisassembleModule(mod1)
	if err != nil {
		t.Fatalf("DisassembleModule(mod1) failed: %v", err)
	}

	expectedSnippets := []string{
		"Function fn006F (p00A4 As String) As Integer",
		"    Dim l00A6 As Integer",
		"    Dim l00A8 As Integer",
		"    Dim l00AA As Long",
		"    Dim l00AC As Integer",
		"l00A8 = Len(p00A4$)",
		"Asc(Mid$(p00A4$, l00A6, 1))",
		"Function fn007D (p00B0 As Integer) As String",
		"    Dim l00B2 As String",
		"    Dim l00B4 As String",
		`    l00B4$ = ""`,
		"    fn007D = l00B4$",
		"Function fn0093 (ByVal p00C8 As Variant) As Variant",
		"    Dim l00CC\n",
		"    Dim l00D0\n",
		"    Dim l00D4\n",
		"    Dim l00D8\n",
		`    l00CC = extfn01DF("AOL FRAME25", 0&)`,
		`    l00D0 = extfn01B7(l00CC, "MDIClient")`,
		`    l00D4 = extfn0242(l00D0)`,
		`    l00D8 = extfn01B7(l00D4, "_AOL_EDIT")`,
		"Sub sub009D ()\n",
		`    l00F2 = extfn01DF("AOL FRAME25", 0&)`,
		`    l00F4 = extfn01B7(l00F2, "MDIClient")`,
	}
	for _, snip := range expectedSnippets {
		if !strings.Contains(code, snip) {
			t.Errorf("Module1 missing expected snippet: %q", snip)
		}
	}
}

func TestModule1StringTyping(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, p := range proj.Procedures {
		if p.Name == "fn00E6" {
			code, err := d.DisassembleProcedure(p)
			if err != nil {
				t.Fatalf("DisassembleProcedure(fn00E6) failed: %v", err)
			}
			if !strings.Contains(code, "Dim l0138 As String") {
				t.Errorf("fn00E6 missing 'Dim l0138 As String':\n%s", code)
			}
			if !strings.Contains(code, "l0138$ = String$(l0134, 0)") {
				t.Errorf("fn00E6 missing 'l0138$ = String$(l0134, 0)':\n%s", code)
			}
		}
		if p.Name == "fn00F5" {
			code, err := d.DisassembleProcedure(p)
			if err != nil {
				t.Fatalf("DisassembleProcedure(fn00F5) failed: %v", err)
			}
			for _, expected := range []string{
				"Dim l014C As String",
				"Dim l014E As String",
				"Dim l0150 As String",
				"l014C$ = String$(50, 0)",
				"l0150$ = String$(50, 0)",
			} {
				if !strings.Contains(code, expected) {
					t.Errorf("fn00F5 missing %q:\n%s", expected, code)
				}
			}
		}
	}
}

func TestEventParameters(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	mouseDownCount := 0
	for _, p := range proj.Procedures {
		if strings.Contains(p.Name, "_MouseDown") {
			code, err := d.DisassembleProcedure(p)
			if err != nil {
				t.Fatalf("DisassembleProcedure failed: %v", err)
			}
			if strings.Contains(code, "If p") {
				t.Errorf("%s contains unmapped parameter reference:\n%s", p.Name, code)
			}
			if strings.Contains(code, "frm7.control1.Caption = ") && !strings.Contains(code, "If Button = 2 Then") {
				t.Errorf("%s expected 'If Button = 2 Then':\n%s", p.Name, code)
			}
			mouseDownCount++
		}
		if p.Name == "control39_KeyPress" {
			code, err := d.DisassembleProcedure(p)
			if err != nil {
				t.Fatalf("DisassembleProcedure(control39_KeyPress) failed: %v", err)
			}
			if !strings.Contains(code, "If KeyAscii = 13 Then") {
				t.Errorf("control39_KeyPress missing 'If KeyAscii = 13 Then':\n%s", code)
			}
		}
	}
	if mouseDownCount < 20 {
		t.Errorf("Expected at least 20 _MouseDown procedures, got %d", mouseDownCount)
	}
}

func TestModuleVariableDeclarations(t *testing.T) {
	_, proj := getTestProject(t)

	var frm2, frm5, frm6, frm8 *pcode.Module
	for _, m := range proj.Modules {
		switch m.Name {
		case "frm2":
			frm2 = m
		case "frm5":
			frm5 = m
		case "frm6":
			frm6 = m
		case "frm8":
			frm8 = m
		}
	}

	if frm2 == nil || frm5 == nil || frm6 == nil || frm8 == nil {
		t.Fatal("One or more test modules not found")
	}

	// frm2 should declare m0020 as array and m0036 as string
	expectedFrm2 := []string{"Dim m0020(1 To 25) As Integer", "Dim m0036 As String"}
	if len(frm2.ModuleVars) != len(expectedFrm2) {
		t.Fatalf("frm2.ModuleVars expected %v, got %v", expectedFrm2, frm2.ModuleVars)
	}
	for i, v := range expectedFrm2 {
		if frm2.ModuleVars[i] != v {
			t.Errorf("frm2.ModuleVars[%d] expected %q, got %q", i, v, frm2.ModuleVars[i])
		}
	}

	// frm5 should declare m001E as Integer
	if len(frm5.ModuleVars) != 1 || frm5.ModuleVars[0] != "Dim m001E As Integer" {
		t.Errorf("frm5.ModuleVars expected ['Dim m001E As Integer'], got %v", frm5.ModuleVars)
	}

	// frm6 should declare m001A as Integer
	if len(frm6.ModuleVars) != 1 || frm6.ModuleVars[0] != "Dim m001A As Integer" {
		t.Errorf("frm6.ModuleVars expected ['Dim m001A As Integer'], got %v", frm6.ModuleVars)
	}

	// frm8 should declare m001E(1 To 30, 3) As String, m0038 As Single, etc.
	expectedFrm8 := []string{
		"Dim m001E(1 To 30, 3) As String",
		"Dim m0038 As Single",
		"Dim m003C As Integer",
		"Dim m003E As Integer",
		"Dim m0040 As Integer",
		"Dim m0042 As Integer",
		"Dim m0044 As Integer",
	}
	if len(frm8.ModuleVars) != len(expectedFrm8) {
		t.Fatalf("frm8.ModuleVars expected %v, got %v", expectedFrm8, frm8.ModuleVars)
	}
	for i, v := range expectedFrm8 {
		if frm8.ModuleVars[i] != v {
			t.Errorf("frm8.ModuleVars[%d] expected %q, got %q", i, v, frm8.ModuleVars[i])
		}
	}
}

func TestOptionButtonProperties(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	var frm8 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm8" {
			frm8 = m
			break
		}
	}
	if frm8 == nil {
		t.Fatal("frm8 module not found")
	}

	code, err := d.DisassembleModule(frm8)
	if err != nil {
		t.Fatalf("DisassembleModule(frm8) failed: %v", err)
	}

	if strings.Contains(code, "control43.ListCount") {
		t.Errorf("frm8 should not contain 'control43.ListCount' (OptionButton has .Value)")
	}
	if !strings.Contains(code, "control43.Value = 1") {
		t.Errorf("frm8 missing 'control43.Value = 1'")
	}
	if !strings.Contains(code, "If control43.Value = 1 Then") {
		t.Errorf("frm8 missing 'If control43.Value = 1 Then'")
	}
}

func TestDisasmBaseline(t *testing.T) {
	_, proj := getTestProject(t)

	if len(proj.Modules) != 10 {
		t.Errorf("Expected 10 modules (1 code module + 9 forms), got %d", len(proj.Modules))
	}

	if len(proj.Procedures) != 208 {
		t.Errorf("Expected 208 procedures, got %d", len(proj.Procedures))
	}

	localCount := 0
	extCount := 0
	for _, p := range proj.Procedures {
		if p.IsLocal {
			localCount++
		} else {
			extCount++
		}
	}

	if localCount != 191 {
		t.Errorf("Expected 191 local procedures, got %d", localCount)
	}
	if extCount != 17 {
		t.Errorf("Expected 17 external API declarations, got %d", extCount)
	}

	d := pcode.NewDisassembler(proj)

	// 1. Verify fn006F decompilation matches expected baseline logic
	var fn006F *pcode.Procedure
	for _, p := range proj.Procedures {
		if p.Name == "fn006F" {
			fn006F = p
			break
		}
	}
	if fn006F == nil {
		t.Fatal("Procedure fn006F not found")
	}

	code6F, err := d.DisassembleProcedure(fn006F)
	if err != nil {
		t.Fatalf("Disassembling fn006F failed: %v", err)
	}

	expectedSubstrings6F := []string{
		"Function fn006F (p00A4 As String) As Integer",
		"    Dim l00A6 As Integer",
		"    Dim l00A8 As Integer",
		"    Dim l00AA As Long",
		"    Dim l00AC As Integer",
		"l00A8 = Len(p00A4$)",
		"For l00A6 = 1 To (l00A8 - 1) Step 1",
		"Asc(Mid$(p00A4$, l00A6, 1))",
		"l00AA = l00AA * 90",
		"Next l00A6",
		"If l00AA > 32767 Then l00AA = -1",
		"l00AC = l00AA",
		"fn006F = l00AC",
		"End Function",
	}
	for _, sub := range expectedSubstrings6F {
		if !strings.Contains(code6F, sub) {
			t.Errorf("fn006F output missing expected snippet: %q\nFull output:\n%s", sub, code6F)
		}
	}

	// 2. Verify fn08BB decompilation (Select Case structure)
	var fn08BB *pcode.Procedure
	for _, p := range proj.Procedures {
		if p.Name == "fn08BB" {
			fn08BB = p
			break
		}
	}
	if fn08BB == nil {
		t.Fatal("Procedure fn08BB not found")
	}

	code8BB, err := d.DisassembleProcedure(fn08BB)
	if err != nil {
		t.Fatalf("Disassembling fn08BB failed: %v", err)
	}

	expectedSubstrings8BB := []string{
		"Select Case p00C2",
		`Case "Tonic"`,
		"fn08BB = 25",
		`Case "Potion"`,
		"fn08BB = 150",
		`Case "Tincture"`,
		"fn08BB = 750",
		"Case Else",
		"fn08BB = 0",
		"End Select",
		"End Function",
	}
	for _, sub := range expectedSubstrings8BB {
		if !strings.Contains(code8BB, sub) {
			t.Errorf("fn08BB output missing expected snippet: %q\nFull output:\n%s", sub, code8BB)
		}
	}

	// 3. Verify sub0AA6 and sub0BB8 disassemble cleanly
	for _, name := range []string{"sub0AA6", "sub0BB8"} {
		var proc *pcode.Procedure
		for _, p := range proj.Procedures {
			if p.Name == name {
				proc = p
				break
			}
		}
		if proc == nil {
			t.Fatalf("Procedure %s not found", name)
		}
		code, err := d.DisassembleProcedure(proc)
		if err != nil {
			t.Errorf("Disassembling %s failed: %v", name, err)
		}
		if !strings.HasPrefix(code, "Sub "+name) {
			t.Errorf("%s missing expected Sub header", name)
		}
		if !strings.HasSuffix(code, "End Sub") {
			t.Errorf("%s missing expected End Sub trailer", name)
		}
	}
}

func TestMissingFormsCoverage(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	// Legacy VBDIS3.67e dropped all 70 procedures across Forms 1-4:
	// Form 1 (Module 2, frm1): 25 procs
	// Form 2 (Module 3, frm2): 21 procs
	// Form 3 (Module 4, frm3): 8 procs
	// Form 4 (Module 5, frm4): 16 procs
	expectedProcsPerForm := map[int]int{
		2: 25,
		3: 21,
		4: 8,
		5: 16,
	}

	recoveredTotal := 0
	for modIdx, expectedCount := range expectedProcsPerForm {
		mod := proj.Modules[modIdx-1]
		if len(mod.Procedures) != expectedCount {
			t.Errorf("Module %d (%s) expected %d procedures, got %d",
				modIdx, mod.Name, expectedCount, len(mod.Procedures))
		}
		recoveredTotal += len(mod.Procedures)

		for _, p := range mod.Procedures {
			if !p.IsLocal {
				continue
			}
			if len(p.Bytecode) == 0 {
				t.Errorf("Procedure %s in module %s has empty bytecode", p.Name, mod.Name)
			}
			code, err := d.DisassembleProcedure(p)
			if err != nil {
				t.Errorf("Error decompiling %s in %s: %v", p.Name, mod.Name, err)
			}
			if len(code) == 0 {
				t.Errorf("Empty decompilation for %s in %s", p.Name, mod.Name)
			}
		}
	}

	if recoveredTotal != 70 {
		t.Errorf("Expected 70 recovered procedures in Forms 1-4, got %d", recoveredTotal)
	}
}

func TestDarkMatterStrings(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	findProc := func(name string, nameID uint16) *pcode.Procedure {
		for _, p := range proj.Procedures {
			if p.Name == name || p.NameID == nameID {
				return p
			}
		}
		return nil
	}

	// 1. Assert 'mimic' is present in recovered procedures:
	// - sub060A (Mod 4 / Form 3): contains " Player Added As a Mimic "
	// - sub065A (Mod 5 / Form 4): contains 'Case "Mimic"'
	sub060A := findProc("sub060A", 0x060A)
	if sub060A == nil {
		t.Fatal("sub060A not found in project")
	}
	code060A, err := d.DisassembleProcedure(sub060A)
	if err != nil {
		t.Fatalf("Decompiling sub060A failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(code060A), "mimic") {
		t.Errorf("sub060A does not contain 'mimic' string")
	}
	if !strings.Contains(code060A, " Player Added As a Mimic ") {
		t.Errorf("sub060A does not contain expected literal ' Player Added As a Mimic '")
	}

	sub065A := findProc("sub065A", 0x065A)
	if sub065A == nil {
		t.Fatal("sub065A not found in project")
	}
	code065A, err := d.DisassembleProcedure(sub065A)
	if err != nil {
		t.Fatalf("Decompiling sub065A failed: %v", err)
	}
	if !strings.Contains(code065A, `Case "Mimic"`) {
		t.Errorf("sub065A does not contain expected 'Case \"Mimic\"'")
	}

	// 2. Assert 'morph' is present in recovered procedures:
	// - sub052F (Mod 4 / Form 3): contains "Confirm? </yes> </lock> </morph>"
	// - sub060A: contains "/morph"
	// - sub0BB8 (Mod 10 / Form 8): contains " morphs into Esper form."
	sub052F := findProc("sub052F", 0x052F)
	if sub052F == nil {
		t.Fatal("sub052F not found in project")
	}
	code052F, err := d.DisassembleProcedure(sub052F)
	if err != nil {
		t.Fatalf("Decompiling sub052F failed: %v", err)
	}
	if !strings.Contains(code052F, "</morph>") {
		t.Errorf("sub052F does not contain expected '</morph>'")
	}

	if !strings.Contains(code060A, `"/morph"`) {
		t.Errorf("sub060A does not contain expected '\"/morph\"'")
	}

	sub0BB8 := findProc("sub0BB8", 0x0BB8)
	if sub0BB8 == nil {
		t.Fatal("sub0BB8 not found in project")
	}
	code0BB8, err := d.DisassembleProcedure(sub0BB8)
	if err != nil {
		t.Fatalf("Decompiling sub0BB8 failed: %v", err)
	}
	if !strings.Contains(code0BB8, " morphs into Esper form.") {
		t.Errorf("sub0BB8 does not contain expected ' morphs into Esper form.'")
	}

	// 3. Assert 'esper' is present in recovered procedure sub0BB8
	if !strings.Contains(strings.ToLower(code0BB8), "esper") {
		t.Errorf("sub0BB8 does not contain 'esper' string")
	}
}

func TestAllModulesDisassemble(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, mod := range proj.Modules {
		modCode, err := d.DisassembleModule(mod)
		if err != nil {
			t.Errorf("Failed to disassemble module %s: %v", mod.Name, err)
		}
		if len(modCode) == 0 {
			t.Errorf("Module %s produced empty source code", mod.Name)
		}
	}
}

func TestFrm7Decompilation(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	var frm7 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm7" {
			frm7 = m
			break
		}
	}
	if frm7 == nil {
		t.Fatal("frm7 module not found")
	}

	code, err := d.DisassembleModule(frm7)
	if err != nil {
		t.Fatalf("DisassembleModule(frm7) failed: %v", err)
	}

	expectedSnippets := []string{
		"Sub Form_Load ()",
		"    extsub024F frm7.hWnd, -1, 0, 0, 0, 0, 3",
		"Sub control1_Click ()",
		"    Unload frm7",
		"Sub sub0A7B (p001E As Integer)",
		"    If p001E = 1 Then frm7.Hide",
	}
	for _, s := range expectedSnippets {
		if !strings.Contains(code, s) {
			t.Errorf("frm7 missing expected snippet %q\nFull code:\n%s", s, code)
		}
	}
}

func TestFrm5Decompilation(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	var frm5 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm5" {
			frm5 = m
			break
		}
	}
	if frm5 == nil {
		t.Fatal("frm5 not found")
	}

	code, err := d.DisassembleModule(frm5)
	if err != nil {
		t.Fatalf("DisassembleModule(frm5) err: %v", err)
	}

	expectedSnippets := []string{
		"Dim m001E As Integer",
		"Sub control19_Click ()",
		"Select Case control19.Text",
		`control22.Caption = "0"`,
		`control22.Caption = "50"`,
		`control22.Caption = "300"`,
		"Sub control12_Click ()",
		"    Unload frm5",
		"    frm1.Show",
		"Sub Form_Load ()",
		`    control19.AddItem "Tonic"`,
		`    control19.AddItem "Potion"`,
		"Sub control20_Click ()",
		"    Dim l002C As Integer",
		"    Dim l0030 As Integer",
		"Sub control20_MouseDown (Button As Integer, Shift As Integer, X As Single, Y As Single)",
	}

	for _, snip := range expectedSnippets {
		if !strings.Contains(code, snip) {
			t.Errorf("frm5 missing expected snippet %q\nFull code snippet:\n%s", snip, code[:min(len(code), 1200)])
		}
	}
}

func TestExternalDeclarations(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	mod1 := proj.Modules[0]
	code, err := d.DisassembleModule(mod1)
	if err != nil {
		t.Fatalf("DisassembleModule(mod1) failed: %v", err)
	}

	expectedDecls := []string{
		`Declare Function extfn01B7 Lib "ff.dll" Alias "FindChildByClass" (ByVal p1%, ByVal p2$) As Integer`,
		`Declare Function extfn01CB Lib "ff.dll" Alias "FindChildByTitle" (ByVal p1%, ByVal p2$) As Integer`,
		`Declare Function extfn01DF Lib "User" Alias "Findwindow" (ByVal p1 As Any, ByVal p2 As Any) As Integer`,
		`Declare Function extfn029A Lib "kernel" Alias "GetCurrentDirectory" () As Long`,
		`Declare Function extfn0270 Lib "User" Alias "GetMenu" () As Integer`,
		`Declare Function extfn01ED Lib "User" Alias "GetMenuItemCount" () As Integer`,
		`Declare Function extfn027B Lib "User" Alias "GetMenuItemID" () As Integer`,
		`Declare Function extfn0201 Lib "User" Alias "GetMenuString" () As Integer`,
		`Declare Function extfn0231 Lib "User" Alias "getnextwindow" (ByVal p1%, ByVal p2%) As Integer`,
		`Declare Function extfn0242 Lib "User" Alias "GetParent" (ByVal p1%) As Integer`,
		`Declare Function extfn028C Lib "User" Alias "GetSubMenu" () As Integer`,
		`Declare Function extfn025F Lib "User" Alias "getwindowtext" (ByVal p1%, ByVal p2$, ByVal p3%) As Integer`,
		`Declare Function extfn02B1 Lib "User" Alias "getwindowtextlength" (ByVal p1%) As Integer`,
		`Declare Function extfn02C8 Lib "VBMsg.Vbx" Alias "ptGetStringFromAddress" () As String`,
		`Declare Function extfn0212 Lib "User" Alias "SendMessage" (ByVal p1%, ByVal p2%, ByVal p3%, ByVal p4&) As Long`,
		`Declare Function extfn0221 Lib "User" Alias "SendMessage" (ByVal p1%, ByVal p2%, ByVal p3%, ByVal p4$) As Long`,
		`Declare Sub extsub024F Lib "User" Alias "setwindowpos" (ByVal p1%, ByVal p2%, ByVal p3%, ByVal p4%, ByVal p5%, ByVal p6%, ByVal p7%)`,
	}
	for _, decl := range expectedDecls {
		if !strings.Contains(code, decl) {
			t.Errorf("Module1 missing expected declaration %q", decl)
		}
	}
}

func TestGlobalsRecovery(t *testing.T) {
	_, proj := getTestProject(t)

	if len(proj.GlobalVars) != 23 {
		t.Fatalf("Expected 23 global variables, got %d", len(proj.GlobalVars))
	}

	expectedGlobals := []string{
		"Global gv0006(1 To 30, 1 To 2) As String",
		"Global gv0020 As String",
		"Global gv0024 As Integer",
		"Global gv002A As String",
		"Global gv002E(1 To 30) As Integer",
		"Global gv0044(1 To 30, 1 To 100) As Integer",
		"Global gv005E(1 To 30) As String",
		"Global gv0074(1 To 30, 1 To 25) As Integer",
		"Global gv008E(100) As String",
		"Global gv00A4(100, 1 To 2) As Integer",
		"Global gv00BE(255) As String",
		"Global gv00D4(255, 1 To 30) As Integer",
		"Global gv00EE(1 To 255) As String",
		"Global gv0104 As String",
		"Global gv0108 As String",
		"Global gv010C As String",
		"Global gv0110 As String",
		"Global gv0114 As String",
		"Global gv0118 As String",
		"Global gv011C As String",
		"Global gv0120 As String",
		"Global gv0124 As String",
		"Global gv0128 As String",
	}

	for i, exp := range expectedGlobals {
		if proj.GlobalVars[i] != exp {
			t.Errorf("[%d] Expected %q, got %q", i, exp, proj.GlobalVars[i])
		}
	}
}

func TestModule1Cleanliness(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	mod1 := proj.Modules[0]
	if len(mod1.ModuleVars) != 0 {
		t.Errorf("Expected 0 module variables in Module1, got %d: %v", len(mod1.ModuleVars), mod1.ModuleVars)
	}

	code, err := d.DisassembleModule(mod1)
	if err != nil {
		t.Fatalf("DisassembleModule failed: %v", err)
	}

	// Verify no bogus m0001..m0004 or m0010 declarations
	if strings.Contains(code, "Dim m00") {
		t.Errorf("Module1 contains unexpected module variable declarations: %s", code[:500])
	}

	// Verify correct array assignment in fn008B: gv00EE(l00C2) = p00C0$
	if !strings.Contains(code, "gv00EE(l00C2) = p00C0$") {
		t.Errorf("Module1 missing expected array assignment 'gv00EE(l00C2) = p00C0$'")
	}
}

func TestFrm5ActivateAndIndentation(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	var frm5 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm5" {
			frm5 = m
			break
		}
	}
	if frm5 == nil {
		t.Fatal("frm5 module not found")
	}

	code, err := d.DisassembleModule(frm5)
	if err != nil {
		t.Fatalf("DisassembleModule(frm5) failed: %v", err)
	}

	// Check m001E count: exactly 18
	m001ECount := strings.Count(code, "m001E")
	if m001ECount != 18 {
		t.Errorf("Expected exactly 18 uses of m001E in frm5, got %d", m001ECount)
	}

	// Check Form_Activate presence
	if !strings.Contains(code, "Sub Form_Activate ()") {
		t.Errorf("frm5 missing 'Sub Form_Activate ()'")
	}

	// Check Case statement body indentation (+4 spaces from Case at 8 spaces = 12 spaces)
	expectedCaseIndent := "        Case \"Tonic\"\n            control22.Caption = \"50\""
	if !strings.Contains(code, expectedCaseIndent) {
		t.Errorf("frm5 missing indented Case body: expected %q", expectedCaseIndent)
	}
}

func TestFrm5ModuleVariablesAndIfIndentation(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	var frm5 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm5" {
			frm5 = m
			break
		}
	}
	if frm5 == nil {
		t.Fatal("frm5 module not found")
	}

	// 1. Verify module variables: ONLY m001E, no bogus m003C, m0048, or m0076
	if len(frm5.ModuleVars) != 1 || frm5.ModuleVars[0] != "Dim m001E As Integer" {
		t.Errorf("Expected only ['Dim m001E As Integer'], got %v", frm5.ModuleVars)
	}

	code, err := d.DisassembleModule(frm5)
	if err != nil {
		t.Fatalf("DisassembleModule(frm5) failed: %v", err)
	}

	if strings.Contains(code, "m003C") {
		t.Errorf("frm5 code should not contain m003C (should be gv005E)")
	}
	if strings.Contains(code, "m0048") {
		t.Errorf("frm5 code should not contain m0048 (should be gv0074)")
	}
	if strings.Contains(code, "m0076") {
		t.Errorf("frm5 code should not contain m0076 (should be fn012A)")
	}

	// 2. Verify accurate resolved calls in control18_Click
	if !strings.Contains(code, "l006C = fn08BB(fn012A(gv0074(m001E, 14)))") {
		t.Errorf("frm5 missing expected resolved call 'l006C = fn08BB(fn012A(gv0074(m001E, 14)))'")
	}
	if !strings.Contains(code, "l006E = MsgBox(") {
		t.Errorf("frm5 missing expected MsgBox function call 'l006E = MsgBox('")
	}

	// 3. Verify nested If...End If indentation
	expectedIfBlock := "    If control7.Caption <> \"\" Then\n        l006C = fn08BB(fn012A(gv0074(m001E, 14)))\n        l006E = MsgBox(\"Sell this item for\" + Str$(l006C) + \" gp?\", 4, \"Sell Item\")\n        If l006E = 6 Then\n            l006A = gv0074(m001E, 13)\n            l006A = l006A + l006C\n            If l006A > 32765 Then l006A = 32765\n            gv0074(m001E, 13) = l006A\n            gv0074(m001E, 14) = 0\n            sub08E4\n        End If\n    End If"
	if !strings.Contains(code, expectedIfBlock) {
		t.Errorf("frm5 missing expected indented If...End If block:\nExpected:\n%s", expectedIfBlock)
	}
}

func TestFrm6Control12Click(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	var frm6 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm6" {
			frm6 = m
			break
		}
	}
	if frm6 == nil {
		t.Fatal("frm6 module not found")
	}

	code, err := d.DisassembleModule(frm6)
	if err != nil {
		t.Fatalf("DisassembleModule(frm6) failed: %v", err)
	}

	expectedSnippets := []string{
		"Sub control12_Click ()",
		"    Dim l0110\n",
		"    Dim l0114 As Integer",
		"    control4.ForeColor = &HC00000&",
		"    control5.ForeColor = &HC00000&",
		"    control6.ForeColor = &HFF&",
		"    control7.ForeColor = &HC00000&",
		"    control8.ForeColor = &HC00000&",
		"    control9.ForeColor = &HC00000&",
		"    control16.Clear",
		"    l0110 = gv0074(m001A, 25) + 14",
		"    For l0114 = 1 To 100 Step 1",
		"    Next l0114",
	}

	for _, snip := range expectedSnippets {
		if !strings.Contains(code, snip) {
			t.Errorf("frm6 missing expected snippet %q", snip)
		}
	}
}

func TestFrm1Control6And33(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	var frm1 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm1" {
			frm1 = m
			break
		}
	}
	if frm1 == nil {
		t.Fatal("frm1 not found")
	}

	var ctrl6Proc, ctrl33Proc *pcode.Procedure
	for _, p := range frm1.Procedures {
		if p.Name == "control6_Click" {
			ctrl6Proc = p
		}
		if p.Name == "control33_Timer" {
			ctrl33Proc = p
		}
	}

	if ctrl6Proc == nil {
		t.Fatal("control6_Click not found in frm1")
	}
	if ctrl33Proc == nil {
		t.Fatal("control33_Timer not found in frm1")
	}

	// 1. Verify control6_Click decompilation
	code6, err := d.DisassembleProcedure(ctrl6Proc)
	if err != nil {
		t.Fatalf("Disassembling control6_Click failed: %v", err)
	}

	expectedCtrl6Snippets := []string{
		"l00C0 = control4.ListCount",
		"For l00BE = 0 To (l00C0 - 1) Step 1",
		"If control4.List(l00BE) = l00C2$ Then l00C6 = l00BE",
		"Next l00BE",
		"control4.RemoveItem l00C6",
	}
	for _, snip := range expectedCtrl6Snippets {
		if !strings.Contains(code6, snip) {
			t.Errorf("control6_Click missing expected snippet: %q\nCode:\n%s", snip, code6)
		}
	}

	if strings.Contains(code6, "pop.var()") {
		t.Errorf("control6_Click contains unhandled 'pop.var()'")
	}
	if strings.Contains(code6, "\n    control4\n") || strings.Contains(code6, "\ncontrol4\n") {
		t.Errorf("control6_Click contains orphaned 'control4' statement")
	}

	// 2. Verify control33_Click decompilation
	code33, err := d.DisassembleProcedure(ctrl33Proc)
	if err != nil {
		t.Fatalf("Disassembling control33_Click failed: %v", err)
	}

	expectedCtrl33Snippets := []string{
		"If frm1.Visible = False Then Exit Sub",
	}
	for _, snip := range expectedCtrl33Snippets {
		if !strings.Contains(code33, snip) {
			t.Errorf("control33_Click missing expected snippet: %q\nCode:\n%s", snip, code33)
		}
	}

	if strings.Contains(code33, "Prop_C02E") {
		t.Errorf("control33_Click contains unmapped Prop_C02E")
	}
	if strings.Contains(code33, "Then Exit\n") || strings.Contains(code33, "Then Exit\r\n") {
		t.Errorf("control33_Click contains bare 'Then Exit' without Sub")
	}
}

func TestCrossFormControlsAndMMControl(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	// 1. Verify frm1.control16_MouseDown resolves frm7.control1.Caption
	var frm1 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm1" {
			frm1 = m
			break
		}
	}
	if frm1 == nil {
		t.Fatal("frm1 not found")
	}

	for _, p := range frm1.Procedures {
		if p.Name == "control16_MouseDown" {
			code, err := d.DisassembleProcedure(p)
			if err != nil {
				t.Fatalf("Disassembling control16_MouseDown failed: %v", err)
			}
			if !strings.Contains(code, "frm7.control1.Caption = ") {
				t.Errorf("control16_MouseDown missing 'frm7.control1.Caption = '\nCode:\n%s", code)
			}
			if strings.Contains(code, "Prop_") {
				t.Errorf("control16_MouseDown contains unresolved Prop_:\n%s", code)
			}
		}
	}

	// 2. Verify frm8 MMControl and frm9.control1 (ListBox) resolution
	var frm8 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm8" {
			frm8 = m
			break
		}
	}
	if frm8 == nil {
		t.Fatal("frm8 not found")
	}

	code8, err := d.DisassembleModule(frm8)
	if err != nil {
		t.Fatalf("Disassembling frm8 failed: %v", err)
	}

	expectedFrm8Snippets := []string{
		`control29.DeviceType = "WaveAudio"`,
		`control29.FileName = "ff.wav"`,
		`control32.DeviceType = "Sequencer"`,
		`control29.Notify = False`,
		`control29.Wait = True`,
		`control29.Shareable = False`,
		`If control29.CanPlay = True Then`,
		`l0330 = frm9.control1.ListCount`,
		`If frm9.control1.List(l0332) = gv0006(1, 1) Then Exit Sub`,
		`frm9.control1.RemoveItem l0332`,
		`frm9.control1.AddItem gv005E(l0328)`,
		`frm1.control10.Caption = "No one"`,
		`frm1.control11.Caption = "  0/0"`,
		`frm1.control12.Caption = 0`,
		`frm1.control15.Caption = 0`,
	}
	for _, line := range strings.Split(code8, "\n") {
		if strings.Contains(line, "frm1.") || strings.Contains(line, "frm9.") {
			t.Logf("frm8 line: %s", line)
		}
	}
	for _, snip := range expectedFrm8Snippets {
		if !strings.Contains(code8, snip) {
			t.Errorf("frm8 missing expected snippet: %q", snip)
		}
	}

	// 3. Verify that NO module has unresolved Prop_ tokens
	for _, m := range proj.Modules {
		code, err := d.DisassembleModule(m)
		if err != nil {
			t.Fatalf("Disassembling module %s failed: %v", m.Name, err)
		}
		if strings.Contains(code, "Prop_") {
			t.Errorf("Module %s contains unresolved Prop_:\n%s", m.Name, code)
		}
	}
}

func TestSub00B5AndLoopDecompilation(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	var mod1 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "Module1" {
			mod1 = m
			break
		}
	}
	if mod1 == nil {
		t.Fatal("Module1 not found")
	}

	code1, err := d.DisassembleModule(mod1)
	if err != nil {
		t.Fatalf("Disassembling Module1 failed: %v", err)
	}

	for _, line := range strings.Split(code1, "\n") {
		if strings.Contains(line, "fn00C9") || strings.Contains(line, "Asc(Mid") || strings.Contains(line, "sub00B5") || strings.Contains(line, "Timer") || strings.Contains(line, "Do While") {
			t.Logf("Line: %s", line)
		}
	}

	expectedSnippets := []string{
		"Sub sub00B5 (p00FA As Variant)",
		"l00FE = Timer",
		"l0102 = Timer + p00FA",
		"Do While l0102 >= Timer",
		"    DoEvents",
		"Loop",
		"Function fn00C9 (p010C As String) As String",
		"Do While (Asc(Mid$(p010C$, l010E, 1))) >= 32 And (Asc(Mid$(p010C$, l010E, 1)) <= 126)",
	}
	for _, snip := range expectedSnippets {
		if !strings.Contains(code1, snip) {
			t.Errorf("Module1 missing expected snippet: %q", snip)
		}
	}
}

func TestClipboardOperationsAndGetText(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	// 1. Verify frm2.control5_Click decompiles to Clipboard.GetText(1)
	var frm2 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm2" {
			frm2 = m
			break
		}
	}
	if frm2 == nil {
		t.Fatal("frm2 module not found")
	}

	var ctrl5Click *pcode.Procedure
	for _, p := range frm2.Procedures {
		if p.Name == "control5_Click" {
			ctrl5Click = p
			break
		}
	}
	if ctrl5Click == nil {
		t.Fatal("frm2.control5_Click procedure not found")
	}

	codeCtrl5, err := d.DisassembleProcedure(ctrl5Click)
	if err != nil {
		t.Fatalf("Disassembling control5_Click failed: %v", err)
	}
	if !strings.Contains(codeCtrl5, "control3 = Clipboard.GetText(1)") {
		t.Errorf("control5_Click missing expected 'control3 = Clipboard.GetText(1)', got:\n%s", codeCtrl5)
	}

	// 2. Verify all procedures across project do not contain any spurious "Module1." or "Method_" calls
	for _, m := range proj.Modules {
		for _, p := range m.Procedures {
			if !p.IsLocal {
				continue
			}
			code, err := d.DisassembleProcedure(p)
			if err != nil {
				continue
			}
			if strings.Contains(code, "Module1.") {
				t.Errorf("%s.%s contains unexpected 'Module1.' call:\n%s", m.Name, p.Name, code)
			}
			if strings.Contains(code, "Method_") {
				t.Errorf("%s.%s contains unmapped 'Method_':\n%s", m.Name, p.Name, code)
			}
		}
	}
}

func TestReDimStatements(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	expectedSnippets := map[string]map[string][]string{
		"Module1": {
			"sub0171": {
				"Dim l01B4() As Integer",
				"ReDim l01B4(1 To 4, 1 To 9) As Integer",
			},
		},
		"frm2": {
			"fn05DB": {
				"Function fn05DB (p00CE As String) As Variant",
				"Dim l00D2() As String",
				"ReDim l00D2(1 To 50) As String",
			},
		},
		"frm3": {
			"control8_Click": {
				"Dim l0044() As Integer",
				"ReDim l0044(1 To 25) As Integer",
			},
		},
		"frm8": {
			"sub0C1D": {
				"Dim l020C() As Integer",
				"ReDim l020C(1 To 3) As Integer",
			},
			"sub0CB0": {
				"Dim l02D4() As Integer",
				"ReDim l02D4(1 To 4, 0 To 8) As Integer",
			},
			"fn0CBE": {
				"Dim l031C()",
				"ReDim l031C(1 To 4, 0 To 8)",
			},
		},
	}

	for modName, procMap := range expectedSnippets {
		var mod *pcode.Module
		for _, m := range proj.Modules {
			if m.Name == modName {
				mod = m
				break
			}
		}
		if mod == nil {
			t.Fatalf("Module %s not found", modName)
		}
		for procName, snippets := range procMap {
			var proc *pcode.Procedure
			for _, p := range mod.Procedures {
				if p.Name == procName {
					proc = p
					break
				}
			}
			if proc == nil {
				t.Fatalf("Procedure %s.%s not found", modName, procName)
			}
			code, err := d.DisassembleProcedure(proc)
			if err != nil {
				t.Fatalf("Disassembling %s.%s failed: %v", modName, procName, err)
			}
			for _, snip := range snippets {
				if !strings.Contains(code, snip) {
					t.Errorf("%s.%s missing expected snippet: %q\nFull output:\n%s", modName, procName, snip, code)
				}
			}
		}
	}

	// Verify no procedure in the entire project contains bare "ReDim" on a line by itself
	for _, m := range proj.Modules {
		for _, p := range m.Procedures {
			if !p.IsLocal {
				continue
			}
			code, err := d.DisassembleProcedure(p)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(code, "\n") {
				if strings.TrimSpace(line) == "ReDim" {
					t.Errorf("%s.%s contains bare 'ReDim' on a line by itself:\n%s", m.Name, p.Name, code)
				}
			}
		}
	}
}

func TestAuditUnhandledOpcodes(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, m := range proj.Modules {
		for _, p := range m.Procedures {
			if !p.IsLocal {
				continue
			}
			_, _ = d.DisassembleProcedure(p)
		}
	}

	t.Logf("Total unhandled opcode occurrences: %d", len(d.Warnings))

	type opSummary struct {
		tokenID  uint16
		altToken uint16
		keyword  string
		opCase   uint16
		count    int
		examples []string
	}

	summaries := make(map[string]*opSummary)
	for _, w := range d.Warnings {
		key := fmt.Sprintf("Token %d (0x%04X) kw=%q case=%d", w.TokenID, w.TokenID, w.Keyword, w.Case)
		s, ok := summaries[key]
		if !ok {
			s = &opSummary{
				tokenID:  w.TokenID,
				altToken: w.AltToken,
				keyword:  w.Keyword,
				opCase:   w.Case,
			}
			summaries[key] = s
		}
		s.count++
		if len(s.examples) < 5 {
			s.examples = append(s.examples, fmt.Sprintf("%s.%s @ 0x%04X params=%v", w.ModuleName, w.ProcName, w.PC, w.Params))
		}
	}

	var keys []string
	for k := range summaries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return summaries[keys[i]].count > summaries[keys[j]].count
	})

	t.Logf("=== SUMMARY OF UNHANDLED OPCODES FALLING INTO DEFAULT ===")
	for _, k := range keys {
		s := summaries[k]
		t.Logf("  * [%d times] %s (altToken=0x%04X)", s.count, k, s.altToken)
		for _, ex := range s.examples {
			t.Logf("      example: %s", ex)
		}
	}

	if len(d.Warnings) != 0 {
		t.Errorf("Expected 0 unhandled opcode warnings, got %d", len(d.Warnings))
	}
}

func TestRemediatedOpcodes(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	getProcCode := func(modName, procName string) string {
		for _, m := range proj.Modules {
			if m.Name == modName {
				for _, p := range m.Procedures {
					if p.Name == procName {
						code, err := d.DisassembleProcedure(p)
						if err != nil {
							t.Fatalf("Failed to disassemble %s.%s: %v", modName, procName, err)
						}
						return code
					}
				}
			}
		}
		t.Fatalf("Procedure %s.%s not found", modName, procName)
		return ""
	}

	// 1. Chr$ in frm8.control39_KeyPress
	codeKeyPress := getProcCode("frm8", "control39_KeyPress")
	for _, snip := range []string{"Chr$(13)", "Chr$(32)", "Chr$(9)"} {
		if !strings.Contains(codeKeyPress, snip) {
			t.Errorf("frm8.control39_KeyPress missing expected snippet: %q", snip)
		}
	}
	if strings.Contains(codeKeyPress, "32 + Chr$") {
		t.Errorf("frm8.control39_KeyPress contains broken Chr$ addition snippet")
	}

	// 2. Chr$ in frm2.fn05EA
	codeFn05EA := getProcCode("frm2", "fn05EA")
	if !strings.Contains(codeFn05EA, "Mid$(p00FE$, l0104, 1) = Chr$(32)") {
		t.Errorf("frm2.fn05EA missing expected Chr$(32) comparison, got:\n%s", codeFn05EA)
	}

	// 3. String$ in Module1.fn0093
	codeFn0093 := getProcCode("Module1", "fn0093")
	if !strings.Contains(codeFn0093, "String$(255, 0)") {
		t.Errorf("Module1.fn0093 missing expected String$(255, 0), got:\n%s", codeFn0093)
	}

	// 4. Load in frm1.control18_Click
	codeControl18 := getProcCode("frm1", "control18_Click")
	if !strings.Contains(codeControl18, "Load frm5") {
		t.Errorf("frm1.control18_Click missing expected 'Load frm5', got:\n%s", codeControl18)
	}

	// 5. Default argument stripping in frm1.control6_Click
	codeControl6 := getProcCode("frm1", "control6_Click")
	if !strings.Contains(codeControl6, "MsgBox \"Error\"") {
		t.Errorf("frm1.control6_Click missing clean 'MsgBox \"Error\"', got:\n%s", codeControl6)
	}
	if strings.Contains(codeControl6, "<default") {
		t.Errorf("frm1.control6_Click contains unstripped <default argument: %s", codeControl6)
	}

	// 6. Randomize Timer in frm1.Form_Load
	codeFormLoad := getProcCode("frm1", "Form_Load")
	if !strings.Contains(codeFormLoad, "Randomize Timer") {
		t.Errorf("frm1.Form_Load missing 'Randomize Timer', got:\n%s", codeFormLoad)
	}

	// 7. Windows-1252 string literal transcoding in Module1.fn00BE
	codeFn00BE := getProcCode("Module1", "fn00BE")
	if !strings.Contains(codeFn00BE, `fn00BE = ">——»›»"`) {
		t.Errorf("Module1.fn00BE missing transcoded Windows-1252 string '>——»›»', got:\n%s", codeFn00BE)
	}

	// 8. Windows-1252 string literal transcoding in Module1.fn018F
	codeFn018F := getProcCode("Module1", "fn018F")
	if !strings.Contains(codeFn018F, `fn018F = "«‹«——<"`) {
		t.Errorf("Module1.fn018F missing transcoded Windows-1252 string '«‹«——<', got:\n%s", codeFn018F)
	}
}

func TestForLoopDecompilation(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	getProcCode := func(modName, procName string) string {
		for _, m := range proj.Modules {
			if m.Name == modName {
				for _, p := range m.Procedures {
					if p.Name == procName {
						code, err := d.DisassembleProcedure(p)
						if err != nil {
							t.Fatalf("Failed to disassemble %s.%s: %v", modName, procName, err)
						}
						return code
					}
				}
			}
		}
		t.Fatalf("Procedure %s.%s not found", modName, procName)
		return ""
	}

	// 1. Module1.fn019C has TokenID 62 (For without Step)
	codeFn019C := getProcCode("Module1", "fn019C")
	if !strings.Contains(codeFn019C, "For l01E2 = 1 To l01E6") {
		t.Errorf("Module1.fn019C missing expected 'For l01E2 = 1 To l01E6', got:\n%s", codeFn019C)
	}

	// 2. frm2.control58_Timer has TokenID 62
	codeFrm2Ctrl58 := getProcCode("frm2", "control58_Timer")
	if !strings.Contains(codeFrm2Ctrl58, "For l01EE = 1 To 30") {
		t.Errorf("frm2.control58_Timer missing expected 'For l01EE = 1 To 30', got:\n%s", codeFrm2Ctrl58)
	}

	// 3. frm8.control2_Click has TokenID 62
	codeFrm8Ctrl2 := getProcCode("frm8", "control2_Click")
	if !strings.Contains(codeFrm8Ctrl2, "For l0134 = 1 To m0040") {
		t.Errorf("frm8.control2_Click missing expected 'For l0134 = 1 To m0040', got:\n%s", codeFrm8Ctrl2)
	}

	// 4. Verify no procedure decompiles to malformed "For  ="
	for _, m := range proj.Modules {
		for _, p := range m.Procedures {
			code, err := d.DisassembleProcedure(p)
			if err != nil {
				continue
			}
			if strings.Contains(code, "For  =") {
				t.Errorf("%s.%s produced malformed 'For  =':\n%s", m.Name, p.Name, code)
			}
		}
	}
}

func TestRunicExpression(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, m := range proj.Modules {
		if m.Name == "frm8" {
			for _, p := range m.Procedures {
				if p.Name == "control16_Timer" {
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					for _, line := range strings.Split(code, "\n") {
						if strings.Contains(line, "Or (l0344 = 89)") {
							t.Logf("Runic If condition line:\n%s", line)
							expected := "If (l0344 = 0) Or (l0344 = 1) Or (l0344 = 4) Or (l0344 = 8)"
							if !strings.Contains(line, expected) {
								t.Errorf("Expected line to contain %q, got:\n%s", expected, line)
							}
							if strings.Contains(line, "((((") {
								t.Errorf("Line still contains excessive nested parens:\n%s", line)
							}
							return
						}
					}
					return
				}
			}
		}
	}
	t.Fatal("control16_Timer not found")
}

func TestFn0093Disasm(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)
	tbl := pcode.GetOpcodeTable()
	i1, i2, i3 := tbl.LookupControl(0x2D21)
	t.Logf("LookupControl(0x2D21): iToken=%d, iToken1=%d, iToken2=%d, dataTypes[i3]=%s", i1, i2, i3, pcode.DataTypes[uint16(i3)])

	for _, m := range proj.Modules {
		if m.Name == "Module1" {
			for _, p := range m.Procedures {
				if p.Name == "fn0093" {
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					t.Logf("fn0093 code:\n%s", code)
				}
			}
		}
	}
}

func TestRndDecompilation(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, m := range proj.Modules {
		if m.Name == "Module1" {
			for _, p := range m.Procedures {
				if p.Name == "fn0102" {
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					if !strings.Contains(code, "Dim l0164 As Integer") {
						t.Errorf("fn0102 missing 'Dim l0164 As Integer':\n%s", code)
					}
					if strings.Contains(code, "Dim l0164 As Long") {
						t.Errorf("fn0102 should not contain 'Dim l0164 As Long':\n%s", code)
					}
					if !strings.Contains(code, "l015C$ = l015C$ + fn007D(l0164)") {
						t.Errorf("fn0102 missing 'l015C$ = l015C$ + fn007D(l0164)':\n%s", code)
					}
					expected := "l015C$ = l015C$ + Chr$(Int(Rnd * 4) + 32)"
					if !strings.Contains(code, expected) {
						t.Errorf("fn0102 missing expected line %q:\n%s", expected, code)
					}
					if strings.Contains(code, "Rnd(") {
						t.Errorf("fn0102 should not contain 'Rnd(':\n%s", code)
					}
					if strings.Contains(code, "0 + Chr$") {
						t.Errorf("fn0102 should not contain '0 + Chr$':\n%s", code)
					}
					return
				}
			}
		}
	}
	t.Fatal("fn0102 not found in Module1")
}

func TestFrm1Control1Click(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, m := range proj.Modules {
		if m.Name == "frm1" {
			for _, p := range m.Procedures {
				if p.Name == "control1_Click" {
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					expected := "If l008E = 6 Then End"
					if !strings.Contains(code, expected) {
						t.Errorf("control1_Click missing expected line %q:\n%s", expected, code)
					}
					if strings.Contains(code, "If l008E = 6 Then\n") {
						t.Errorf("control1_Click contains unclosed Block If:\n%s", code)
					}
					return
				}
			}
		}
	}
	t.Fatal("control1_Click not found in frm1")
}

func TestFrm1Control19Click(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, m := range proj.Modules {
		if m.Name == "frm1" {
			for _, p := range m.Procedures {
				if p.Name == "control19_Click" {
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					if !strings.Contains(code, "Dim l00D6 As Integer") {
						t.Errorf("control19_Click missing 'Dim l00D6 As Integer':\n%s", code)
					}
					if !strings.Contains(code, "l00D8$ = fn0102(l00D6)") {
						t.Errorf("control19_Click missing 'l00D8$ = fn0102(l00D6)':\n%s", code)
					}
					return
				}
			}
		}
	}
	t.Fatal("control19_Click not found in frm1")
}

func TestFrm1Control33Timer(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, m := range proj.Modules {
		if m.Name == "frm8" {
			for _, p := range m.Procedures {
				if p.Name == "control41_Click" {
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					if !strings.Contains(code, "If control41.Caption = \"Time Limit: On\" Then") {
						t.Errorf("control41_Click missing Block If:\n%s", code)
					}
					if !strings.Contains(code, "Else\n") {
						t.Errorf("control41_Click missing Block Else:\n%s", code)
					}
					if !strings.Contains(code, "End If") {
						t.Errorf("control41_Click missing Block End If:\n%s", code)
					}
				}
			}
		}
		if m.Name == "frm1" {
			for _, p := range m.Procedures {
				if p.Name == "control33_Timer" {
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					expected := "If l014A = 1 Then control21.Visible = True Else control21.Visible = False"
					if !strings.Contains(code, expected) {
						t.Errorf("control33_Timer missing %q:\n%s", expected, code)
					}
					if strings.Contains(code, "\n    Else\n") {
						t.Errorf("control33_Timer contains orphaned Else:\n%s", code)
					}
					return
				}
			}
		}
	}
	t.Fatal("control33_Timer not found in frm1")
}

func TestFrm1Control4Click(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, m := range proj.Modules {
		if m.Name == "frm1" {
			for _, p := range m.Procedures {
				if p.Name == "control4_Click" {
					tbl := pcode.GetOpcodeTable()
					for pc := 0; pc < len(p.Bytecode); {
						startPC := pc
						tok := binary.LittleEndian.Uint16(p.Bytecode[pc : pc+2])
						pc += 2
						info, altToken := tbl.Lookup(tok)
						if info.Case == 8 {
							totLen := int(binary.LittleEndian.Uint16(p.Bytecode[pc : pc+2]))
							pc += 4
							strLen := int(binary.LittleEndian.Uint16(p.Bytecode[pc : pc+2]))
							pc += 2 + strLen
							rem := totLen - strLen - 4
							if rem > 0 {
								pc += rem
							}
							continue
						}
						params := make([]uint16, info.NumParams)
						for i := 0; i < info.NumParams && pc+2 <= len(p.Bytecode); i++ {
							params[i] = binary.LittleEndian.Uint16(p.Bytecode[pc : pc+2])
							pc += 2
						}
						t.Logf("[%04X] tok=0x%04X alt=0x%04X kw=%q case=%d params=%v",
							startPC, tok, altToken, info.Keyword, info.Case, params)
					}
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					if strings.Contains(code, "frm1 =") || strings.Contains(code, "gv005E(frm1)") {
						t.Errorf("control4_Click incorrectly contains frm1 variable reference:\n%s", code)
					}
					if !strings.Contains(code, "l001C = l001A") || !strings.Contains(code, "gv005E(l001C)") {
						t.Errorf("control4_Click expected to contain l001C variable reference:\n%s", code)
					}
					return
				}
			}
		}
	}
	t.Fatal("control4_Click not found in frm1")
}

func TestFrm2Control4Click(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, m := range proj.Modules {
		if m.Name == "frm2" {
			for _, p := range m.Procedures {
				if p.Name == "control4_Click" {
					tbl := pcode.GetOpcodeTable()
					for pc := 0; pc < len(p.Bytecode); {
						startPC := pc
						tok := binary.LittleEndian.Uint16(p.Bytecode[pc : pc+2])
						pc += 2
						info, altToken := tbl.Lookup(tok)
						if info.Case == 8 {
							totLen := int(binary.LittleEndian.Uint16(p.Bytecode[pc : pc+2]))
							pc += 4
							strLen := int(binary.LittleEndian.Uint16(p.Bytecode[pc : pc+2]))
							strBytes := p.Bytecode[pc : pc+strLen]
							pc += 2 + strLen
							rem := totLen - strLen - 4
							if rem > 0 {
								pc += rem
							}
							t.Logf("[%04X] LITERAL %q", startPC, string(strBytes))
							continue
						}
						params := make([]uint16, info.NumParams)
						for i := 0; i < info.NumParams && pc+2 <= len(p.Bytecode); i++ {
							params[i] = binary.LittleEndian.Uint16(p.Bytecode[pc : pc+2])
							pc += 2
						}
						if startPC < 0x00A0 {
							t.Logf("[%04X] tok=0x%04X alt=0x%04X kw=%q case=%d params=%v",
								startPC, tok, altToken, info.Keyword, info.Case, params)
						}
					}
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					expectedLine := `l0058 = fn0093(fn018F() + " Confirmation Ended " + fn00BE())`
					if !strings.Contains(code, expectedLine) {
						t.Errorf("frm2 control4_Click missing expected line %q:\n%s", expectedLine, code)
					}
					if strings.Contains(code, "0 + fn00BE") {
						t.Errorf("frm2 control4_Click should not contain '0 + fn00BE':\n%s", code)
					}
					return
				}
			}
		}
	}
	t.Fatal("control4_Click not found in frm2")
}

func TestFrm2Control58Timer(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	for _, m := range proj.Modules {
		if m.Name == "frm2" {
			for _, p := range m.Procedures {
				if p.Name == "control58_Timer" {
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					if !strings.Contains(code, "control59.Text = fn00E6()") {
						t.Errorf("control58_Timer missing expected call 'control59.Text = fn00E6()':\n%s", code)
					}
					if !strings.Contains(code, "l01F4$ = fn00F5(control59.Text, 1)") {
						t.Errorf("control58_Timer missing expected call 'l01F4$ = fn00F5(control59.Text, 1)':\n%s", code)
					}
					return
				}
			}
		}
	}
	t.Fatal("control58_Timer not found in frm2")
}

func TestByValParameters(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	expectedHeaders := map[string]string{
		"fn008B": "Function fn008B (ByVal p00C0 As String) As Variant",
		"fn0093": "Function fn0093 (ByVal p00C8 As Variant) As Variant",
		"fn00F5": "Function fn00F5 (ByVal p0142 As String, p0144 As Variant) As Variant",
		"fn08BB": "Function fn08BB (ByVal p00C2 As String) As Variant",
		"fn08CF": "Function fn08CF (ByVal p00C8 As String) As Variant",
		"fn006F": "Function fn006F (p00A4 As String) As Integer",
		"fn007D": "Function fn007D (p00B0 As Integer) As String",
		"fn0102": "Function fn0102 (p015A As Integer) As Variant",
	}

	for _, m := range proj.Modules {
		for _, p := range m.Procedures {
			if expected, ok := expectedHeaders[p.Name]; ok {
				code, err := d.DisassembleProcedure(p)
				if err != nil {
					t.Fatalf("DisassembleProcedure(%s) failed: %v", p.Name, err)
				}
				lines := strings.Split(code, "\n")
				if len(lines) == 0 || lines[0] != expected {
					t.Errorf("Procedure %s header mismatch:\nExpected: %q\nGot:      %q", p.Name, expected, lines[0])
				}
			}
		}
	}
}

func TestUnreferencedAndEventParameters(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	expectedHeaders := map[string]map[string]string{
		"frm2": {
			"fn05DB":      "Function fn05DB (p00CE As String) As Variant",
			"Form_Unload": "Sub Form_Unload (Cancel As Integer)",
		},
		"frm4": {
			"Form_Unload": "Sub Form_Unload (Cancel As Integer)",
		},
		"frm6": {
			"Form_Unload": "Sub Form_Unload (Cancel As Integer)",
		},
		"frm8": {
			"Form_Unload": "Sub Form_Unload (Cancel As Integer)",
		},
	}

	for _, m := range proj.Modules {
		if expectedProcs, ok := expectedHeaders[m.Name]; ok {
			for _, p := range m.Procedures {
				if expectedHeader, ok := expectedProcs[p.Name]; ok {
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure(%s.%s) failed: %v", m.Name, p.Name, err)
					}
					lines := strings.Split(code, "\n")
					if len(lines) == 0 || lines[0] != expectedHeader {
						t.Errorf("Procedure %s.%s header mismatch:\nExpected: %q\nGot:      %q", m.Name, p.Name, expectedHeader, lines[0])
					}
				}
			}
		}
	}
}

func TestSub0CCFVariableTyping(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)
	for _, m := range proj.Modules {
		if m.Name == "frm8" {
			for _, p := range m.Procedures {
				if p.Name == "sub0CCF" {
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure failed: %v", err)
					}
					expectedSnippets := []string{
						"Dim l0328 As Integer",
						"Dim l032A As Integer",
						"Dim l032E As Integer",
						"Dim l0330 As Integer",
						"Dim l0332 As Integer",
						"gv0006(l032A, 2) = fn0102(l0328)",
					}
					for _, snip := range expectedSnippets {
						if !strings.Contains(code, snip) {
							t.Errorf("sub0CCF missing expected snippet %q:\n%s", snip, code)
						}
					}
				}
			}
		}
	}
}

func TestFormAndControlEventMapping(t *testing.T) {
	_, proj := getTestProject(t)
	d := pcode.NewDisassembler(proj)

	var frm1, frm8 *pcode.Module
	for _, m := range proj.Modules {
		if m.Name == "frm1" {
			frm1 = m
		} else if m.Name == "frm8" {
			frm8 = m
		}
	}

	if frm1 == nil {
		t.Fatal("frm1 module not found")
	}
	if frm8 == nil {
		t.Fatal("frm8 module not found")
	}

	// 1. Verify frm1 Form_Click prompts for battle, and Form_Paint does NOT prompt for battle
	var frm1FormClick, frm1FormPaint, frm1Timer *pcode.Procedure
	for _, p := range frm1.Procedures {
		switch p.Name {
		case "Form_Click":
			frm1FormClick = p
		case "Form_Paint":
			frm1FormPaint = p
		case "control33_Timer":
			frm1Timer = p
		}
	}

	if frm1FormClick == nil {
		t.Fatalf("Form_Click procedure not found in frm1")
	}
	clickCode, err := d.DisassembleProcedure(frm1FormClick)
	if err != nil {
		t.Fatalf("DisassembleProcedure(Form_Click) failed: %v", err)
	}
	if !strings.Contains(clickCode, "Do you wish to enter battle?") {
		t.Errorf("Form_Click expected to contain battle prompt, got:\n%s", clickCode)
	}

	if frm1FormPaint != nil {
		paintCode, err := d.DisassembleProcedure(frm1FormPaint)
		if err != nil {
			t.Fatalf("DisassembleProcedure(Form_Paint) failed: %v", err)
		}
		if strings.Contains(paintCode, "Do you wish to enter battle?") {
			t.Errorf("Form_Paint should NOT contain battle prompt, got:\n%s", paintCode)
		}
	}

	// 2. Verify frm1 control33_Timer exists and contains Static l014A As Integer
	if frm1Timer == nil {
		t.Fatalf("control33_Timer procedure not found in frm1")
	}
	timerCode, err := d.DisassembleProcedure(frm1Timer)
	if err != nil {
		t.Fatalf("DisassembleProcedure(control33_Timer) failed: %v", err)
	}
	if !strings.Contains(timerCode, "Static l014A As Integer") {
		t.Errorf("control33_Timer expected 'Static l014A As Integer', got:\n%s", timerCode)
	}

	// 3. Verify frm8 Form_DblClick exists
	foundDblClick := false
	for _, p := range frm8.Procedures {
		if p.Name == "Form_DblClick" {
			foundDblClick = true
			break
		}
	}
	if !foundDblClick {
		t.Errorf("Form_DblClick procedure not found in frm8")
	}
}
