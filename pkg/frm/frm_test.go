package frm_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vb3dec/internal/fixture"
	"vb3dec/pkg/frm"
	"vb3dec/pkg/ne"
)

func TestExtractFormsWithNameMapping(t *testing.T) {
	exePath := fixture.Path(t, fixture.FF, "FF.EXE")
	f, err := ne.Open(exePath)
	if err != nil {
		t.Fatalf("Open %s: %v", exePath, err)
	}
	extracted, _, err := frm.ExtractForms(f, frm.ExtractOptions{
		NamesDir: fixture.Path(t, fixture.FF, "names"),
	})
	if err != nil {
		t.Fatalf("Extract with names: %v", err)
	}

	// In FRM1 with name mapping, control 4 should be named control4 (or mapped name)
	if len(extracted) == 0 || extracted[0].Form == nil {
		t.Fatalf("Expected extracted forms")
	}
	// Verify ComboBox control4 properties
	foundCtrl4 := false
	for _, c := range extracted[0].Form.Controls {
		if c.ID == 4 {
			foundCtrl4 = true
			if c.TypeName != "ComboBox" {
				t.Errorf("Expected control 4 to be ComboBox, got %s", c.TypeName)
			}
			break
		}
	}
	if !foundCtrl4 {
		t.Errorf("Control 4 not found in FRM1")
	}
}

func TestDecodeFormStreamRejectsTruncatedControl(t *testing.T) {
	stream := []byte{
		0xFF, 0xCC, 0x2C, 0, 0, 0, 0, 0, 0,
		8, 0, 0, 0, // form record length
		0, 0, 0x0D, 0xFF,
		1,          // child control tag
		4, 0, 0, 0, // invalid record length (< header + control ID)
	}
	if _, _, err := frm.DecodeFormStream(stream, frm.FormRef{FormName: "frm1"}, nil); err == nil {
		t.Fatal("DecodeFormStream accepted a truncated control record")
	}
}

func TestDecodeFormStreamReportsTruncatedProperty(t *testing.T) {
	stream := []byte{
		0xFF, 0xCC, 0x2C, 0, 0, 0, 0, 0, 0,
		8, 0, 0, 0,
		0, 0, 0x0D, 0xFF,
		1,          // child control tag
		9, 0, 0, 0, // control record length
		1, 0, 0x01, 0xFF, 0, // Label with a caption property missing its length byte
	}
	form, _, err := frm.DecodeFormStream(stream, frm.FormRef{FormName: "frm1"}, nil)
	if err != nil {
		t.Fatalf("a truncated property must not abort the form: %v", err)
	}
	if len(form.Controls) != 1 || form.Controls[0].TypeName != "Label" {
		t.Fatalf("expected the Label control to be kept, got %+v", form.Controls)
	}
	if len(form.Controls[0].Properties) != 0 {
		t.Errorf("expected no properties from the truncated record, got %+v", form.Controls[0].Properties)
	}
	if len(form.Warnings) != 1 || !strings.Contains(form.Warnings[0], "Caption") {
		t.Errorf("expected one warning naming the Caption property, got %q", form.Warnings)
	}
}

// formStream wraps child control records in a minimal form stream.
func formStream(children ...[]byte) []byte {
	stream := []byte{
		0xFF, 0xCC, 0x2C, 0, 0, 0, 0, 0, 0,
		8, 0, 0, 0, // form record length
		0, 0, 0x0D, 0xFF,
	}
	for _, c := range children {
		stream = append(stream, c...)
	}
	return append(stream, 4)
}

// controlRecord builds a control record with the given tag, ID, type and property bytes.
func controlRecord(tag byte, id byte, typeID byte, props ...byte) []byte {
	body := append([]byte{id, 0, typeID}, props...)
	n := uint32(4 + len(body))
	return append([]byte{tag, byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)}, body...)
}

func propertyMap(node *frm.ControlNode) map[string]string {
	m := make(map[string]string)
	for _, p := range node.Properties {
		m[p.Name] = p.Value
	}
	return m
}

func TestDecodeCommandButtonUsesRuntimePropertyTable(t *testing.T) {
	// A BASCODE.EXE CommandButton: Caption (0x00), Left..Height (0x04),
	// Font (0x0B, which an earlier decoder misread as one byte), TabIndex (0x11).
	button := controlRecord(1, 7, 0x04,
		0xFF,
		0x00, 3, 'O', 'K', '.',
		0x04, 0x78, 0x00, 0xF0, 0x00, 0x7F, 0x08, 0x77, 0x01,
		0x0B, 4, 'A', 'r', 'i', 'a', 0x00, 0x00, 0x1C, 0x41, 0x01,
		0x11, 0x02, 0x00,
		0xFF, 0x00, // empty event table
	)
	form, _, err := frm.DecodeFormStream(formStream(button, []byte{2}), frm.FormRef{FormName: "frm1"}, nil)
	if err != nil {
		t.Fatalf("DecodeFormStream: %v", err)
	}
	if len(form.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %q", form.Warnings)
	}
	got := propertyMap(form.Controls[0])
	want := map[string]string{
		"Caption":  `"OK."`,
		"Left":     "120",
		"Top":      "240",
		"Width":    "2175",
		"Height":   "375",
		"FontName": `"Aria"`,
		"FontSize": "9.75",
		"FontBold": "-1",
		"TabIndex": "2",
	}
	for name, value := range want {
		if got[name] != value {
			t.Errorf("%s = %q, want %q", name, got[name], value)
		}
	}
}

