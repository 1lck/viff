// Package svncli runs the Subversion command line client and parses its
// machine readable output.
//
// The package stays a thin, dependency-free adapter: it imports neither Viva
// nor Viff's UI packages and returns no UI nodes, so the parsing rules can be
// unit tested against recorded svn output without a window or a working copy.
// Callers reach it through the internal/vcs contract, which keeps the UI
// replaceable and stops Subversion specifics from leaking upwards.
package svncli
