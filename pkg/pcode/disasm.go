package pcode

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"vb3dec/pkg/vbx"
	"vb3dec/pkg/win1252"
)

// EventParam is one parameter of an event procedure.
type EventParam struct {
	Name string
	Type string
}

// eventParamDefs lists the documented parameters of the built-in VB3 events.
var eventParamDefs = map[string][]EventParam{
	"MouseDown": {
		{"Button", "Integer"},
		{"Shift", "Integer"},
		{"X", "Single"},
		{"Y", "Single"},
	},
	"MouseMove": {
		{"Button", "Integer"},
		{"Shift", "Integer"},
		{"X", "Single"},
		{"Y", "Single"},
	},
	"MouseUp": {
		{"Button", "Integer"},
		{"Shift", "Integer"},
		{"X", "Single"},
		{"Y", "Single"},
	},
	"KeyDown": {
		{"KeyCode", "Integer"},
		{"Shift", "Integer"},
	},
	"KeyUp": {
		{"KeyCode", "Integer"},
		{"Shift", "Integer"},
	},
	"KeyPress": {
		{"KeyAscii", "Integer"},
	},
	"Unload": {
		{"Cancel", "Integer"},
	},
	"QueryUnload": {
		{"Cancel", "Integer"},
		{"UnloadMode", "Integer"},
	},
	"DragDrop": {
		{"Source", "Control"},
		{"X", "Single"},
		{"Y", "Single"},
	},
	"DragOver": {
		{"Source", "Control"},
		{"X", "Single"},
		{"Y", "Single"},
		{"State", "Integer"},
	},
	"LinkOpen": {
		{"Cancel", "Integer"},
	},
	"LinkError": {
		{"LinkErr", "Integer"},
	},
	"LinkExecute": {
		{"CmdStr", "String"},
		{"Cancel", "Integer"},
	},
	"Error": {
		{"DataErr", "Integer"},
		{"Response", "Integer"},
	},
	"Validate": {
		{"Action", "Integer"},
		{"Save", "Integer"},
	},
}

// standardMethods maps VB3 standard control/form method IDs to their names.
var standardMethods = map[uint16]string{
	0:  "PrintForm",
	1:  "LinkSend",
	2:  "AddItem",
	3:  "RemoveItem",
	4:  "Refresh",
	5:  "LinkPoke",
	6:  "LinkRequest",
	7:  "LinkExecute",
	8:  "GetText",
	9:  "GetData",
	10: "SetText",
	11: "SetData",
	12: "Clear",
	13: "GetFormat",
	14: "Arrange",
	15: "Show",
	16: "Hide",
	17: "EndDoc",
	18: "NewPage",
	19: "SetFocus",
	20: "Drag",
	21: "Move",
	22: "ZOrder",
	23: "Close",
	24: "Delete",
}

// formatProperty names the property a 16-bit property ID refers to. Property
// IDs 0xC000+n select property n of the model of the object they apply to;
// when that model is unknown, a generic mapping guesses the name.
func formatProperty(model *vbx.Model, propID uint16, isWrite bool, val string) string {
	if model != nil && propID&0xC000 == 0xC000 {
		if n := int(propID &^ 0xC000); n < len(model.Props) {
			if name := strings.TrimSpace(model.Props[n].Name); name != "" {
				return name
			}
		}
	}

	// Generic / fallback mapping
	switch propID {
	case 0xC000:
		return "Caption"
	case 0xC002:
		if isWrite || val == "True" || val == "False" {
			return "Enabled"
		}
		return "hWnd"
	case 0xC003:
		return "BackColor"
	case 0xC004:
		return "ForeColor"
	case 0xC005:
		return "Left"
	case 0xC006:
		return "Top"
	case 0xC007:
		return "Width"
	case 0xC008:
		return "Enabled"
	case 0xC009:
		return "Visible"
	case 0xC00A:
		return "MousePointer"
	case 0xC00B:
		return "Text"
	case 0xC00C:
		return "FontName"
	case 0xC00D:
		return "FontBold"
	case 0xC00E:
		return "FontItalic"
	case 0xC00F:
		return "FontStrikethru"
	case 0xC010:
		return "FontUnderline"
	case 0xC011:
		return "FontSize"
	case 0xC012:
		return "TabIndex"
	case 0xC013:
		return "ListCount"
	case 0xC014:
		return "ListIndex"
	case 0xC015:
		return "List"
	case 0xC016:
		return "BorderStyle"
	case 0xC017:
		return "TabStop"
	case 0xC018:
		return "Tag"
	case 0xC025:
		return "Command"
	case 0xC028:
		return "DeviceType"
	case 0xC02A:
		return "FileName"
	case 0xC02E:
		return "Visible"
	case 0xC031:
		return "KeyPreview"
	case 0xC032:
		return "ClipControls"
	case 0xC036:
		return "Value"
	case 0xC037:
		return "Min"
	case 0xC038:
		return "Max"
	default:
		return fmt.Sprintf("Prop_%04X", propID)
	}
}

// UnhandledOpcodeWarning records an opcode that was not explicitly handled
// by the decompiler and fell through to the default handler.
type UnhandledOpcodeWarning struct {
	ModuleName string
	ProcName   string
	PC         int
	TokenID    uint16
	AltToken   uint16
	Keyword    string
	Case       uint16
	Params     []uint16
}

// Disassembler decodes P-code byte streams into valid Visual Basic 3 source code.
type Disassembler struct {
	table    *OpcodeTable
	proj     *Project
	Warnings []UnhandledOpcodeWarning
}

// NewDisassembler creates a new P-code disassembler instance.
func NewDisassembler(proj *Project) *Disassembler {
	return &Disassembler{
		table: GetOpcodeTable(),
		proj:  proj,
	}
}

// DisassembleProcedure decompiles a single procedure into clean VB3 source code.
func (d *Disassembler) DisassembleProcedure(proc *Procedure) (string, error) {
	if !proc.IsLocal {
		return d.formatDeclaration(proc), nil
	}
	if len(proc.Bytecode) == 0 {
		return d.formatEmptyProcedure(proc), nil
	}

	var mod *Module
	if d.proj != nil {
		for _, m := range d.proj.Modules {
			if m.Index == proc.ModuleIndex {
				mod = m
				break
			}
		}
		if mod == nil {
			for _, m := range d.proj.Modules {
				for _, p := range m.Procedures {
					if p == proc {
						mod = m
						break
					}
				}
				if mod != nil {
					break
				}
			}
		}
	}

	state := &procDisasmState{
		proc:      proc,
		mod:       mod,
		proj:      d.proj,
		table:     d.table,
		bc:        proc.Bytecode,
		vars:      make(map[uint16]string),
		varTypes:  make(map[uint16]string),
		arrayVars: make(map[uint16]bool),
	}

	code, err := state.disassemble()
	if len(state.warnings) > 0 {
		d.Warnings = append(d.Warnings, state.warnings...)
	}
	return code, err
}

