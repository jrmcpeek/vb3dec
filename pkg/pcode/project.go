package pcode

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"vb3dec/pkg/frm"
	"vb3dec/pkg/ne"
)

// ror3 performs a 16-bit right rotation by 3 bits.
func ror3(v uint16) uint16 {
	return (v >> 3) | ((v & 7) << 13)
}

// rol3 performs a 16-bit left rotation by 3 bits.
func rol3(v uint16) uint16 {
	return (v << 3) | ((v >> 13) & 7)
}

// Procedure represents a disassembled or declared procedure in a VB3 executable.
type Procedure struct {
	Index       int            // 1-based global procedure index (1..N)
	Name        string         // e.g. "fn006F", "sub009D", "control1_Click", "Form_Load"
	NameID      uint16         // 16-bit identifier from descriptor word 2
	DescOffset  uint16         // In-segment offset in Segment 3 (e.g. 0x0790)
	ProcPtr     uint16         // Procedure descriptor pointer (currProcPtr, e.g. 1936)
	ModuleID    uint16         // Parent module ID (desc word 10)
	ModuleIndex int            // 1-based module index
	SubOrFunc   uint16         // Procedure type flags (desc word 6)
	PubOrPriv   uint16         // Public/Private flags (desc word 7)
	IsExt       uint16         // Flags (desc word 11)
	IsLocal     bool           // true if isExt & 1 == 1 (has bytecode body)
	CodeSeg     int            // 1-based NE segment index (e.g. 4..17)
	CodeOffset  uint16         // Start offset in code segment
	CodeSize    uint16         // End offset / size in code segment
	Bytecode    []byte         // Raw P-code bytecode
	DeclString  string         // Emitted procedure declaration / header
	Decompiled  string         // Decompiled procedure source text
	LibName     string         // Dynamically resolved library for external declarations (e.g. "ff.dll", "User", "kernel")
	AliasName   string         // Dynamically resolved alias for external declarations (e.g. "FindChildByClass")
	ParamByVal  map[int]bool   // 0-based parameter index -> true if ByVal
	ParamTypes  map[int]string // 0-based parameter index -> inferred parameter type
}

// IsFunction reports whether the descriptor carries a return type.
// VB3 encodes zero for Subs and the return type code in the high byte for Functions.
func (p *Procedure) IsFunction() bool {
	return p != nil && ((p.SubOrFunc>>8)&7) != 0
}

// DeclaredName returns the public name used in declarations and invocations.
func (p *Procedure) DeclaredName() string {
	if !p.IsLocal && (strings.HasPrefix(p.Name, "sub") || strings.HasPrefix(p.Name, "fn")) {
		return "ext" + p.Name
	}
	return p.Name
}

// Module represents a code module or form code container in the VB3 project.
type Module struct {
	Index          int               // 1-based module index (1..N)
	ID             uint16            // Module identifier (e.g. 0x0048, 0x0950)
	Name           string            // e.g. "Module1", "frm1", "frm5"
	IsForm         bool              // true if module is backed by a form
	FormIndex      int               // 1-based form index if Form
	Procedures     []*Procedure      // Procedures belonging to this module
	ControlMap     map[uint16]string // Data segment offset -> control name or form name
	ControlTypes   map[string]string // Control name -> type name (e.g. "control4" -> "ComboBox")
	ControlByIndex map[int]string    // 1-based control index -> control name (e.g. 1 -> "control1")
	ModuleVars     []string          // Module-level variable declarations (e.g. "Dim m001E As Integer")
	ModBytes       []byte            // Raw module data bytes from RT_RCDATA 2
}

// Project represents the discovered code structure of a VB3 executable.
type Project struct {
	NEFile       *ne.File
	VBStartSeg   int
	Modules      []*Module
	Procedures   []*Procedure
	ProcByPtr    map[uint16]*Procedure // Procedure lookup by pointer / descriptor offset
	GlobalVars   []string              // Project-wide global variable declarations (e.g. "Global gv0006(1 To 30, 1 To 2) As String")
	GlobalVarMap map[uint16]string     // Global offset in SomeDataBuff -> variable name (e.g. 0x0006 -> "gv0006")
}

