# Known Limitations

`vb3dec` is tested against three Visual Basic 3 programs (see [`test_input/`](../test_input/README.md)): the VBGuard-protected `FF.EXE` and the unprotected `BASCODE.EXE` and `EMPIRE.EXE`. Every token of their forms and code decodes, but the decompiled code still needs review: statements that the samples do not use may not be decompiled yet, and some information is not stored in executables at all.

---

## 1. Not Yet Implemented

### 1.1 Statements Without a Decoder
Tokens without a handler are emitted as their bare keyword, and their operands can be left behind on separate lines or attached to the next statement. Each one is marked in the decompiled code with a comment before the affected line, which names the token and its offset in the procedure's bytecode:

```vb
' vb3dec: unhandled P-code: "Write " (token 162 @ 0x002E)
```

The console output ends the module table with the number of unhandled tokens and the most frequent ones, and each token is also logged to standard error as `[WARN] ... unhandled opcode`. None of the sample programs has any, and `TestSampleUnhandledOpcodeBudget` keeps it so. Known statements without a decoder, none of which the samples use:
- `On Error GoTo 0`.
- File I/O: `Write #`, `Seek`, `Lock`, `Unlock`, `Put` without a record number, and `Open` with an `Access` or `Lock` clause.
- Graphics methods: `Line` other than a filled box (`BF`), `PSet`, and `Scale (x1, y1)-(x2, y2)`.
- Run-time library functions called through the `fin` token other than `LoadPicture`. The token's first operand identifies the function.

`On <expression> GoTo` and `On <expression> GoSub` are not decoded either, and are not marked: the statement is skipped and its selector expression is left behind, where it can corrupt the next statement. Its targets still get line labels.

Handlers belong in `decodeInstruction` in `pkg/pcode/disasm.go`.

### 1.2 User-Defined Types (`Type ... End Type`)
Compiled code accesses the members of a record by offset. `vb3dec` emits them as placeholder properties (for example `m0024(l002A).Prop_0080` in `EMPIRE.EXE`) and declares record variables with a placeholder type (`Dim m0024 As Integer`). Recovering the `Type` definitions requires reconstructing each record's layout from the member offsets and sizes the code uses.

### 1.3 `Shape` and `Data` Controls
A control record identifies its type with a one-byte type ID. The IDs of `Shape` and `Data` are unknown because no sample program uses them, so these controls are emitted as `UnknownControl_0xNN` without properties, with a warning. Their models are already in `pkg/vbx/vbrun_tables.go`; with a sample, their type IDs can be added to `builtinModelNames` in `pkg/frm/types.go`. The `Data` control also has database properties whose data types the decoder cannot size yet.

### 1.4 Untested Controls and Statements
These are decoded like any other control, but no sample program uses them: menus (`Begin Menu` blocks), `CheckBox`, `VScrollBar`, `DirListBox`, and `DriveListBox`, as well as string properties longer than 255 characters. MDI forms are decoded as ordinary forms.

Likewise, these statements have a decoder but no sample program uses them: `MkDir`, `RmDir`, `SetAttr`, `Resume` without a label, and `Resume Next`.

### 1.5 Control Arrays in Protected Executables
Control array members are recognized from their form records, and their event procedures get the leading `Index As Integer` parameter. Protected executables do not store control names, so `vb3dec` names each control after its ID (`control1`, `control2`, ...) and the members of an array would not share a name. No sample program has a control array with more than one member at design time (`FF.EXE` has none, and each array in `EMPIRE.EXE` has one member), so this has not been observed. With such a sample, the members could be grouped by the event procedures they share, and code references to them would need the same name.

---

## 2. Information Not Stored in Executables

- **Procedure, variable, and code module names:** Executables keep form and control names (unless VBGuard removed them) but not the names of procedures, variables, or code modules. `vb3dec` names them after their descriptors and offsets: `subXXXX` / `fnXXXX` procedures, `pXXXX` parameters, `lXXXX` locals, `mXXXX` module variables, `gvXXXX` globals, and `Module1`, `Module2`, ... code modules.
- **Line labels and line numbers:** `vb3dec` names the targets of `GoTo`, `GoSub`, `On Error GoTo`, and `Resume` `LXXXX` after their offset in the procedure's bytecode.
- **Comments:** The compiler drops them; none of the sample programs contains any.
- **`DefInt`, `DefStr`, and the other `Def`*type* statements:** They set default types at compile time. The compiled code records each variable's actual type instead, which `vb3dec` declares with an explicit `As` clause.
- **`Declare` parameter lists:** Executables keep a declaration's library and entry point but not its parameters. `vb3dec` infers them from the declaration's call sites, so a declaration that is never called has no parameters, and an argument whose type is unknown becomes `As Any`.
- **Types of variables used only by address:** A global variable that the code only passes by reference carries no type information and is declared `As Variant`.

---

## 3. Known Approximations

- **Variable types:** Types are inferred from P-code variable references and from how values are used, not from stack slot sizes. Type conversions contribute only for the conversion tokens the samples use.
- **Property names in code:** A property is named from the model of the object it applies to: a form, a control, a global object (`App`, `Clipboard`, `Printer`, `Screen`), or a parameter that every call site passes the same kind of form or control (declared `As Form` or `As Control`). For an object of unknown type, such as a parameter that receives different control types or a variable assigned with `Set`, a generic mapping guesses the name. No sample program has such a reference.
- **Form window bounds:** Forms store their client area. The outer `Left`, `Top`, `Width`, and `Height` written to `.FRM` files are derived from it with fixed border and caption sizes for the form's `BorderStyle`.
- **Filled boxes:** A `Line` statement's operand selects a plain line, a box (`B`), or a filled box (`BF`). Operand 2 is decoded as `BF` because `EMPIRE.EXE` uses it to draw color swatches; the other operands are unknown.
- **Project window settings:** The `ProjWinSize` and `ProjWinShow` lines of the `.MAK` file are fixed defaults; executables do not record them.

---

## 4. Scope

`vb3dec` reads 16-bit New Executable (NE) programs compiled with Visual Basic 3 (`VBRUN300.DLL`). Programs from later versions, such as the 32-bit Portable Executable (PE) programs of Visual Basic 5 and 6 (`MSVBVM50.DLL`, `MSVBVM60.DLL`), use other formats and are not supported. Visual Basic 3 compiles only to P-code, so every VB3 program can be decompiled this way.
