# Local shell session provenance

The session buffering and the platform backends in this package are derived from
Motty's `internal/localshell` (https://github.com/t8y2/motty), Copyright 2026
Motty contributors, used under the Apache License 2.0.

- Upstream commit: `fbd26b8` (2026-11-05)
- License: `licenses/Motty-Apache-2.0.txt`
- Notice: `licenses/Motty-NOTICE.txt`
- Changes: no Windows Subsystem for Linux, no directory-report prompt
  integration, no leaked-shell counter; shell discovery left to the caller.

The shell is started on a real pty (Unix) or a pseudo console (Windows). It
deliberately bypasses `internal/process`: a terminal needs a console host, and
the hidden-console flags documented in ADR 0022 would defeat it.
