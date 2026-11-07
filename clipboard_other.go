//go:build !darwin && !windows

package main

// Linux has no agreed way for a password manager to mark a copy. KDE and KeePassXC offer an
// "x-kde-passwordManagerHint" target, but few managers set it and reading it would mean running
// xclip or wl-paste a second time on every tick. So nothing is treated as concealed here, and the
// secret heuristic in secrets.go is the only safeguard.
func clipboardIsConcealed() bool {
	return false
}
