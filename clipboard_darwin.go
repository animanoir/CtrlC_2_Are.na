//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// Password managers add these types to what they copy so that clipboard tools leave it alone
// (see http://nspasteboard.org). Only the types are asked for, never the contents.
static int pasteboardIsConcealed(void) {
	@autoreleasepool {
		NSArray *types = [[NSPasteboard generalPasteboard] types];
		return [types containsObject:@"org.nspasteboard.ConcealedType"]
			|| [types containsObject:@"org.nspasteboard.TransientType"];
	}
}
*/
import "C"

// Reports whether the copy on the clipboard is marked as concealed or transient, as password managers mark theirs
func clipboardIsConcealed() bool {
	return C.pasteboardIsConcealed() != 0
}
