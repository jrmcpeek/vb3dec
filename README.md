# vb3dec

A modern 16-bit **Visual Basic 3.0** (New Executable) decompiler written in pure Go.

`vb3dec` reverse engineers compiled 16-bit Visual Basic 3 executables (`.EXE`) and reconstructs Visual Basic 3 project artifacts, including `.MAK` project files, `.FRM` visual form definitions with embedded event procedures, `.FRX` binary asset containers, standalone media files (`.bmp`, `.ico`), and `.bas` code modules. Output fidelity varies by executable and by the language features it uses; see [Known Limitations & Language Scope](docs/limitations.md).

---

## Key Features

- **16-Bit New Executable (NE) Engine (`pkg/ne`)**:
  - Full parser for 16-bit Windows 3.1 NE headers, segment tables, relocation chains, module references, and resource directories.
- **Visual Form & Asset Reconstruction (`pkg/frm`)**:
  - Extracts and decodes many standard VB3 controls (`CommandButton`, `TextBox`, `Label`, `OptionButton`, `CheckBox`, `ListBox`, `ComboBox`, `HScrollBar`, `VScrollBar`, `Timer`, `PictureBox`, `Image`, `FileListBox`, `DirListBox`, `DriveListBox`, `Frame`, `Line`, `Shape`) and custom VBX controls (`MCI.VBX`).
  - Decodes coordinates, colors, fonts, styles, and control visibility.
  - Reconstructs `.FRX` binary resource containers and extracts standalone `.bmp` and `.ico` image assets.
  - Generates `VERSION 2.00` `.FRM` files intended for use with the original 16-bit Visual Basic 3 IDE.
- **Pure Go P-Code Disassembler (`pkg/pcode`)**:
  - Typed opcode tables supporting 1-byte, 2-byte, 3-byte, and control tokens reverse-engineered from the `VBRUN300.DLL` runtime engine.
  - **Zero opaque binary dependencies**: Replaces legacy `.DAT` binary blobs with native, type-safe Go tables.
  - Resolves authentic 23-slot form event tables (`Form_Load`, `Form_Click`, `Form_Paint`, `Form_DblClick`, `Form_Resize`, etc.) and control-specific event mappings (`Click`, `Timer`, `Change`, `Scroll`, `MouseDown`, `MouseUp`).
- **High-Level Basic Reconstruction**:
  - **Structured Control Flow**: Decompiles block `If/Then/Else/End If`, single-line `If`, `For/Next` loops with `Step`, `While/Wend`, `Do/Loop` variants, `Select Case`, `On Error GoTo`, and `Exit Sub/Function/For/Do`.
  - **Type Inference & Physical Stack Frames**: Reconciles physical 16-bit stack frame slot sizes (`modBytes`) to distinguish `Integer`, `Long`, `Single`, `Double`, `Currency`, `String`, and `Variant`.
  - **Static Variable Retention**: Automatically detects static procedure data allocations (`bp == 0`) and emits `Static <var> As <type>` to preserve state across timer ticks.
  - **Array Dimension Extraction**: Reconstructs multi-dimensional array declarations (`Dim arr(1 To 25) As Integer`) directly from module descriptor bounds records.
  - **Inter-Procedural Parameter Analysis**: Inspects project-wide call sites to deduce `ByVal` vs. `ByRef` parameter passing conventions.
  - **Windows-1252 ANSI Transcoding (`pkg/win1252`)**: Correctly maps 1990s CP-1252 high-ASCII characters (such as prompt banners and stylized chat symbols) into clean, modern UTF-8 documents without corruption.
- **Production Packaging**:
  - Automatically stitches decompiled event procedures directly below `End` in `.FRM` files so controls remain active and interactive.
  - Generates relative project `.MAK` files.
  - Automatically copies companion dependencies (`.dll`, `.vbx`) from the target binary directory into the output folder for further testing.

---

## Historical Motivation: *Final Fantasy Extreme* (FFE)

`vb3dec` was born out of a preservation effort for ***Final Fantasy Extreme* (FFE)**, a beloved mid-1990s America Online (AOL 2.5 / 3.0) multiplayer chat-room RPG based on Square's *Final Fantasy III* (*Final Fantasy VI* in Japan).