// DisassembleModule decompiles all procedures in a module into full VB3 module source code.
func (d *Disassembler) DisassembleModule(mod *Module) (string, error) {
	var sb strings.Builder

	// Module header
	sb.WriteString(fmt.Sprintf("' %s.bas\n", mod.Name))
	sb.WriteString("Option Explicit\n\n")

	if len(mod.ModuleVars) > 0 {
		for _, mv := range mod.ModuleVars {
			sb.WriteString(mv)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Separate declarations from procedures
	var decls []*Procedure
	var locals []*Procedure

	for _, p := range mod.Procedures {
		if p.IsLocal {
			locals = append(locals, p)
		} else {
			decls = append(decls, p)
		}
	}

	// Emit API Declarations
	for _, p := range decls {
		declText := d.formatDeclaration(p)
		if declText != "" {
			sb.WriteString(declText)
			sb.WriteString("\n")
		}
	}
	if len(decls) > 0 {
		sb.WriteString("\n")
	}

	// Emit Local Procedures
	for i, p := range locals {
		code, err := d.DisassembleProcedure(p)
		if err != nil {
			sb.WriteString(fmt.Sprintf("' Error decompiling %s: %v\n", p.Name, err))
			continue
		}
		sb.WriteString(code)
		if i < len(locals)-1 {
			sb.WriteString("\n\n")
		}
	}

	return sb.String(), nil
}

// DisassembleFormCode decompiles declarations and procedures for appending directly below End in a .FRM file.
func (d *Disassembler) DisassembleFormCode(mod *Module) (string, error) {
	var sb strings.Builder
	sb.WriteString("\nOption Explicit\n\n")

	if len(mod.ModuleVars) > 0 {
		for _, mv := range mod.ModuleVars {
			sb.WriteString(mv)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	var decls []*Procedure
	var locals []*Procedure

	for _, p := range mod.Procedures {
		if p.IsLocal {
			locals = append(locals, p)
		} else {
			decls = append(decls, p)
		}
	}

	for _, p := range decls {
		declText := d.formatDeclaration(p)
		if declText != "" {
			sb.WriteString(declText)
			sb.WriteString("\n")
		}
	}
	if len(decls) > 0 {
		sb.WriteString("\n")
	}

	for i, p := range locals {
		code, err := d.DisassembleProcedure(p)
		if err != nil {
			sb.WriteString(fmt.Sprintf("' Error decompiling %s: %v\n", p.Name, err))
			continue
		}
		sb.WriteString(code)
		if i < len(locals)-1 {
			sb.WriteString("\n\n")
		}
	}

	return sb.String(), nil
}

func (d *Disassembler) formatDeclaration(p *Procedure) string {
	procType := "Sub"
	retType := ""
	fnType := (p.SubOrFunc >> 8) & 7
	if fnType != 0 {
		procType = "Function"
		if dt, ok := DataTypes[fnType]; ok {
			retType = " As " + dt
		} else {
			retType = " As Variant"
		}
	}

	name := p.DeclaredName()

	lib := p.LibName
	if lib == "" {
		lib = "User"
	}
	alias := p.AliasName

	aliasClause := ""
	if alias != "" && alias != name {
		aliasClause = " Alias " + win1252.QuoteVBString(alias)
	}

	params := strings.Join(p.ExternParams, ", ")

	return fmt.Sprintf("Declare %s %s Lib %s%s (%s)%s", procType, name, win1252.QuoteVBString(lib), aliasClause, params, retType)
}

func (d *Disassembler) formatEmptyProcedure(p *Procedure) string {
	procType := "Sub"
	retType := ""
	if p.IsFunction() {
		procType = "Function"
		fnType := (p.SubOrFunc >> 8) & 7
		if dt, ok := DataTypes[fnType]; ok {
			retType = " As " + dt
		} else {
			retType = " As Variant"
		}
	}
	return fmt.Sprintf("%s %s ()%s\nEnd %s", procType, p.Name, retType, procType)
}

type procDisasmState struct {
	proc             *Procedure
	mod              *Module
	proj             *Project
	table            *OpcodeTable
	bc               []byte
	pc               int
	stack            []string
	lines            []string
	labels           map[int]string
	vars             map[uint16]string // offset -> variable name
	varTypes         map[uint16]string // offset -> variable type ("Integer", "Long", "String")
	retSlot          uint16            // Return value slot for functions
	hasRet           bool
	indent           int
	prevOpWas        bool
	ifCond           string                   // In-flight single-line If condition
	lastSingleLineIf bool                     // Tracks if the last emitted line was a single-line If
	singleLineElse   bool                     // In-flight single-line Else clause
	joinLine         bool                     // The next line continues the last one after a ":" separator
	inputFile        string                   // File number of an in-flight "Input #" or "Line Input #"
	inputVars        []string                 // Variables of an in-flight "Input #"
	printing         bool                     // A Print statement is in flight
	printFile        bool                     // The in-flight Print statement is "Print #"
	printDepth       int                      // Stack depth of the Print target; its items are above it
	printItems       string                   // Items and separators of the in-flight Print statement
	caseIndentStack  []bool                   // Tracks indentation inside Case blocks for nested Select Case
	arrayVars        map[uint16]bool          // offset -> true if variable is an array (e.g. ReDim'd)
	pendingAsType    string                   // In-flight "As <Type>" clause for ReDim
	pendingLet       bool                     // In-flight "Let" keyword for the next assignment
	warnings         []UnhandledOpcodeWarning // Warnings for unhandled opcodes
	// unhandledNotes holds, by index into lines, the unhandled tokens to
	// report in a comment before that line (index len(lines): at the end).
	// They are inserted when the procedure is rendered, so they never become
	// the last line that a single-line Else is appended to.
	unhandledNotes map[int][]string
	noteIndent     map[int]int
}

// targetModel returns the model of the object a property reference applies to:
// a global object, a form, or a control ("Text1", "frm2.Text1", or a control
// array element "Text1(i)"). It returns nil if the object's type is unknown.
func (s *procDisasmState) targetModel(target string) *vbx.Model {
	if i := strings.Index(target, "("); i >= 0 {
		target = target[:i]
	}
	if m, ok := vbx.ObjectModels[target]; ok {
		return m
	}
	if m := s.paramModel(target); m != nil {
		return m
	}
	if i := strings.Index(target, "."); i >= 0 {
		if owner := s.moduleNamed(target[:i]); owner != nil {
			return owner.ControlModels[target[i+1:]]
		}
		return nil
	}
	if s.mod != nil {
		if m := s.mod.ControlModels[target]; m != nil {
			return m
		}
	}
	if owner := s.moduleNamed(target); owner != nil && owner.IsForm {
		return owner.ControlModels[owner.Name]
	}
	if s.proj != nil {
		for _, m := range s.proj.Modules {
			if cm := m.ControlModels[target]; cm != nil {
				return cm
			}
		}
	}
	return nil
}

// paramModel returns the model of the form or control that call sites pass
// for the named parameter, or nil. Parameters are numbered in data slot
// order, as in the procedure's declaration.
func (s *procDisasmState) paramModel(name string) *vbx.Model {
	if s.proc == nil || len(s.proc.ParamModels) == 0 || !strings.HasPrefix(name, "p") {
		return nil
	}
	var offs []uint16
	for off, v := range s.vars {
		if strings.HasPrefix(v, "p") {
			offs = append(offs, off)
		}
	}
	sort.Slice(offs, func(i, j int) bool { return offs[i] < offs[j] })
	for i, off := range offs {
		if s.vars[off] == name {
			return s.proc.ParamModels[i]
		}
	}
	return nil
}

// objectTypeName returns the declared type of a parameter that receives a
// form or control with the given model.
func objectTypeName(m *vbx.Model) string {
	if m == vbx.BuiltinModels["Form"] {
		return "Form"
	}
	return "Control"
}

// moduleNamed returns the module a name refers to ("Me" is the current
// module), or nil.
func (s *procDisasmState) moduleNamed(name string) *Module {
	if name == "Me" {
		return s.mod
	}
	if s.proj != nil {
		for _, m := range s.proj.Modules {
			if strings.EqualFold(m.Name, name) {
				return m
			}
		}
	}
	return nil
}

func (s *procDisasmState) resolveControlName(target string, ctrlIdx int) string {
	var targetMod *Module
	if s.proj != nil {
		for _, m := range s.proj.Modules {
			if strings.EqualFold(m.Name, target) {
				targetMod = m
				break
			}
		}
	}
	if targetMod == nil {
		targetMod = s.mod
	}
	if targetMod != nil && targetMod.ControlByIndex != nil {
		if name, ok := targetMod.ControlByIndex[ctrlIdx]; ok && name != "" {
			return name
		}
	}
	return fmt.Sprintf("control%d", ctrlIdx)
}

func (s *procDisasmState) formatPropertyForTarget(target string, propID uint16, isWrite bool, val string) string {
	return formatProperty(s.targetModel(target), propID, isWrite, val)
}

func typeRank(t string) int {
	switch t {
	case "String":
		return 7
	case "Currency":
		return 6
	case "Double":
		return 5
	case "Single":
		return 4
	case "Long":
		return 3
	case "Integer":
		return 2
	case "Variant":
		return 1
	default:
		return 0
	}
}

func (s *procDisasmState) recordVarTypeByOffset(off uint16, typ string) {
	curr := s.varTypes[off]
	if typeRank(typ) > typeRank(curr) {
		s.varTypes[off] = typ
	}
}

func (s *procDisasmState) recordVarType(varName string, typ string) {
	clean := strings.TrimRight(varName, "$%&!#")
	for off, name := range s.vars {
		if name == clean {
			s.recordVarTypeByOffset(off, typ)
			return
		}
	}
}

func (s *procDisasmState) getVarTypeByName(varName string) (string, bool) {
	clean := strings.TrimRight(varName, "$%&!#")
	for off, name := range s.vars {
		if name == clean {
			t, ok := s.varTypes[off]
			return t, ok
		}
	}
	return "", false
}

func (s *procDisasmState) formatVarRef(offset uint16) string {
	name := s.getVarName(offset)
	if s.varTypes[offset] == "String" && (strings.HasPrefix(name, "p") || strings.HasPrefix(name, "l") || strings.HasPrefix(name, "gv")) {
		return name + "$"
	}
	return name
}

func (s *procDisasmState) push(expr string) {
	s.stack = append(s.stack, expr)
	s.prevOpWas = false
}

func (s *procDisasmState) pop() string {
	if len(s.stack) == 0 {
		return ""
	}
	val := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	return val
}

// flushStatement emits the statement left on the expression stack, such as a
// method call, and clears the stack. It reports whether the stack held one.
func (s *procDisasmState) flushStatement() bool {
	if len(s.stack) == 0 {
		return false
	}
	stmt := s.pop()
	if stmt != "" && !strings.HasPrefix(stmt, "'") && stmt != "End" && stmt != "End " {
		s.emitStatement(stmt)
	}
	s.stack = s.stack[:0]
	return true
}

func (s *procDisasmState) emitLine(line string) {
	if line == "" {
		return
	}
	if s.joinLine && !s.singleLineElse {
		// After a ":" separator. A single-line If stays single-line, so
		// that an Else after it still joins it.
		s.lines[len(s.lines)-1] += ": " + line
		s.joinLine = false
		return
	}
	s.joinLine = false
	if s.singleLineElse {
		if len(s.lines) > 0 {
			s.lines[len(s.lines)-1] += " Else " + line
		} else {
			pad := strings.Repeat("    ", s.indent)
			s.lines = append(s.lines, pad+"Else "+line)
		}
		s.singleLineElse = false
		s.lastSingleLineIf = false
		return
	}
	s.lastSingleLineIf = false
	pad := strings.Repeat("    ", s.indent)
	s.lines = append(s.lines, pad+line)
}

func (s *procDisasmState) emitStatement(stmt string) {
	if stmt == "" {
		return
	}
	if s.pendingLet {
		stmt = "Let " + stmt
		s.pendingLet = false
	}
	if s.ifCond != "" {
		s.emitLine(fmt.Sprintf("If %s Then %s", s.ifCond, stmt))
		s.ifCond = ""
		s.lastSingleLineIf = true
		return
	}
	s.emitLine(stmt)
}

// identifierKeyword matches opcode keywords that are plain Basic names, such
// as "Err", "Date$" or "TextWidth", as opposed to punctuation ("; ") or
// internal mnemonics with spaces or dots.
var identifierKeyword = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*[$%&!#@]?$`)

func isIdentifierKeyword(kw string) bool {
	return identifierKeyword.MatchString(kw)
}

// qualifyMember returns member applied to the object expression target, or
// member alone when it applies to the form itself.
func qualifyMember(target, member string) string {
	if target == "" || target == "Me" {
		return member
	}
	return target + "." + member
}

func formatArgList(args []string) string {
	for len(args) > 0 && strings.HasPrefix(args[len(args)-1], "<default") {
		args = args[:len(args)-1]
	}
	res := make([]string, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "<default") {
			res[i] = ""
		} else {
			res[i] = a
		}
	}
	return strings.Join(res, ", ")
}

func (s *procDisasmState) getVarName(offset uint16) string {
	if s.hasRet && offset == s.retSlot {
		return s.proc.Name
	}
	if name, ok := s.vars[offset]; ok {
		return name
	}

	// In Form/code modules: check mod.ControlMap first!
	if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
		mod := s.proj.Modules[s.proc.ModuleIndex-1]
		if ctrlName, ok := mod.ControlMap[offset]; ok {
			s.vars[offset] = ctrlName
			return ctrlName
		}
	}

	var name string
	isEvent := s.proc.IsEvent
	var modBytes []byte
	if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
		modBytes = s.proj.Modules[s.proc.ModuleIndex-1].ModBytes
	}
	// Global variables are named through the module's ControlMap; any other
	// slot is a parameter or local of this procedure.
	if isEvent {
		name = fmt.Sprintf("l%04X", offset)
	} else if (offset >= 0x0010 && offset < 0x0500) || int(offset)+2 <= len(modBytes) {
		if len(modBytes) > 0 && int(offset)+2 <= len(modBytes) {
			bp := int16(binary.LittleEndian.Uint16(modBytes[offset : offset+2]))
			if bp >= 6 && bp%2 == 0 && bp <= 60 {
				name = fmt.Sprintf("p%04X", offset)
			} else {
				name = fmt.Sprintf("l%04X", offset)
			}
		} else if offset < 0x0040 {
			name = fmt.Sprintf("p%04X", offset)
		} else {
			name = fmt.Sprintf("l%04X", offset)
		}
	} else {
		name = fmt.Sprintf("gv%04X", offset)
	}

	s.vars[offset] = name
	return name
}

// isLabelJump reports whether a keyword is a statement whose operand is a jump
// to a line label. The operands of "On Error" and "Resume" can also be
// special values, which lie outside the procedure and get no label.
func isLabelJump(kw string) bool {
	switch strings.TrimSpace(kw) {
	case "GoTo", "GoSub", "On Error", "Resume":
		return true
	}
	return false
}

// labelName names the line label at a jump operand. Jump operands are offsets
// in the procedure's code segment; labels are named by their bytecode offset.
func (s *procDisasmState) labelName(operand uint16) string {
	return fmt.Sprintf("L%04X", int(operand)-int(s.proc.CodeOffset))
}

// labelAt names the line label at a jump operand, if it lies within the
// procedure's bytecode.
func (s *procDisasmState) labelAt(operand uint16) (string, bool) {
	target := int(operand) - int(s.proc.CodeOffset)
	if target < 0 || target >= len(s.bc) {
		return "", false
	}
	return s.labelName(operand), true
}

// hasLabelAt reports whether a jump operand lies within the procedure.
func (s *procDisasmState) hasLabelAt(operand uint16) bool {
	_, ok := s.labelAt(operand)
	return ok
}

// addLabel records the line label at a jump operand if it lies within the
// procedure's bytecode.
func (s *procDisasmState) addLabel(operand uint16) {
	target := int(operand) - int(s.proc.CodeOffset)
	if target >= 0 && target < len(s.bc) {
		s.labels[target] = s.labelName(operand)
	}
}

// First pass: scan jump targets and variable references
func (s *procDisasmState) scanPass() {
	s.labels = make(map[int]string)
	pc := 0
	bc := s.bc

	isFunc := s.proc.IsFunction()
	var varRefs []uint16
	var assignedVars []uint16
	tokenTypes := make(map[uint16]string)

	for pc+2 <= len(bc) {
		token := binary.LittleEndian.Uint16(bc[pc : pc+2])
		info, altToken := s.table.Lookup(token)
		pc += 2

		if info == nil {
			continue
		}

		if info.Case == 8 {
			if pc+6 <= len(bc) {
				totLen := int(binary.LittleEndian.Uint16(bc[pc : pc+2]))
				pc += totLen + 2
			}
			continue
		}

		if info.Case == 13 {
			if pc+2 <= len(bc) {
				cnt := int(binary.LittleEndian.Uint16(bc[pc:pc+2])) / 2
				pc += 2
				for i := 0; i < cnt && pc+2 <= len(bc); i++ {
					s.addLabel(binary.LittleEndian.Uint16(bc[pc : pc+2]))
					pc += 2
				}
			}
			continue
		}

		typeCode := (altToken >> 10) & 0x07
		var params []uint16
		for i := 0; i < info.NumParams && pc+2 <= len(bc); i++ {
			pVal := binary.LittleEndian.Uint16(bc[pc : pc+2])
			pc += 2
			params = append(params, pVal)
		}

		if isLabelJump(info.Keyword) && len(params) > 0 {
			s.addLabel(params[0])
		}

		if strings.HasPrefix(info.Keyword, "var") {
			var varSlot uint16
			if (info.Keyword == "var()" || info.Keyword == "var()=") && len(params) >= 2 {
				varSlot = params[1]
			} else if len(params) >= 1 {
				varSlot = params[0]
			}

			if varSlot != 0 {
				_, _, iToken2 := s.table.LookupControl(token)
				var tName string
				if iToken2 >= 1 && iToken2 <= 7 {
					tName = DataTypes[uint16(iToken2)]
				}
				if tName != "" {
					if typeRank(tName) > typeRank(tokenTypes[varSlot]) {
						tokenTypes[varSlot] = tName
					}
				}
				if typeCode == 7 || iToken2 == 7 {
					s.recordVarTypeByOffset(varSlot, "String")
				}
				if strings.HasPrefix(info.Keyword, "var=") && info.Keyword != "var()=" {
					assignedVars = append(assignedVars, varSlot)
				} else {
					varRefs = append(varRefs, varSlot)
				}
			}
		}
	}

	// Return slot determination:
	// In VB3 compiled code, if a function assigns a return value, the assignment target
	// is allocated at a specific BP offset: BP - 22 - sizeof(ReturnType).
	// Integer: -24 (0xFFE8)
	// Long/Single: -26 (0xFFE6)
	// Double/Currency: -30 (0xFFE2)
	// Variant: -38 (0xFFDA)
	// String: 1
	retSlotBpMap := map[uint16]int16{
		1: -24, // Integer
		2: -26, // Long
		3: -26, // Single
		4: -30, // Double
		5: -30, // Currency
		6: -38, // Variant
		7: 1,   // String
	}

	var modBytes []byte
	if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
		modBytes = s.proj.Modules[s.proc.ModuleIndex-1].ModBytes
	}

	if isFunc && len(modBytes) > 0 {
		fnType := (s.proc.SubOrFunc >> 8) & 7
		if expectedBp, ok := retSlotBpMap[fnType]; ok {
			for _, v := range assignedVars {
				if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
					mod := s.proj.Modules[s.proc.ModuleIndex-1]
					if _, ok := mod.ControlMap[v]; ok {
						continue
					}
				}
				if int(v)+2 <= len(modBytes) {
					bp := int16(binary.LittleEndian.Uint16(modBytes[v : v+2]))
					if bp == expectedBp {
						s.retSlot = v
						s.hasRet = true
						break
					}
				}
			}
		}
	} else if isFunc && len(assignedVars) > 0 {
		var validCandidates []uint16
		for _, v := range assignedVars {
			if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
				mod := s.proj.Modules[s.proc.ModuleIndex-1]
				if _, ok := mod.ControlMap[v]; ok {
					continue
				}
			}
			validCandidates = append(validCandidates, v)
		}
		if len(validCandidates) > 0 {
			minAssigned := validCandidates[0]
			for _, v := range validCandidates {
				if v < minAssigned {
					minAssigned = v
				}
			}
			s.retSlot = minAssigned
			s.hasRet = true
		}
	}

	// Classify parameter variables:
	isEvent := s.proc.IsEvent
	numParams := int(s.proc.PubOrPriv >> 9)
	declaredParams := numParams
	var evParams []EventParam
	if isEvent {
		evParams = s.proc.EventParams
		numParams = len(evParams)
	}

	allVars := append(varRefs, assignedVars...)
	sort.Slice(allVars, func(i, j int) bool { return allVars[i] < allVars[j] })

	paramCount := 0
	for _, v := range allVars {
		if s.hasRet && v == s.retSlot {
			continue
		}
		if _, exists := s.vars[v]; exists {
			continue
		}
		if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
			mod := s.proj.Modules[s.proc.ModuleIndex-1]
			if ctrlName, ok := mod.ControlMap[v]; ok {
				s.vars[v] = ctrlName
				continue
			}
		}
		if isEvent {
			// Event parameters are passed ByRef as far pointers in Pascal
			// order, so parameter i of n is at BP + 6 + 4*(n-1-i). When the
			// descriptor's parameter count disagrees with the event's, the
			// event name is unreliable; assign names in slot order instead.
			paramIdx := -1
			if declaredParams != len(evParams) {
				if paramCount < len(evParams) && v >= 0x0010 && v < 0x0500 {
					paramIdx = paramCount
				}
			} else if len(modBytes) > 0 && int(v)+2 <= len(modBytes) {
				bp := int(int16(binary.LittleEndian.Uint16(modBytes[v : v+2])))
				if rel := bp - 6; rel >= 0 && rel%4 == 0 && rel/4 < len(evParams) {
					paramIdx = len(evParams) - 1 - rel/4
				}
			}
			if paramIdx >= 0 {
				s.vars[v] = evParams[paramIdx].Name
				s.recordVarTypeByOffset(v, evParams[paramIdx].Type)
				paramCount++
			} else {
				_ = s.getVarName(v)
				if tName, ok := tokenTypes[v]; ok {
					s.recordVarTypeByOffset(v, tName)
				}
			}
		} else {
			isParam := false
			if numParams > 0 && paramCount < numParams {
				if len(modBytes) > 0 && int(v)+2 <= len(modBytes) {
					bp := int16(binary.LittleEndian.Uint16(modBytes[v : v+2]))
					if bp >= 6 && bp%2 == 0 && bp <= 60 {
						isParam = true
					}
				} else if len(modBytes) == 0 {
					isParam = true
				}
			}

			if isParam && v >= 0x0010 && v < 0x0500 {
				s.vars[v] = fmt.Sprintf("p%04X", v)
				if tName, ok := tokenTypes[v]; ok {
					s.recordVarTypeByOffset(v, tName)
				}
				paramCount++
			} else {
				_ = s.getVarName(v)
				if tName, ok := tokenTypes[v]; ok {
					s.recordVarTypeByOffset(v, tName)
				}
			}
		}
	}

	if !isEvent && paramCount == 0 && numParams > 0 {
		targetParams := numParams
		if s.proc != nil && len(s.proc.ParamTypes) > 0 && len(s.proc.ParamTypes) < targetParams {
			targetParams = len(s.proc.ParamTypes)
		}
		candOff := uint16(0x0020)
		if s.hasRet && s.retSlot != 0 {
			candOff = s.retSlot + 2
		} else if len(allVars) > 0 {
			minOff := allVars[0]
			for _, v := range allVars {
				if v < minOff {
					minOff = v
				}
			}
			if minOff > uint16(targetParams*2) {
				candOff = minOff - uint16(targetParams*2)
			}
		}
		for paramCount < targetParams {
			for {
				if _, exists := s.vars[candOff]; !exists {
					break
				}
				candOff += 2
			}
			pName := fmt.Sprintf("p%04X", candOff)
			s.vars[candOff] = pName
			pType := "Variant"
			if s.proc != nil && s.proc.ParamTypes != nil && s.proc.ParamTypes[paramCount] != "" {
				pType = s.proc.ParamTypes[paramCount]
			}
			s.recordVarTypeByOffset(candOff, pType)
			paramCount++
			candOff += 2
		}
	}
}