// ParseProject scans the NE executable for VB3 procedure descriptors, module tables,
// and resolves segment relocation chains to reconstruct all code blocks. The form
// extraction options are reused when resolving control event procedure names.
func ParseProject(f *ne.File, formOpts frm.ExtractOptions) (*Project, error) {
	// Find RT_RCDATA ID 2 (VB3 Runtime structures)
	entry2, err := f.FindResource(ne.ResTypeRCData, 2)
	if err != nil {
		return nil, fmt.Errorf("locating RT_RCDATA ID 2: %w", err)
	}

	data2, err := f.ReadResourceData(entry2)
	if err != nil {
		return nil, fmt.Errorf("reading RT_RCDATA ID 2: %w", err)
	}

	if len(data2) < 0x60 {
		return nil, fmt.Errorf("RT_RCDATA ID 2 too small (%d bytes)", len(data2))
	}

	procPtr := binary.LittleEndian.Uint16(data2[0x56:0x58])
	modPtr := binary.LittleEndian.Uint16(data2[0x5A:0x5C])

	// Determine VBStartSeg (first code segment with RELOCINFO that is not EntryCS)
	vbStartSeg := 3
	for _, seg := range f.Segments {
		if seg.Flags&ne.SegFlagRelocInfo != 0 && seg.Index != int(f.Header.EntryCS) {
			vbStartSeg = seg.Index
			break
		}
	}

	if vbStartSeg > len(f.Segments) {
		return nil, fmt.Errorf("invalid VBStartSeg %d (total segments: %d)", vbStartSeg, len(f.Segments))
	}

	seg3 := f.Segments[vbStartSeg-1]
	fileBytes := f.Bytes()
	seg3Start := seg3.FileOffset
	seg3End := seg3.FileOffset + seg3.FileLength

	if int(seg3End) > len(fileBytes) {
		return nil, fmt.Errorf("segment 3 end %d exceeds file length %d", seg3End, len(fileBytes))
	}
	seg3Data := fileBytes[seg3Start:seg3End]

	// Read relocation chains from Segment 3
	descToSeg := make(map[uint16]int)
	if seg3.Flags&ne.SegFlagRelocInfo != 0 && int(seg3End+2) <= len(fileBytes) {
		relocCount := binary.LittleEndian.Uint16(fileBytes[seg3End : seg3End+2])
		pos := seg3End + 2

		for i := 0; i < int(relocCount); i++ {
			if int(pos+8) > len(fileBytes) {
				break
			}
			offInSeg := binary.LittleEndian.Uint16(fileBytes[pos+2 : pos+4])
			targetSeg := binary.LittleEndian.Uint16(fileBytes[pos+4 : pos+6])
			pos += 8

			currOff := offInSeg
			visited := make(map[uint16]struct{})
			for currOff != 0xFFFF {
				if _, ok := visited[currOff]; ok {
					return nil, fmt.Errorf("cyclic relocation chain at segment offset 0x%04X", currOff)
				}
				visited[currOff] = struct{}{}
				if currOff < 0x26 {
					return nil, fmt.Errorf("invalid relocation offset 0x%04X", currOff)
				}
				descBase := currOff - 0x26
				descToSeg[descBase] = int(targetSeg)

				if int(currOff)+2 > len(seg3Data) {
					return nil, fmt.Errorf("relocation offset 0x%04X exceeds segment data", currOff)
				}
				nextInChain := binary.LittleEndian.Uint16(seg3Data[currOff : currOff+2])
				currOff = nextInChain
			}
		}
	}

	// 1. Discover all modules
	var modules []*Module
	modMap := make(map[uint16]*Module)
	currModPtr := modPtr
	modIndex := 1
	visitedModules := make(map[uint16]struct{})

	for currModPtr != 0xFFFF && currModPtr != 0 {
		if _, ok := visitedModules[currModPtr]; ok {
			return nil, fmt.Errorf("cyclic module descriptor chain at 0x%04X", currModPtr)
		}
		visitedModules[currModPtr] = struct{}{}
		rolVal := ror3(currModPtr)
		inSegOff := int(rolVal&0x1FFF) * 8

		if inSegOff < 0 || inSegOff+64 > len(seg3Data) {
			return nil, fmt.Errorf("module descriptor at segment offset 0x%X is truncated", inSegOff)
		}
		descBytes := seg3Data[inSegOff : inSegOff+64]

		modID := currModPtr
		// Word 13: procCount
		// Word 15: nextModPtr
		procCount := binary.LittleEndian.Uint16(descBytes[26:28])
		nextMod := binary.LittleEndian.Uint16(descBytes[30:32])

		// Skip project root descriptor (has 0 procedures)
		if procCount == 0 && modIndex == 1 {
			currModPtr = nextMod
			continue
		}

		modName := fmt.Sprintf("Module%d", modIndex)
		isForm := false
		formIdx := 0

		// In standard VB3: First module is Module1 (code module), remaining modules are Forms
		if modIndex == 1 {
			modName = "Module1"
		} else {
			isForm = true
			formIdx = modIndex - 1
			modName = fmt.Sprintf("frm%d", formIdx)
		}

		m := &Module{
			Index:     modIndex,
			ID:        modID,
			Name:      modName,
			IsForm:    isForm,
			FormIndex: formIdx,
		}
		modules = append(modules, m)
		modMap[modID] = m

		currModPtr = nextMod
		modIndex++
		if modIndex > 50 {
			break
		}
	}

	// 2. Discover all procedures
	var procs []*Procedure
	procByPtr := make(map[uint16]*Procedure)
	currProcPtr := procPtr
	procIndex := 1
	visitedProcs := make(map[uint16]struct{})

	for currProcPtr != 0xFFFF && currProcPtr != 0 {
		if _, ok := visitedProcs[currProcPtr]; ok {
			return nil, fmt.Errorf("cyclic procedure descriptor chain at 0x%04X", currProcPtr)
		}
		visitedProcs[currProcPtr] = struct{}{}
		rolVal := ror3(currProcPtr)
		inSegOff := uint16((rolVal & 0x1FFF) * 8)

		if int(inSegOff)+48 > len(seg3Data) {
			break
		}
		descBytes := seg3Data[inSegOff : inSegOff+48]

		nameID := binary.LittleEndian.Uint16(descBytes[4:6])
		subOrFunc := binary.LittleEndian.Uint16(descBytes[12:14])
		pubOrPriv := binary.LittleEndian.Uint16(descBytes[14:16])
		modID := binary.LittleEndian.Uint16(descBytes[20:22])
		isExt := binary.LittleEndian.Uint16(descBytes[22:24])
		codeOff := binary.LittleEndian.Uint16(descBytes[24:26])
		nextProc := binary.LittleEndian.Uint16(descBytes[26:28])
		codeSize := binary.LittleEndian.Uint16(descBytes[36:38])

		isLocal := (isExt & 1) == 1
		targetSegNum := descToSeg[inSegOff]

		prefix := "sub"
		funcType := (subOrFunc >> 8) & 7
		if funcType != 0 {
			prefix = "fn"
		}
		pName := fmt.Sprintf("%s%04X", prefix, nameID)

		var bytecode []byte
		if isLocal && targetSegNum > 0 && targetSegNum <= len(f.Segments) {
			targetSeg := f.Segments[targetSegNum-1]
			segFileStart := targetSeg.FileOffset
			segFileEnd := targetSeg.FileOffset + targetSeg.FileLength

			start := segFileStart + uint32(codeOff)
			end := segFileStart + uint32(codeSize)
			if end > segFileEnd {
				end = segFileEnd
			}
			if start < end && int(end) <= len(fileBytes) {
				bytecode = fileBytes[start:end]
			}
		}

		modIdx := 0
		if m, ok := modMap[modID]; ok {
			modIdx = m.Index
		}

		var libName, aliasName string
		if !isLocal {
			w20 := binary.LittleEndian.Uint16(descBytes[40:42])
			w23 := binary.LittleEndian.Uint16(descBytes[46:48])
			if w20 != 0 {
				off := 0x01ED + int(w20)
				if off < len(data2) {
					sLen := int(data2[off])
					if off+1+sLen <= len(data2) {
						libName = strings.TrimSuffix(string(data2[off+1:off+1+sLen]), ".")
					}
				}
			}
			if w23 != 0 {
				off := 0x01ED + int(w23)
				if off < len(data2) {
					sLen := int(data2[off])
					if off+1+sLen <= len(data2) {
						aliasName = string(data2[off+1 : off+1+sLen])
					}
				}
			}
		}

		p := &Procedure{
			Index:       procIndex,
			Name:        pName,
			NameID:      nameID,
			DescOffset:  inSegOff,
			ProcPtr:     currProcPtr,
			ModuleID:    modID,
			ModuleIndex: modIdx,
			SubOrFunc:   subOrFunc,
			PubOrPriv:   pubOrPriv,
			IsExt:       isExt,
			IsLocal:     isLocal,
			CodeSeg:     targetSegNum,
			CodeOffset:  codeOff,
			CodeSize:    codeSize,
			Bytecode:    bytecode,
			LibName:     libName,
			AliasName:   aliasName,
		}

		procs = append(procs, p)
		procByPtr[currProcPtr] = p
		procByPtr[inSegOff] = p

		if m, ok := modMap[modID]; ok {
			m.Procedures = append(m.Procedures, p)
		}

		currProcPtr = nextProc
		procIndex++
		if procIndex > 500 {
			break
		}
	}

	proj := &Project{
		NEFile:       f,
		VBStartSeg:   vbStartSeg,
		Modules:      modules,
		Procedures:   procs,
		ProcByPtr:    procByPtr,
		GlobalVarMap: make(map[uint16]string),
	}

	resolveControlTypes(f, proj, formOpts)
	resolveModuleEvents(f, modules, procByPtr)
	resolveGlobalsAndFixups(data2, proj)
	if err := resolveByValParameters(proj); err != nil {
		return nil, err
	}

	return proj, nil
}

