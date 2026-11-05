# Terminal emulator provenance

`internal/ui/term` is the VT/xterm emulator from [Motty](https://github.com/t8y2/motty),
Copyright 2026 Motty contributors, used under the Apache License 2.0.

- Upstream commit: `fbd26b8` (2026-11-05)
- License: `licenses/Motty-Apache-2.0.txt`
- Notice: `licenses/Motty-NOTICE.txt`
- Changes: package path only; the files are otherwise unchanged.

The emulator is not safe for concurrent use. Feed it from the window thread, as
`internal/ui/terminal.go` does.
