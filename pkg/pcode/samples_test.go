package pcode_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"vb3dec/internal/fixture"
	"vb3dec/pkg/frm"
	"vb3dec/pkg/ne"
	"vb3dec/pkg/pcode"
)

func parseSample(t *testing.T, sample, exe string) *pcode.Project {
	t.Helper()
	f, err := ne.Open(fixture.Path(t, sample, exe))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	proj, err := pcode.ParseProject(f, frm.ExtractOptions{VBXDirs: []string{fixture.Dir(t, sample)}})
	if err != nil {
		t.Fatalf("ParseProject: %v", err)
	}
	return proj
}

var globalName = regexp.MustCompile(`\bgv[0-9A-F]{4}\b`)

// TestSampleGlobalsAreDeclared requires every global variable referenced by
// decompiled code to be declared in the project globals.
func TestSampleGlobalsAreDeclared(t *testing.T) {
	samples := []struct{ dir, exe string }{
		{fixture.FF, "FF.EXE"},
		{fixture.Bascode, "BASCODE.EXE"},
		{fixture.Empire, "EMPIRE.EXE"},
	}
	for _, s := range samples {
		t.Run(s.dir, func(t *testing.T) {
			proj := parseSample(t, s.dir, s.exe)
			declared := make(map[string]bool)
			for _, g := range proj.GlobalVars {
				declared[globalName.FindString(g)] = true
			}
			d := pcode.NewDisassembler(proj)
			for _, m := range proj.Modules {
				code, err := d.DisassembleModule(m)
				if err != nil {
					t.Fatalf("DisassembleModule(%s): %v", m.Name, err)
				}
				for _, name := range globalName.FindAllString(code, -1) {
					if !declared[name] {
						t.Errorf("%s references undeclared global %s", m.Name, name)
						declared[name] = true
					}
				}
			}
		})
	}
}

func TestEmpireDeclarationModule(t *testing.T) {
	// EMPIRE.EXE's first code module holds only declarations; its module data
	// block must not be assigned to the next module.
	proj := parseSample(t, fixture.Empire, "EMPIRE.EXE")
	if len(proj.Modules) != 13 {
		t.Fatalf("expected 13 modules, got %d", len(proj.Modules))
	}
	decls := proj.Modules[0]
	if decls.IsForm || len(decls.Procedures) != 13 {
		t.Fatalf("Module1 = form %v with %d procedures, want a code module with 13 declarations", decls.IsForm, len(decls.Procedures))
	}
	code, err := pcode.NewDisassembler(proj).DisassembleModule(decls)
	if err != nil {
		t.Fatalf("DisassembleModule: %v", err)
	}
	for _, want := range []string{
		`Declare Function extfn00A1 Lib "GDI" Alias "BitBlt" (ByVal p1%, ByVal p2%, ByVal p3%, ByVal p4%, ByVal p5%, ByVal p6%, ByVal p7%, ByVal p8%, ByVal p9&) As Integer`,
		`Declare Function extfn00D9 Lib "c:\mwin\system\MMsystem" Alias "mciGetErrorString" (ByVal p1&, ByVal p2$, ByVal p3%) As Integer`,
	} {
		if !strings.Contains(code, want) {
			t.Errorf("Module1 is missing %q", want)
		}
	}
}

// TestSampleEventParameters requires every event procedure's parameters to
// match the parameter count recorded in its procedure descriptor.
func TestSampleEventParameters(t *testing.T) {
	samples := []struct {
		dir, exe string
		events   int
	}{
		{fixture.FF, "FF.EXE", 131},
		{fixture.Bascode, "BASCODE.EXE", 43},
		{fixture.Empire, "EMPIRE.EXE", 165},
	}
	for _, s := range samples {
		t.Run(s.dir, func(t *testing.T) {
			proj := parseSample(t, s.dir, s.exe)
			events := 0
			for _, p := range proj.Procedures {
				if !p.IsEvent {
					continue
				}
				events++
				if declared := int(p.PubOrPriv >> 9); declared != len(p.EventParams) {
					t.Errorf("%s declares %d parameters but has %v", p.Name, declared, p.EventParams)
				}
			}
			if events != s.events {
				t.Errorf("got %d event procedures, want %d", events, s.events)
			}
		})
	}
}

