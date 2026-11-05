package updater

import (
	"fmt"
	"strconv"
)

// helperScriptPrefix names the detached swap helpers. cleanupDir removes files
// with this prefix left over from an interrupted install.
const helperScriptPrefix = "viff-update-"

// darwinHelperScript is the POSIX shell script that swaps a macOS app bundle
// after Viff exits. It is invoked as: sh script target new pid [args...]. User
// arguments are passed as argv, never interpolated into the script text.
const darwinHelperScript = `#!/bin/sh
# Viff self-update helper: wait for the old process, swap the app bundle, and
# relaunch the new version. Arguments: target new pid [args...].
set -u
target=$1
new=$2
pid=$3
shift 3
while kill -0 "$pid" 2>/dev/null; do
  sleep 1
done
backup="${target}.old"
rm -rf "$backup"
if ! mv "$target" "$backup"; then
  exit 1
fi
if ! ditto "$new" "$target"; then
  rm -rf "$target"
  mv "$backup" "$target"
  exit 1
fi
rm -rf "$backup"
if [ "$#" -gt 0 ]; then
  open -n "$target" --args "$@"
else
  open -n "$target"
fi
rm -rf "$new"
rm -f "$0"
`

// linuxHelperScript is the POSIX shell script that replaces the Linux binary
// after Viff exits. It is invoked as: sh script target new pid [args...].
const linuxHelperScript = `#!/bin/sh
# Viff self-update helper: wait for the old process, install the new binary,
# and exec it. Arguments: target new pid [args...].
set -u
target=$1
new=$2
pid=$3
shift 3
while kill -0 "$pid" 2>/dev/null; do
  sleep 1
done
backup="${target}.old"
rm -f "$backup"
if ! mv "$target" "$backup"; then
  exit 1
fi
if ! install -m 0755 "$new" "$target"; then
  mv "$backup" "$target"
  exit 1
fi
rm -f "$backup" "$new"
rm -f "$0"
exec "$target" "$@"
`

// windowsHelperScript is the batch script that replaces viff.exe after Viff
// exits. It is invoked as: cmd /c script target new pid [args...]. The first
// three arguments are consumed with shift; the remaining arguments are
// re-quoted into userArgs and passed to the new process.
const windowsHelperScript = `@echo off
setlocal EnableExtensions EnableDelayedExpansion
rem Viff self-update helper: wait for the old process, replace viff.exe, and
rem relaunch. Arguments: target new pid [args...].
set "target=%~1"
set "new=%~2"
set "pid=%~3"
shift
shift
shift
set "userArgs="
:collect
if "%~1"=="" goto wait
set userArgs=!userArgs! "%~1"
shift
goto collect
:wait
tasklist /FI "PID eq %pid%" /NH 2>nul | findstr /I /C:"viff.exe" >nul
if not errorlevel 1 (
  ping -n 2 127.0.0.1 >nul 2>nul
  goto wait
)
set "backup=%target%.old"
if exist "%backup%" del /f /q "%backup%" >nul 2>nul
move /y "%target%" "%backup%" >nul 2>nul
if errorlevel 1 exit /b 1
move /y "%new%" "%target%" >nul 2>nul
if errorlevel 1 (
  move /y "%backup%" "%target%" >nul 2>nul
  exit /b 1
)
start "" "%target%" !userArgs!
del /f /q "%backup%" >nul 2>nul
del /f /q "%~f0" >nul 2>nul
exit /b 0
`

// shHelperArgs builds the argv for launching a POSIX shell helper, keeping the
// arguments passed to Viff.
func shHelperArgs(script, target, staged string, pid int, args []string) []string {
	argv := make([]string, 0, 4+len(args))
	argv = append(argv, script, target, staged, strconv.Itoa(pid))
	return append(argv, args...)
}

// windowsHelperArgs builds the argv for launching the batch helper through cmd.
func windowsHelperArgs(script, target, staged string, pid int, args []string) []string {
	argv := make([]string, 0, 5+len(args))
	argv = append(argv, "/c", script, target, staged, strconv.Itoa(pid))
	return append(argv, args...)
}

// helperName is a unique helper file name for this process and clock.
func (c *Client) helperName(extension string) string {
	return fmt.Sprintf("%s%d-%d.%s", helperScriptPrefix, currentPID(), c.now().UnixNano(), extension)
}
