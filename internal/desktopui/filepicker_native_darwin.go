//go:build darwin

package desktopui

/*
#cgo darwin LDFLAGS: -framework AppKit
#include <stdlib.h>
int glance_choose_save_path(const char *title, const char *name, char **path, char **error);
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Call from a background job: AppKit presents the modal panel on its main thread.
func chooseNativeSavePath(title, name string) (string, error) {
	nativeTitle, nativeName := C.CString(title), C.CString(name)
	defer C.free(unsafe.Pointer(nativeTitle))
	defer C.free(unsafe.Pointer(nativeName))
	var path, message *C.char
	status := C.glance_choose_save_path(nativeTitle, nativeName, &path, &message)
	defer C.free(unsafe.Pointer(path))
	defer C.free(unsafe.Pointer(message))
	if status < 0 {
		return "", errors.New(C.GoString(message))
	}
	return C.GoString(path), nil
}
