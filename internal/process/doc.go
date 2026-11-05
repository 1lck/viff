// Package process prepares child processes for the platform Viff runs on.
//
// Viff is a GUI-subsystem application on Windows: it has no console of its
// own. Windows then allocates a fresh console for every console child process
// it starts, which shows up as a black window flashing over the user's screen.
// Git is run for every refresh and repository operation, so each command would
// flash one console window. HideConsoleWindow suppresses those windows; it is
// a no-op on platforms where the problem does not exist.
//
// The integrated terminal is the one deliberate exception: internal/platform/pty
// starts a shell on a real pty or a pseudo console and must not hide it. See
// docs/decisions/0028-integrated-terminal.md.
package process