func TestDecodeDropdownListComboOmitsText(t *testing.T) {
	// Style = 2 (Dropdown List) before the separator; Text (0x0B) then carries
	// no payload, and TabIndex (0x12) and TabStop (0x1D) follow directly.
	combo := controlRecord(1, 4, 0x07,
		0x1F, 0x02,
		0xFF,
		0x0B,
		0x12, 0x13, 0x00,
		0x1D, 0x00,
		0xFF, 0x00,
	)
	form, _, err := frm.DecodeFormStream(formStream(combo, []byte{2}), frm.FormRef{FormName: "frm1"}, nil)
	if err != nil {
		t.Fatalf("DecodeFormStream: %v", err)
	}
	got := propertyMap(form.Controls[0])
	if _, ok := got["Text"]; ok {
		t.Errorf("dropdown-list ComboBox should not have a Text property")
	}
	if got["Style"] != "2" || got["TabIndex"] != "19" || got["TabStop"] != "0" {
		t.Errorf("unexpected ComboBox properties: %v", got)
	}
}

func TestDecodeNestedContainers(t *testing.T) {
	// Tag 1 opens a level under the preceding control and tag 2 closes it.
	frame := controlRecord(1, 1, 0x03, 0xFF, 0xFF, 0x00)
	inner := controlRecord(1, 2, 0x01, 0xFF, 0xFF, 0x00)
	sibling := controlRecord(3, 3, 0x01, 0xFF, 0xFF, 0x00)
	after := controlRecord(3, 4, 0x01, 0xFF, 0xFF, 0x00)
	stream := formStream(frame, inner, sibling, []byte{2}, after, []byte{2})
	form, _, err := frm.DecodeFormStream(stream, frm.FormRef{FormName: "frm1"}, nil)
	if err != nil {
		t.Fatalf("DecodeFormStream: %v", err)
	}
	root := form.Root
	if len(root.Children) != 2 || root.Children[0].ID != 1 || root.Children[1].ID != 4 {
		t.Fatalf("expected form children [1 4], got %+v", root.Children)
	}
	if kids := root.Children[0].Children; len(kids) != 2 || kids[0].ID != 2 || kids[1].ID != 3 {
		t.Fatalf("expected frame children [2 3], got %+v", kids)
	}
}

func TestExtractFormsFFExe(t *testing.T) {
	exePath := fixture.Path(t, fixture.FF, "FF.EXE")
	f, err := ne.Open(exePath)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", exePath, err)
	}

	forms, proj, err := frm.ExtractForms(f, frm.ExtractOptions{})
	if err != nil {
		t.Fatalf("Failed to extract forms: %v", err)
	}

	// Verify VBGuard protection detection
	if !proj.VBGuard {
		t.Errorf("Expected VBGuard protection to be detected on FF.EXE, got false")
	}

	// Verify custom controls detected (MCI.VBX)
	if len(proj.CustomVBXs) == 0 {
		t.Errorf("Expected custom VBX controls (MCI.VBX) to be detected")
	} else {
		foundMCI := false
		for _, vbx := range proj.CustomVBXs {
			if strings.Contains(strings.ToUpper(vbx), "MCI.VBX") {
				foundMCI = true
				break
			}
		}
		if !foundMCI {
			t.Errorf("Expected MCI.VBX in custom VBX list, got: %v", proj.CustomVBXs)
		}
	}

	// Expected control counts per form for FF.EXE
	expectedCounts := map[string]int{
		"FRM1.FRM": 35,
		"FRM2.FRM": 63,
		"FRM3.FRM": 9,
		"FRM4.FRM": 13,
		"FRM5.FRM": 22,
		"FRM6.FRM": 34,
		"FRM7.FRM": 1,
		"FRM8.FRM": 45,
		"FRM9.FRM": 4,
	}

	if len(forms) != len(expectedCounts) {
		t.Fatalf("Expected %d forms, got %d", len(expectedCounts), len(forms))
	}

	totalControls := 0
	for _, ef := range forms {
		expectedCount, ok := expectedCounts[ef.Ref.FileName]
		if !ok {
			t.Errorf("Unexpected extracted form: %s", ef.Ref.FileName)
			continue
		}

		ctrlCount := len(ef.Form.Controls)
		totalControls += ctrlCount

		if ctrlCount != expectedCount {
			t.Errorf("Form %s: expected %d controls, got %d", ef.Ref.FileName, expectedCount, ctrlCount)
		}

		// Verify text output begins with VERSION 2.00
		if !strings.HasPrefix(ef.TextFRM, "VERSION 2.00\r\n") {
			t.Errorf("Form %s: missing VERSION 2.00 header", ef.Ref.FileName)
		}

		// Verify Form node
		if ef.Form.Root == nil || ef.Form.Root.TypeName != "Form" {
			t.Errorf("Form %s: missing root Form node", ef.Ref.FileName)
		}

		// Verify FRX assets: FRM7 and FRM9 should have no FRX, others should have FRX
		hasFRX := len(ef.RawFRX) > 0
		if ef.Ref.FileName == "FRM7.FRM" || ef.Ref.FileName == "FRM9.FRM" {
			if hasFRX {
				t.Errorf("Form %s should not have FRX assets, got %d bytes", ef.Ref.FileName, len(ef.RawFRX))
			}
		} else {
			if !hasFRX {
				t.Errorf("Form %s expected FRX assets, got none", ef.Ref.FileName)
			}
		}
	}

	if totalControls != 226 {
		t.Errorf("Expected total 226 controls across all 9 forms, got %d", totalControls)
	}

	// Test saving to temporary directory
	tempDir, err := os.MkdirTemp("", "vb3_test_forms_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	if err := frm.SaveExtractedForms(forms, tempDir, true); err != nil {
		t.Fatalf("Failed to save extracted forms: %v", err)
	}

	// Verify all 9 .FRM files exist on disk
	for fileName := range expectedCounts {
		p := filepath.Join(tempDir, fileName)
		if info, err := os.Stat(p); err != nil || info.Size() == 0 {
			t.Errorf("Extracted file %s missing or empty", p)
		}
	}
}

