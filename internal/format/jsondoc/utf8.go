package jsondoc

import "unicode/utf8"

func validUTF8(b []byte) bool { return utf8.Valid(b) }
