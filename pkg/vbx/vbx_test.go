package vbx_test

import (
	"testing"

	"vb3dec/internal/fixture"
	"vb3dec/pkg/vbx"
)

func propIndex(m *vbx.Model, name string) int {
	for i, p := range m.Props {
		if p.Name == name {
			return i
		}
	}
	return -1
}

func TestStdPropsOrder(t *testing.T) {
	if len(vbx.StdProps) != 43 {
		t.Fatalf("expected 43 standard properties, got %d", len(vbx.StdProps))
	}
	if p := vbx.StdProps[vbx.StdLeft]; p.Name != "Left" || p.Type() != vbx.DTXPos {
		t.Errorf("StdProps[StdLeft] = %+v", p)
	}
	if p := vbx.StdProps[vbx.StdFontName]; p.Name != "FontName" || p.Type() != vbx.DTHSZ {
		t.Errorf("StdProps[StdFontName] = %+v", p)
	}
}

func TestBuiltinCommandButtonPropertyIDs(t *testing.T) {
	m := vbx.BuiltinModels["Command"]
	if m == nil {
		t.Fatal("missing Command model")
	}
	// Property IDs observed in compiled forms of FF.EXE and BASCODE.EXE.
	want := map[string]int{"Caption": 0x00, "Left": 0x04, "Enabled": 0x08, "Visible": 0x09, "FontName": 0x0B, "TabIndex": 0x11}
	for name, id := range want {
		if got := propIndex(m, name); got != id {
			t.Errorf("Command.%s at 0x%02X, want 0x%02X", name, got, id)
		}
	}
}

func TestParseFileMCI(t *testing.T) {
	path := fixture.Path(t, fixture.FF, "MCI.VBX")
	models, err := vbx.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(models) != 1 || models[0].ClassName != "MMControl" {
		t.Fatalf("expected the MMControl model, got %+v", models)
	}
	m := models[0]
	if got := propIndex(m, "Visible"); got != 0x0E {
		t.Errorf("MMControl.Visible at 0x%02X, want 0x0E", got)
	}
	if p := m.Props[propIndex(m, "Left")]; p.Std != vbx.StdLeft {
		t.Errorf("MMControl.Left should be the standard Left property, got %+v", p)
	}
}