func (s *procDisasmState) disassemble() (string, error) {
	s.scanPass()

	s.pc = 0
	s.indent = 1
	bc := s.bc

	for s.pc < len(bc) {
		startPC := s.pc

		if lbl, ok := s.labels[startPC]; ok {
			// A label starts a line, even after a ":" separator.
			s.lines = append(s.lines, lbl+":")
			s.joinLine = false
		}

		if s.pc+2 > len(bc) {
			break
		}

		token := binary.LittleEndian.Uint16(bc[s.pc : s.pc+2])
		info, altToken := s.table.Lookup(token)
		s.pc += 2

		if info == nil {
			s.emitLine(fmt.Sprintf("' <UNKNOWN_OPCODE: 0x%04X @ offset 0x%04X>", token, startPC))
			continue
		}

		if info.Case == 8 {
			if s.pc+6 <= len(bc) {
				totLen := int(binary.LittleEndian.Uint16(bc[s.pc : s.pc+2]))
				s.pc += 4 // totLen (2) + dummy (2)
				strLen := 0
				if totLen > 2 && s.pc+2 <= len(bc) {
					strLen = int(binary.LittleEndian.Uint16(bc[s.pc : s.pc+2]))
					s.pc += 2
				}

				var rawBytes []byte
				if s.pc+strLen <= len(bc) {
					rawBytes = bc[s.pc : s.pc+strLen]
					s.pc += strLen
				}
				rem := totLen - strLen - 4
				if rem > 0 && s.pc+rem <= len(bc) {
					s.pc += rem
				}

				s.push(win1252.FormatVBString(rawBytes))
			}
			continue
		}

		if info.Case == 13 {
			if s.pc+2 <= len(bc) {
				cnt := int(binary.LittleEndian.Uint16(bc[s.pc:s.pc+2])) / 2
				s.pc += 2
				s.pc += cnt * 2
			}
			continue
		}

		var params []uint16
		for i := 0; i < info.NumParams && s.pc+2 <= len(bc); i++ {
			params = append(params, binary.LittleEndian.Uint16(bc[s.pc:s.pc+2]))
			s.pc += 2
		}

		s.decodeInstruction(info, altToken, params, startPC)
		if info.Case == 4 || info.Keyword == "eos" {
			break
		}
	}

	return s.renderProcedure(), nil
}