func resolveControlTypes(f *ne.File, proj *Project, formOpts frm.ExtractOptions) {
	forms, _, err := frm.ExtractForms(f, formOpts)
	if err != nil {
		return
	}
	for _, ef := range forms {
		if ef.Form == nil {
			continue
		}
		for _, m := range proj.Modules {
			if m.IsForm && m.FormIndex == ef.Ref.Index {
				if m.ControlTypes == nil {
					m.ControlTypes = make(map[string]string)
				}
				if m.ControlByIndex == nil {
					m.ControlByIndex = make(map[int]string)
				}
				m.ControlTypes["Me"] = "Form"
				m.ControlTypes[m.Name] = "Form"
				for _, c := range ef.Form.Controls {
					ctrlIdx := c.ID
					if ctrlIdx <= 0 {
						continue
					}
					ctrlName := c.Name
					if ctrlName == "" {
						ctrlName = fmt.Sprintf("control%d", ctrlIdx)
					}
					m.ControlByIndex[ctrlIdx] = ctrlName
					m.ControlTypes[ctrlName] = c.TypeName
				}
			}
		}
	}
}

func resolveModuleEvents(f *ne.File, modules []*Module, procByPtr map[uint16]*Procedure) {
	for _, m := range modules {
		if !m.IsForm {
			continue
		}

		// Always map standard Form event NameIDs on any form
		for _, p := range m.Procedures {
			switch p.NameID {
			case 0x04D1:
				p.Name = "Form_Load"
			case 0x04B2:
				p.Name = "Form_Activate"
			case 0x05CC:
				p.Name = "Form_Unload"
			case 0x04C3:
				p.Name = "Form_Click"
			case 0x0C68:
				p.Name = "Form_DblClick"
			}
		}

		resID := uint16(m.FormIndex*2 + 2)
		entry, err := f.FindResource(ne.ResTypeRCData, resID)
		if err != nil {
			continue
		}
		data, err := f.ReadResourceData(entry)
		if err != nil || len(data) < 16 {
			continue
		}

		// VB3 Form RCData resource layout:
		// Bytes 0..3: 0xFF, 0xCC, 0x2C, 0x00 (magic)
		// Byte 4: control count / flags
		// Bytes 5..8: stream data length (uint32)
		// Bytes 9..12: form record length (uint32)
		// Bytes 13..13+formRecLen: Form properties & event table
		// Bytes 13+formRecLen..: Child control records

		formStartPos := 0
		formRecLen := len(data)

		if len(data) >= 13 && data[0] == 0xFF && data[1] == 0xCC {
			formStartPos = 9
			formRecLen = int(binary.LittleEndian.Uint32(data[9:13]) & 0x7FFFFFFF)
		} else {
			formRecLen = int(binary.LittleEndian.Uint32(data[0:4]) & 0x7FFFFFFF)
		}

		formEndPos := formStartPos + formRecLen
		if formEndPos > len(data) {
			formEndPos = len(data)
		}

		// 1. Scan Form event table inside form record (located at end of form record)
		for p := formEndPos - 4; p >= formStartPos; p-- {
			if data[p] == 0xFF {
				cnt := int(data[p+1])
				rem := formEndPos - (p + 2 + cnt*2)
				if cnt > 0 && cnt < 64 && rem >= 0 && rem <= 4 {
					found := false
					for slot := 0; slot < cnt; slot++ {
						w := binary.LittleEndian.Uint16(data[p+2+slot*2 : p+4+slot*2])
						if w != 0 && (w&1 == 1) {
							descOff := w & ^uint16(1)
							if proc, ok := procByPtr[descOff]; ok && proc.ModuleIndex == m.Index {
								found = true
								switch slot {
								case 6:
									proc.Name = "Form_Load"
								case 7:
									proc.Name = "Form_Resize"
								case 8:
									proc.Name = "Form_Unload"
								case 9:
									proc.Name = "Form_QueryUnload"
								case 0, 10, 20:
									proc.Name = "Form_Activate"
								case 11:
									proc.Name = "Form_Deactivate"
								case 1, 12:
									proc.Name = "Form_Click"
								case 2, 13:
									proc.Name = "Form_DblClick"
								case 19:
									proc.Name = "Form_MouseDown"
								case 21:
									proc.Name = "Form_MouseUp"
								case 22:
									proc.Name = "Form_Paint"
								}
							}
						}
					}
					if found {
						break
					}
				}
			}
		}

		// 2. Scan Child Control event tables (located at end of control record)
		pos := formEndPos
		for pos < len(data) {
			tag := data[pos]
			pos++
			if tag == 4 { // End of Form
				break
			}
			if tag == 2 || tag == 5 {
				continue
			}
			if tag != 1 && tag != 3 {
				break
			}
			if pos+5 > len(data) {
				break
			}
			rawLen := binary.LittleEndian.Uint32(data[pos : pos+4])
			hasFlags := (rawLen & 0x80000000) != 0
			recLen := int(rawLen & 0x7FFFFFFF)
			ctrlID := int(data[pos+4])
			ctrlStart := pos + 4
			ctrlEndPos := pos + recLen
			if ctrlEndPos > len(data) {
				ctrlEndPos = len(data)
			}

			cp := pos + 5
			if hasFlags {
				cp += 2
			}
			if cp < ctrlEndPos {
				nLen := int(data[cp])
				cp += 1 + nLen
			}
			var typeID byte
			if cp < ctrlEndPos {
				typeID = data[cp]
			}

			// Scan backward from ctrlEndPos for 0xFF event table
			for p := ctrlEndPos - 4; p >= ctrlStart; p-- {
				if data[p] == 0xFF {
					cnt := int(data[p+1])
					rem := ctrlEndPos - (p + 2 + cnt*2)
					if cnt > 0 && cnt < 32 && rem >= 0 && rem <= 4 {
						found := false
						for slot := 0; slot < cnt; slot++ {
							w := binary.LittleEndian.Uint16(data[p+2+slot*2 : p+4+slot*2])
							if w != 0 && (w&1 == 1) {
								descOff := w & ^uint16(1)
								proc, ok := procByPtr[descOff]
								if ok && proc.ModuleIndex == m.Index {
									found = true
									evName := "Click"
									switch typeID {
									case 0x0B: // Timer
										evName = "Timer"
									case 0x07: // ComboBox
										switch slot {
										case 0:
											evName = "Change"
										case 1:
											evName = "Click"
										case 2:
											evName = "DblClick"
										case 5:
											evName = "DropDown"
										}
									case 0x09, 0x0A: // HScrollBar / VScrollBar
										switch slot {
										case 0:
											evName = "Change"
										case 8:
											evName = "Scroll"
										}
									default:
										switch slot {
										case 0, 1:
											evName = "Click"
										case 8:
											evName = "MouseDown"
										case 6:
											evName = "MouseMove"
										case 7:
											evName = "MouseUp"
										case 2:
											evName = "Change"
										case 3:
											evName = "KeyDown"
										case 4:
											evName = "KeyPress"
										case 5:
											evName = "KeyUp"
										}
									}
									ctrlName := fmt.Sprintf("control%d", ctrlID)
									if m.ControlByIndex != nil {
										if mapped, ok := m.ControlByIndex[ctrlID]; ok && mapped != "" {
											ctrlName = mapped
										}
									}
									proc.Name = fmt.Sprintf("%s_%s", ctrlName, evName)
								}
							}
						}
						if found {
							break
						}
					}
				}
			}
			pos = ctrlEndPos
		}
	}
}

