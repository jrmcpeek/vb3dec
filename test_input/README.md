# Test input

This directory holds optional sample inputs for the fixture-based tests and the examples in the root README. The Go packages and command-line tool build without these files. Tests that open the sample executable require the fixture to be present; package tests that do not need it can run independently.

## Sample files

- **`FF.EXE`** is the Visual Basic 3 New Executable used as the primary sample. It is the compiled *Final Fantasy Extreme* application discussed in the root README. The decompiler reads its NE headers, VB3 form resources, and P-code; it does not execute the file.
- **`FF.DLL`** is the sample application's companion library. It is copied into generated project output when present, but it is not required to parse the executable.
- **`MCI.VBX`** is the Microsoft Multimedia Control custom VBX used by the sample for media playback. It is identified from the executable's project references and is copied into generated output when present.
- **`VBRUN300.DLL`** is Microsoft's Visual Basic 3 runtime library. It is a well-known shared runtime distributed with many VB3 applications and development environments. It provides the runtime support expected by the sample application; the decompiler uses its documented presence and the reverse-engineered VB3 P-code conventions, but does not load or execute the DLL.

The `names/` directory contains text mappings for form and control names that were stripped by VBGuard in the sample executable. These mappings are small, human-readable project data and may remain versioned with the source.

## Obtaining fixtures

Place files here only when you have the legal right to possess and use them. Appropriate sources may include your own archival copy, an authorized Visual Basic 3 or Windows distribution, or another source whose license permits the relevant use. The project does not redistribute the binaries as part of its source license, and each file may have separate copyright or redistribution terms. Verify those terms before sharing the files or adding them to a public repository.

The directory-level `.gitignore` excludes `.exe`, `.dll`, and `.vbx` files so locally supplied binaries are not accidentally added in future commits.
