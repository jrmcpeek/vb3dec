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
	if len(m.Events) != 40 {
		t.Fatalf("expected 40 MMControl events, got %d", len(m.Events))
	}
	if e := m.Events[4]; e.Name != "Done" || e.Params != 1 || e.Profile != "NotifyCode As Integer" {
		t.Errorf("MMControl event slot 4 = %+v, want Done(NotifyCode As Integer)", e)
	}
}

func TestBuiltinFormEventSlots(t *testing.T) {
	if len(vbx.StdEvents) != 17 || vbx.StdEvents[0].Name != "Click" || vbx.StdEvents[16].Name != "Change" {
		t.Fatalf("unexpected standard events: %+v", vbx.StdEvents)
	}
	events := vbx.BuiltinModels["Form"].Events
	want := map[int]string{6: "Load", 8: "Unload", 9: "QueryUnload", 12: "Click", 19: "MouseDown", 20: "MouseMove", 22: "Paint"}
	for slot, name := range want {
		if slot >= len(events) || events[slot].Name != name {
			t.Errorf("Form event slot %d is not %s", slot, name)
		}
	}
	if e := events[9]; e.Params != 2 {
		t.Errorf("QueryUnload has %d parameters, want 2", e.Params)
	}
}

func TestObjectModelPropertyIDs(t *testing.T) {
	// Property IDs observed in compiled code of BASCODE.EXE and EMPIRE.EXE.
	want := map[string]map[string]int{
		"Screen":  {"Width": 0x02, "Height": 0x03, "MousePointer": 0x08, "TwipsPerPixelX": 0x09, "TwipsPerPixelY": 0x0A},
		"Printer": {"FontName": 0x05, "FontSize": 0x06, "FontBold": 0x07, "CurrentX": 0x0C, "ScaleMode": 0x12},
		"App":     {"Path": 0x02},
	}
	for obj, props := range want {
		m := vbx.ObjectModels[obj]
		if m == nil {
			t.Errorf("missing %s model", obj)
			continue
		}
		for name, id := range props {
			if got := propIndex(m, name); got != id {
				t.Errorf("%s.%s at 0x%02X, want 0x%02X", obj, name, got, id)
			}
		}
	}
}
