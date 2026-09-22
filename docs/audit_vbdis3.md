# Technical Audit: Legacy VBDIS3 Architecture and Reconstruction in vb3dec

## Executive Summary

An in-depth architectural audit of legacy 16-bit Visual Basic 3 decompilation tooling—specifically Hans-Peter Diettrich's foundational *Visual Basic Discompiler* (VBDIS 3.67e)—was conducted to analyze its internal algorithms, failure modes, and table parsing mechanics.

The audit documents why raw legacy decompilation was incomplete for the protected sample (omitting 70 procedures across four forms) and why its output required substantial manual patching before it could be tested with the 16-bit VB3 compiler.

Our analysis identified the root cause of the missing code: a bit-rotation arithmetic mismatch during relocation traversal in `MODULE10.BAS`, combined with rigid segment assumptions. By addressing these issues, `vb3dec` discovers, disassembles, and reconstructs the 191 local bytecode procedures observed across the 11 modules and 14 code segments in the target binary, including the feature strings (`'mimic'`, `'morph'`, and `'esper'`).

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
The `RotateRight3` algorithm shifts the 16-bit descriptor offset right by 3, while rotating the lowest 3 bits into bits 13–15 (`((v And 6) * &H1000)`):
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
- When descriptor base offsets have non-zero rotation bits (`offset & 6 != 0`), the computed index diverges.
- For example, a relocation pointing to Segment 5 starting at offset `0x09B6` has descriptor base `0x09B6 - 0x26 = 0x0990`.
- In `sub0986`, `RotateRight3(0x0996)` yields `0xC132` (high bits `0xC000` set).
- In `Applyreloc1`, `(0x09B6 \ 8) - 4` computes `0x0132` (high bits `0x0000`).
- `NamesLookUpTbl(0x0132)` is `0` (not `gc0E1A`).
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
When Segment 3 has `curSegSize = 12424` (`12424 \ 8 = 1553`), keys with rotated high bits like `0xC019` (49177) exceed the array bounds. In a standard VB3 runtime environment without error suppression, this triggers a fatal `Subscript out of range` error.

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
  The `vb3dec` engine directly reads the event binding table located at the tail of each form and control record in the form's `RT_RCDATA` resource stream (`0xFF, count`, followed by event descriptor words). Each non-zero word is rotated to match the procedure descriptor in Segment 3. This binds events directly to their true controls (`control19_Click`, `control20_Click`, `control12_Click`), eliminating collisions and ensuring full compilation readiness.

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
  `VBDIS3` inspected the running machine's `C:\WINDOWS\system\` directory during decompilation and hardcoded `C:\WINDOWS\system\MCI.VBX` into the `.MAK` file. When opened on any machine without that exact 16-bit directory path, VB3 errors on project load. `vb3dec` extracts custom VBX library names directly from the executable's manifest and emits clean relative paths (`MCI.VBX`).
