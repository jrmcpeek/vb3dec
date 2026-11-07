package win1252

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// win1252ToRune maps Windows-1252 bytes 0x80..0xFF to Unicode code points.
var win1252ToRune = [128]rune{
	0x20AC, // 0x80: Euro sign €
	0x0081, // 0x81: unassigned
	0x201A, // 0x82: Single low-9 quotation mark ‚
	0x0192, // 0x83: Latin small letter f with hook ƒ
	0x201E, // 0x84: Double low-9 quotation mark „
	0x2026, // 0x85: Horizontal ellipsis …
	0x2020, // 0x86: Dagger †
	0x2021, // 0x87: Double dagger ‡
	0x02C6, // 0x88: Modifier letter circumflex accent ˆ
	0x2030, // 0x89: Per mille sign ‰
	0x0160, // 0x8A: Latin capital letter S with caron Š
	0x2039, // 0x8B: Single left-pointing angle quotation mark ‹
	0x0152, // 0x8C: Latin capital ligature OE Œ
	0x008D, // 0x8D: unassigned
	0x017D, // 0x8E: Latin capital letter Z with caron Ž
	0x008F, // 0x8F: unassigned
	0x0090, // 0x90: unassigned
	0x2018, // 0x91: Left single quotation mark ‘
	0x2019, // 0x92: Right single quotation mark ’
	0x201C, // 0x93: Left double quotation mark “
	0x201D, // 0x94: Right double quotation mark ”
	0x2022, // 0x95: Bullet •
	0x2013, // 0x96: En dash –
	0x2014, // 0x97: Em dash —
	0x02DC, // 0x98: Small tilde ˜
	0x2122, // 0x99: Trade mark sign ™
	0x0161, // 0x9A: Latin small letter s with caron š
	0x203A, // 0x9B: Single right-pointing angle quotation mark ›
	0x0153, // 0x9C: Latin small ligature oe œ
	0x009D, // 0x9D: unassigned
	0x017E, // 0x9E: Latin small letter z with caron ž
	0x0178, // 0x9F: Latin capital letter Y with diaeresis Ÿ
	// 0xA0..0xFF are identical to ISO-8859-1 (U+00A0..U+00FF)
	0x00A0, 0x00A1, 0x00A2, 0x00A3, 0x00A4, 0x00A5, 0x00A6, 0x00A7,
	0x00A8, 0x00A9, 0x00AA, 0x00AB, 0x00AC, 0x00AD, 0x00AE, 0x00AF,
	0x00B0, 0x00B1, 0x00B2, 0x00B3, 0x00B4, 0x00B5, 0x00B6, 0x00B7,
	0x00B8, 0x00B9, 0x00BA, 0x00BB, 0x00BC, 0x00BD, 0x00BE, 0x00BF,
	0x00C0, 0x00C1, 0x00C2, 0x00C3, 0x00C4, 0x00C5, 0x00C6, 0x00C7,
	0x00C8, 0x00C9, 0x00CA, 0x00CB, 0x00CC, 0x00CD, 0x00CE, 0x00CF,
	0x00D0, 0x00D1, 0x00D2, 0x00D3, 0x00D4, 0x00D5, 0x00D6, 0x00D7,
	0x00D8, 0x00D9, 0x00DA, 0x00DB, 0x00DC, 0x00DD, 0x00DE, 0x00DF,
	0x00E0, 0x00E1, 0x00E2, 0x00E3, 0x00E4, 0x00E5, 0x00E6, 0x00E7,
	0x00E8, 0x00E9, 0x00EA, 0x00EB, 0x00EC, 0x00ED, 0x00EE, 0x00EF,
	0x00F0, 0x00F1, 0x00F2, 0x00F3, 0x00F4, 0x00F5, 0x00F6, 0x00F7,
	0x00F8, 0x00F9, 0x00FA, 0x00FB, 0x00FC, 0x00FD, 0x00FE, 0x00FF,
}

// runeToWin1252 is the inverse of win1252ToRune. It includes the five
// unassigned bytes (0x81, 0x8D, 0x8F, 0x90, 0x9D), which Decode maps to the
// C1 control characters of the same value, so that every byte round-trips.
var runeToWin1252 = func() map[rune]byte {
	m := make(map[rune]byte, len(win1252ToRune))
	for i, r := range win1252ToRune {
		m[r] = byte(0x80 + i)
	}
	return m
}()

// UnrepresentableError reports text that has no Windows-1252 encoding.
type UnrepresentableError struct {
	Offset  int  // Byte offset of the character in the UTF-8 input
	Rune    rune // The character that cannot be encoded
	Invalid bool // The input is not valid UTF-8 at Offset
}

func (e *UnrepresentableError) Error() string {
	if e.Invalid {
		return fmt.Sprintf("invalid UTF-8 at byte offset %d", e.Offset)
	}
	return fmt.Sprintf("character %q (U+%04X) at byte offset %d cannot be encoded in Windows-1252", e.Rune, e.Rune, e.Offset)
}

// Encode converts a UTF-8 string to Windows-1252 bytes, the inverse of
// Decode. It returns an *UnrepresentableError for invalid UTF-8 or for a
// character outside Windows-1252.
func Encode(s string) ([]byte, error) {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		if s[i] < 0x80 {
			out = append(out, s[i])
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			return nil, &UnrepresentableError{Offset: i, Rune: utf8.RuneError, Invalid: true}
		}
		b, ok := runeToWin1252[r]
		if !ok {
			return nil, &UnrepresentableError{Offset: i, Rune: r}
		}
		out = append(out, b)
		i += size
	}
	return out, nil
}

// Decode converts raw Windows-1252 (ANSI) bytes into a valid UTF-8 string.
func Decode(raw []byte) string {
	var sb strings.Builder
	for _, b := range raw {
		if b < 0x80 {
			sb.WriteByte(b)
		} else {
			sb.WriteRune(win1252ToRune[b-0x80])
		}
	}
	return sb.String()
}

// FormatVBString escapes and quotes a raw Windows-1252 byte slice as a valid VB string literal in UTF-8.
func FormatVBString(raw []byte) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, b := range raw {
		if b == '"' {
			sb.WriteString(`""`)
		} else if b < 0x80 {
			sb.WriteByte(b)
		} else {
			sb.WriteRune(win1252ToRune[b-0x80])
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// QuoteVBString quotes an already-decoded string as a VB string literal.
// VB escapes embedded quotes by doubling them and leaves backslashes literal.
func QuoteVBString(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
