package win1252_test

import (
	"testing"

	"vb3dec/pkg/win1252"
)

func TestDecode(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "ASCII only",
			input:    []byte("Final Fantasy"),
			expected: "Final Fantasy",
		},
		{
			name:     "Right arrow prompt (fn00BE)",
			input:    []byte{0x3E, 0x97, 0x97, 0xBB, 0x9B, 0xBB},
			expected: ">——»›»",
		},
		{
			name:     "Left arrow prompt (fn018F)",
			input:    []byte{0xAB, 0x8B, 0xAB, 0x97, 0x97, 0x3C},
			expected: "«‹«——<",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := win1252.Decode(tt.input)
			if got != tt.expected {
				t.Errorf("Decode() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatVBString(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "Quotes escaping",
			input:    []byte(`Hello "World"`),
			expected: `"Hello ""World"""`,
		},
		{
			name:     "High-ASCII characters",
			input:    []byte{0x3E, 0x97, 0x97, 0xBB, 0x9B, 0xBB},
			expected: `">——»›»"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := win1252.FormatVBString(tt.input)
			if got != tt.expected {
				t.Errorf("FormatVBString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestQuoteVBString(t *testing.T) {
	got := win1252.QuoteVBString(`Say "Hi" at C:\Temp`)
	want := `"Say ""Hi"" at C:\Temp"`
	if got != want {
		t.Fatalf("QuoteVBString() = %q, want %q", got, want)
	}
}
