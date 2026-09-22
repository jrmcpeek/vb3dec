package ne_test

import (
	"path/filepath"
	"testing"

	"vb3dec/pkg/ne"
)

func TestResourceTable(t *testing.T) {
	exePath := filepath.Join("..", "..", "test_input", "FF.EXE")
	f, err := ne.Open(exePath)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", exePath, err)
	}

	if len(f.Resources) == 0 {
		t.Fatalf("Expected resources in %s, got none", exePath)
	}

	// Verify RT_RCDATA type exists
	rcData := f.ResourcesByType(ne.ResTypeRCData)
	if len(rcData) != 20 {
		t.Errorf("Expected 20 RT_RCDATA resources in FF.EXE, got %d", len(rcData))
	}

	// Verify ID 1 (Project structure) exists and can be read
	res1, err := f.FindResource(ne.ResTypeRCData, 1)
	if err != nil {
		t.Fatalf("Failed to find RT_RCDATA ID 1: %v", err)
	}
	if res1.Offset == 0 || res1.Length == 0 {
		t.Errorf("Expected non-zero offset and length for RCData 1, got offset=0x%X, len=%d", res1.Offset, res1.Length)
	}
	data1, err := f.ReadResourceData(res1)
	if err != nil {
		t.Fatalf("Failed to read RCData 1: %v", err)
	}
	if len(data1) == 0 {
		t.Errorf("Read 0 bytes for RCData 1")
	}

	// Verify ID 4 (Form 1) exists and has data starting with FF CC 2C
	res4, err := f.FindResource(ne.ResTypeRCData, 4)
	if err != nil {
		t.Fatalf("Failed to find RT_RCDATA ID 4: %v", err)
	}
	data4, err := f.ReadResourceData(res4)
	if err != nil {
		t.Fatalf("Failed to read RCData 4: %v", err)
	}
	if len(data4) < 3 || data4[0] != 0xFF || data4[1] != 0xCC || data4[2] != 0x2C {
		t.Errorf("RCData 4 magic mismatch: expected FF CC 2C, got %02X %02X %02X", data4[0], data4[1], data4[2])
	}

	// Verify ID 5 (Form 1 stripped names) has offset 0 (VBGuard protected)
	res5, err := f.FindResource(ne.ResTypeRCData, 5)
	if err != nil {
		t.Fatalf("Failed to find RT_RCDATA ID 5: %v", err)
	}
	if res5.Offset != 0 {
		t.Errorf("Expected offset 0 for stripped RCData 5, got 0x%X", res5.Offset)
	}
	_, err = f.ReadResourceData(res5)
	if err == nil {
		t.Errorf("Expected error reading stripped resource with offset 0, got nil")
	}
}
