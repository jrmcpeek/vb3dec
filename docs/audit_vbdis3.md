# Technical Audit: Legacy VBDIS3 Architecture and Reconstruction in vb3dec

## Executive Summary

This audit examines Hans-Peter Diettrich's *Visual Basic Discompiler* (VBDIS 3.67e): why it decompiled the protected sample `FF.EXE` incompletely, omitting 70 procedures across four forms, and why its output needed substantial manual fixes before the 16-bit VB3 compiler would accept it.

The missing code comes from a bit-rotation mismatch during relocation traversal in `MODULE10.BAS`, combined with fixed segment assumptions. `vb3dec` avoids both and recovers all 191 bytecode procedures in the executable's 10 modules and 14 code segments, including those containing the feature strings `mimic`, `morph`, and `esper`.

---

## 1. Table Traversal & Loop Bounds Flaws

### 1.1 The Critical Bit-Rotation Lookup Bug (Dropped Modules / Forms)
In `VBDIS3.67e`, procedure descriptors are indexed into an in-memory lookup table (`NamesLookUpTbl`) and subsequently matched against relocation chains to determine their code segment.

In `MODULE10.BAS` (`sub0986`), procedure descriptors in Segment 3 are loaded and indexed using `RotateRight3`:
```vb
l00A2 = RotateRight3(l00A4)
Get hVBFile, VBAbsOffs(l00A2), l00AC
NamesLookUpTbl(l00A2 + gc0E24) = gSubroutines
```
`RotateRight3` shifts the 16-bit descriptor pointer right by 3 and moves its bits 1–2, which select the descriptor's segment, into bits 13–14:
```vb
Function RotateRight3(pv0048%) As Integer
  RotateRight3 = (UDiv(pv0048, 8) And &H1FFF&) + ((pv0048 And 6) * &H1000)
End Function
```

However, in `Applyreloc1`, when traversing the NE relocation chain to bind each procedure to its executable code segment (`segm0`), the lookup index is computed **without** applying `RotateRight3`:
```vb
l0052 = Uint32(l004C.Size_M13F4)
l0046 = l0054 + (l0052 \ 8) - 4
If NamesLookUpTbl(l0046) <> gc0E1A Then MakeErrBeep
...
Do
  If NamesLookUpTbl(l0046) <> gc0E1A Then ShowErrMsg mc0062
  g_TKStruct(NamesLookUpTbl(l0046 + gc0E24)).segm0 = l0048
  Get hVBFile, curSegOffs + l0052, l0050
  l0052 = Uint32(l0050)
  l0046 = l0054 + (l0052 \ 8) - 4
Loop Until l0050 = &HFFFF
```

**Consequence:**
- When a descriptor pointer has segment bits set (`pointer And 6 <> 0`), the two indexes differ.
- For example, a relocation chain entry at offset `0x09B6` belongs to the descriptor at `0x09B6 - 0x26 = 0x0990`, whose pointer is `0x0996`.
- In `sub0986`, `RotateRight3(&H0996)` yields `&H6132` (segment bits `&H6000`).
- In `Applyreloc1`, `(&H09B6 \ 8) - 4` yields `&H0132` (no segment bits).
- `NamesLookUpTbl(&H0132)` is `0` (not `gc0E1A`).
- The condition `If NamesLookUpTbl(l0046) <> gc0E1A` fails immediately, calls `MakeErrBeep`, and **aborts the entire relocation chain**.
- `g_TKStruct(...).segm0` is never set (remains `0`).
- When `DecompileSubroutine` subsequently runs:
  ```vb
  If g_TKStruct(Subroutine).segm0 Then
    SeekToSegment g_TKStruct(Subroutine).segm0
    DecompSubroutine
  End If
  ```
  It silently skips every procedure with `segm0 == 0`.
- **Impact:** In executables with multiple code segments (like `FF.EXE`), all 70 procedures in Form 1, Form 2, Form 3, and Form 4 were completely omitted from decompilation.

