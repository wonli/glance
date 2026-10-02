//go:build !darwin || !cgo

package printservice

import "context"

type unavailablePrinter struct{}

func New() Printer { return unavailablePrinter{} }

func (unavailablePrinter) Available() bool { return false }

func (unavailablePrinter) Print(context.Context, []byte, Options) error {
	return ErrUnavailable
}
