package frm_test

import (
	"testing"

	"vb3dec/internal/fixture"
	"vb3dec/pkg/frm"
	"vb3dec/pkg/ne"
)

// openBascode opens BasCode for Windows 1.2, an unprotected VB3 executable that
// uses THREED.VBX and CMDIALOG.VBX.
func openBascode(t *testing.T) *ne.File {
	t.Helper()
	path := fixture.Path(t, fixture.Bascode, "BASCODE.EXE")
	f, err := ne.Open(path)
	if err != nil {
		t.Fatalf("Open %s: %v", path, err)
	}
	return f
}

func TestBascodeProjectNames(t *testing.T) {
	f := openBascode(t)
	proj, err := frm.ParseVBProject(f)
	if err != nil {
		t.Fatalf("ParseVBProject: %v", err)
	}
	if proj.VBGuard {
		t.Error("BASCODE.EXE is not VBGuard-protected")
	}
	want := []struct {
		file, name string
		module     uint16
	}{
		{"BASCODE4.FRM", "Form4", 0x0990},
		{"BIG.FRM", "big", 0x0AB0},
		{"BASCODE2.FRM", "Form2", 0x0FC0},
		{"RETREIVE.FRM", "retreive", 0x10E0},
		{"ST_INF.FRM", "st_inf", 0x12A8},
		{"BASCODE3.FRM", "bascode3", 0x1438}, // empty name table
	}
	if len(proj.Forms) != len(want) {
		t.Fatalf("expected %d forms, got %d", len(want), len(proj.Forms))
	}
	for i, w := range want {
		got := proj.Forms[i]
		if got.FileName != w.file || got.FormName != w.name || got.ModuleID != w.module {
			t.Errorf("form %d = {%s %s %04X}, want {%s %s %04X}", i+1, got.FileName, got.FormName, got.ModuleID, w.file, w.name, w.module)
		}
	}
	if got := proj.Forms[2].ControlNames[1]; got != "Frame3D1" {
		t.Errorf("Form2 control 1 name = %q, want Frame3D1", got)
	}
}

func TestBascodeExtractForms(t *testing.T) {
	f := openBascode(t)
	forms, proj, err := frm.ExtractForms(f, frm.ExtractOptions{VBXDirs: []string{fixture.Dir(t, fixture.Bascode)}})
	if err != nil {
		t.Fatalf("ExtractForms: %v", err)
	}
	if len(proj.Warnings) != 0 {
		t.Errorf("unexpected project warnings: %q", proj.Warnings)
	}
	total := 0
	for _, ef := range forms {
		total += len(ef.Form.Controls)
		if len(ef.Form.Warnings) != 0 {
			t.Errorf("%s: unexpected warnings: %q", ef.Ref.FileName, ef.Form.Warnings)
		}
	}
	if total != 106 {
		t.Errorf("expected 106 controls, got %d", total)
	}

	// Form2 (BASCODE2.FRM): two 3D frames, each containing its own controls.
	form2 := forms[2].Form
	if len(form2.Root.Children) != 2 {
		t.Fatalf("expected 2 top-level controls on Form2, got %d", len(form2.Root.Children))
	}
	frame := form2.Root.Children[0]
	if frame.TypeName != "SSFrame" || frame.Name != "Frame3D1" || len(frame.Children) != 4 {
		t.Errorf("Form2 first control = %s %s with %d children, want SSFrame Frame3D1 with 4", frame.TypeName, frame.Name, len(frame.Children))
	}
	props := propertyMap(form2.Root.Children[1].Children[0])
	if props["Caption"] != `"Create Library."` || props["FontSize"] != "9.75" || props["TabIndex"] != "2" {
		t.Errorf("unexpected Command1 properties: %v", props)
	}
}