### 1.2 Undersized Array Bounds
In `MODULE10.BAS` (`DoDecompilation`):
```vb
ScanGlobalData
ReDim NamesLookUpTbl(curSegSize \ 8)
```
When Segment 3 has `curSegSize = 12424` (`12424 \ 8 = 1553`), keys with segment bits, such as `&H6132` (24882), exceed the array bounds. In a standard VB3 runtime environment without error suppression, this triggers a fatal `Subscript out of range` error.

### 1.3 Segment Masking in `VBAbsOffs`
In `MODULE17.BAS`:
```vb
Function VBAbsOffs(ByVal Value As Integer) As Long
  FirstThreeByte = (Value \ &H2000) And 3
  VBAbsOffs = 1 + GetSegOffset(FirstThreeByte + VBStartSeg) + CLng(Value And &H1FFF) * 8
End Function
```
The segment index calculation masks with `3` (`And 3`), implicitly assuming that descriptors can never span more than 4 segments (`VBStartSeg` to `VBStartSeg + 3`).

### 1.4 Hardcoded Segment Iteration and Error Suppression
In `MODULE10.BAS`:
```vb
'*** New***
Applyreloc1 VBStartSeg
'  For i_frm = 3 To 6
'    If (Segs(i_frm).Flags And RELOCINFO) = 0 Then Exit For
'    Applyreloc1 i_frm
'  Next
```
Relocation processing was hardcoded solely to `VBStartSeg`, commenting out multi-segment relocation iteration. Combined with extensive use of `On Error Resume Next` in `MODULE2.BAS` and `MODULE6.BAS`, failures to resolve descriptors or offsets failed silently without diagnostics.

### 1.5 Control Event Binding Off-by-N Bug (Duplicate Procedures & Misassigned Controls)
In `MODULE4.BAS` and `MODULE16.BAS`, `VBDIS3.67e` attempted to associate form controls with their event procedures:
- Control names were generated sequentially (`control1`, `control2`, ...), but the mapping between control records in the form resource stream and procedure descriptor offsets in Segment 3 relied on an inaccurate global array index (`m000E` and `gFrmCount2`).
- **Duplicate Subroutines (`Sub control16_Click` generated twice):**
  In legacy decompiled output for Form 5:
  - Line 5: `Sub control16_Click ()`
  - Line 167: `Sub control16_Click ()`
  Visual Basic 3 strictly prohibits duplicate procedure names in the same module; this causes a fatal compilation error: `"Ambiguous name detected"`.
- **Misassigned Event Handlers:**
  - Control 19 in Form 5 is `Begin ComboBox control19`. The subroutine contained `Select Case control19.Text`—the Click event of `control19`. `VBDIS3` mislabeled it as `control16_Click`.
  - Control 20 is `Begin CommandButton control20` (the "Buy" button), prompting `"Are you sure you want to buy 1..."`. `VBDIS3` misnamed it as `control22_Click`, which is actually a `Label` (`Begin Label control22` displaying item price).
  - Control 12 is `Begin CommandButton control12` (the "Exit" button), executing `Unload frm5` and `frm1.Show`. `VBDIS3` misnamed it as `control3_Click` (where control 3 is a `Label`).
- **Resolution in `vb3dec`:**
  The `vb3dec` engine directly reads the event binding table located at the tail of each form and control record in the form's `RT_RCDATA` resource stream (`0xFF, count`, followed by one word per event slot). Each slot indexes the event list of the control's model in `VBRUN300.DLL` or its `.VBX`, and a slot word with bit 0 set refers to the event procedure's descriptor in Segment 3. This binds events directly to their true controls (`control19_Click`, `control20_Click`, `control12_Click`) and names them after the actual event, eliminating collisions.