func TestSampleEventNames(t *testing.T) {
	want := []struct {
		dir, exe, module, signature string
	}{
		{fixture.FF, "FF.EXE", "frm8", "Sub control39_KeyDown (KeyCode As Integer, Shift As Integer)"},
		{fixture.FF, "FF.EXE", "frm8", "Sub control32_Done (NotifyCode As Integer)"}, // MCI.VBX event
		{fixture.Empire, "EMPIRE.EXE", "CountMap", "Sub Box_Attackables_KeyPress (Index As Integer, KeyAscii As Integer)"},
	}
	for _, w := range want {
		proj := parseSample(t, w.dir, w.exe)
		var code string
		for _, m := range proj.Modules {
			if m.Name == w.module {
				var err error
				if code, err = pcode.NewDisassembler(proj).DisassembleModule(m); err != nil {
					t.Fatalf("DisassembleModule(%s): %v", m.Name, err)
				}
			}
		}
		if !strings.Contains(code, w.signature) {
			t.Errorf("%s %s is missing %q", w.dir, w.module, w.signature)
		}
	}
}

var (
	labelRef = regexp.MustCompile(`\b(?:GoTo|GoSub|Resume) (L[0-9A-F]{4})\b`)
	labelDef = regexp.MustCompile(`(?m)^(L[0-9A-F]{4}):\r?$`)
)

// TestSampleJumpTargetsHaveLabels requires every GoTo, GoSub, On Error GoTo
// and Resume target to be defined as a line label, starting in the first column, in the same procedure.
func TestSampleJumpTargetsHaveLabels(t *testing.T) {
	samples := []struct{ dir, exe string }{
		{fixture.FF, "FF.EXE"},
		{fixture.Bascode, "BASCODE.EXE"},
		{fixture.Empire, "EMPIRE.EXE"},
	}
	for _, s := range samples {
		t.Run(s.dir, func(t *testing.T) {
			proj := parseSample(t, s.dir, s.exe)
			d := pcode.NewDisassembler(proj)
			refs := 0
			for _, m := range proj.Modules {
				for _, p := range m.Procedures {
					if !p.IsLocal {
						continue
					}
					code, err := d.DisassembleProcedure(p)
					if err != nil {
						t.Fatalf("DisassembleProcedure(%s.%s): %v", m.Name, p.Name, err)
					}
					defined := make(map[string]bool)
					for _, def := range labelDef.FindAllStringSubmatch(code, -1) {
						defined[def[1]] = true
					}
					for _, ref := range labelRef.FindAllStringSubmatch(code, -1) {
						refs++
						if !defined[ref[1]] {
							t.Errorf("%s.%s jumps to undefined label %s", m.Name, p.Name, ref[1])
						}
					}
				}
			}
			t.Logf("%d label references checked", refs)
		})
	}
}

// TestSamplePropertyNames checks that property references in code are named
// from the model of the object they apply to: built-in and custom controls,
// control array elements, and global objects.
func TestSamplePropertyNames(t *testing.T) {
	samples := []struct {
		dir, exe string
		want     []string
	}{
		{fixture.FF, "FF.EXE", nil},
		{fixture.Bascode, "BASCODE.EXE", []string{
			"Screen.MousePointer = 11",
			"Printer.NewPage",
			`big.CMDialog1.Filter = "All Files. (*.*)|*.*|BAS Files. (*.bas)|*.bas|"`, // CMDIALOG.VBX
			"big.Option3D2.Value = True", // THREED.VBX
			"l0106 = p00F4.Height",       // parameter that receives a form
		}},
		{fixture.Empire, "EMPIRE.EXE", []string{
			"gv01F2$ = App.Path",
			`Printer.FontName = "Arial"`,
			`((Screen.Width / Screen.TwipsPerPixelX) - 636) \ 2`,
			"CountMap.CountryButton(l021A).Left = l021C - 90",
		}},
	}
	for _, s := range samples {
		t.Run(s.dir, func(t *testing.T) {
			proj := parseSample(t, s.dir, s.exe)
			d := pcode.NewDisassembler(proj)
			var all strings.Builder
			for _, m := range proj.Modules {
				code, err := d.DisassembleModule(m)
				if err != nil {
					t.Fatalf("DisassembleModule(%s): %v", m.Name, err)
				}
				all.WriteString(code)
			}
			code := all.String()
			for _, w := range s.want {
				if !strings.Contains(code, w) {
					t.Errorf("decompiled code lacks %q", w)
				}
			}
			// Property IDs 0xC000+n index a model's property list; a
			// placeholder means the referenced object's model was not found.
			if ids := regexp.MustCompile(`\.Prop_C[0-9A-F]{3}\b`).FindAllString(code, -1); len(ids) > 0 {
				t.Errorf("unresolved property names: %v", ids)
			}
		})
	}
}

