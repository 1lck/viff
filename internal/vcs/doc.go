// Package vcs is the contract Viff's use cases depend on, so a second version
// control system can be added without the application layer knowing which one
// is in use. It holds three things: the small interfaces a backend must
// satisfy, the optional capabilities a backend may or may not provide, and the
// value types that cross the boundary.
//
// Dependency direction: vcs -> domain only. Like domain it must not import
// Viva, start processes, or read .git/.svn itself. The concrete adapters
// (internal/gitcli, internal/svncli) implement these interfaces, and cmd/viff
// picks one after probing the directory.
//
// See docs/decisions/0024-svn-support.md for why the contract is split into a
// required Backend plus optional capability fields rather than one large
// interface: Subversion has no rebase, stash, worktree, reflog or submodules,
// so a single interface would force a dozen meaningless stubs.
package vcs