func (s *procDisasmState) decodeInstruction(info *OpcodeInfo, altToken uint16, params []uint16, pc int) {
	kw := info.Keyword

	switch {
	case info.Case == 5 || kw == "nl" || info.Case == 4 || kw == "eos":
		// End of the line, or of the procedure.
		s.lastSingleLineIf = false
		s.pendingLet = false
		s.joinLine = false
		s.inputFile, s.inputVars = "", nil
		s.printing = false
		if !s.flushStatement() && s.ifCond != "" {
			s.emitLine(fmt.Sprintf("If %s Then", s.ifCond))
			s.indent++
			s.ifCond = ""
		}

	case info.TokenID >= 96 && info.TokenID <= 99:
		// A coordinate pair of a graphics method: "(x, y)", or the end point
		// of a Line, "-(x, y)", either one relative with "Step". Token 98
		// is "-", which must not be decoded as a subtraction.
		y := s.pop()
		x := s.pop()
		s.push(fmt.Sprintf("%s(%s, %s)", info.Keyword, x, y))

	case info.TokenID == 23:
		// The omitted lower bound of a ReDim dimension.
		s.push(defaultLowerBound)

	case kw == ": " || kw == ":":
		// Statement separator: the next statement continues this line.
		s.flushStatement()
		s.printing = false
		s.joinLine = len(s.lines) > 0

	case kw == "pop.var=":
		var propID uint16
		if len(params) > 0 {
			propID = params[0]
		}
		target := s.pop()
		val := s.pop()
		if target == "" {
			target = "Me"
		}
		if propID&0xC000 == 0x8000 {
			ctrlIdx := int(propID & 0x3FFF)
			ctrlName := s.resolveControlName(target, ctrlIdx)
			targetExpr := ctrlName
			if target != "" && target != "Me" {
				targetExpr = fmt.Sprintf("%s.%s", target, ctrlName)
			}
			stmt := fmt.Sprintf("%s = %s", targetExpr, val)
			s.emitStatement(stmt)
			break
		}
		propName := s.formatPropertyForTarget(target, propID, true, val)
		stmt := fmt.Sprintf("%s.%s = %s", target, propName, val)
		s.emitStatement(stmt)

	case kw == "pop.var":
		var propID uint16
		if len(params) > 0 {
			propID = params[0]
		}
		target := s.pop()
		if propID&0xC000 == 0x8000 {
			ctrlIdx := int(propID & 0x3FFF)
			ctrlName := s.resolveControlName(target, ctrlIdx)
			if target == "" || target == "Me" {
				s.push(ctrlName)
			} else {
				s.push(fmt.Sprintf("%s.%s", target, ctrlName))
			}
			break
		}
		if target == "" {
			target = "Me"
		}
		propName := s.formatPropertyForTarget(target, propID, false, "")
		s.push(fmt.Sprintf("%s.%s", target, propName))

	case kw == "pop.var()=":
		numArgs := 1
		propID := uint16(0)
		if len(params) > 0 {
			numArgs = int(params[0])
		}
		if len(params) > 1 {
			propID = params[1]
		}
		target := s.pop()
		var args []string
		for i := 0; i < numArgs && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		val := s.pop()
		if target == "" {
			target = "Me"
		}
		if propID&0xC000 == 0x8000 {
			ctrlIdx := int(propID & 0x3FFF)
			ctrlName := s.resolveControlName(target, ctrlIdx)
			targetExpr := ctrlName
			if target != "" && target != "Me" {
				targetExpr = fmt.Sprintf("%s.%s", target, ctrlName)
			}
			stmt := fmt.Sprintf("%s(%s) = %s", targetExpr, strings.Join(args, ", "), val)
			s.emitStatement(stmt)
			break
		}
		propName := s.formatPropertyForTarget(target, propID, true, val)
		stmt := fmt.Sprintf("%s.%s(%s) = %s", target, propName, strings.Join(args, ", "), val)
		s.emitStatement(stmt)

	case kw == "pop.var()":
		numArgs := 1
		propID := uint16(0)
		if len(params) > 0 {
			numArgs = int(params[0])
		}
		if len(params) > 1 {
			propID = params[1]
		}
		target := s.pop()
		var args []string
		for i := 0; i < numArgs && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		if propID&0xC000 == 0x8000 {
			ctrlIdx := int(propID & 0x3FFF)
			ctrlName := s.resolveControlName(target, ctrlIdx)
			targetExpr := ctrlName
			if target != "" && target != "Me" {
				targetExpr = fmt.Sprintf("%s.%s", target, ctrlName)
			}
			s.push(fmt.Sprintf("%s(%s)", targetExpr, strings.Join(args, ", ")))
			break
		}
		if target == "" {
			target = "Me"
		}
		propName := s.formatPropertyForTarget(target, propID, false, "")
		s.push(fmt.Sprintf("%s.%s(%s)", target, propName, strings.Join(args, ", ")))

	case kw == "met":
		methodID := uint16(0)
		argCount := 0
		if len(params) > 0 {
			methodID = params[0]
		}
		if len(params) > 1 {
			argCount = int(params[1])
		}
		var args []string
		for i := 0; i < argCount && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		target := s.pop()
		if target == "" {
			target = "Me"
		}
		normID := methodID
		if normID >= 0xC000 {
			normID = normID & 0x0FFF
		}
		methodName, ok := standardMethods[normID]
		if !ok {
			methodName, ok = standardMethods[methodID]
		}
		if !ok {
			methodName = fmt.Sprintf("Method_%d", methodID)
		}

		isFunction := methodID >= 0xC000 || normID == 8 || normID == 9 || normID == 13 || normID == 6
		var callExpr string
		formattedArgs := formatArgList(args)
		if isFunction {
			if formattedArgs != "" {
				callExpr = fmt.Sprintf("%s.%s(%s)", target, methodName, formattedArgs)
			} else {
				callExpr = fmt.Sprintf("%s.%s()", target, methodName)
			}
		} else {
			if formattedArgs != "" {
				callExpr = fmt.Sprintf("%s.%s %s", target, methodName, formattedArgs)
			} else {
				callExpr = fmt.Sprintf("%s.%s", target, methodName)
			}
		}
		s.push(callExpr)

	case kw == "eom" && s.printing:
		// Follows the file number of "Print #".

	case kw == "eom":
		if len(s.stack) > 0 {
			stmt := s.pop()
			s.emitStatement(stmt)
		}

	case info.TokenID == 55 || (kw == "End" && info.TokenID != 57):
		// Application statement: End (terminates application execution)
		s.emitStatement("End")

	case info.TokenID == 57 || kw == "End ":
		// Structural procedure return epilogue (End Sub / End Function).
		// The closing statement is emitted structurally by renderProcedure().
		return

	case strings.HasPrefix(kw, "var()="):
		numDims := 1
		var varOff uint16
		if len(params) >= 2 {
			numDims = int(params[0])
			varOff = params[1]
		} else if len(params) == 1 {
			varOff = params[0]
		}
		if numDims < 1 {
			numDims = 1
		}
		indices := make([]string, numDims)
		for d := numDims - 1; d >= 0; d-- {
			idx := s.pop()
			indices[d] = idx
		}
		val := s.pop()
		varName := strings.TrimRight(s.getVarName(varOff), "$")
		stmt := fmt.Sprintf("%s(%s) = %s", varName, strings.Join(indices, ", "), val)
		s.emitStatement(stmt)

	case kw == "As":
		if len(params) > 0 {
			if dt, ok := DataTypes[params[0]]; ok {
				s.pendingAsType = dt
			}
		}

	case strings.HasPrefix(kw, "var()") && len(params) >= 2 && params[0]&wholeArray != 0:
		// The array itself rather than one of its elements, as in
		// "Erase a" or "UBound(a, 1)".
		s.push(strings.TrimRight(s.getVarName(params[1]), "$"))

	case strings.HasPrefix(kw, "var()"):
		numBounds := 1
		var varOff uint16
		if len(params) >= 2 {
			numBounds = int(params[0])
			varOff = params[1]
		} else if len(params) == 1 {
			varOff = params[0]
		}

		isReDim := false
		if s.pc+2 <= len(s.bc) {
			nextTok := binary.LittleEndian.Uint16(s.bc[s.pc : s.pc+2])
			nextInfo, _ := s.table.Lookup(nextTok)
			if nextInfo != nil && (nextInfo.Keyword == "ReDim" || nextInfo.Keyword == "ReDim Preserve") {
				isReDim = true
			}
		}

		if isReDim {
			numDims := numBounds / 2
			if numDims < 1 {
				numDims = 1
			}
			dims := make([]string, numDims)
			for d := numDims - 1; d >= 0; d-- {
				upper := s.pop()
				lower := ""
				if len(s.stack) > 0 {
					lower = s.pop()
				}
				if lower != "" && lower != defaultLowerBound {
					dims[d] = fmt.Sprintf("%s To %s", lower, upper)
				} else {
					dims[d] = upper
				}
			}
			varName := strings.TrimRight(s.getVarName(varOff), "$")
			s.arrayVars[varOff] = true
			if s.pendingAsType != "" {
				s.recordVarTypeByOffset(varOff, s.pendingAsType)
			}
			s.push(fmt.Sprintf("%s(%s)", varName, strings.Join(dims, ", ")))
		} else {
			numDims := numBounds
			if numDims < 0 {
				numDims = 0
			}
			indices := make([]string, numDims)
			for d := numDims - 1; d >= 0; d-- {
				idx := s.pop()
				indices[d] = idx
			}
			varName := strings.TrimRight(s.getVarName(varOff), "$")
			s.push(fmt.Sprintf("%s(%s)", varName, strings.Join(indices, ", ")))
		}

	case kw == "ReDim" || kw == "ReDim Preserve":
		target := s.pop()
		stmt := fmt.Sprintf("%s %s", kw, target)
		if s.pendingAsType != "" {
			stmt = fmt.Sprintf("%s %s As %s", kw, target, s.pendingAsType)
			s.pendingAsType = ""
		}
		s.emitStatement(stmt)

	case strings.HasPrefix(kw, "var="):
		var off uint16
		if len(params) > 0 {
			off = params[0]
		}
		val := s.pop()
		if val == "" {
			val = "0"
		}
		if strings.HasPrefix(val, "Len(") || strings.HasPrefix(val, "Asc(") || strings.HasPrefix(val, "MsgBox") {
			s.recordVarTypeByOffset(off, "Integer")
		} else if strings.HasPrefix(val, `"`) || (strings.HasSuffix(val, "$") && !strings.Contains(val, " ")) {
			s.recordVarTypeByOffset(off, "String")
		} else if strings.HasPrefix(val, "Mid$(") || strings.HasPrefix(val, "Left$(") || strings.HasPrefix(val, "Right$(") || strings.HasPrefix(val, "Chr$(") || strings.HasPrefix(val, "LCase$(") || strings.HasPrefix(val, "UCase$(") || strings.HasPrefix(val, "Str$(") || strings.HasPrefix(val, "String$(") || strings.HasPrefix(val, "Space$(") || strings.HasPrefix(val, "Format$(") || strings.HasPrefix(val, "Trim$(") || strings.HasPrefix(val, "Hex$(") || strings.HasPrefix(val, "Oct$(") {
			s.recordVarTypeByOffset(off, "String")
		} else if _, err := strconv.Atoi(val); err == nil {
			s.recordVarTypeByOffset(off, "Integer")
		}

		rawName := s.getVarName(off)
		cleanTarget := strings.TrimRight(rawName, "$%&!#")
		if cleanTarget == s.proc.Name {
			fnType := (s.proc.SubOrFunc >> 8) & 7
			if dt, ok := DataTypes[fnType]; ok && dt != "" && dt != "Variant" {
				cleanVal := strings.TrimRight(val, "$%&!#")
				for o, name := range s.vars {
					if name == cleanVal {
						s.varTypes[o] = dt
						break
					}
				}
			}
		}
		varName := s.formatVarRef(off)
		s.emitStatement(fmt.Sprintf("%s = %s", varName, val))

	case strings.HasPrefix(kw, "var"):
		varName := "var"
		if len(params) > 0 {
			varName = s.formatVarRef(params[0])
		}
		s.push(varName)

	case kw == "pop.":
		// Object / property pop token in method dispatch (no-op on expression stack)

	case kw == "exe":
		// VM execution / frame setup / method dispatch glue (no-op on expression stack)

	case kw == "call" || kw == "Call ":
		// A procedure call statement, written with or without "Call".
		argCount := 0
		var targetProc *Procedure
		if len(params) >= 2 {
			argCount = int(params[0])
			procPtr := params[1]
			if s.proj != nil {
				targetProc = s.proj.ProcByPtr[procPtr]
			}
		} else if len(params) == 1 {
			argCount = int(params[0])
		}

		var args []string
		for i := 0; i < argCount && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}

		targetName := ""
		if targetProc != nil {
			targetName = targetProc.DeclaredName()
		} else if len(s.stack) > 0 {
			targetName = s.pop()
		}

		if targetName != "" {
			line := targetName
			formattedArgs := formatArgList(args)
			switch {
			case kw == "Call " && formattedArgs != "":
				line = fmt.Sprintf("Call %s(%s)", targetName, formattedArgs)
			case kw == "Call ":
				line = "Call " + targetName
			case formattedArgs != "":
				line = fmt.Sprintf("%s %s", targetName, formattedArgs)
			}
			s.emitStatement(line)
		}

	case kw == "c%":
		val := int(int16(altToken&0xFC00) >> 10)
		if len(params) > 0 {
			val = int(int16(params[0]))
		}
		s.push(fmt.Sprintf("%d", val))

	case kw == "c&":
		var val int32
		if len(params) >= 2 {
			val = int32(uint32(params[0]) | (uint32(params[1]) << 16))
		} else {
			val = int32(int16(altToken&0xFC00) >> 10)
		}
		s.push(fmt.Sprintf("%d&", val))

	case kw == "c!":
		var bits uint32
		if len(params) >= 2 {
			bits = uint32(params[0]) | (uint32(params[1]) << 16)
		}
		fVal := math.Float32frombits(bits)
		s.push(fmt.Sprintf("%g", fVal))

	case kw == "c#":
		var bits uint64
		if len(params) >= 4 {
			bits = uint64(params[0]) | (uint64(params[1]) << 16) |
				(uint64(params[2]) << 32) | (uint64(params[3]) << 48)
		}
		dVal := math.Float64frombits(bits)
		s.push(fmt.Sprintf("%g", dVal))

	case kw == "&Hc&":
		var val uint32
		if len(params) >= 2 {
			val = uint32(params[0]) | (uint32(params[1]) << 16)
		} else if len(params) == 1 {
			val = uint32(params[0])
		}
		s.push(fmt.Sprintf("&H%X&", val))

	case kw == "&Hc%":
		var val uint16
		if len(params) > 0 {
			val = params[0]
		}
		s.push(fmt.Sprintf("&H%X", val))

	case kw == "+" || kw == "*" || kw == "/" || kw == "\\" || kw == "^" || kw == "Mod" || kw == "&" || kw == "Like" || kw == "Is":
		r := s.pop()
		l := s.pop()
		if l == "" {
			l = "0"
		}
		if r == "" {
			r = "0"
		}
		s.push(fmt.Sprintf("%s %s %s", l, kw, r))

	case kw == "-":
		// Unary minus vs Binary subtraction
		if (altToken&0x01FF) == altUnaryMinus || len(s.stack) == 1 || s.prevOpWas {
			val := s.pop()
			if val == "" {
				val = "0"
			}
			s.push(fmt.Sprintf("-%s", val))
		} else if len(s.stack) >= 2 {
			r := s.pop()
			l := s.pop()
			s.push(fmt.Sprintf("%s - %s", l, r))
		}

	case kw == "=" || kw == "<>" || kw == "<" || kw == "<=" || kw == ">" || kw == ">=":
		r := s.pop()
		l := s.pop()
		for _, op := range []string{l, r} {
			cleanOp := strings.TrimRight(op, "$%&!#")
			if num, err := strconv.Atoi(cleanOp); err == nil {
				if (num >= 32765 && (kw == ">" || kw == ">=")) || num > 32767 || num < -32768 {
					other := l
					if op == l {
						other = r
					}
					s.recordVarType(other, "Long")
				}
			}
		}
		s.push(fmt.Sprintf("%s %s %s", l, kw, r))

	case kw == "And" || kw == "Or" || kw == "Xor" || kw == "Eqv" || kw == "Imp":
		r := s.pop()
		l := s.pop()
		s.push(fmt.Sprintf("%s %s %s", l, kw, r))

	case kw == "Not ":
		// Unary operator on the top of the stack. Explicit parentheses are
		// their own token, so the operand is emitted as compiled.
		arg := s.pop()
		if arg == "" {
			arg = "0"
		}
		s.push("Not " + arg)

	case kw == "Len":
		arg := s.pop()
		s.recordVarType(arg, "String")
		s.push(fmt.Sprintf("Len(%s)", arg))

	case kw == "Asc":
		arg := s.pop()
		s.recordVarType(arg, "String")
		s.push(fmt.Sprintf("Asc(%s)", arg))

	case info.TokenID == 238 || (info.Case == 2 && kw == "" && info.Flags == 8 && info.BitForward2 == 1):
		arg := s.pop()
		cleanArg := strings.TrimSpace(arg)
		if strings.HasPrefix(cleanArg, "(") && strings.HasSuffix(cleanArg, ")") {
			s.push(cleanArg)
		} else if cleanArg != "" {
			s.push(fmt.Sprintf("(%s)", cleanArg))
		}

	case kw == "Chr$" || kw == "Space$" || kw == "Hex$" || kw == "Oct$" ||
		kw == "Trim$" || kw == "LTrim$" || kw == "RTrim$" ||
		kw == "Rnd" || kw == "Int" || kw == "Fix" ||
		kw == "Abs" || kw == "Sgn" || kw == "Sqr" || kw == "Sin" || kw == "Cos" ||
		kw == "Tan" || kw == "LCase$" || kw == "UCase$" || kw == "Str$" || kw == "Val" ||
		kw == "DateValue" || kw == "TimeValue" || kw == "Day" || kw == "Month" || kw == "Year" ||
		kw == "Weekday" || kw == "Hour" || kw == "Minute" || kw == "Second" ||
		kw == "IsEmpty" || kw == "IsNull" || kw == "IsNumeric" || kw == "VarType" || kw == "Error$":
		if info.BitForward2 == 0 {
			s.push(kw)
		} else {
			arg := s.pop()
			s.push(fmt.Sprintf("%s(%s)", kw, arg))
		}

	case kw == "Mid$":
		// The expression forms have two or three arguments: Mid$(text, start)
		// and Mid$(text, start, length). The assignment forms are separate
		// opcodes (Flags == 1) and must not be decoded as a three-argument call.
		argCount := int(info.BitForward2)
		if info.Flags != 8 {
			// Assignment forms encode the destination expression and RHS in the
			// same argument vector; the RHS is pushed last.
			args := make([]string, 0, argCount)
			for i := 0; i < argCount && len(s.stack) > 0; i++ {
				args = append([]string{s.pop()}, args...)
			}
			if len(args) >= 3 {
				s.emitStatement(fmt.Sprintf("Mid$(%s) = %s", strings.Join(args[:len(args)-1], ", "), args[len(args)-1]))
			} else if kw != "" {
				s.push(kw)
			}
			break
		}
		if argCount < 2 || argCount > 3 {
			if kw != "" {
				s.push(kw)
			}
			break
		}
		args := make([]string, 0, argCount)
		for i := 0; i < argCount && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		if len(args) > 0 {
			s.recordVarType(args[0], "String")
		}
		s.push(fmt.Sprintf("Mid$(%s)", strings.Join(args, ", ")))

	case kw == "Left$" || kw == "Right$" || kw == "InStr":
		argCount := int(info.BitForward2)
		if argCount < 2 {
			argCount = 2
		}
		args := make([]string, 0, argCount)
		for i := 0; i < argCount && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		if kw == "InStr" {
			textIndex := 0
			if len(args) >= 3 {
				textIndex = 1
			}
			if textIndex < len(args) {
				s.recordVarType(args[textIndex], "String")
			}
		} else if len(args) > 0 {
			s.recordVarType(args[0], "String")
		}
		s.push(fmt.Sprintf("%s(%s)", kw, strings.Join(args, ", ")))

	case kw == "String$":
		charArg := s.pop()
		cntArg := s.pop()
		s.push(fmt.Sprintf("String$(%s, %s)", cntArg, charArg))

	case kw == "True" || kw == "False" || kw == "Null":
		s.push(kw)

	case strings.HasPrefix(kw, "<default"):
		s.push(kw)

	case kw == "If ":
		s.ifCond = strings.TrimSpace(s.pop())

	case kw == "For ":
		if info.TokenID == 62 {
			toVal := s.pop()
			startVal := s.pop()
			loopVar := s.pop()
			s.emitLine(fmt.Sprintf("For %s = %s To %s", loopVar, startVal, toVal))
		} else {
			step := s.pop()
			toVal := s.pop()
			startVal := s.pop()
			loopVar := s.pop()
			if step == "1" || step == "" {
				s.emitLine(fmt.Sprintf("For %s = %s To %s Step 1", loopVar, startVal, toVal))
			} else {
				s.emitLine(fmt.Sprintf("For %s = %s To %s Step %s", loopVar, startVal, toVal, step))
			}
		}
		s.indent++

	case kw == "Next":
		if s.indent > 1 {
			s.indent--
		}
		varName := s.pop()
		if varName != "" {
			s.emitLine(fmt.Sprintf("Next %s", varName))
		} else {
			s.emitLine("Next")
		}

	case kw == "Exit Sub" || kw == "Exit Function" || kw == "Exit For" || kw == "Exit Do" || strings.TrimSpace(kw) == "Exit":
		exitCmd := kw
		if strings.TrimSpace(kw) == "Exit" {
			if s.proc.IsFunction() {
				exitCmd = "Exit Function"
			} else {
				exitCmd = "Exit Sub"
			}
		}
		s.emitStatement(exitCmd)

	case kw == "GoTo " || kw == "GoTo":
		target := ""
		if len(params) > 0 {
			target = s.labelName(params[0])
		}
		s.emitStatement(fmt.Sprintf("GoTo %s", target))

	case kw == "GoSub " || kw == "GoSub":
		target := ""
		if len(params) > 0 {
			target = s.labelName(params[0])
		}
		s.emitLine(fmt.Sprintf("GoSub %s", target))

	case kw == "Return":
		s.emitLine("Return")

	case kw == "On Error " && len(params) == 1 && params[0] == onErrorResumeNext:
		s.emitStatement("On Error Resume Next")

	case kw == "On Error " && len(params) == 1 && s.hasLabelAt(params[0]):
		target, _ := s.labelAt(params[0])
		s.emitStatement("On Error GoTo " + target)

	case kw == "Resume" && len(params) == 1 && s.hasLabelAt(params[0]):
		target, _ := s.labelAt(params[0])
		s.emitStatement("Resume " + target)

	case (kw == "Resume" || kw == "Resume Next") && info.NumParams == 0:
		s.emitStatement(kw)

	case strings.TrimSpace(kw) == "Do":
		s.emitLine("Do")
		s.indent++

	case strings.HasPrefix(kw, "Do While"):
		cond := strings.TrimSpace(s.pop())
		s.emitLine(fmt.Sprintf("Do While %s", cond))
		s.indent++

	case strings.HasPrefix(kw, "Do Until"):
		cond := strings.TrimSpace(s.pop())
		s.emitLine(fmt.Sprintf("Do Until %s", cond))
		s.indent++

	case strings.HasPrefix(kw, "While"):
		cond := strings.TrimSpace(s.pop())
		s.emitLine(fmt.Sprintf("While %s", cond))
		s.indent++

	case strings.TrimSpace(kw) == "Loop":
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine("Loop")

	case strings.HasPrefix(kw, "Loop While"):
		if s.indent > 1 {
			s.indent--
		}
		cond := strings.TrimSpace(s.pop())
		s.emitLine(fmt.Sprintf("Loop While %s", cond))

	case strings.HasPrefix(kw, "Loop Until"):
		if s.indent > 1 {
			s.indent--
		}
		cond := strings.TrimSpace(s.pop())
		s.emitLine(fmt.Sprintf("Loop Until %s", cond))

	case strings.TrimSpace(kw) == "Wend":
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine("Wend")

	case strings.HasPrefix(kw, "Select Case"):
		expr := s.pop()
		s.emitLine(fmt.Sprintf("Select Case %s", expr))
		s.indent++
		s.caseIndentStack = append(s.caseIndentStack, false)

	case strings.TrimSpace(kw) == "Case":
		var caseExpr string
		if info.TokenID == 39 || info.BitForward2 == 2 {
			toVal := s.pop()
			fromVal := s.pop()
			if fromVal != "" {
				caseExpr = fmt.Sprintf("%s To %s", fromVal, toVal)
			} else {
				caseExpr = toVal
			}
		} else {
			caseExpr = s.pop()
		}
		if len(s.caseIndentStack) > 0 && s.caseIndentStack[len(s.caseIndentStack)-1] {
			s.indent--
		}
		s.emitLine(fmt.Sprintf("Case %s", caseExpr))
		s.indent++
		if len(s.caseIndentStack) > 0 {
			s.caseIndentStack[len(s.caseIndentStack)-1] = true
		}

	case info.TokenID >= 40 && info.TokenID <= 45:
		arg := s.pop()
		op := strings.TrimSpace(kw)
		s.push(fmt.Sprintf("%s %s", op, arg))

	case strings.TrimSpace(kw) == "Case Else":
		if len(s.caseIndentStack) > 0 && s.caseIndentStack[len(s.caseIndentStack)-1] {
			s.indent--
		}
		s.emitLine("Case Else")
		s.indent++
		if len(s.caseIndentStack) > 0 {
			s.caseIndentStack[len(s.caseIndentStack)-1] = true
		}

	case strings.TrimSpace(kw) == "End Select":
		if len(s.caseIndentStack) > 0 {
			wasIndented := s.caseIndentStack[len(s.caseIndentStack)-1]
			s.caseIndentStack = s.caseIndentStack[:len(s.caseIndentStack)-1]
			if wasIndented {
				s.indent--
			}
		}
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine("End Select")

	case kw == "Unload " || kw == "Unload":
		target := s.pop()
		if target == "" && s.proc != nil && s.proc.ModuleIndex > 1 && s.proj != nil && s.proc.ModuleIndex <= len(s.proj.Modules) {
			target = s.proj.Modules[s.proc.ModuleIndex-1].Name
		}
		if target != "" {
			s.emitStatement(fmt.Sprintf("Unload %s", target))
		} else {
			s.emitStatement("Unload")
		}

	case kw == "Load " || kw == "Load":
		target := s.pop()
		s.emitStatement(fmt.Sprintf("Load %s", target))

	case kw == "Randomize":
		if info.BitForward2 > 0 && len(s.stack) > 0 {
			seed := s.pop()
			s.emitStatement(fmt.Sprintf("Randomize %s", seed))
		} else {
			s.emitStatement("Randomize")
		}

	case kw == "DoEvents":
		s.emitStatement("DoEvents")

	case kw == "Beep":
		s.emitStatement("Beep")

	case kw == "#":
		s.push("#" + s.pop())

	case kw == "Open " && len(params) == 1 && fileModes[params[0]] != "":
		recLen := ""
		if info.TokenID == 143 { // With a "Len =" clause
			recLen = s.pop()
		}
		fileNum := s.pop()
		stmt := fmt.Sprintf("Open %s For %s As %s", s.pop(), fileModes[params[0]], fileNum)
		if recLen != "" {
			stmt += " Len = " + recLen
		}
		s.emitStatement(stmt)

	case kw == "Close" && len(params) == 1:
		fileNums := make([]string, params[0])
		for i := len(fileNums) - 1; i >= 0; i-- {
			fileNums[i] = s.pop()
		}
		s.emitStatement(strings.TrimSpace("Close " + strings.Join(fileNums, ", ")))

	case (kw == "Get" || kw == "Put") && info.BitForward2 == 3:
		// Get #file, record, variable
		variable := s.pop()
		record := s.pop()
		s.emitStatement(fmt.Sprintf("%s %s, %s, %s", kw, s.pop(), record, variable))

	case info.TokenID == 128:
		// Get #file, , variable
		variable := s.pop()
		s.emitStatement(fmt.Sprintf("Get %s, , %s", s.pop(), variable))

	case info.TokenID == 103:
		// Starts "Input #file, ..." or "Line Input #file, ...".
		s.inputFile = s.pop()
		s.inputVars = nil

	case info.TokenID == 130:
		// A variable of "Input #file, ...".
		s.inputVars = append(s.inputVars, s.pop())

	case info.TokenID == 104:
		// Ends "Input #file, ...".
		s.emitStatement(fmt.Sprintf("Input %s, %s", s.inputFile, strings.Join(s.inputVars, ", ")))
		s.inputFile, s.inputVars = "", nil

	case kw == "Line Input ":
		s.emitStatement(fmt.Sprintf("Line Input %s, %s", s.inputFile, s.pop()))
		s.inputFile = ""

	case kw == "v0":
		// Starts a Print statement, or a graphics method, on the object
		// expression below it.
		s.startPrint(false)

	case info.TokenID == 91:
		// Starts "Print #file, ..." on the file number below it.
		s.startPrint(true)

	case s.printing && (info.TokenID == 112 || info.TokenID == 113 || info.TokenID == 115 || info.TokenID == 116):
		// A Print item followed by its separator, ";" or ","; 112 and 113
		// follow Tab() and Spc().
		if len(s.stack) > s.printDepth {
			s.printItems += s.pop()
		}
		s.printItems += strings.TrimSpace(kw) + " "

	case s.printing && kw == "eol":
		// Ends a Print statement, after its last item if any.
		if len(s.stack) > s.printDepth {
			s.printItems += s.pop()
		}
		s.endPrint()

	case s.printing && info.TokenID >= 92 && info.TokenID <= 95:
		// v1 to v4: the number of optional arguments of a graphics method,
		// which are already on the stack.

	case s.printing && info.TokenID == 133 && len(params) == 1 && params[0] == lineBoxFilled:
		// Line [(x1, y1)]-(x2, y2), color, BF
		args := s.graphicsArgs()
		stmt := qualifyMember(s.pop(), "Line") + " "
		for i, a := range args {
			if i > 0 && !strings.HasPrefix(a, "-") {
				stmt += ", "
			}
			stmt += a
		}
		s.emitStatement(stmt + ", BF")

	case s.printing && kw == "Circle":
		// Circle (x, y), radius[, color[, start, end[, aspect]]]
		args := s.graphicsArgs()
		s.emitStatement(qualifyMember(s.pop(), "Circle") + " " + formatArgList(args))

	case kw == "fin" && len(params) == 2 && runtimeFunctions[params[0]] != "":
		// A call to a function of the run-time library.
		args := make([]string, params[1])
		for i := len(args) - 1; i >= 0; i-- {
			args[i] = s.pop()
		}
		s.push(fmt.Sprintf("%s(%s)", runtimeFunctions[params[0]], strings.Join(args, ", ")))

	case s.printing && kw == "eop":
		// Ends a Print statement after a separator, without a new line.
		s.endPrint()

	case kw == "Let ":
		// "Let" comes before the tokens of the assignment it introduces.
		s.pendingLet = true

	case info.Flags == 3 && simpleStatements[kw]:
		args := make([]string, info.BitForward2)
		for i := len(args) - 1; i >= 0; i-- {
			args[i] = s.pop()
		}
		stmt := strings.TrimSpace(kw)
		if len(args) > 0 {
			stmt += " " + strings.Join(args, ", ")
		}
		s.emitStatement(stmt)

	case kw == "Timer":
		s.push("Timer")

	case kw == "Print":
		arg := s.pop()
		s.emitStatement(fmt.Sprintf("Print %s", arg))

	case kw == "C<typ>":
		// Coercion token (preserves top of stack)

	case strings.TrimSpace(kw) == "End If":
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine("End If")

	case strings.TrimSpace(kw) == "Else":
		if s.lastSingleLineIf {
			s.singleLineElse = true
			break
		}
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine("Else")
		s.indent++

	case strings.HasPrefix(kw, "ElseIf"):
		cond := strings.TrimSpace(s.pop())
		if s.indent > 1 {
			s.indent--
		}
		s.emitLine(fmt.Sprintf("ElseIf %s Then", cond))
		s.indent++

	case kw == "MsgBox":
		argCount := len(s.stack)
		if argCount > 3 {
			argCount = 3
		}
		var args []string
		for i := 0; i < argCount; i++ {
			args = append([]string{s.pop()}, args...)
		}
		formattedArgs := formatArgList(args)
		if info.Case == 2 {
			if formattedArgs != "" {
				s.push(fmt.Sprintf("MsgBox(%s)", formattedArgs))
			} else {
				s.push("MsgBox()")
			}
		} else {
			line := "MsgBox"
			if formattedArgs != "" {
				line = fmt.Sprintf("MsgBox %s", formattedArgs)
			}
			s.emitStatement(line)
		}

	case info.Flags == 8 && info.BitForward2 > 0 && kw != "":
		argCount := info.BitForward2
		var args []string
		for i := 0; i < argCount && len(s.stack) > 0; i++ {
			args = append([]string{s.pop()}, args...)
		}
		s.push(fmt.Sprintf("%s(%s)", kw, formatArgList(args)))

	case info.Flags == 9 && info.BitForward2 == 0 && isIdentifierKeyword(kw):
		// Graphics method without arguments (Cls, Scale) on the object
		// expression below it: "Picture1.Cls", or "Cls" for the form itself.
		s.emitStatement(qualifyMember(s.pop(), kw))

	case info.Flags == 10 && info.BitForward2 > 0 && isIdentifierKeyword(kw):
		// Graphics method that returns a value (TextWidth, TextHeight, Point):
		// the object expression is below its arguments.
		args := make([]string, info.BitForward2)
		for i := len(args) - 1; i >= 0; i-- {
			args[i] = s.pop()
		}
		target := s.pop()
		s.push(fmt.Sprintf("%s(%s)", qualifyMember(target, kw), strings.Join(args, ", ")))

	case info.Flags == 4 && info.BitForward2 == 0 && isIdentifierKeyword(kw):
		// Function or value without arguments: Err, Erl, FreeFile, Date$,
		// Time$, Now, CurDir$, Dir$, Command$, Nothing, ...
		s.push(kw)

	default:
		if kw != "" && kw != "?" {
			s.push(kw)
		}
		modName := ""
		if s.mod != nil {
			modName = s.mod.Name
		}
		warn := UnhandledOpcodeWarning{
			ModuleName: modName,
			ProcName:   s.proc.Name,
			PC:         pc,
			TokenID:    info.TokenID,
			AltToken:   altToken,
			Keyword:    kw,
			Case:       info.Case,
			Params:     params,
		}
		s.warnings = append(s.warnings, warn)
		s.noteUnhandled(info, pc)
		log.Printf("[WARN] %s.%s @ 0x%04X: unhandled opcode token=%d (0x%04X) alt=0x%04X kw=%q case=%d params=%v",
			modName, s.proc.Name, pc, info.TokenID, info.TokenID, altToken, kw, info.Case, params)
	}
}