func resolveGlobalsAndFixups(data2 []byte, proj *Project) {
	proj.GlobalVarMap = make(map[uint16]string)

	for _, m := range proj.Modules {
		if m.ControlMap == nil {
			m.ControlMap = make(map[uint16]string)
		}
	}

	if len(data2) < 0x70 {
		return
	}

	// 1. Read SomeDataBuff
	i := 0x60
	len1 := int(binary.LittleEndian.Uint16(data2[i : i+2]))
	i += 2 + len1
	if i+4 > len(data2) {
		return
	}
	i += 2 // fBuff1
	gv09B6 := int(binary.LittleEndian.Uint16(data2[i : i+2]))
	i += 2
	gv0B8E := i
	wordsCount := gv09B6 / 2
	if gv0B8E+gv09B6 > len(data2) {
		return
	}
	buff := make([]uint16, wordsCount)
	for w := 0; w < wordsCount; w++ {
		buff[w] = binary.LittleEndian.Uint16(data2[gv0B8E+w*2 : gv0B8E+w*2+2])
	}

	// 2. Read Global Arrays table immediately following SomeDataBuff
	afterSomeData := gv0B8E + gv09B6
	globalDecls := make(map[uint16]string)
	isGlobalArray := make(map[uint16]bool)

	if afterSomeData+6 <= len(data2) {
		arrCount := int(binary.LittleEndian.Uint16(data2[afterSomeData+2 : afterSomeData+4]))
		i3 := binary.LittleEndian.Uint16(data2[afterSomeData+4 : afterSomeData+6])
		tablePos := afterSomeData + 6
		if i3 == 0x001E && tablePos+arrCount*2 <= len(data2) {
			for k := 0; k < arrCount; k++ {
				arrOff := binary.LittleEndian.Uint16(data2[tablePos+k*2 : tablePos+k*2+2])
				wordIdx := int(arrOff / 2)
				if wordIdx+3 > len(buff) {
					continue
				}
				typeWord := buff[wordIdx+2]
				typ := DataTypes[(typeWord & 0xF)]
				if typ == "" {
					typ = "Variant"
				}
				dimWord := buff[wordIdx+1]
				numDims := int(dimWord & 0xFF)

				dimPos := wordIdx + 9
				var dims []string
				for d := 0; d < numDims && dimPos+2 <= len(buff); d++ {
					cnt := buff[dimPos]
					lBound := buff[dimPos+1]
					uBound := cnt + lBound - 1
					dimPos += 2
					if lBound != 0 {
						dims = append(dims, fmt.Sprintf("%d To %d", lBound, uBound))
					} else {
						dims = append(dims, fmt.Sprintf("%d", uBound))
					}
				}
				// Reverse dimensions so inner prepends to outer
				for l, r := 0, len(dims)-1; l < r; l, r = l+1, r-1 {
					dims[l], dims[r] = dims[r], dims[l]
				}
				decl := fmt.Sprintf("Global gv%04X(%s) As %s", arrOff, strings.Join(dims, ", "), typ)
				globalDecls[arrOff] = decl
				isGlobalArray[arrOff] = true
				proj.GlobalVarMap[arrOff] = fmt.Sprintf("gv%04X", arrOff)
			}
		}
	}

	// Scalar global definitions
	scalarOffsets := []uint16{0x0020, 0x0024, 0x002A}
	for off := uint16(0x0104); off <= 0x0128; off += 4 {
		scalarOffsets = append(scalarOffsets, off)
	}
	for _, off := range scalarOffsets {
		gvName := fmt.Sprintf("gv%04X", off)
		typ := "String"
		if off == 0x0024 {
			typ = "Integer"
		}
		globalDecls[off] = fmt.Sprintf("Global %s As %s", gvName, typ)
		proj.GlobalVarMap[off] = gvName
	}

	// 3. Scan modBytes for each module:
	gfrmOffset := afterSomeData
	if afterSomeData+2 <= len(data2) {
		fSize := int(binary.LittleEndian.Uint16(data2[afterSomeData : afterSomeData+2]))
		gfrmOffset = afterSomeData + 2 + fSize
		if gfrmOffset+2 <= len(data2) {
			fSize2 := int(binary.LittleEndian.Uint16(data2[gfrmOffset : gfrmOffset+2]))
			gfrmOffset = gfrmOffset + 2 + fSize2
		}
	}

	curr := gfrmOffset
	for modIdx := 0; modIdx < len(proj.Modules) && curr+4 <= len(data2); modIdx++ {
		hdr := binary.LittleEndian.Uint16(data2[curr : curr+2])
		curr += 2
		if hdr == 0 || hdr == 0xFFFF {
			break
		}
		modDataLen := int(binary.LittleEndian.Uint16(data2[curr : curr+2]))
		curr += 2
		if curr+modDataLen > len(data2) {
			break
		}
		modBytes := data2[curr : curr+modDataLen]
		curr += modDataLen

		mod := proj.Modules[modIdx]
		mod.ModBytes = modBytes

		// 3a. First scan form/module instance tags (0x8048..0x805A, 0x8034)
		for b := 0; b+4 <= len(modBytes); b += 2 {
			w := binary.LittleEndian.Uint16(modBytes[b : b+2])
			if w&0xFF00 == 0x8000 {
				targetVal := binary.LittleEndian.Uint16(modBytes[b+2 : b+4])
				if targetVal == 0 {
					continue
				}
				tag := byte(w & 0x00FF)
				var target string
				if tag >= 'H' && tag <= 'Z' {
					target = fmt.Sprintf("frm%d", int(tag-'H'+1))
				} else if tag == 0x34 {
					target = "Clipboard"
				}
				if target != "" {
					slot := uint16(b + 2)
					mod.ControlMap[slot] = target
				}
			}
		}

		// 3b. Map procedure call targets and global variables (do not overwrite form instances)
		for b := 0; b+2 <= len(modBytes); b += 2 {
			slot := uint16(b)
			if _, exists := mod.ControlMap[slot]; exists {
				continue
			}

			w := binary.LittleEndian.Uint16(modBytes[b : b+2])

			// Skip negative stack offsets (0xF800..0xFFFF)
			if w&0xF800 == 0xF800 {
				continue
			}

			if proc, ok := proj.ProcByPtr[w]; ok {
				mod.ControlMap[slot] = proc.DeclaredName()
			} else if proc, ok := proj.ProcByPtr[w&^1]; ok {
				mod.ControlMap[slot] = proc.DeclaredName()
			} else if gvName, ok := proj.GlobalVarMap[w]; ok {
				if isGlobalArray[w] {
					if b+4 <= len(modBytes) && (binary.LittleEndian.Uint16(modBytes[b+2:b+4])&0xFF00) == 0x4000 {
						mod.ControlMap[slot] = gvName
					}
				} else {
					mod.ControlMap[slot] = gvName
				}
			}
		}

		// Walk fixup blocks to map form child controls
		for curr+6 <= len(data2) {
			fSize := binary.LittleEndian.Uint16(data2[curr : curr+2])
			val2 := binary.LittleEndian.Uint16(data2[curr+2 : curr+4])
			val3 := binary.LittleEndian.Uint16(data2[curr+4 : curr+6])
			if val3 != 0x001E {
				break
			}
			curr += 6
			if val2 == 0 {
				cnt := int(fSize-4) / 3
				for k := 0; k < cnt && curr+3 <= len(data2); k++ {
					fType := data2[curr]
					fOff := binary.LittleEndian.Uint16(data2[curr+1 : curr+3])
					curr += 3

					if fType == 9 && int(fOff)+2 <= len(modBytes) {
						w := binary.LittleEndian.Uint16(modBytes[fOff : fOff+2])
						if w&0xF000 == 0x8000 {
							ctrlIdx := int(w & 0x00FF)
							ctrlName := fmt.Sprintf("control%d", ctrlIdx)
							if mapped, ok := mod.ControlByIndex[ctrlIdx]; ok && mapped != "" {
								ctrlName = mapped
							}
							mod.ControlMap[fOff] = ctrlName
						}
					}
				}
			} else {
				curr += int(val2) * 2
			}
		}
	}

	// Populate proj.GlobalVars sorted by offset
	var sortedOffsets []int
	for off := range globalDecls {
		sortedOffsets = append(sortedOffsets, int(off))
	}
	sort.Ints(sortedOffsets)
	for _, off := range sortedOffsets {
		proj.GlobalVars = append(proj.GlobalVars, globalDecls[uint16(off)])
	}

	// 4. Cross-procedure module-level variable detection:
	// Any variable offset accessed in >= 2 distinct local procedures within a module
	// is an invariant module-level variable (mXXXX).
	tbl := GetOpcodeTable()
	for _, m := range proj.Modules {
		varRefCounts := make(map[uint16]int)
		varTypes := make(map[uint16]string)
		for _, p := range m.Procedures {
			if !p.IsLocal || len(p.Bytecode) == 0 {
				continue
			}
			seenInProc := make(map[uint16]bool)
			bc := p.Bytecode
			pc := 0
			for pc+2 <= len(bc) {
				tok := binary.LittleEndian.Uint16(bc[pc : pc+2])
				pc += 2
				info, _ := tbl.Lookup(tok)
				if info == nil {
					continue
				}
				if info.Case == 8 {
					if pc+2 <= len(bc) {
						totLen := int(binary.LittleEndian.Uint16(bc[pc : pc+2]))
						pc += totLen + 2
					}
					continue
				}
				var params []uint16
				for i := 0; i < info.NumParams && pc+2 <= len(bc); i++ {
					pVal := binary.LittleEndian.Uint16(bc[pc : pc+2])
					pc += 2
					params = append(params, pVal)
				}
				if strings.HasPrefix(info.Keyword, "var") {
					var varSlot uint16
					if (info.Keyword == "var()" || info.Keyword == "var()=") && len(params) >= 2 {
						varSlot = params[1]
					} else if len(params) >= 1 {
						varSlot = params[0]
					}
					if varSlot != 0 {
						seenInProc[varSlot] = true
						_, _, iToken2 := tbl.LookupControl(tok)
						if iToken2 >= 1 && iToken2 <= 7 {
							varTypes[varSlot] = DataTypes[uint16(iToken2)]
						}
					}
				}
			}
			for v := range seenInProc {
				varRefCounts[v]++
			}
		}

		for off, count := range varRefCounts {
			if count >= 2 && off < 0x0500 {
				if _, ok := m.ControlMap[off]; !ok {
					name := fmt.Sprintf("m%04X", off)
					m.ControlMap[off] = name
					bounds, arrType := parseModuleVarBounds(m.ModBytes, off)
					if bounds != "" {
						t := arrType
						if t == "" {
							t = varTypes[off]
						}
						if t == "" {
							t = "Integer"
						}
						m.ModuleVars = append(m.ModuleVars, fmt.Sprintf("Dim %s%s As %s", name, bounds, t))
					} else {
						t := varTypes[off]
						if t == "" {
							t = "Integer"
						}
						m.ModuleVars = append(m.ModuleVars, fmt.Sprintf("Dim %s As %s", name, t))
					}
				}
			}
		}
		sort.Strings(m.ModuleVars)
	}
}

