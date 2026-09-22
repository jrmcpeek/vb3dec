# Known Limitations & Language Scope

This document outlines the current architectural scope and known non-implemented Visual Basic 3.0 language features in `vb3dec`.

`vb3dec` provides broad coverage for its primary target (`FF.EXE`) and many protected 16-bit VB3 applications. Some output may still require manual review or adjustment, and several less common Visual Basic 3 language features are not yet implemented.

---

## 1. Non-Implemented Visual Basic 3 Features

### 1.1 Menu Bar Hierarchies (`Begin Menu`)
- **Description:** Visual Basic 3 forms can define native Windows pull-down menus via the Menu Design window. In text `.FRM` files, these appear as hierarchical `Begin Menu <MenuName>` blocks with properties such as `Caption`, `Shortcut`, `Checked`, and `Enabled`.
- **Current Behavior:** `vb3dec` extracts supported windowed child controls and custom VBX controls (`CommandButton`, `TextBox`, `Image`, `Timer`, `MCI.VBX`, etc.), but does not currently parse native menu hierarchy records from the form resource stream.
- **Contribution Path:** The menu definitions reside in the form's `RT_RCDATA` resource stream following the form header. Adding a menu parser in `pkg/frm/decoder.go` to emit `Begin Menu` blocks would extend menu bar reconstruction.

### 1.2 Control Arrays (`Index As Integer`)
- **Description:** Visual Basic allows multiple controls of the same type to share a common name, distinguished by an integer `Index` property (e.g., `Command1(0)`, `Command1(1)`). When an event fires on any control in the array, the VB runtime invokes an event handler with an added first parameter: `Sub Command1_Click (Index As Integer)`.
- **Current Behavior:** In protected binaries where original control names were stripped at compile time (such as VBGuard-protected binaries), `vb3dec` assigns unique sequential identifiers based on control IDs (`control1`, `control2`, ...). Each control binds to its own dedicated event handler (e.g., `control1_Click`).
- **Contribution Path:** When external name mapping files provide identical names across distinct control IDs with `Index` properties, `pkg/pcode/project.go` could aggregate these into a single event handler signature accepting `Index As Integer`.

### 1.3 User-Defined Types (`Type ... End Type` / UDTs)
- **Description:** Visual Basic allows developers to define custom C-like record structures using `Type`:
  ```vb
  Type PlayerRecord
      Name As String * 20
      Level As Integer
      HP As Long
  End Type
  ```
- **Current Behavior:** In compiled P-code, structure member accesses are represented as raw offset displacements from the base record pointer. `vb3dec` currently treats structured local variables as untyped byte buffers or Variants.
- **Contribution Path:** Recovering structured `Type` definitions requires static type reconstruction by analyzing field byte offsets and sizes across member access opcodes.

### 1.4 Data-Bound Controls & DAO (`Data` Control)
- **Description:** VB3 Professional Edition introduced data-aware controls bound to Microsoft Access 1.1 / 2.0 Jet databases via the `Data` control (`DatabaseName`, `RecordSource`).
- **Current Behavior:** Supported standard UI controls are decoded, but property payloads specific to the Jet database engine and DAO binding are not modeled.
- **Contribution Path:** Implement the proprietary property serialization tags used by the VB3 `Data` control in `pkg/frm/decoder.go`.

### 1.5 Obscure / Deprecated Control Structures
- **Description:** Early Basic dialect features:
  - `GoSub ... Return`: Intra-procedure jumps to line labels.
  - `On <expression> GoTo / GoSub`: Multi-way computed jumps.
  - `DefInt`, `DefLng`, `DefStr`: Letter-range implicit variable typing.
- **Current Behavior:** Common structured constructs (`If/Then/Else/End If`, `For/Next` with `Step`, `While/Wend`, `Do/Loop`, `Select Case`, `On Error GoTo`, `Exit Sub/Function/For/Do`) are supported. Intra-procedure `GoSub` calls are rare in compiled P-code and are currently emitted as linear jumps with diagnostic labels.

---

## 2. Platform & Target Scope

### 2.1 16-Bit New Executable (NE) Only
- `vb3dec` is specifically engineered for **16-bit Windows 3.1 New Executable (NE)** binaries compiled with Visual Basic 3.0 (`VBRUN300.DLL`).
- It does **not** support 32-bit Portable Executable (PE) binaries produced by 32-bit Visual Basic 4, 5, or 6 (`MSVBVM50.DLL` / `MSVBVM60.DLL`).
- Note: Visual Basic 3 only ever supported P-code compilation. Native x86 machine code generation was only introduced in Visual Basic 5.0 (1997). Therefore, all genuine VB3 executables are P-code applications.