// startPrint starts a Print statement on the target on top of the stack.
func (s *procDisasmState) startPrint(file bool) {
	s.printing = true
	s.printFile = file
	s.printDepth = len(s.stack)
	s.printItems = ""
}

// endPrint emits the in-flight Print statement.
func (s *procDisasmState) endPrint() {
	items := strings.TrimSpace(s.printItems)
	target := s.pop()
	stmt := qualifyMember(target, "Print")
	if s.printFile {
		stmt = "Print " + target + ","
	}
	if items != "" {
		stmt += " " + items
	}
	s.printing = false
	s.emitStatement(stmt)
}

// graphicsArgs pops the arguments of the in-flight graphics method, which are
// above its target object on the stack.
func (s *procDisasmState) graphicsArgs() []string {
	n := len(s.stack) - s.printDepth
	if n < 0 {
		n = 0
	}
	args := make([]string, n)
	for i := n - 1; i >= 0; i-- {
		args[i] = s.pop()
	}
	s.printing = false
	return args
}

// defaultLowerBound stands for the omitted lower bound of a ReDim dimension.
const defaultLowerBound = "<default lower bound>"

// lineBoxFilled is the operand of a Line statement that draws a filled box,
// "BF". It is inferred from EMPIRE.EXE, which draws its color swatches so;
// the operands of plain lines and of unfilled boxes ("B") are unknown.
const lineBoxFilled = 2