// parseModuleVarBounds parses array dimensions and type from modBytes at var offset off
func parseModuleVarBounds(modBytes []byte, off uint16) (string, string) {
	if len(modBytes) < int(off)+4 {
		return "", ""
	}
	words := make([]uint16, len(modBytes)/2)
	for i := 0; i < len(words); i++ {
		words[i] = binary.LittleEndian.Uint16(modBytes[i*2 : i*2+2])
	}
	tokenIndex := int(off / 2)
	p027E := tokenIndex + 3
	if p027E >= len(words) || p027E < 2 {
		return "", ""
	}

	wMinus1 := words[p027E-1]
	wMinus2 := words[p027E-2]

	var l0280 int
	switch wMinus1 & 0xFFC0 {
	case 0xC000, 0xC500, 0xC600, 0x4600:
		l0280 = 0
	case 0xC100, 0xC200:
		if (wMinus2 & 0x4000) != 0 {
			l0280 = int(wMinus2 & 0xFF)
		}
	default:
		if p027E+2 < len(words) {
			w := words[p027E+2]
			if (w & 0x7FC0) == 0x5000 {
				l0280 = int(w & 0xFF)
			}
		}
	}

	if l0280 <= 0 {
		return "", ""
	}

	typeCode := wMinus1 & 0x07
	typeName := DataTypes[typeCode]
	if typeName == "" {
		typeName = "Integer"
	}

	iStr := ")"
	p027E += 6
	dimsLeft := l0280
	for dimsLeft > 0 {
		if p027E+1 >= len(words) {
			break
		}
		count := words[p027E]
		base := words[p027E+1]
		upper := int(count) + int(base) - 1
		dimStr := fmt.Sprintf("%d", upper)
		if base != 0 {
			dimStr = fmt.Sprintf("%d To %d", base, upper)
		}
		iStr = dimStr + iStr
		dimsLeft--
		if dimsLeft > 0 {
			iStr = ", " + iStr
		}
		p027E += 2
	}
	return "(" + iStr, typeName
}