// TestSampleCallStatementArguments checks parameter declarations that are
// inferred from the arguments of Call statements.
func TestSampleCallStatementArguments(t *testing.T) {
	samples := []struct {
		dir, exe string
		want     []string
	}{
		{fixture.Bascode, "BASCODE.EXE", []string{
			"Sub sub00F4 (p00F4 As Form)",   // Call sub00F4(Form2) and others
			"Sub sub01F0 (p0016 As String)", // Call sub01F0(l001E$)
		}},
		{fixture.Empire, "EMPIRE.EXE", []string{
			`Declare Sub extsub0139 Lib "user" Alias "MoveWindow" (ByVal p1%, ByVal p2%, ByVal p3%, ByVal p4%, ByVal p5%, ByVal p6%)`,
			`Declare Sub extsub0074 Lib "MMSystem" Alias "SndPlaySound" (ByVal p1$, ByVal p2%)`,
		}},
	}
	for _, s := range samples {
		t.Run(s.dir, func(t *testing.T) {
			proj := parseSample(t, s.dir, s.exe)
			d := pcode.NewDisassembler(proj)
			var all strings.Builder
			for _, m := range proj.Modules {
				code, err := d.DisassembleModule(m)
				if err != nil {
					t.Fatalf("DisassembleModule(%s): %v", m.Name, err)
				}
				all.WriteString(code)
			}
			for _, w := range s.want {
				if !strings.Contains(all.String(), w) {
					t.Errorf("decompiled code lacks %q", w)
				}
			}
		})
	}
}

// TestSampleCodeEncodesAsWindows1252 requires the decompiled code of every
// sample to be representable in Windows-1252, the encoding the VB3 IDE reads,
// and FF.EXE's chat banners to come out as their original ANSI bytes.
func TestSampleCodeEncodesAsWindows1252(t *testing.T) {
	samples := []struct{ dir, exe string }{
		{fixture.FF, "FF.EXE"},
		{fixture.Bascode, "BASCODE.EXE"},
		{fixture.Empire, "EMPIRE.EXE"},
	}
	banner := []byte{0xAB, 0x8B, 0xAB, 0x97, 0x97, '<'} // «‹«——<
	for _, s := range samples {
		t.Run(s.dir, func(t *testing.T) {
			proj := parseSample(t, s.dir, s.exe)
			d := pcode.NewDisassembler(proj)
			foundBanner := false
			for _, m := range proj.Modules {
				code, err := d.DisassembleModule(m)
				if err != nil {
					t.Fatalf("DisassembleModule(%s): %v", m.Name, err)
				}
				data, err := frm.EncodeText(code, frm.EncodingWindows1252)
				if err != nil {
					t.Errorf("EncodeText(%s): %v", m.Name, err)
					continue
				}
				if bytes.Contains(data, banner) {
					foundBanner = true
				}
			}
			if s.dir == fixture.FF && !foundBanner {
				t.Errorf("no module contains the Windows-1252 banner bytes % X", banner)
			}
		})
	}
}

// unhandledBudget is the number of P-code tokens the decompiler cannot decode
// yet in each sample's modules. Decoding more of them lowers it; lower the
// budget with them so that it can never grow back.
var unhandledBudget = []struct {
	dir, exe string
	max      int
}{
	{fixture.FF, "FF.EXE", 0},
	{fixture.Bascode, "BASCODE.EXE", 88},
	{fixture.Empire, "EMPIRE.EXE", 631},
}

// TestSampleUnhandledOpcodeBudget keeps the number of unhandled tokens of
// each sample within its budget and requires each of them to be marked in
// the decompiled code.
func TestSampleUnhandledOpcodeBudget(t *testing.T) {
	for _, s := range unhandledBudget {
		t.Run(s.dir, func(t *testing.T) {
			proj := parseSample(t, s.dir, s.exe)
			d := pcode.NewDisassembler(proj)
			markers := 0
			for _, m := range proj.Modules {
				code, err := d.DisassembleModule(m)
				if err != nil {
					t.Fatalf("DisassembleModule(%s): %v", m.Name, err)
				}
				for _, line := range strings.Split(code, "\n") {
					if i := strings.Index(line, pcode.UnhandledMarker); i >= 0 {
						markers += strings.Count(line[i:], "(token ")
					}
				}
			}
			got := len(d.Warnings)
			if got > s.max {
				t.Errorf("%d unhandled tokens, want at most %d", got, s.max)
			} else if got < s.max {
				t.Logf("%d unhandled tokens, under the budget of %d: lower the budget", got, s.max)
			}
			if markers != got {
				t.Errorf("%d unhandled tokens are marked in the code, want %d", markers, got)
			}
		})
	}
}