// runtimeFunctions names the functions of the run-time library that the
// "fin" token calls, by its first operand.
var runtimeFunctions = map[uint16]string{
	0x8043: "LoadPicture",
}

// fileModes names the file modes of Open statements by their operand.
var fileModes = map[uint16]string{
	1:  "Input",
	2:  "Output",
	4:  "Random",
	8:  "Append",
	32: "Binary",
}

// onErrorResumeNext is the operand of "On Error Resume Next"; other
// operands of "On Error" are jump targets.
const onErrorResumeNext = 0xFFFE

// wholeArray is set in the dimension count of an array reference that names
// the whole array instead of an element.
const wholeArray = 0x8000

// simpleStatements are the statements whose arguments are a fixed number
// of expressions, listed after the keyword and separated by commas.
var simpleStatements = map[string]bool{
	"ChDir":    true,
	"ChDrive":  true,
	"Erase":    true,
	"Error":    true,
	"FileCopy": true,
	"Kill ":    true,
	"MkDir":    true,
	"Reset":    true,
	"RmDir":    true,
	"SetAttr":  true,
}

// UnhandledMarker starts the comment that marks code the decompiler could
// not decode, so that it can be found in the output.
const UnhandledMarker = "' vb3dec: unhandled P-code: "

// noteUnhandled records an unhandled token for a comment before the next
// emitted line.
func (s *procDisasmState) noteUnhandled(info *OpcodeInfo, pc int) {
	if s.unhandledNotes == nil {
		s.unhandledNotes = make(map[int][]string)
		s.noteIndent = make(map[int]int)
	}
	at := len(s.lines)
	if s.joinLine {
		at-- // The next statement continues the last line.
	}
	if _, ok := s.noteIndent[at]; !ok {
		s.noteIndent[at] = s.indent
	}
	s.unhandledNotes[at] = append(s.unhandledNotes[at],
		fmt.Sprintf("%q (token %d @ 0x%04X)", info.Keyword, info.TokenID, pc))
}