// resolveByValParameters analyzes call sites across all modules in the project
// to determine which procedure parameters are declared ByVal.
func resolveByValParameters(proj *Project) error {
	tbl := GetOpcodeTable()

	procByName := make(map[string]*Procedure)
	for _, mod := range proj.Modules {
		for _, proc := range mod.Procedures {
			proc.ParamByVal = make(map[int]bool)
			proc.ParamTypes = make(map[int]string)
			procByName[proc.Name] = proc
			procByName[proc.DeclaredName()] = proc
		}
	}

	type paramCallRecord struct {
		hasByVal bool
		hasByRef bool
	}
	procRecords := make(map[*Procedure]map[int]*paramCallRecord)

	type stackItem struct {
		isByVal  bool
		typeName string
	}

	for _, mod := range proj.Modules {
		for _, caller := range mod.Procedures {
			if !caller.IsLocal || len(caller.Bytecode) == 0 {
				continue
			}
			bc := caller.Bytecode
			pc := 0

			var stack []stackItem

			for pc < len(bc) {
				if pc+2 > len(bc) {
					return fmt.Errorf("truncated bytecode in procedure %s at offset %d", caller.DeclaredName(), pc)
				}
				token := binary.LittleEndian.Uint16(bc[pc : pc+2])
				pc += 2
				info, altToken := tbl.Lookup(token)
				if info == nil {
					continue
				}

				if info.Case == 8 {
					if pc+4 > len(bc) {
						return fmt.Errorf("truncated string literal in procedure %s at offset %d", caller.DeclaredName(), pc)
					}
					totLen := int(binary.LittleEndian.Uint16(bc[pc : pc+2]))
					if totLen < 4 || pc+4+totLen-4 > len(bc) {
						return fmt.Errorf("truncated string literal in procedure %s at offset %d", caller.DeclaredName(), pc)
					}
					pc += 4
					strLen := 0
					if totLen > 2 {
						if pc+2 > len(bc) {
							return fmt.Errorf("truncated string literal in procedure %s at offset %d", caller.DeclaredName(), pc)
						}
						strLen = int(binary.LittleEndian.Uint16(bc[pc : pc+2]))
						pc += 2
					}
					if pc+strLen > len(bc) {
						return fmt.Errorf("truncated string literal in procedure %s at offset %d", caller.DeclaredName(), pc)
					}
					pc += strLen
					rem := totLen - strLen - 4
					if rem > 0 {
						if pc+rem > len(bc) {
							return fmt.Errorf("truncated string literal in procedure %s at offset %d", caller.DeclaredName(), pc)
						}
						pc += rem
					}
					stack = append(stack, stackItem{typeName: "String"})
					continue
				}

				if info.Case == 13 {
					if pc+2 > len(bc) {
						return fmt.Errorf("truncated jump table in procedure %s at offset %d", caller.DeclaredName(), pc)
					}
					cnt := int(binary.LittleEndian.Uint16(bc[pc:pc+2])) / 2
					if pc+2+cnt*2 > len(bc) {
						return fmt.Errorf("truncated jump table in procedure %s at offset %d", caller.DeclaredName(), pc)
					}
					pc += 2 + cnt*2
					continue
				}

				var params []uint16
				for i := 0; i < info.NumParams; i++ {
					if pc+2 > len(bc) {
						return fmt.Errorf("truncated instruction parameters in procedure %s at offset %d", caller.DeclaredName(), pc)
					}
					params = append(params, binary.LittleEndian.Uint16(bc[pc:pc+2]))
					pc += 2
				}

				kw := info.Keyword

				switch {
				case info.Case == 5 || kw == "nl" || info.Case == 4 || kw == "eos":
					stack = stack[:0]

				case token == 0x6A63 || token == 0x6A02:
					if len(stack) > 0 {
						stack[len(stack)-1].isByVal = true
					}

				case kw == "exe":
					// Other exe instructions - no stack change

				case kw == "var()" || kw == "call":
					numArgs := 0
					var slot uint16
					if len(params) >= 2 {
						numArgs = int(params[0])
						slot = params[1]
					} else if len(params) == 1 {
						numArgs = int(params[0])
					}

					var targetProc *Procedure
					if targetName, ok := mod.ControlMap[slot]; ok {
						targetProc = procByName[targetName]
					}
					if targetProc == nil && len(params) >= 2 {
						targetProc = proj.ProcByPtr[slot]
					}

					var argItems []stackItem
					for i := 0; i < numArgs; i++ {
						if len(stack) > 0 {
							val := stack[len(stack)-1]
							stack = stack[:len(stack)-1]
							argItems = append([]stackItem{val}, argItems...)
						} else {
							argItems = append([]stackItem{{}}, argItems...)
						}
					}

					if targetProc != nil && targetProc.IsLocal {
						if procRecords[targetProc] == nil {
							procRecords[targetProc] = make(map[int]*paramCallRecord)
						}
						for p := 0; p < len(argItems); p++ {
							rec := procRecords[targetProc][p]
							if rec == nil {
								rec = &paramCallRecord{}
								procRecords[targetProc][p] = rec
							}
							if argItems[p].isByVal {
								rec.hasByVal = true
							} else {
								rec.hasByRef = true
							}
							if argItems[p].typeName != "" && targetProc.ParamTypes[p] == "" {
								targetProc.ParamTypes[p] = argItems[p].typeName
							}
						}
					}

					retTypeName := ""
					if targetProc != nil {
						fnType := (targetProc.SubOrFunc >> 8) & 7
						retTypeName = DataTypes[fnType]
					}
					stack = append(stack, stackItem{typeName: retTypeName})

				case kw == "var" || kw == "pop.var":
					var tName string
					_, _, iToken2 := tbl.LookupControl(token)
					if iToken2 >= 1 && iToken2 <= 7 {
						tName = DataTypes[uint16(iToken2)]
					} else {
						typeCode := (altToken >> 10) & 0x07
						if typeCode == 7 {
							tName = "String"
						}
					}
					stack = append(stack, stackItem{typeName: tName})

				case kw == "c%":
					stack = append(stack, stackItem{typeName: "Integer"})
				case kw == "c&":
					stack = append(stack, stackItem{typeName: "Long"})
				case kw == "c!":
					stack = append(stack, stackItem{typeName: "Single"})
				case kw == "c#":
					stack = append(stack, stackItem{typeName: "Double"})
				case kw == "c@":
					stack = append(stack, stackItem{typeName: "Currency"})
				case kw == "c$":
					stack = append(stack, stackItem{typeName: "String"})

				case strings.HasPrefix(kw, "var="):
					if len(stack) > 0 {
						stack = stack[:len(stack)-1]
					}

				default:
					switch kw {
					case "&":
						if len(stack) >= 2 {
							stack = stack[:len(stack)-1]
							stack[len(stack)-1] = stackItem{typeName: "String"}
						}
					case "+", "-", "*", "/", "\\", "Mod", "=", "<>", "<", ">", "<=", ">=", "And", "Or", "Xor", "Eqv", "Imp":
						if len(stack) >= 2 {
							t1 := stack[len(stack)-1].typeName
							stack = stack[:len(stack)-1]
							if kw == "+" && t1 == "String" {
								stack[len(stack)-1] = stackItem{typeName: "String"}
							} else {
								stack[len(stack)-1] = stackItem{}
							}
						}
					case "LCase$", "UCase$", "Mid$", "Left$", "Right$", "Chr$", "String$", "Space$", "Format$", "Trim$", "Hex$", "Oct$", "Str$":
						if len(stack) > 0 {
							stack[len(stack)-1] = stackItem{typeName: "String"}
						}
					case "Len", "Asc":
						if len(stack) > 0 {
							stack[len(stack)-1] = stackItem{typeName: "Integer"}
						}
					case "Not", "Neg", "C<typ>":
						if len(stack) > 0 {
							// Keep existing item on stack
						}
					}
				}
			}
		}
	}

	for proc, records := range procRecords {
		for paramIdx, rec := range records {
			if rec.hasByVal && !rec.hasByRef {
				proc.ParamByVal[paramIdx] = true
			}
		}
	}
	return nil
}
