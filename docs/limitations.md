# Known Limitations & Language Scope

This document outlines the current architectural scope and known limitations of `vb3dec`.

`vb3dec` is tested against three Visual Basic 3 programs (see [`test_input/`](../test_input/README.md)): the VBGuard-protected `FF.EXE` and the unprotected `BASCODE.EXE` and `EMPIRE.EXE`. All of their forms decode completely, but the decompiled code still needs manual review: several statements are not yet decompiled, and some information is not stored in compiled executables at all.

---

## 1. Non-Implemented Visual Basic 3 Features

### 1.1 P-Code Statements Without a Decoder
- **Description:** The disassembler reconstructs statements from the P-code token stream. Tokens without a dedicated handler are logged to standard error as `[WARN] ... unhandled opcode` and emitted as their bare keyword.
- **Current Behavior:** The following statements are not yet decompiled correctly (none of them occur in `FF.EXE`):
  - The `:` statement separator. Statements following it are emitted on separate lines, which moves them out of a single-line `If`.
  - `Call` statements, `Let`, and the arguments of `ChDir` and `ChDrive`.
  - `On Error` and `Resume`.
  - File I/O: `Open`, `Close`, `Input #`, `Line Input #`, `Print #`, and the `#` file number syntax.
  - The array operand of `Erase`.
- **Contribution Path:** Add handlers to `decodeInstruction` in `pkg/pcode/disasm.go`; the tests in `pkg/pcode` exercise `BASCODE.EXE` and `EMPIRE.EXE`, which use these statements.

### 1.2 Line Labels, `GoSub`, and Computed Jumps
- **Description:** Early Basic dialect features:
  - `GoTo` and `GoSub ... Return` with line labels.
  - `On <expression> GoTo / GoSub`: Multi-way computed jumps.
  - `DefInt`, `DefLng`, `DefStr`: Letter-range implicit variable typing.
- **Current Behavior:** Common structured constructs (`If/Then/Else/End If`, `For/Next` with `Step`, `While/Wend`, `Do/Loop`, `Select Case`, `Exit Sub/Function/For/Do`) are supported. `GoTo` and `GoSub` statements are emitted with `LXXXX` targets, but the target labels are not emitted, so such procedures do not compile without manual fixes. Computed jump tables are skipped.

### 1.3 User-Defined Types (`Type ... End Type` / UDTs)
- **Description:** Visual Basic allows developers to define custom C-like record structures using `Type`:
  ```vb
  Type PlayerRecord
      Name As String * 20
      Level As Integer
      HP As Long
  End Type
  ```
- **Current Behavior:** In compiled P-code, structure member accesses are represented as offsets from the record. `vb3dec` emits them as unnamed property references (for example `m0024(l002A).Prop_0080` in `EMPIRE.EXE`) and declares structured variables with a placeholder type (`Dim m0024 As Integer`).
- **Contribution Path:** Recovering structured `Type` definitions requires static type reconstruction by analyzing field byte offsets and sizes across member access opcodes.

### 1.4 Unmapped Control Types (`Shape`, `Data`)
- **Description:** Each control record in a form stream identifies its type with a one-byte type ID. `vb3dec` has the runtime's property and event lists for every built-in control, but the type IDs of the `Shape` and `Data` controls have not been identified because none of the sample programs uses them.
- **Current Behavior:** Controls with an unknown type ID are emitted as `UnknownControl_0xNN` without properties, and a warning is printed.
- **Contribution Path:** With a sample that uses these controls, add their type IDs to `builtinModelNames` in `pkg/frm/types.go`; the property tables in `pkg/vbx/vbrun_tables.go` already cover them. The `Data` control's database properties use data types the decoder does not yet size.

### 1.5 Menus and Untested Control Types
- **Description:** Visual Basic 3 forms can define native Windows pull-down menus via the Menu Design window. In text `.FRM` files, these appear as hierarchical `Begin Menu <MenuName>` blocks with properties such as `Caption`, `Shortcut`, `Checked`, and `Enabled`.
- **Current Behavior:** Menu records are decoded with the runtime's `Menu` model like any other control, including nesting, but none of the sample programs has menus, so this path is untested. The same applies to the `CheckBox`, `VScrollBar`, `DirListBox`, `DriveListBox`, and `MDIForm` types, and to string properties longer than 255 characters.

### 1.6 Control Arrays (`Index As Integer`)
- **Description:** Visual Basic allows multiple controls of the same type to share a common name, distinguished by an integer `Index` property (e.g., `Command1(0)`, `Command1(1)`). When an event fires on any control in the array, the VB runtime invokes an event handler with an added first parameter: `Sub Command1_Click (Index As Integer)`.
- **Current Behavior:** Control array members are recognized from their form records, and their shared event handlers are emitted with the leading `Index As Integer` parameter. In executables that keep their name tables, the members share their original name. In protected binaries where names were stripped at compile time (such as VBGuard-protected binaries), `vb3dec` assigns unique sequential identifiers based on control IDs (`control1`, `control2`, ...), so the members of an array do not share a name.
- **Contribution Path:** Members of a stripped control array can be identified because their event tables reference the same handlers; `pkg/frm` could give them a common name.

---

## 2. Information Not Stored in Compiled Executables

- **Procedure, variable, and code module names:** Compiled VB3 executables keep form and control names (unless VBGuard removed them) but not the names of procedures, variables, or code modules. `vb3dec` names them after their descriptors and offsets: `subXXXX` / `fnXXXX` procedures, `pXXXX` parameters, `lXXXX` locals, `mXXXX` module variables, `gvXXXX` globals, and `Module1`, `Module2`, ... code modules.
- **`Declare` parameter lists:** Executables keep a declaration's library and entry point but not its parameter list. `vb3dec` reconstructs the list from the declaration's call sites, so a declaration that is never called is emitted without parameters, and an argument whose type cannot be determined becomes `As Any`.
- **Types of variables used only by address:** A global variable that the code only passes by reference carries no type information and is declared `As Variant`.

---

## 3. Known Approximations

- **Property names in code:** Property references in P-code are decoded with a property table that was built for `FF.EXE`'s forms. Properties of other objects can get wrong names; for example, `Screen.MousePointer = 11` in `BASCODE.EXE` is emitted as `Screen.Enabled = 11`.
- **Form window bounds:** Forms store their client area. The outer `Left`, `Top`, `Width`, and `Height` written to `.FRM` files are derived from it with fixed border and caption sizes for the form's `BorderStyle`.
- **Type conversions:** The target types of P-code type conversions are known for the conversion tokens that occur in the sample programs; others do not contribute to type inference.
- **Project window settings:** The `ProjWinSize` and `ProjWinShow` lines of the generated `.MAK` file are fixed defaults; executables do not record them.

---

## 4. Platform & Target Scope

### 4.1 16-Bit New Executable (NE) Only
- `vb3dec` is specifically engineered for **16-bit Windows 3.1 New Executable (NE)** binaries compiled with Visual Basic 3.0 (`VBRUN300.DLL`).
- It does **not** support 32-bit Portable Executable (PE) binaries produced by 32-bit Visual Basic 4, 5, or 6 (`MSVBVM50.DLL` / `MSVBVM60.DLL`).
- Note: Visual Basic 3 only ever supported P-code compilation. Native x86 machine code generation was only introduced in Visual Basic 5.0 (1997). Therefore, all genuine VB3 executables are P-code applications.
