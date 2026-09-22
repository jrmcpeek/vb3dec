# `vb3dec` Technical Documentation

This directory contains technical documentation, reverse engineering research, and architectural references for the `vb3dec` Visual Basic 3 decompiler.

---

## Documentation Index

### 1. [Technical Audit: Legacy VBDIS3 Architecture & Reconstruction](audit_vbdis3.md)
A deep-dive technical paper analyzing the internal architecture and failure modes of Hans-Peter Diettrich's original 1996–1997 *Visual Basic Discompiler* (VBDIS 3.67e).
- **Core topics covered**:
  - The bit-rotation lookup bug in `MODULE10.BAS` that caused legacy tools to drop entire forms and code segments.
  - Form control event binding flaws, off-by-N indexing, and duplicate procedure collisions.
  - Physical 16-bit stack frame analysis (`modBytes`) vs. flawed Variant inference.
  - Windows-1252 ANSI encoding vs. UTF-8 corruption on high-ASCII prompt banners.
  - Reconstructing missing "dark matter" feature strings and procedures.
  - Eliminating opaque binary `.DAT` tables in favor of typed, native Go opcode tables.

### 2. [Known Limitations & Language Scope](limitations.md)
A detailed breakdown of the boundaries of `vb3dec`'s current decompilation coverage and known non-implemented Visual Basic 3.0 language features.
- **Core topics covered**:
  - Menu bar hierarchies (`Begin Menu`).
  - Control arrays and `Index As Integer` event parameters.
  - User-Defined Types (`Type ... End Type` / UDTs).
  - Data-bound controls and DAO / Jet database property serialization.
  - Obscure / legacy control structures (`GoSub ... Return`, `On <n> GoTo`).
  - Scope boundaries (16-bit NE vs 32-bit PE).

---

## Related Links
- [Root Project README](../README.md)
- [Test Fixtures (`test_input/`)](../test_input/)
