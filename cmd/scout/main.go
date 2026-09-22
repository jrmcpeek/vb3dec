package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"vb3dec/pkg/ne"
)

type stringMatch struct {
	Keyword        string
	MatchedText    string
	Offset         uint32
	Segment        *ne.Segment
	OffsetInSeg    uint32
	ContextSnippet string
}

func main() {
	targets := []string{
		filepath.Join("test_input", "FF.EXE"),
		filepath.Join("test_input", "FF.DLL"),
	}

	keywords := []string{"mimic", "morph", "esper"}

	for _, target := range targets {
		if err := inspectBinary(target, keywords); err != nil {
			fmt.Fprintf(os.Stderr, "Error inspecting %s: %v\n", target, err)
			os.Exit(1)
		}
		fmt.Println()
	}
}

func inspectBinary(path string, keywords []string) error {
	file, err := ne.Open(path)
	if err != nil {
		return err
	}

	data := file.Bytes()
	hdr := file.Header

	separator := strings.Repeat("=", 80)
	subseparator := strings.Repeat("-", 80)

	fmt.Println(separator)
	fmt.Printf("File: %s\n", path)
	fmt.Printf("File Size: %d bytes (0x%X)\n", len(data), len(data))
	fmt.Printf("NE Header Offset: 0x%04X\n", file.NEOffset)
	fmt.Printf("Linker Version: %d.%02d\n", hdr.LinkerMajor, hdr.LinkerMinor)
	fmt.Printf("Sector Size: %d bytes (Shift: %d)\n", file.SectorSize, hdr.AlignShiftCount)
	fmt.Printf("Entry Point (CS:IP): %d:0x%04X\n", hdr.EntryCS, hdr.EntryIP)
	fmt.Printf("Auto Data Segment: %d\n", hdr.AutoDataSegIndex)
	fmt.Printf("Initial Heap: %d bytes, Stack: %d bytes\n", hdr.InitHeapSize, hdr.InitStackSize)
	fmt.Printf("Segment Count: %d\n", len(file.Segments))
	fmt.Println(subseparator)

	// Print Segment Table
	fmt.Println("NE SEGMENT TABLE:")
	fmt.Printf("%-5s | %-12s | %-14s | %-14s | %-8s | %s\n",
		"Seg #", "File Offset", "File Length", "Alloc Size", "Flags", "Attributes")
	fmt.Println(subseparator)

	for _, seg := range file.Segments {
		var offStr, lenStr string
		if seg.SectorOffset == 0 && seg.Length == 0 {
			offStr = "None (0x0)"
			lenStr = "0 bytes"
		} else {
			offStr = fmt.Sprintf("0x%08X", seg.FileOffset)
			lenStr = fmt.Sprintf("0x%04X (%d)", seg.FileLength, seg.FileLength)
		}
		allocStr := fmt.Sprintf("0x%04X (%d)", seg.MinAllocSize, seg.MinAllocSize)
		flagsStr := fmt.Sprintf("0x%04X", seg.Flags)

		fmt.Printf("%-5d | %-12s | %-14s | %-14s | %-8s | %s\n",
			seg.Index, offStr, lenStr, allocStr, flagsStr, seg.FlagString())
	}

	fmt.Println(subseparator)
	fmt.Printf("STRING SEARCH (%s):\n", strings.Join(keywords, ", "))
	fmt.Println(subseparator)

	var allMatches []stringMatch
	for _, kw := range keywords {
		matches := searchKeyword(file, data, kw)
		allMatches = append(allMatches, matches...)
	}

	if len(allMatches) == 0 {
		fmt.Println("  No matches found.")
	} else {
		for _, m := range allMatches {
			var segInfo string
			if m.Segment != nil {
				segInfo = fmt.Sprintf("Segment %d (+0x%04X / offset %d)", m.Segment.Index, m.OffsetInSeg, m.OffsetInSeg)
			} else {
				segInfo = "Outside segments (Header/Resource/Overlay)"
			}

			fmt.Printf("  • Match %q (for keyword %q):\n", m.MatchedText, m.Keyword)
			fmt.Printf("      File Offset : 0x%08X (%d)\n", m.Offset, m.Offset)
			fmt.Printf("      NE Location : %s\n", segInfo)
			fmt.Printf("      Context     : %s\n", m.ContextSnippet)
		}
		fmt.Printf("\n  Total matches in %s: %d\n", filepath.Base(path), len(allMatches))
	}

	return nil
}

func searchKeyword(file *ne.File, data []byte, keyword string) []stringMatch {
	var matches []stringMatch
	kwLower := strings.ToLower(keyword)
	kwLen := len(kwLower)

	// ASCII-only case-folding ensures exact 1:1 byte mapping with no UTF-8 rune shifts
	lowerData := make([]byte, len(data))
	for i, b := range data {
		if b >= 'A' && b <= 'Z' {
			lowerData[i] = b + ('a' - 'A')
		} else {
			lowerData[i] = b
		}
	}
	kwBytes := []byte(kwLower)

	startPos := 0
	for {
		idx := bytes.Index(lowerData[startPos:], kwBytes)
		if idx == -1 {
			break
		}
		absOffset := uint32(startPos + idx)
		matchedText := string(data[absOffset : absOffset+uint32(kwLen)])

		seg := file.SegmentAt(absOffset)
		var offsetInSeg uint32
		if seg != nil {
			offsetInSeg = absOffset - seg.FileOffset
		}

		// Context snippet (up to 12 bytes before, 24 bytes after)
		cStart := int(absOffset) - 12
		if cStart < 0 {
			cStart = 0
		}
		cEnd := int(absOffset) + kwLen + 24
		if cEnd > len(data) {
			cEnd = len(data)
		}

		snippet := formatContext(data[cStart:cEnd], int(absOffset)-cStart, kwLen)

		matches = append(matches, stringMatch{
			Keyword:        keyword,
			MatchedText:    matchedText,
			Offset:         absOffset,
			Segment:        seg,
			OffsetInSeg:    offsetInSeg,
			ContextSnippet: snippet,
		})

		startPos = int(absOffset) + 1
	}

	return matches
}

func formatContext(raw []byte, matchRelStart int, matchLen int) string {
	var sb strings.Builder
	for i, b := range raw {
		if i == matchRelStart {
			sb.WriteString("[")
		}
		if b >= 32 && b <= 126 {
			sb.WriteByte(b)
		} else if b == 0 {
			sb.WriteString("\\0")
		} else {
			sb.WriteString(fmt.Sprintf("\\x%02x", b))
		}
		if i == matchRelStart+matchLen-1 {
			sb.WriteString("]")
		}
	}
	result := sb.String()
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) || r == '[' || r == ']' || r == '\\' {
			return r
		}
		return '.'
	}, result)
}