- **`ProjWinSize` / `ProjWinShow` IDE Artifacts:**
  In `MODULE20.BAS`, `VBDIS3` opened `autoload.mak` (the VB3 IDE's default project template on the host machine) and scraped `ProjWinSize=152,402,248,215`. These coordinates are not present in compiled executables; they are user-preference window positions for the VB3 Project Window.

### 1.8 Flawed Local Variable Type Inference vs. Physical Stack Frame Allocations

When declaring local variables (`Dim lXXXX`), `VBDIS3.67e` exhibits two pervasive defects:
1. **False Variant Inference (`Dim lXXXX As Variant ' 86`):**
   In expressions assigning arithmetic results to local variables (e.g. `l0110 = gv0074(m001A, 25) + 14`), `VBDIS3` encounters intermediate P-code coercion tokens (such as `0x0EB0` `C<typ>`). In `MODULE11.BAS` and `MODULE14.BAS`, `VBDIS3` misinterprets this opcode as a coercion to `Variant` (type code `0x06`). It tags the destination variable with type byte `0x86` (`0x80` local variable flag | `0x06` Variant), emitting `Dim l0110 As Variant ' 86`.
2. **Missing Loop Variable Inference (`Dim lYYYY ' 80`):**
   In `For` loops (e.g. `For l0114 = 1 To 100 Step 1`), `VBDIS3`'s type signature lookup does not constrain the loop counter variable from its integer start/end bounds. Because no typing rule fires, the variable's type entry remains `0x00`. `DetermDeclareType` in `MODULE14.BAS` defaults untyped variables to emitting no type clause, emitting `Dim l0114 ' 80`.

**Binary Ground Truth (Stack Frame Allocations in `modBytes`):**
In compiled 16-bit Visual Basic 3 executables, procedure stack frames are explicitly laid out in the module descriptor stream (`modBytes` in `RT_RCDATA ID 2`):
- For `control12_Click` in `frm6`:
  - `l0110` is allocated at BP - 38 (`0xFFDA`).
  - `l0114` is allocated at BP - 40 (`0xFFD8`).
- The distance between `l0110` and `l0114` is exactly **2 bytes** (`(-38) - (-40) = 2`).
- In 16-bit Visual Basic 3, a `Variant` structure requires **16 bytes** (2 bytes `VARTYPE` + 6 bytes reserved + 8 bytes data payload). It is physically impossible to fit a 16-byte Variant into a 2-byte stack slot.
- The 2-byte slot allocation in the binary definitively proves that the VB3 compiler allocated **16-bit Integers** for both variables, not Variants.
- This pattern recurs across click handlers where pairs of variables allocated at BP - 38 and BP - 40 were consistently misdecompiled by `VBDIS3` as `Variant` (`' 86`) and untyped (`' 80`).

**Resolution in `vb3dec`:**
The `vb3dec` engine applies semantic type propagation across:
1. Array index usage (`gv00D4(l0114, l0110)` marks both index operands as `Integer`).
2. Numeric loop bounds (`For l0114 = 1 To 100 Step 1` infers `Integer`).
3. Integer array arithmetic (`gv0074(...) + 14` infers `Integer`).
This produces `Dim l0110 As Integer` and `Dim l0114 As Integer`, accurately reflecting both the semantic intent and the physical 2-byte stack frame allocation.

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
   In `frm8.bas` (`sub0BA5`), the battle notification engine concatenates these two functions around a player's combat readiness message:
   ```vb
   fn018F() & " " & charName & " is ready [" & hp & "/" & maxhp & "] " & fn00BE()
   ```
   In 1990s America Online (AOL 2.5/3.0) chat rooms, this generated the stylized announcement banner:
   ```text
   «‹«——< Cecil is ready [100/100] >——»›»
   ```

**Resolution in `vb3dec`:**
The `vb3dec` engine introduces a dedicated zero-dependency ANSI Windows-1252 transcoding package (`pkg/win1252`):
- All string literals in P-code (Opcode Case 8) and form property streams (captions, control texts, tooltips, font names) are decoded through `win1252.Decode` and `win1252.FormatVBString`.
- High-ASCII characters (`0x80..0xFF`) are accurately transcoded to standard multi-byte UTF-8 representations.
- Emitted `.BAS` and `.FRM` files are written as UTF-8 documents for use in modern editors and command-line tools.

### 3.3 Hardcoded Absolute System Paths in Project Files
In `FF.MAK`, `VBDIS3` emitted absolute machine paths:
```diff
--- Legacy VBDIS3 Output (Project.MAK)
+++ Clean VB3 Project File
-C:\WINDOWS\system\MCI.VBX
+MCI.VBX
```
This broke project loading whenever the development environment was relocated.

### 3.4 Leftover Temporary Scrap Files
`VBDIS3` created temporary scratch files during decompilation (`VDM101.tmp`, `VDM102.tmp`, etc.) that were left on disk.

---

## 4. Dark Matter & Missing Strings Coverage Analysis

Missing feature strings (`'mimic'`, `'morph'`, and `'esper'`) were previously unaccounted for in legacy decompiled output.

### 4.1 Root Cause of Missing Feature Strings
In `VBDIS3.67e`:
- Only 121 procedures were emitted (Module 1, Form 5, Form 6, Form 7, Form 8, Form 9).
- Forms 1, 2, 3, and 4 (comprising 70 procedures) were completely dropped due to the bit-rotation bug described in Section 1.1.
- These dropped procedures were located in **Segments 5, 6, 7, 8, and 9**.

### 4.2 Location of Missing Strings
By directly tracing the 14 relocation chains in Segment 3 to Segments 4–17, `vb3dec` identified and reconstructed all 18 occurrences of `'mimic'`, `'morph'`, and `'esper'`:

| Keyword | Segment | Procedure | Module | Context Snippet |
|---------|---------|-----------|--------|-----------------|
| `mimic` | Seg 4 | `fn0113` | Module 1 | `fn0113 = "Mimic"` *(Only one found by VBDIS3)* |
| `mimic` | **Seg 7** | **`sub060A`** | **Form 2 (DROPPED BY VBDIS3)** | `" Player Added As a Mimic "` |
| `mimic` | **Seg 8** | **`sub065A`** | **Form 3 (DROPPED BY VBDIS3)** | `"Mimic"` |
| `morph` | **Seg 7** | **`sub052F`** | **Form 2 (DROPPED BY VBDIS3)** | `"</morph>"` |
| `morph` | **Seg 7** | **`sub060A`** | **Form 2 (DROPPED BY VBDIS3)** | `"/morph"`, `"/morph"`, `"/morph"` |
| `morph` | Seg 14 | `sub0BB8` | Form 8 | `" morphs into Esper form."` |
| `morph` | Seg 14 | `sub0C10` | Form 8 | `l0200$ = "morph"` |
| `morph` | Seg 15 | `sub0CDF` | Form 8 | `" cannot morph again."`, `" begins to morph.."` |
| `morph` | Seg 15 | `sub0CF3` | Form 8 | `"morph"` |
| `esper` | Seg 14 | `sub0BB8` | Form 8 | `" morphs into Esper form."` |
| `esper` | Seg 15 | `sub0CDF` | Form 8 | `" already in Esper form."` |

All 70 previously missing procedures in Forms 1–4 are fully recovered and emitted by `vb3dec`.

---

## 5. Architectural Implementation in `vb3dec`

1. **Direct Relocation Resolution**:
   Does not rely on fragile array indexing. Directly reads the 14 NE relocation entries following Segment 3, and follows each relocation chain at descriptor offset `+0x26` to bind every procedure to its true code segment.
2. **Total Module & Procedure Completeness**:
   Traverses all 11 modules and all 208 descriptors (191 local bytecode procedures + 17 external API declarations).
3. **Resilient Bytecode Stream Parsing**:
   Decodes 16-bit P-code tokens via native Go opcode definitions and token translation tables (`baseOpcodes` and `flagTokenData` in `pkg/pcode/opcodes_table.go`), completely eliminating opaque external binary `.DAT` blobs. For unknown or complex tokens, emits inline diagnostic comments (`' <UNKNOWN_OPCODE: 0xXXXX>`) and advances by operand size without terminating disassembly.
4. **Clean Code Generation**:
   Emits syntactically valid Visual Basic 3 code:
   - Strips type annotations from inside parameter parentheses (`(p00A4 As String)` instead of `(p00A4 As String ' 47)`).
   - Emits valid API declarations (`Declare Function ... Lib ... Alias ...`).
   - Formats branch targets as clear labels (`LXXXX:`).
   - Generates relative project files (`FF.MAK`).
   - Appends event handlers directly inside `.FRM` files so controls function interactively.
   - Automatically copies required companion dependencies (`.dll`, `.vbx`) alongside the generated project.

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
- The 512 opcode definitions and 10,838-entry flag translation array were parsed and converted into typed, readable static Go structures in `pkg/pcode/opcodes_table.go`:
  - `var baseOpcodes = [512]OpcodeInfo{ ... }`
  - `var flagTokenData = [10838]uint16{ ... }`
- Benefits:
  - **Zero opaque binary dependencies** in the source tree.
  - **Type safety and transparency**: Opcode keywords, parameter counts, and case categories are directly inspectable in Go code.
  - **Single standalone binary**: Compiles cleanly with standard Go toolchain without runtime asset loading or parsing overhead.
