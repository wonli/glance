//go:build !darwin || !cgo

package pdfdoc

import "image"

func renderNative(_ []byte, _, _, _ int) image.Image { return nil }
