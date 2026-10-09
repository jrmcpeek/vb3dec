# Test input

This directory holds the sample Visual Basic 3 programs used by the fixture-based tests and the examples in the root README. Each sample lives in its own directory together with the companion files it was distributed with, because the decompiler resolves custom controls (`.VBX`) and copies companion libraries from the executable's directory. Samples ship different versions of shared files (for example `THREED.VBX`), so they are not shared between directories.

The Go packages and command-line tools build without these files. Tests locate fixtures through `internal/fixture` and fail when a fixture they need is missing.

## Layout

```text
test_input/
  ff/        Final Fantasy Extreme (VBGuard-protected)
    names/   Hand-written control name mappings for the -names option
  bascode/   BasCode for Windows 1.2 (unprotected; THREED.VBX, CMDIALOG.VBX)
  empire/    World Empire III (unprotected; THREED.VBX)
  */_archive Original distribution archives the sample was extracted from
```

## Samples

| Directory | Program | Executable | Notes |
| :--- | :--- | :--- | :--- |
| `ff/` | *Final Fantasy Extreme* | `FF.EXE` | Primary sample discussed in the root README. Control and form names were stripped by VBGuard; `names/` restores readable names. |
| `bascode/` | BasCode for Windows 1.2 | `BASCODE.EXE` | Basic source code librarian. Keeps its form and control name tables; two code modules and six forms with nested 3D frames. |
| `empire/` | World Empire III | `EMPIRE.EXE` | Strategy game. Keeps its name tables; uses control arrays, Line controls and many pictures. The game's data files (`.BMP`, `.MID`, `.VBL`, ...) are kept as distributed. |

- **`ff/FF.DLL`** is the Final Fantasy Extreme companion library. It is copied into generated project output but is not required to parse the executable.
- **`ff/MCI.VBX`** is the Microsoft Multimedia Control used by `FF.EXE`; its control model is read to decode `MMControl` properties.
- **`ff/VBRUN300.DLL`** is Microsoft's Visual Basic 3 runtime. The decompiler neither loads nor executes it; `cmd/vbrunprops` reads its built-in control models to regenerate `pkg/vbx/vbrun_tables.go`.

## Checksums (SHA-256)

| File | SHA-256 |
| :--- | :--- |
| `ff/FF.EXE` | `e11763a4728ca1709285b2af12f29bd84f678c0a479d0e00f8264513412cab6f` |
| `ff/FF.DLL` | `586713ad158f200f7d11750cd7208e0872e9a6eac51b97de79b3ed9cc70fdb7d` |
| `ff/MCI.VBX` | `5ddfe8dccea577264feb8acf27761aaf757bb538ebec78fe5ff48fbb04e8dbb1` |
| `ff/VBRUN300.DLL` | `eb66f71dd14b01eb3df7409b0ad73e41589046ad5bf16152f06617093f63087e` |
| `bascode/BASCODE.EXE` | `dfdb4d5e559c81c1e66d744bd8f3ca6313fd54a4307cf6b56e6b77a35a37a0a3` |
| `bascode/THREED.VBX` | `825fccdc01f2119c9584378eed46825791d5fd416d366b390122e508b85f65c4` |
| `bascode/CMDIALOG.VBX` | `e16247d7c0a2d157c3d52f81cb8c93b84698903c8e06100346c2802f7cf1b3ab` |
| `bascode/COMMDLG.DLL` | `b962e6787daaa2afb14dc56c854c00159410ac50e2d5ba902b9052ee3080000d` |
| `bascode/_archive/bascde12.zip` | `b840928ede94a83ec2e48df8db4b78f486f34f20e0f30d5fbda355000be158fa` |
| `empire/EMPIRE.EXE` | `314b2b991a8ca06acedd1bb7e58c6b7b730660c9195363d6f3df3604f6f9b512` |
| `empire/THREED.VBX` | `dca653cc793c8581504b2b614e275da9782573882da6ede2d8a64c9840671e09` |
| `empire/_archive/_9empwin.zip` | `40708faade452db5f57cc8ebb23f24e221ccbf655e059b2d6a82dc8f3104f4c5` |

## Obtaining fixtures

The samples are third-party software (BasCode and World Empire III are shareware). The decompiler only reads them; do not execute them. Place files here only when you have the legal right to possess and use them. Appropriate sources may include your own archival copy, an authorized Visual Basic 3 or Windows distribution, or another source whose license permits the relevant use. The project does not redistribute the binaries as part of its source license, and each file may have separate copyright or redistribution terms.

The directory-level `.gitignore` is an allow list: only this README, the `.gitignore` itself and the `ff/names/` mappings can be committed, so locally supplied programs, data files and archives are not added by accident.

## Adding a sample

1. Create `test_input/<name>/` and extract the program's distribution into it, keeping the original archive under `_archive/`.
2. Add the directory to the tables above, with checksums of the executable and its companion files.
3. Add a constant for it in `internal/fixture` and write tests that open it through `fixture.Path`.

Candidate VB3 programs identified but not yet added: IGNITION v1.01, The Craps Engine 1.1, World Flags, 12 Months Screen Calendar 1.0, WinView v1.10.
