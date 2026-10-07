//go:build darwin && cgo

package pdfdoc

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
static int renderPDF(const void *data, size_t length, int index, unsigned char *pixels, int width, int height) {
 CGDataProviderRef provider = CGDataProviderCreateWithData(NULL, data, length, NULL);
 if (!provider) return 0;
 CGPDFDocumentRef document = CGPDFDocumentCreateWithProvider(provider);
 CGDataProviderRelease(provider);
 if (!document) return 0;
 CGPDFPageRef page = CGPDFDocumentGetPage(document, index + 1);
 if (!page) { CGPDFDocumentRelease(document); return 0; }
 CGColorSpaceRef space = CGColorSpaceCreateDeviceRGB();
 CGContextRef context = CGBitmapContextCreate(pixels, width, height, 8, width * 4, space, kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
 CGColorSpaceRelease(space);
 if (!context) { CGPDFDocumentRelease(document); return 0; }
 CGContextSetRGBFillColor(context, 1, 1, 1, 1);
 CGContextFillRect(context, CGRectMake(0, 0, width, height));
 CGRect box = CGPDFPageGetBoxRect(page, kCGPDFMediaBox);
 CGFloat pageWidth = box.size.width, pageHeight = box.size.height;
 int rotation = CGPDFPageGetRotationAngle(page);
 if (rotation % 180 != 0) { CGFloat swap = pageWidth; pageWidth = pageHeight; pageHeight = swap; }
 if (pageWidth <= 0 || pageHeight <= 0) { CGContextRelease(context); CGPDFDocumentRelease(document); return 0; }
 CGContextScaleCTM(context, width/pageWidth, height/pageHeight);
 CGContextConcatCTM(context, CGPDFPageGetDrawingTransform(page, kCGPDFMediaBox, CGRectMake(0, 0, pageWidth, pageHeight), 0, true));
 CGContextDrawPDFPage(context, page);
 CGContextRelease(context);
 CGPDFDocumentRelease(document);
 return 1;
}
*/
import "C"

import (
	"image"
	"unsafe"
)

// The native renderer consumes the current immutable model revision, never the
// source path. Read/edit modes therefore cannot accidentally display old bytes.
func renderNative(data []byte, index, width, height int) image.Image {
	if len(data) == 0 || width <= 0 || height <= 0 {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if C.renderPDF(unsafe.Pointer(&data[0]), C.size_t(len(data)), C.int(index), (*C.uchar)(unsafe.Pointer(&img.Pix[0])), C.int(width), C.int(height)) == 0 {
		return nil
	}
	return img
}
