package frm_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vb3dec/pkg/frm"
	"vb3dec/pkg/ne"
)

func TestDetectAssetFormat(t *testing.T) {
	tests := []struct {
		name     string
		payload  []byte
		expected string
	}{
		{"BMP", []byte("BM\x00\x00\x00\x00"), "bmp"},
		{"GIF87a", []byte("GIF87a\x00\x00"), "gif"},
		{"GIF89a", []byte("GIF89a\x00\x00"), "gif"},
		{"PNG", []byte("\x89PNG\r\n\x1a\n"), "png"},
		{"JPEG", []byte("\xFF\xD8\xFF\xE0\x00\x10"), "jpg"},
		{"ICO", []byte("\x00\x00\x01\x00\x01\x00"), "ico"},
		{"CUR", []byte("\x00\x00\x02\x00\x01\x00"), "cur"},
		{"WMF_APM", []byte("\xD7\xCD\xC6\x9A\x00\x00"), "wmf"},
		{"WMF_Standard", []byte("\x01\x00\x09\x00\x00\x03"), "wmf"},
		{"WAV", []byte("RIFF\x24\x00\x00\x00WAVEfmt "), "wav"},
		{"MIDI", []byte("MThd\x00\x00\x00\x06"), "mid"},
		{"Generic", []byte("\x12\x34\x56\x78\x9A"), "bin"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ext, _ := frm.DetectAssetFormat(tc.payload)
			if ext != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, ext)
			}
		})
	}
}

func TestExtractFRXAssetsFFExe(t *testing.T) {
	exePath := filepath.Join("..", "..", "test_input", "FF.EXE")
	f, err := ne.Open(exePath)
	if err != nil {
		t.Fatalf("Open %s: %v", exePath, err)
	}

	extracted, _, err := frm.ExtractForms(f, frm.ExtractOptions{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(extracted) != 9 {
		t.Fatalf("Expected 9 forms, got %d", len(extracted))
	}

	// Verify FRM1 assets
	frm1 := extracted[0]
	if len(frm1.Assets) == 0 {
		t.Fatalf("Expected assets in FRM1, got 0")
	}

	iconCount := 0
	bmpCount := 0
	for _, asset := range frm1.Assets {
		if asset.Format == "ico" {
			iconCount++
			if len(asset.Data) < 4 || asset.Data[0] != 0 || asset.Data[1] != 0 || asset.Data[2] != 1 || asset.Data[3] != 0 {
				t.Errorf("Asset %s has invalid ICO header", asset.FileName)
			}
		}
		if asset.Format == "bmp" {
			bmpCount++
			if len(asset.Data) < 2 || asset.Data[0] != 'B' || asset.Data[1] != 'M' {
				t.Errorf("Asset %s has invalid BMP header", asset.FileName)
			}
		}

		// Ensure filename is well-formed
		if !strings.HasPrefix(asset.FileName, "FRM1_") {
			t.Errorf("Asset filename expected prefix FRM1_, got %s", asset.FileName)
		}
	}

	if iconCount != 1 {
		t.Errorf("Expected 1 icon in FRM1, got %d", iconCount)
	}
	if bmpCount != 14 {
		t.Errorf("Expected 14 bitmaps in FRM1 (1 form picture + 13 control images), got %d", bmpCount)
	}

	// Test saving with assets to temp directory
	tempDir, err := os.MkdirTemp("", "vb3_asset_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	if err := frm.SaveExtractedForms(extracted, tempDir, false); err != nil {
		t.Fatalf("SaveExtractedForms: %v", err)
	}

	assetsDir := filepath.Join(tempDir, "assets")
	entries, err := os.ReadDir(assetsDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", assetsDir, err)
	}

	if len(entries) == 0 {
		t.Errorf("Expected standalone asset files in %s, got none", assetsDir)
	}

	// Check that FRM1_frm1_Icon_0000.ico exists
	iconPath := filepath.Join(assetsDir, "FRM1_frm1_Icon_0000.ico")
	if info, err := os.Stat(iconPath); err != nil || info.Size() == 0 {
		t.Errorf("Expected %s to exist and be non-empty", iconPath)
	}
}

func TestSaveExtractedFormsRejectsEscapingNames(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vb3_path_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	forms := []*frm.ExtractedForm{
		{Ref: frm.FormRef{FileName: `..\escaped.FRM`}, TextFRM: "VERSION 2.00\r\n"},
	}
	if err := frm.SaveExtractedFormsWithAssets(forms, tempDir, false, false, ""); err == nil {
		t.Fatal("expected path traversal filename to be rejected")
	}
}
