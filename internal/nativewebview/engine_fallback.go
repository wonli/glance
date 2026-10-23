//go:build (!darwin && !windows) || (darwin && !cgo)

package nativewebview

import (
	"context"
)

func newPlatformEngine() (Engine, error) {
	return nil, ErrUnsupportedKind
}