During the golden era of 1990s dial-up AOL, online communities thrived in private chat rooms through user-created community applications known as "proggies" or chat-room RPGs. Written in Microsoft Visual Basic 3.0, *Final Fantasy Extreme* allowed players to create and train classic characters (Terra, Locke, Edgar, Sabin, Celes, Shadow, Cyan, Gau, Setzer, Mog, Strago, Relm, and Gogo), purchase weapons, armor, and relics, learn spells, and battle other players directly inside AOL chat rooms via automated text commands (`/att`, `/cast`, `/morph`, `/steal`, etc.). The game featured custom character password saves, background MIDI/WAV battle audio via `MCI.VBX`, and stylized ASCII chat announcement banners:

```text
«‹«——< Cecil is ready [100/100] >——»›»
```

The original author compiled `FF.EXE` with Visual Basic 3.0 Professional and applied **VBGuard**, a 16-bit obfuscation tool that stripped form and control names to hinder reverse engineering. Over the decades, the original Visual Basic source code was lost.

Existing legacy decompilation tools did not recover the entire program: bit-rotation arithmetic issues caused procedures across several forms to be omitted, while type inference and character-encoding problems reduced the usability of the output.

`vb3dec` was created to investigate these decades-old reverse engineering challenges. For the included sample, it recovers 191 local bytecode procedures and produces a project that can be further examined and tested in an authentic 16-bit Visual Basic 3 environment.

---

## Homage to Dr. Hans-Peter Diettrich ("DoDi")

This project owes an enormous intellectual debt to **Dr. Hans-Peter Diettrich ("DoDi")**, the legendary German computer scientist and reverse engineer who pioneered the study of Microsoft's 16-bit Visual Basic P-code virtual machine.

Between 1994 and 1997, Dr. Diettrich authored **VBDIS** and **VBDIS3** (*Visual Basic Discompiler*). Working without official specifications, source code, or modern debugging frameworks, Dr. Diettrich painstakingly analyzed `VBRUN300.DLL`, reverse-engineering the 512 core P-code opcodes, token execution models, and internal executable structures. His work unlocked the inner workings of Visual Basic for thousands of developers worldwide.

`vb3dec` stands upon the shoulders of Dr. Diettrich's foundational discoveries. By translating his insights into modern, type-safe Go and correcting historical arithmetic and table-indexing bugs, this project seeks to honor and celebrate his pioneering contributions to the retro-computing and software preservation communities.

---

## AI Development Attestation

`vb3dec` was designed, researched, and engineered through an intensive **human-AI collaborative pair-programming** process between the human project author and **Google DeepMind's Antigravity** advanced AI coding assistant.

- **Human Guidance & Domain Strategy**: Architecture definition, 16-bit environment execution testing (VB3 IDE under vintage Windows/DOSBox), problem identification, binary sample provision, and strategic requirements direction.
- **AI Agentic Implementation**: Deep binary reversing of the NE format and P-code token streams, mathematical root-cause analysis of legacy bit-rotation bugs, implementation of the pure Go disassembler and opcode tables, type inference propagation, and automated test suite creation.

This project stands as a practical demonstration of how modern agentic AI can work alongside human software engineers to solve complex, decades-old binary decompilation, reverse engineering, and digital preservation problems.

---

## Getting Started

### Prerequisites