func TestImageControlProperties(t *testing.T) {
	exePath := fixture.Path(t, fixture.FF, "FF.EXE")
	f, err := ne.Open(exePath)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", exePath, err)
	}

	forms, _, err := frm.ExtractForms(f, frm.ExtractOptions{})
	if err != nil {
		t.Fatalf("Failed to extract forms: %v", err)
	}

	var frm1 *frm.ExtractedForm
	for _, ef := range forms {
		if ef.Ref.FileName == "FRM1.FRM" {
			frm1 = ef
			break
		}
	}
	if frm1 == nil {
		t.Fatal("FRM1.FRM not found in extracted forms")
	}

	propsByCtrlID := make(map[int]map[string]string)
	for _, c := range frm1.Form.Controls {
		propsByCtrlID[c.ID] = make(map[string]string)
		for _, p := range c.Properties {
			propsByCtrlID[c.ID][p.Name] = p.Value
		}
	}

	// Verify AOL logo control34 properties (bottom-left placement)
	p34, ok := propsByCtrlID[34]
	if !ok {
		t.Fatal("control34 not found in FRM1")
	}
	if p34["Left"] != "480" || p34["Top"] != "2520" || p34["Width"] != "255" || p34["Height"] != "240" {
		t.Errorf("control34 coordinates mismatch: Left=%s, Top=%s, Width=%s, Height=%s",
			p34["Left"], p34["Top"], p34["Width"], p34["Height"])
	}

	// Verify animation frame control32 properties (hidden by default)
	p32, ok := propsByCtrlID[32]
	if !ok {
		t.Fatal("control32 not found in FRM1")
	}
	if p32["Visible"] != "0" {
		t.Errorf("control32 Visible expected '0', got %q", p32["Visible"])
	}
	if p32["Left"] != "0" || p32["Top"] != "2565" || p32["Width"] != "720" || p32["Height"] != "720" {
		t.Errorf("control32 coordinates mismatch: Left=%s, Top=%s, Width=%s, Height=%s",
			p32["Left"], p32["Top"], p32["Width"], p32["Height"])
	}
}

func TestDecodeEventTable(t *testing.T) {
	// A CommandButton whose event table binds slot 0 (Click) and slot 8
	// (MouseDown) to procedure descriptors 0x0120 and 0x0158.
	button := controlRecord(1, 1, 0x04,
		0xFF,
		0x11, 0x00, 0x00, // TabIndex
		0xFF, 0x09,
		0x21, 0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x59, 0x01,
	)
	form, _, err := frm.DecodeFormStream(formStream(button, []byte{2}), frm.FormRef{FormName: "frm1"}, nil)
	if err != nil {
		t.Fatalf("DecodeFormStream: %v", err)
	}
	events := form.Controls[0].Events
	if len(events) != 2 {
		t.Fatalf("expected 2 bound events, got %+v", events)
	}
	if events[0].Event.Name != "Click" || events[0].ProcRef != 0x0120 || events[1].Event.Name != "MouseDown" || events[1].ProcRef != 0x0158 {
		t.Errorf("unexpected bindings: %+v", events)
	}
}
