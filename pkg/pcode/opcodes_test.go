package pcode_test

import (
	"testing"

	"vb3dec/pkg/pcode"
)

func TestOpcodeTableIntegrity(t *testing.T) {
	tbl := pcode.GetOpcodeTable()
	if tbl == nil {
		t.Fatal("Failed to get opcode table")
	}

	// 1. Lookup 0 (newline token)
	info0, alt0 := tbl.Lookup(0)
	if info0 == nil || info0.Keyword != "nl" || alt0 != 0 {
		t.Errorf("Lookup(0) expected 'nl' and alt 0, got %+v, alt %d", info0, alt0)
	}

	// 2. Lookup known token from flag table
	// pToken = 21 * 3 = 63 -> altToken 0x000E -> tokenID 14: "var()"
	infoVar, altVar := tbl.Lookup(63)
	if infoVar == nil || infoVar.Keyword != "var()" || altVar != 0x000E {
		t.Errorf("Lookup(63) expected 'var()' and alt 0x000E, got %+v, alt 0x%04X", infoVar, altVar)
	}

	// 3. Lookup invalid/out of range pToken
	infoOOB, _ := tbl.Lookup(0xFFFF)
	if infoOOB != nil {
		t.Errorf("Expected nil for out of bounds pToken 0xFFFF, got %+v", infoOOB)
	}
}
