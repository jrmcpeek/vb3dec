package pcode

import (
	"sync"
)

// VB3 standard data types mapped from type codes (1..8).
var DataTypes = map[uint16]string{
	1: "Integer",
	2: "Long",
	3: "Single",
	4: "Double",
	5: "Currency",
	6: "Variant",
	7: "String",
	8: "String *",
}

// VB3 type conversion prefixes (1..7).
var TypeConv = map[uint16]string{
	1: "Int",
	2: "Lng",
	3: "Sng",
	4: "Dbl",
	5: "Cur",
	6: "Var",
	7: "Str",
}

// OpcodeInfo represents metadata for a 9-bit VB3 P-code token (0..511).
type OpcodeInfo struct {
	TokenID       uint16 // 0..511
	AltToken      uint16 // Full 16-bit flag token from VBDIS3X
	Keyword       string // Disassembly keyword or mnemonic
	KeyWordStrIdx int16  // Index in keyword pool
	Case          uint16 // Action category (TK_Case1_8bit & 0xF)
	NumParams     int    // Number of 16-bit argument words immediately following the opcode
	Flags         uint16 // Secondary token flags (mTokenFlags & 0xF)
	BitForward2   int    // (mTokenFlags & 0xF000) >> 12
	RawCase       uint16 // TK_Case1_8bit
	RawFlags      uint16 // mTokenFlags
}

// OpcodeTable holds the opcode and keyword definitions.
type OpcodeTable struct {
	tokens        [512]OpcodeInfo
	flagTokenData []uint16
}

var (
	defaultTable *OpcodeTable
	tableOnce    sync.Once
)

// GetOpcodeTable returns the singleton OpcodeTable initialized from native Go tables.
func GetOpcodeTable() *OpcodeTable {
	tableOnce.Do(func() {
		defaultTable = &OpcodeTable{
			tokens:        baseOpcodes,
			flagTokenData: flagTokenData[:],
		}
	})
	return defaultTable
}

// Lookup decodes a raw 16-bit P-code word token into its OpcodeInfo and AltToken.
func (t *OpcodeTable) Lookup(pToken uint16) (*OpcodeInfo, uint16) {
	if pToken == 0 {
		return &t.tokens[0], 0
	}
	idx := int(pToken) / 3
	if idx >= len(t.flagTokenData) {
		return nil, 0
	}
	altToken := t.flagTokenData[idx]
	tokenID := altToken & 0x1FF
	info := t.tokens[tokenID]
	info.AltToken = altToken
	return &info, altToken
}

// LookupControl returns the control bytes (iToken, iToken1, iToken2) for a given P-code token.
func (t *OpcodeTable) LookupControl(pToken uint16) (iToken, iToken1, iToken2 byte) {
	idx := int(pToken)
	if idx <= 0 || idx+1 >= len(controlTokenData) {
		return 0, 0, 0
	}
	return controlTokenData[idx-1], controlTokenData[idx], controlTokenData[idx+1]
}
