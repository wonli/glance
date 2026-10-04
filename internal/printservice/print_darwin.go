//go:build darwin && cgo

package printservice

/*
#cgo LDFLAGS: -framework AppKit -framework PDFKit -framework Foundation
#include <stdlib.h>
#include <stdint.h>
typedef struct GlancePrintJob GlancePrintJob;
GlancePrintJob *glance_print_pdf_start(const void *data, size_t length, int copies, int first_page, int last_page);
int glance_print_pdf_done(GlancePrintJob *job);
int glance_print_pdf_outcome(GlancePrintJob *job);
const char *glance_print_pdf_error(GlancePrintJob *job);
void glance_print_pdf_free(GlancePrintJob *job);
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type darwinPrinter struct{}

func New() Printer { return darwinPrinter{} }

func (darwinPrinter) Available() bool { return true }

func (darwinPrinter) Print(ctx context.Context, data []byte, options Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	pageCount, err := validatePDF(data)
	if err != nil {
		return err
	}
	options, err = normalizeOptions(options, pageCount)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("PDF data is empty")
	}
	buffer := C.CBytes(data)
	defer C.free(buffer)
	job := C.glance_print_pdf_start(buffer, C.size_t(len(data)), C.int(options.Copies), C.int(options.FirstPage), C.int(options.LastPage))
	if job == nil {
		return errors.New("could not create native print operation")
	}
	defer C.glance_print_pdf_free(job)
	for C.glance_print_pdf_done(job) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	switch C.glance_print_pdf_outcome(job) {
	case 0:
		return nil
	case 1:
		return ErrCanceled
	default:
		message := C.GoString(C.glance_print_pdf_error(job))
		if message == "" {
			message = "PDFKit or AppKit rejected the print job"
		}
		return fmt.Errorf("native print failed: %s", message)
	}
}
