//go:build darwin && cgo

package fontcatalog

/*
#cgo LDFLAGS: -framework CoreText -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <CoreText/CoreText.h>
static CFIndex availableFontArrayCount(CFArrayRef values) {
	return values == NULL ? -1 : CFArrayGetCount(values);
}
static Boolean availableFontNameAtIndex(CFArrayRef names, CFIndex index, UInt8 *buffer, CFIndex capacity) {
	if (names == NULL) return false;
	CFStringRef name = (CFStringRef)CFArrayGetValueAtIndex(names, index);
	return name != NULL && CFStringGetCString(name, (char *)buffer, capacity, kCFStringEncodingUTF8);
}
*/
import "C"

import (
	"path/filepath"
	"sort"
	"unsafe"
)

func availableSystemFontFiles() ([]string, error) {
	urls := C.CTFontManagerCopyAvailableFontURLs()
	count := int(C.availableFontArrayCount(urls))
	if count < 0 {
		return nil, nil
	}
	defer C.CFRelease(C.CFTypeRef(urls))

	paths := make([]string, 0, count)
	seen := make(map[string]bool, count)
	buffer := make([]byte, 32<<10)
	for i := 0; i < count; i++ {
		value := C.CFArrayGetValueAtIndex(urls, C.CFIndex(i))
		if value == nil {
			continue
		}
		url := C.CFURLRef(value)
		if C.CFURLGetFileSystemRepresentation(url, C.Boolean(1), (*C.UInt8)(unsafe.Pointer(&buffer[0])), C.CFIndex(len(buffer))) == 0 {
			continue
		}
		path := filepath.Clean(C.GoString((*C.char)(unsafe.Pointer(&buffer[0]))))
		if path == "." || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func availableSystemFontFamilyNames() ([]string, error) {
	names := C.CTFontManagerCopyAvailableFontFamilyNames()
	count := int(C.availableFontArrayCount(names))
	if count < 0 {
		return nil, nil
	}
	defer C.CFRelease(C.CFTypeRef(names))

	families := make([]string, 0, count)
	buffer := make([]byte, 32<<10)
	for i := 0; i < count; i++ {
		if C.availableFontNameAtIndex(names, C.CFIndex(i), (*C.UInt8)(unsafe.Pointer(&buffer[0])), C.CFIndex(len(buffer))) == 0 {
			continue
		}
		if name := C.GoString((*C.char)(unsafe.Pointer(&buffer[0]))); name != "" {
			families = append(families, name)
		}
	}
	return families, nil
}
