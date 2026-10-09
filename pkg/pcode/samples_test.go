package pcode_test

import (
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