// writeUnhandledNote writes the comment for the unhandled tokens noted before line i.
func (s *procDisasmState) writeUnhandledNote(sb *strings.Builder, i int) {
	notes := s.unhandledNotes[i]
	if len(notes) == 0 {
		return
	}
	sb.WriteString(strings.Repeat("    ", s.noteIndent[i]))
	sb.WriteString(UnhandledMarker)
	sb.WriteString(strings.Join(notes, ", "))
	sb.WriteString("\n")
}

func (s *procDisasmState) renderProcedure() string {
	var sb strings.Builder

	isFunc := s.proc.IsFunction()
	procType := "Sub"
	retType := ""

	if isFunc {
		procType = "Function"
		fnType := (s.proc.SubOrFunc >> 8) & 7
		if dt, ok := DataTypes[fnType]; ok {
			retType = " As " + dt
		} else {
			retType = " As Variant"
		}
	}

	var paramList []string
	var localVars []string

	var sortedOffsets []uint16
	for off := range s.vars {
		sortedOffsets = append(sortedOffsets, off)
	}
	sort.Slice(sortedOffsets, func(i, j int) bool { return sortedOffsets[i] < sortedOffsets[j] })

	isEvent := s.proc.IsEvent
	for _, ep := range s.proc.EventParams {
		paramList = append(paramList, fmt.Sprintf("%s As %s", ep.Name, ep.Type))
	}

	paramIdx := 0
	for _, off := range sortedOffsets {
		vName := s.vars[off]
		if s.hasRet && vName == s.proc.Name {
			continue
		}
		tName := s.varTypes[off]
		if strings.HasPrefix(vName, "p") {
			if !isEvent {
				if m := s.proc.ParamModels[paramIdx]; m != nil && (tName == "" || tName == "Variant") {
					tName = objectTypeName(m)
				}
				if tName == "" {
					tName = "Variant"
				}
				arrSuffix := ""
				if s.arrayVars[off] {
					arrSuffix = "()"
				}
				byValPrefix := ""
				if s.proc != nil && s.proc.ParamByVal != nil && s.proc.ParamByVal[paramIdx] {
					byValPrefix = "ByVal "
				}
				paramList = append(paramList, fmt.Sprintf("%s%s%s As %s", byValPrefix, vName, arrSuffix, tName))
				paramIdx++
			}
		} else if strings.HasPrefix(vName, "l") {
			declKw := "Dim"
			if s.proc != nil && s.proj != nil && s.proc.ModuleIndex > 0 && s.proc.ModuleIndex <= len(s.proj.Modules) {
				modBytes := s.proj.Modules[s.proc.ModuleIndex-1].ModBytes
				if len(modBytes) > 0 && int(off)+2 <= len(modBytes) {
					bp := int16(binary.LittleEndian.Uint16(modBytes[off : off+2]))
					if bp == 0 {
						declKw = "Static"
					}
				}
			}
			typeStr := ""
			if tName != "" && tName != "Variant" {
				typeStr = " As " + tName
			}
			arrSuffix := ""
			if s.arrayVars[off] {
				arrSuffix = "()"
			}
			localVars = append(localVars, fmt.Sprintf("    %s %s%s%s", declKw, vName, arrSuffix, typeStr))
		}
	}

	paramsStr := strings.Join(paramList, ", ")
	sb.WriteString(fmt.Sprintf("%s %s (%s)%s\n", procType, s.proc.Name, paramsStr, retType))

	for _, lv := range localVars {
		sb.WriteString(lv)
		sb.WriteString("\n")
	}

	for i, line := range s.lines {
		s.writeUnhandledNote(&sb, i)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	s.writeUnhandledNote(&sb, len(s.lines))

	sb.WriteString(fmt.Sprintf("End %s", procType))

	return sb.String()
}