- [Go 1.23+](https://golang.org/dl/)

### Building

Clone the repository and build the `vb3dec` executable:

```bash
git clone https://github.com/jrmcpeek/vb3dec.git
cd vb3dec
go build -o vb3dec.exe ./cmd/vb3dec
```

### Running Tests

Run the complete test suite:

```bash
go test -v ./...
```

---

## Usage

Decompile an executable to a target directory:

```bash
vb3dec [options] <path-to-vb3-exe> [output-dir]
```

### Example

Decompile the bundled *Final Fantasy Extreme* test fixture into `out_decompiled/`:

```bash
vb3dec test_input/FF.EXE out_decompiled
```

### Command-Line Options

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-out <dir>` | `./out_decompiled` | Target output directory for decompiled files. |
| `-forms` | `true` | Extract and decompile form resources (`.FRM` and `.FRX`). |
| `-code` | `true` | Disassemble and decompile P-code procedures into Basic code (`.bas`). |
| `-assets` | `true` | Extract standalone media asset files (`.bmp`, `.ico`) from `.FRX` containers. |
| `-assets-dir <dir>` | `<out>/assets` | Custom output directory for standalone media assets. |
| `-names <dir>` | `""` | Optional directory containing external control name mappings (`frmX.FRM.txt`). |
| `-raw` | `false` | Also dump raw binary form streams (`.raw.bin`) for debugging. |

### Output Artifacts

Upon completion, the target directory will contain:
- `<Project>.MAK`: The primary Visual Basic 3 project file.
- `*.FRM`: Visual form definition files containing control layouts, properties, and embedded event procedures.
- `*.FRX`: Binary form asset containers (icons, pictures, bitmaps).
- `*.bas`: Standalone Basic code modules and form procedure mirrors.
- `Globals.bas`: Global variable declarations extracted from the project manifest.
- `assets/`: Standalone `.bmp` and `.ico` image assets.
- Companion `.DLL` and `.VBX` files copied from the input directory.

To run or modify the decompiled application, open `<Project>.MAK` inside Microsoft Visual Basic 3.0 Professional Edition running under 16-bit Windows 3.11, Windows 95/98, or a compatible 16-bit emulator (e.g., 86Box, DOSBox-X, or Wine with 16-bit support).

### Example Output

Below is example console output from running `vb3dec` against `test_input/FF.EXE`:

```text
$ vb3dec test_input/FF.EXE out_decompiled
Visual Basic 3 Decompiler (vb3dec)
Target: test_input/FF.EXE
Output: out_decompiled

Project Info:
  Title:           Final Fantasy
  Startup:         Sub Main
  VBGuard Status:  Protected (Control & form names stripped at compile/link time)
  Custom VBXs:     [MCI.VBX]
  Forms Found:     9

Extracted Forms (out_decompiled):
Form     File       Res ID   Controls   FRX Size     Standalone Assets
-------------------------------------------------------------------------
frm1     FRM1.FRM   4        35         130934 bytes 15 (1 .ico, 14 .bmp)
frm2     FRM2.FRM   6        63         141050 bytes 1 (1 .bmp)
frm3     FRM3.FRM   8        9          44282 bytes  1 (1 .bmp)
frm4     FRM4.FRM   10       13         62282 bytes  1 (1 .bmp)
frm5     FRM5.FRM   12       22         51482 bytes  1 (1 .bmp)
frm6     FRM6.FRM   14       34         141050 bytes 1 (1 .bmp)
frm7     FRM7.FRM   16       1          -            -
frm8     FRM8.FRM   18       45         100662 bytes 1 (1 .bmp)
frm9     FRM9.FRM   20       4          -            -
-------------------------------------------------------------------------
Total: 9 forms, 226 controls decompiled.
Extracted 21 standalone media asset files into: out_decompiled/assets

Decompiled Modules (out_decompiled):
Module     Type     Local Procs  API Decls    Output File
-------------------------------------------------------------
Module1    Code     22           17           Module1.bas
frm1       Form     25           0            frm1.bas
frm2       Form     21           0            frm2.bas
frm3       Form     8            0            frm3.bas
frm4       Form     16           0            frm4.bas
frm5       Form     18           0            frm5.bas
frm6       Form     24           0            frm6.bas
frm7       Form     3            0            frm7.bas
frm8       Form     50           0            frm8.bas
frm9       Form     4            0            frm9.bas
-------------------------------------------------------------
Total: 10 modules, 208 procedures (191 local bytecode, 17 API declarations).

Generated Visual Basic 3 project file: out_decompiled/FF.MAK

Copied companion dependency: FF.DLL -> out_decompiled/FF.DLL
Copied companion dependency: MCI.VBX -> out_decompiled/MCI.VBX
Copied companion dependency: VBRUN300.DLL -> out_decompiled/VBRUN300.DLL
```

---

## Documentation

Comprehensive reverse engineering documentation is available in the [`docs/`](docs/) directory:
- **[Technical Audit: Legacy VBDIS3 Architecture & Reconstruction](docs/audit_vbdis3.md)**: In-depth analysis of VBDIS 3.67e failure modes, bit-rotation lookup bugs, stack frame allocations, and the pure Go reconstruction.
- **[Known Limitations & Language Scope](docs/limitations.md)**: Non-implemented VB3 features (menus, control arrays, UDTs, DAO) and future contribution paths.

---

## License

This project is licensed under the MIT License.
