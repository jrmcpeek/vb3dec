package frm_test

import (
	"testing"

	"vb3dec/internal/fixture"
	"vb3dec/pkg/frm"
	"vb3dec/pkg/ne"
)

// TestSampleFormsDecodeCompletely decodes every form of every sample with its
// custom controls available and requires that no property is left undecoded.
func TestSampleFormsDecodeCompletely(t *testing.T) {
	samples := []struct {
		dir      string
		exe      string
		forms    int
		controls int
	}{
		{fixture.FF, "FF.EXE", 9, 226},
		{fixture.Bascode, "BASCODE.EXE", 6, 106},
		{fixture.Empire, "EMPIRE.EXE", 10, 289},
	}
	for _, s := range samples {
		t.Run(s.dir, func(t *testing.T) {
			f, err := ne.Open(fixture.Path(t, s.dir, s.exe))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			forms, proj, err := frm.ExtractForms(f, frm.ExtractOptions{VBXDirs: []string{fixture.Dir(t, s.dir)}})
			if err != nil {
				t.Fatalf("ExtractForms: %v", err)
			}
			for _, w := range proj.Warnings {
				t.Errorf("project warning: %s", w)
			}
			controls := 0
			for _, ef := range forms {
				controls += len(ef.Form.Controls)
				for _, w := range ef.Form.Warnings {
					t.Errorf("%s: %s", ef.Ref.FileName, w)
				}
			}
			if len(forms) != s.forms || controls != s.controls {
				t.Errorf("got %d forms with %d controls, want %d with %d", len(forms), controls, s.forms, s.controls)
			}
		})
	}
}
