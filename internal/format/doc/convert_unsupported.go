//go:build !darwin

package doc

import (
	"context"
	"errors"
)

func convertToTempDOCX(ctx context.Context, _ string) (*TempDOCX, error) {
	if ctx == nil {
		return nil, errors.New("DOC import context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("旧版 .doc 导入目前仅支持 macOS")
}
