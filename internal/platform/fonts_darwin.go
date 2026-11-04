//go:build darwin

package platform

/*
#cgo LDFLAGS: -framework CoreText -framework CoreFoundation
#include <CoreText/CoreText.h>
#include <stdbool.h>

static bool viffRegisterFont(const char *path) {
	CFStringRef s = CFStringCreateWithCString(NULL, path, kCFStringEncodingUTF8);
	CFURLRef url = CFURLCreateWithFileSystemPath(NULL, s, kCFURLPOSIXPathStyle, false);
	CFErrorRef error = NULL;
	bool ok = CTFontManagerRegisterFontsForURL(url, kCTFontManagerScopeProcess, &error);
	if (error != NULL) {
		// Already registered counts as success.
		if (CFErrorGetCode(error) == kCTFontManagerErrorAlreadyRegistered) {
			ok = true;
		}
		CFRelease(error);
	}
	CFRelease(url);
	CFRelease(s);
	return ok;
}
*/
import "C"

import "unsafe"

// RegisterFonts makes the bundled fonts available to this process only;
// nothing is installed on the system.
func RegisterFonts() bool {
	paths, err := writeFonts()
	if err != nil {
		return false
	}
	ok := true
	for _, p := range paths {
		cs := C.CString(p)
		if !C.viffRegisterFont(cs) {
			ok = false
		}
		C.free(unsafe.Pointer(cs))
	}
	return ok
}