### 1.6 Form Event Code Disconnection (`frmX.bas` vs `.FRM` Files)
In authentic Visual Basic 3.0:
- A form's code does **not** exist in an external `.BAS` file.
- All form-level event handlers (`Sub Form_Load()`, `Sub control19_Click()`, etc.) and form-level `Dim` variables are saved **directly inside the `.FRM` file itself**, immediately following the form definition's closing `End` statement.
- If control event handlers like `Sub control19_Click()` are placed in an external `.BAS` module, the VB3 compiler treats them as ordinary disconnected subroutines; the events never fire when controls are clicked or changed.

**Legacy VBDIS3 Flaw:**
- `VBDIS3` extracted visual form structures into `.FRM` files, but dumped disassembled P-code procedures into separate `.bas` files (`frm5.bas`..`frm9.bas`).
- Crucially, `VBDIS3` never stitched the event code back into the `.FRM` files, and never included `frmX.bas` in the project file (`.MAK`).
- As a result, opening the project in VB3 loaded completely "dead" forms with non-functional controls.
- **Resolution in `vb3dec`:** The engine appends each form's decompiled procedures directly to the bottom of its corresponding `.FRM` file below `End`, allowing the forms to be tested in VB3, while also emitting standalone `.bas` files for modern editor browsing.

### 1.7 Global Definitions (`main.txt`), Host Path Leakage, and IDE Layout
In `MODULE20.BAS` (`Sub Create_MAK_File`), `VBDIS3` constructed project `.MAK` files with several severe architectural quirks:
- **`main.txt` as a Code Module:**
  VBDIS3 dumped project-wide global definitions into `main.txt` and literally wrote `main.txt` into the `.MAK` file. While VB3's `.MAK` loader treats any file not ending in `.FRM` or `.VBX` as Basic code regardless of extension, standard VB3 conventions require `.BAS` modules. `vb3dec` replaces this with a canonical `Globals.bas` module.
- **Hardcoded Absolute Host System Paths for VBXs:**
  `VBDIS3` inspected the running machine's `C:\WINDOWS\system\` directory during decompilation and hardcoded `C:\WINDOWS\system\MCI.VBX` into the `.MAK` file. On any machine without that exact path, VB3 fails to load the project. `vb3dec` reads the custom control names from the executable and writes them as relative paths (`MCI.VBX`).
