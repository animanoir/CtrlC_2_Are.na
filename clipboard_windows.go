//go:build windows

package main

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	procRegisterClipboardFormatW   = user32.NewProc("RegisterClipboardFormatW")
	procIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")

	// Password managers put this format on the clipboard so that clipboard tools leave the copy alone.
	// Registering a name returns the same ID for the rest of the session, so it's done once.
	concealedFormat     uintptr
	concealedFormatOnce sync.Once
)

// Reports whether the copy on the clipboard is marked as excluded from clipboard monitoring, as password managers mark theirs.
// Asking whether a format is present doesn't need the clipboard opened, so this can't block or clash with another app.
func clipboardIsConcealed() bool {
	concealedFormatOnce.Do(func() {
		name, err := windows.UTF16PtrFromString("ExcludeClipboardContentFromMonitorProcessing")
		if err != nil {
			return
		}
		concealedFormat, _, _ = procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	})
	if concealedFormat == 0 {
		return false // Registering failed, so there's nothing to check for
	}
	available, _, _ := procIsClipboardFormatAvailable.Call(concealedFormat)
	return available != 0
}
