//go:build darwin && cgo

package systemlocale

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
static CFIndex preferredLanguageCount(CFArrayRef languages) {
	return languages == NULL ? 0 : CFArrayGetCount(languages);
}
static Boolean preferredLanguageAtIndex(CFArrayRef languages, CFIndex index, UInt8 *buffer, CFIndex capacity) {
	if (languages == NULL) return false;
	CFStringRef value = (CFStringRef)CFArrayGetValueAtIndex(languages, index);
	return value != NULL && CFStringGetCString(value, (char *)buffer, capacity, kCFStringEncodingUTF8);
}
*/
import "C"

import (
	"unsafe"

	"golang.org/x/text/language"
)

func PreferredLanguages() []language.Tag {
	languages := C.CFLocaleCopyPreferredLanguages()
	if languages == 0 {
		return nil
	}
	defer C.CFRelease(C.CFTypeRef(languages))

	count := int(C.preferredLanguageCount(languages))
	values := make([]string, 0, count)
	buffer := make([]byte, 256)
	for index := 0; index < count; index++ {
		if C.preferredLanguageAtIndex(languages, C.CFIndex(index), (*C.UInt8)(unsafe.Pointer(&buffer[0])), C.CFIndex(len(buffer))) != 0 {
			values = append(values, C.GoString((*C.char)(unsafe.Pointer(&buffer[0]))))
		}
	}
	return parsePreferredLanguages(values)
}
