//go:build !darwin

package platform

// RegisterFonts is a no-op where the bundled font cannot be registered per
// process yet; the editor then uses an installed monospace font.
func RegisterFonts() bool { return false }
