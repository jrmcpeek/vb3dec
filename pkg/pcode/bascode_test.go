package pcode_test

import (
	"strings"
	"testing"

	"vb3dec/internal/fixture"
	"vb3dec/pkg/frm"
	"vb3dec/pkg/ne"
	"vb3dec/pkg/pcode"
)

func parseBascode(t *testing.T) *pcode.Project {
	t.Helper()
	path := fixture.Path(t, fixture.Bascode, "BASCODE.EXE")
	f, err := ne.Open(path)
	if err != nil {
		t.Fatalf("Open %s: %v", path, err)
	}
	proj, err := pcode.ParseProject(f, frm.ExtractOptions{VBXDirs: []string{fixture.Dir(t, fixture.Bascode)}})
	if err != nil {
		t.Fatalf("ParseProject: %v", err)
	}
	return proj
}

func TestBascodeModules(t *testing.T) {
	proj := parseBascode(t)
	want := []struct {
		name   string
		isForm bool
		file   string
	}{
		{"Module1", false, ""},
		{"Module2", false, ""},
		{"Form4", true, "BASCODE4.FRM"},
		{"big", true, "BIG.FRM"},
		{"Form2", true, "BASCODE2.FRM"},
		{"retreive", true, "RETREIVE.FRM"},
		{"st_inf", true, "ST_INF.FRM"},
		{"bascode3", true, "BASCODE3.FRM"},
	}
	if len(proj.Modules) != len(want) {
		t.Fatalf("expected %d modules, got %d", len(want), len(proj.Modules))
	}
	for i, w := range want {
		m := proj.Modules[i]
		if m.Name != w.name || m.IsForm != w.isForm || m.FileName != w.file {
			t.Errorf("module %d = {%s %v %s}, want {%s %v %s}", i+1, m.Name, m.IsForm, m.FileName, w.name, w.isForm, w.file)
		}
	}
}

func TestBascodeFormCode(t *testing.T) {
	proj := parseBascode(t)
	d := pcode.NewDisassembler(proj)
	var form2, module2 *pcode.Module
	for _, m := range proj.Modules {
		switch m.Name {
		case "Form2":
			form2 = m
		case "Module2":
			module2 = m
		}
	}

	code, err := d.DisassembleModule(form2)
	if err != nil {
		t.Fatalf("DisassembleModule(Form2): %v", err)
	}
	for _, want := range []string{
		"Sub Command1_Click ()",
		"Sub Command2_Click ()",
		"Sub Form_Unload (Cancel As Integer)",
		"If InStr(Form2.Text1.Text, \" \") <> 0 Then",
		"big.Enabled = True",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("Form2 code is missing %q", want)
		}
	}

	// Module2 indexes a module-level array whose descriptor word coincides
	// with an API declaration's pointer; it must not decompile as a call.
	code, err = d.DisassembleModule(module2)
	if err != nil {
		t.Fatalf("DisassembleModule(Module2): %v", err)
	}
	if strings.Contains(code, "extfn") {
		t.Errorf("Module2 code references an external declaration it never calls")
	}
	if !strings.Contains(code, "m0018(l0054, 1)") {
		t.Errorf("Module2 code is missing the array access m0018(l0054, 1)")
	}
}