// TestSampleDecodedTokens checks tokens whose omission left their operands
// behind, which corrupted the surrounding statements.
func TestSampleDecodedTokens(t *testing.T) {
	samples := []struct {
		dir, exe string
		want     []string // Lines or statements expected in the decompiled modules
	}{
		{fixture.Bascode, "BASCODE.EXE", []string{
			"fn0123 = (p0116 And Not &HFFFF&) / &H10000& And &HFFFF&", // Not
			"While Not EOF(1)",
			"ChDir gv0162$",
			"ChDrive Left$(gv0162$, 2)",
			`FileCopy "0342basc.tmp", p0100$`,
			"Kill p0100$",
			"Reset",
			"Erase gv01A8", // Whole array
			"If Len(Form2.Text1.Text) > 8 Then Let l0024$ = Left$(Form2.Text1.Text, 8) Else l0024$ = Form2.Text1.Text",
			"Let gv0166$ = big.CMDialog1.Filename",
			`If gv0166$ = "" Then Error 32755`,
			"On Error GoTo L012E",
			"Resume L0122",
			"Call sub0251",
			"ChDir gv0162$: ChDrive Left$(gv0162$, 2)",
			`Open p002C$ + ".LIB" For Output As #1`,
			"Line Input #1, l0044$",
			"Close #1",
			`If LOF(1) >= (31.5 * 1024) Then l008E = fn014A("Unable To Complete Storage Operation.", 9005, "Your Code Section Exceeds 32Kbytes"): p0088 = -10: GoTo L0104`,
			"Call sub01BC(l0024$, l0026$)",
			"Call sub0272(gv0166$, gv0172(), l0040)",
			`If gv01DE = 1 Then Call sub0294(gv01A8(0, 2) + ".LIB")`,
		}},
		{fixture.Empire, "EMPIRE.EXE", []string{
			"If Not gv0248 Then Exit Sub",
			"CountMap.Box_Attackables(Index).Cls", // Method on a control, not the form
			`l0032 = (Printer.ScaleWidth - Printer.TextWidth("Fax (02)516-4236")) - 0.3`,
			"l02D2 = ContMap.Picture1.Point(l02CA, l02CC)",
			"l010E = FreeFile",
			`Kill "EMPERORS.HOF"`,
			"For l018E = 1 To UBound(l01BE, 1)", // Whole array
			"On Error Resume Next",
			"Call sub0202(l0110(), gv0336(gv039A), l0186)",
			"If gv01EE = 0 Then Call extsub0074(l0022, l0028)", // Declared procedure
			`If gv0552 = 0 Then gv0552 = 1: Button_Blitz.Caption = "On" Else gv0552 = 0: Button_Blitz.Caption = "Off"`,
			"For l00F2 = 1 To p00E6: p00DE(l00F2) = l00E8(l00F2): Next l00F2",
			`Case "4": OptionMoneyOrder.Value = -1`, // Separator after Case
			`Open gv01F2$ + "\" + "tountdat.vbl" For Random As l010E Len = Len(m006A(0))`,
			`Open gv01F2$ + "\c1.wav" For Binary As l010E`,
			"Input #l0070, l0074$",
			"Get l00C6, l00C8 + l00CC, l00CE",
			"Get l010E, , gv01E6$", // No record number
			"Put l009C, l00A0, l0096(l00A0)",
			"Loop: Close l0260",
		}},
	}
	for _, s := range samples {
		t.Run(s.dir, func(t *testing.T) {
			proj := parseSample(t, s.dir, s.exe)
			d := pcode.NewDisassembler(proj)
			var all strings.Builder
			for _, m := range proj.Modules {
				code, err := d.DisassembleModule(m)
				if err != nil {
					t.Fatalf("DisassembleModule(%s): %v", m.Name, err)
				}
				all.WriteString(code)
				all.WriteString("\n")
			}
			lines := make(map[string]bool)
			for _, line := range strings.Split(all.String(), "\n") {
				lines[strings.TrimSpace(line)] = true
				// The statements of a line with ":" separators also match.
				for _, stmt := range strings.Split(line, ": ") {
					lines[strings.TrimSpace(stmt)] = true
				}
			}
			for _, want := range s.want {
				if !lines[want] {
					t.Errorf("missing line %q", want)
				}
			}
			for _, w := range d.Warnings {
				switch w.Keyword {
				case "Not ", "Cls", "TextWidth", "Point", "FreeFile", "Err", "Date$", "CurDir$",
					"ChDir", "ChDrive", "Erase", "Error", "FileCopy", "Kill ", "Let ", "Reset",
					"On Error ", "Resume", "Resume Next", "Call ", ": ", ":",
					"#", "Open ", "Close", "Get", "Put", "Input ", "Line Input ":
					t.Errorf("%s.%s @ 0x%04X: %q is unhandled", w.ModuleName, w.ProcName, w.PC, w.Keyword)
				}
			}
		})
	}
}