- **`ProjWinSize` / `ProjWinShow` IDE Artifacts:**
  In `MODULE20.BAS`, `VBDIS3` opened `autoload.mak` (the VB3 IDE's default project template on the host machine) and scraped `ProjWinSize=152,402,248,215`. These coordinates are not present in compiled executables; they are user-preference window positions for the VB3 Project Window. `vb3dec` writes the same values as fixed defaults rather than reading them from the host machine.

### 1.8 Local Variable Declarations

`VBDIS3.67e` writes its internal type byte as a comment after each local declaration, and leaves loop counters untyped. In `control12_Click` of `frm6`:
```vb
Dim l0110 As Variant ' 86
Dim l0114 ' 80
...
l0110 = gv0074(m001A, 25) + 14
For l0114 = 1 To 100 Step 1
```
- `l0110` receives a value converted by token `0x0EB0`, which converts an `Integer` to a `Variant`; the type byte `0x86` is the local variable flag `0x80` with type `0x06` (`Variant`).
- `l0114` gets no type rule, so its type byte stays `0x80` and the declaration has no type, although its loop bounds are integers.

The procedure's stack frame in the module data (`modBytes` in `RT_RCDATA` 2) places `l0114` at BP − 40 and `l0110` at BP − 38, so `l0114` occupies 2 bytes: it is an `Integer`.

**Resolution in `vb3dec`:** `vb3dec` infers types from the data types recorded in variable references and from how values are used, such as numeric loop bounds, and writes no type comments. It declares `l0114 As Integer`. Like `VBDIS3`, it reads `0x0EB0` as a conversion to `Variant`, so `l0110` is declared without a type.

---

## 2. P-Code Block Walking & Procedure Boundary Analysis

### 2.1 Fixed Descriptor Sizes vs. Relocation Chains
In Visual Basic 3 executables, procedures do not terminate arbitrarily:
- Procedure descriptors in Segment 3 explicitly record:
  - `codeOff` (word 12, byte offset 24): Start offset in target code segment.
  - `codeSize` (word 18, byte offset 36): End offset / allocation ceiling in target code segment.
  - `isExtOrLocal` (word 11, byte offset 22): Bit 0 indicates whether the descriptor is a local procedure (`1`) or an external API declaration (`0`).
- The actual target code segment index is specified in the NE relocation records located at `FileOffset + FileLength` of Segment 3.
- Each relocation entry points to the head of a linked list located at offset `+0x26` within the 48-byte procedure descriptor, chaining from one descriptor to the next until `0xFFFF`.

### 2.2 Token Stream Boundary Conditions
In `MODULE6.BAS` (`CodeDecompile`):
- Procedure blocks end on explicit exit tokens:
  - Opcode Case 4: `Exit Sub`, `End Sub`, `Exit Function`, `End Function`.
  - Opcode Case 5 (`nl`): Newline marker, which validates that the expression stack (`ESP`) is empty.
- Premature termination occurred in `VBDIS3` when encountering unknown tokens or unhandled operands: `VBDIS3` jumped to `ERR_TK_Unknown` and advanced until token type `0x30` or bailed out, corrupting procedure output.

---

## 3. Syntactic Incompatibilities in Legacy Output

Examining the raw output generated by `VBDIS3.67e` against valid Visual Basic 3 syntax highlights why the legacy output failed compilation:

### 3.1 Inline Comments Inside Parameter Lists
`VBDIS3` emitted internal parameter type codes as inline comments within procedure signature parentheses:
```diff
--- Legacy VBDIS3 Output (Module1.bas)
+++ Valid Visual Basic 3 Syntax
-Function fn006F (p00A4 As String ' 47) As Integer ' 1
+Function fn006F (p00A4 As String) As Integer ' 1

--- Legacy VBDIS3 Output (frm8.bas)
+++ Valid Visual Basic 3 Syntax
-Sub sub0AA6 (p0046 As Integer ' 41)
+Sub sub0AA6 (p0046 As Integer)
```
In Visual Basic 3.0, a comment (`' ...`) inside parameter parentheses is a syntax error. This pattern occurred across **hundreds** of procedure signatures.

### 3.2 ANSI Windows-1252 vs. UTF-8 String Corruption (High-ASCII Literals)

**The Defect:**
In 16-bit Windows 3.1 and Visual Basic 3.0, all strings—both in form resource streams (`RT_RCDATA`) and compiled P-code bytecode (Opcode Case 8)—were stored as single-byte ANSI strings encoded in Windows-1252 (CP-1252 / Western European).

`VBDIS3.67e` authored its output files by dumping raw 8-bit bytes directly to disk with zero character encoding awareness. Under modern development environments and version control systems (Git) that expect UTF-8:
- Standalone bytes `0x80..0xFF` violate UTF-8 decoding rules and render as garbled replacement characters (`\uFFFD` / ``) or cause parser re-encoding corruptions.
- When developers previously attempted manual fixes, they frequently truncated or deleted these strings because their text editors displayed them as invalid characters (`fn00BE = ">??????"` truncated to `fn00BE = ">"`).

**Case Study: AOL Chat Room Banners (`fn00BE` and `fn018F` in `Module1.bas`):**
A prominent example of this defect occurs in `Module1.bas`:

1. **`fn00BE` (Segment 4):**
   - Raw P-Code bytes: `3E 97 97 BB 9B BB`
   - Windows-1252 byte-by-byte mapping:
     - `0x3E` = `>` (ASCII greater-than)
     - `0x97` = `—` (`U+2014`, Em dash)
     - `0x97` = `—` (`U+2014`, Em dash)
     - `0xBB` = `»` (`U+00BB`, Right-pointing double angle quotation mark)
     - `0x9B` = `›` (`U+203A`, Single right-pointing angle quotation mark)
     - `0xBB` = `»` (`U+00BB`, Right-pointing double angle quotation mark)
   - Emitted by `VBDIS3`: Raw bytes `3E 97 97 BB 9B BB` (rendered as `>??????` or `>` in UTF-8).
   - True Decoded Text: `">——»›»"` (right-pointing prompt banner).

2. **`fn018F` (Segment 4):**
   - Raw P-Code bytes: `AB 8B AB 97 97 3C`
   - Windows-1252 byte-by-byte mapping:
     - `0xAB` = `«` (`U+00AB`, Left-pointing double angle quotation mark)
     - `0x8B` = `‹` (`U+2039`, Single left-pointing angle quotation mark)
     - `0xAB` = `«` (`U+00AB`, Left-pointing double angle quotation mark)
     - `0x97` = `—` (`U+2014`, Em dash)
     - `0x97` = `—` (`U+2014`, Em dash)
     - `0x3C` = `<` (ASCII less-than)
   - Emitted by `VBDIS3`: Raw bytes `AB 8B AB 97 97 3C` (rendered as `<` in UTF-8).
   - True Decoded Text: `"«‹«——<"` (left-pointing prompt banner).

3. **Semantic Purpose in Code:**
   In `frm8.bas` (`sub0AA6`), the battle notification engine concatenates these two functions around a player's combat readiness message:
   ```vb
   l006C = fn008B(fn018F() + " " + gv005E(p0046) + " is ready [" + Str$(gv0044(p0046, 1)) + "/" + Str$(gv0044(p0046, 2)) + "] " + fn00BE())
   ```
   In 1990s America Online (AOL 2.5/3.0) chat rooms, this generated the stylized announcement banner:
   ```text
   «‹«——< Cecil is ready [100/100] >——»›»
   ```

**Resolution in `vb3dec`:**
The `vb3dec` engine introduces a dedicated zero-dependency ANSI Windows-1252 transcoding package (`pkg/win1252`):
- All string literals in P-code (Opcode Case 8) and form property streams (captions, control texts, font names) are decoded through `win1252.Decode` and `win1252.FormatVBString`.
- High-ASCII characters (`0x80..0xFF`) are transcoded to their Unicode equivalents, so the decompiler works on valid text, and `win1252.Encode` converts them back when the files are written.
- Emitted `.bas`, `.FRM`, and `.MAK` files are Windows-1252, which the VB3 IDE reads, so `fn00BE` is written as the original bytes `3E 97 97 BB 9B BB`. Modern editors must open them as Windows-1252 to show these characters, or the files can be written in UTF-8 for viewing with `-encoding utf-8`.

### 3.3 Leftover Temporary Scrap Files
`VBDIS3` created temporary scratch files during decompilation (`VDM101.tmp`, `VDM102.tmp`, etc.) that were left on disk.

---

## 4. Missing Feature Strings

The feature strings `mimic`, `morph`, and `esper` were largely missing from the legacy output.

### 4.1 Root Cause
`VBDIS3.67e` emitted only 121 procedures (`Module1` and forms 5–9). The 70 procedures of forms 1–4, in segments 5–9, were dropped by the bit-rotation bug described in Section 1.1.

### 4.2 Recovered Strings
Following all 14 relocation chains from Segment 3 to segments 4–17, `vb3dec` recovers all 18 occurrences:

| Procedure | Module | Segment | Strings | In VBDIS3 output |
|-----------|--------|---------|---------|------------------|
| `fn0113` | `Module1` | 4 | `"Mimic"` | yes |
| `control4_Click` | `frm2` | 7 | `"Confirm? </yes> </lock> </morph>"` | **no** |
| `control58_Timer` | `frm2` | 7 | `" Player Added As a Mimic "`, `"/morph"` (3×) | **no** |
| `control8_Click` | `frm3` | 8 | `"Mimic"` | **no** |
| `sub0BB8` | `frm8` | 14 | `" morphs into Esper form."` | yes |
| `sub0C10` | `frm8` | 14 | `"morph"` | yes |
| `control15_Timer` | `frm8` | 15 | `"morph"` (4×) | yes |
| `control16_Timer` | `frm8` | 15 | `" cannot morph again."`, `" begins to morph.."`, `" is already in Esper form."`, `"morph"` | yes |

---

## 5. Implementation in `vb3dec`

1. **Relocation resolution:** `vb3dec` reads the 14 NE relocation entries after Segment 3 and follows each chain through descriptor offset `+0x26`, binding every procedure to its code segment without a lookup table.
2. **Completeness:** It finds all 10 modules and all 208 procedure descriptors of `FF.EXE` (191 bytecode procedures and 17 `Declare` statements).
3. **Robust token parsing:** Tokens missing from the opcode tables are emitted as diagnostic comments (`' <UNKNOWN_OPCODE: 0xXXXX>`), and known tokens without a decoder are marked with a comment (see [Known Limitations](limitations.md)); either way, decoding continues with the next token.
4. **Code generation:** The output avoids the syntax errors of Section 3 and the project problems of Sections 1.6 and 1.7, and `Declare` statements get parameter lists inferred from their call sites. Remaining gaps are listed in [Known Limitations](limitations.md).

---

## 6. Opcode Table Provenance & Elimination of Binary Blobs

### 6.1 Provenance of `vbdis3i.dat` and `VBDIS3X.DAT`
- **Origin**: Neither file originates from Microsoft SDKs, compilers, or developer kits. They were authored in 1996–1997 by Hans-Peter Diettrich ("DoDi") for his *Visual Basic Discompiler* (`VBDIS3`).
- **Composition**:
  - `vbdis3i.dat` (4,792 bytes): Encodes 512 9-bit opcode records (keyword strings, action category `Case`, operand word count `NumParams`, and opcode flags) reverse-engineered from `VBRUN300.DLL` opcode dispatch loops.
  - `VBDIS3X.DAT` (86,702 bytes): Encodes a 10,838-word translation array (`flagTokenData`) that maps `pToken / 3` to full 16-bit flag tokens (`AltToken`), along with a 32,511-byte `controlToken` table.
- **Comparison (1997 vs 2007)**:
  - An exact binary comparison of `vbdis3i.dat` and `VBDIS3X.DAT` from the original 1997 `vb3src` archive and CW2K's 2007 `VBDIS3.67e` release shows that the `.DAT` files are 100% identical.
  - The improvements made in the 2007 revision were strictly confined to control flow in VB source code (case-insensitive `vbrun300.dll` string matching, dynamic segment discovery, and reconstructing stripped form/control tables from `RT_RCDATA ID 2`).
  - All 2007 improvements have been natively implemented in `vb3dec` (`pkg/ne` and `pkg/pcode`).

### 6.2 Migration to Pure Go Tables
- Binary `.DAT` files and `//go:embed` directives are completely eliminated.
- The 512 opcode definitions, the 10,838-entry flag translation array, and the control token table were parsed and converted into typed, readable static Go structures:
  - `var baseOpcodes = [512]OpcodeInfo{ ... }` and `var flagTokenData = [10838]uint16{ ... }` in `pkg/pcode/opcodes_table.go`
  - `var controlTokenData = [32514]byte{ ... }` in `pkg/pcode/control_table.go` (the 32,511-byte table and the 3 bytes that follow it in the file)
- The control and global object models are likewise generated Go source (`pkg/vbx/vbrun_tables.go`), extracted from `VBRUN300.DLL` by `cmd/vbrunprops`.
- Benefits:
  - **Zero opaque binary dependencies** in the source tree.
  - **Type safety and transparency**: Opcode keywords, parameter counts, and case categories are directly inspectable in Go code.
  - **Single standalone binary**: Compiles cleanly with standard Go toolchain without runtime asset loading or parsing overhead.
