package win1252_test

import (
	"bytes"
	"errors"
	"testing"
	"unicode/utf8"

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

func TestEncodeRoundTripsEveryByte(t *testing.T) {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	got, err := win1252.Encode(win1252.Decode(all))
	if err != nil {
		t.Fatalf("Encode(Decode(0x00..0xFF)): %v", err)
	}
	if !bytes.Equal(got, all) {
		t.Fatalf("Encode(Decode(0x00..0xFF)) = % X, want % X", got, all)
	}
}

func TestEncode(t *testing.T) {
	got, err := win1252.Encode(`"«‹«——< Cecil is ready >——»›»"`)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want := append([]byte{'"', 0xAB, 0x8B, 0xAB, 0x97, 0x97}, []byte("< Cecil is ready >")...)
	want = append(want, 0x97, 0x97, 0xBB, 0x9B, 0xBB, '"')
	if !bytes.Equal(got, want) {
		t.Fatalf("Encode() = % X, want % X", got, want)
	}
}

func TestEncodeRejectsUnrepresentable(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		offset  int
		r       rune
		invalid bool
	}{
		{name: "Character outside Windows-1252", input: "ok ☃", offset: 3, r: '☃'},
		{name: "Replacement character", input: "�", offset: 0, r: '�'},
		{name: "Invalid UTF-8", input: "ab\xAB", offset: 2, r: utf8.RuneError, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := win1252.Encode(tt.input)
			var ue *win1252.UnrepresentableError
			if !errors.As(err, &ue) {
				t.Fatalf("Encode(%q) error = %v, want *UnrepresentableError", tt.input, err)
			}
			if ue.Offset != tt.offset || ue.Rune != tt.r || ue.Invalid != tt.invalid {
				t.Errorf("Encode(%q) error = %+v, want offset %d rune %q invalid %v", tt.input, *ue, tt.offset, tt.r, tt.invalid)
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
