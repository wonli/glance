//go:build darwin && cgo

package ocr

/*
#cgo LDFLAGS: -framework Vision -framework Foundation -framework ImageIO -framework CoreGraphics
#include <stdlib.h>
int glance_ocr_characters(const void *data, size_t length, char **output, char **error);
*/
import "C"

import (
	"encoding/json"
	"errors"
	"unsafe"
)

func recognizeCharacterImage(data []byte) ([]detectedTextLine, error) {
	if len(data) == 0 {
		return nil, errors.New("ocr: empty character image")
	}
	var output, message *C.char
	ok := C.glance_ocr_characters(unsafe.Pointer(&data[0]), C.size_t(len(data)), &output, &message)
	if output != nil {
		defer C.free(unsafe.Pointer(output))
	}
	if message != nil {
		defer C.free(unsafe.Pointer(message))
	}
	if ok == 0 {
		if message != nil {
			return nil, errors.New(C.GoString(message))
		}
		return nil, errors.New("ocr: native character localization failed")
	}
	var lines []detectedTextLine
	err := json.Unmarshal([]byte(C.GoString(output)), &lines)
	return lines, err
}
