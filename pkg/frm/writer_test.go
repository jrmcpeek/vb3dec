package frm_test

import (
	"bytes"
	"strings"
	"testing"

	"vb3dec/pkg/frm"
)

func TestParseTextEncoding(t *testing.T) {
	tests := []struct {
		name string
		want frm.TextEncoding
	}{
		{"windows-1252", frm.EncodingWindows1252},
		{"CP1252", frm.EncodingWindows1252},
		{"utf-8", frm.EncodingUTF8},
		{" UTF8 ", frm.EncodingUTF8},
	}
	for _, tt := range tests {
		got, err := frm.ParseTextEncoding(tt.name)
		if err != nil || got != tt.want {
			t.Errorf("ParseTextEncoding(%q) = %v, %v; want %v", tt.name, got, err, tt.want)
		}
	}
	if _, err := frm.ParseTextEncoding("latin9"); err == nil {
		t.Error(`ParseTextEncoding("latin9") succeeded, want an error`)
	}
}

func TestEncodeText(t *testing.T) {
	text := "fn018F = \"«‹«——<\"\n"

	got, err := frm.EncodeText(text, frm.EncodingWindows1252)
	if err != nil {
		t.Fatalf("EncodeText(Windows-1252): %v", err)
	}
	want := append([]byte(`fn018F = "`), 0xAB, 0x8B, 0xAB, 0x97, 0x97, '<', '"', '\r', '\n')
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeText(Windows-1252) = % X, want % X", got, want)
	}

	got, err = frm.EncodeText(text, frm.EncodingUTF8)
	if err != nil {
		t.Fatalf("EncodeText(UTF-8): %v", err)
	}
	if want := strings.ReplaceAll(text, "\n", "\r\n"); string(got) != want {
		t.Errorf("EncodeText(UTF-8) = %q, want %q", got, want)
	}
}

func TestEncodeTextNamesUnrepresentableLine(t *testing.T) {
	_, err := frm.EncodeText("Sub Main ()\nx = \"☃\"\nEnd Sub\n", frm.EncodingWindows1252)
	if err == nil || !strings.Contains(err.Error(), "line 2:") {
		t.Fatalf("EncodeText error = %v, want one naming line 2", err)
	}
}
