package pptx

import (
	"encoding/binary"
	"fmt"
	"image"

	emf "github.com/iEvan-lhr/emfToPng"
)

const (
	maxEMFBytes      = 2 << 20
	maxEMFRecords    = 4096
	maxEMFDimension  = 8192
	maxEMFPixelCount = 16 << 20
	emfSignature     = 0x464d4520
	emfHeaderSize    = 108
	emfEndRecord     = 14
	emfBezierTo16    = 88
	emfPolylineTo16  = 89
	emfExtCreatePen  = 95
)

// The current renderer supports the vector record subset used by common EMF
// artwork, including the supplied VCU footer assets. It rejects other records
// before passing data to the decoder, whose parser does not bound reads to a
// record's declared size.
var supportedEMFRecordSizes = map[uint32]uint32{
	1:  emfHeaderSize,
	9:  16, // SETWINDOWEXTEX
	10: 16, // SETWINDOWORGEX
	11: 16, // SETVIEWPORTEXTEX
	12: 16, // SETVIEWPORTORGEX
	13: 16, // SETBRUSHORGEX
	14: 20, // EOF
	17: 12, // SETMAPMODE
	18: 12, // SETBKMODE
	19: 12, // SETPOLYFILLMODE
	21: 12, // SETSTRETCHBLTMODE
	22: 12, // SETTEXTALIGN
	27: 16, // MOVETOEX
	37: 12, // SELECTOBJECT
	39: 24, // CREATEBRUSHINDIRECT
	40: 12, // DELETEOBJECT
	43: 24, // RECTANGLE
	58: 12, // SETMITERLIMIT
	59: 8,  // BEGINPATH
	60: 8,  // ENDPATH
	61: 8,  // CLOSEFIGURE
	62: 24, // FILLPATH
	64: 24, // STROKEPATH
	67: 12, // SELECTCLIPPATH
	70: 8,  // COMMENT
	75: 8,  // EXTSELECTCLIPRGN (ignored by the decoder)
	88: 28, // POLYBEZIERTO16, followed by point count and points
	89: 28, // POLYLINETO16, followed by point count and points
	95: 52, // EXTCREATEPEN, followed by optional style/bitmap data
}

func decodeEMFImage(data []byte) (img image.Image, err error) {
	if err := validateEMF(data); err != nil {
		return nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			img = nil
			err = fmt.Errorf("EMF decoder panic: %v", recovered)
		}
	}()

	file, err := emf.ReadFile(data)
	if err != nil {
		return nil, err
	}
	if file.Header == nil {
		return nil, fmt.Errorf("EMF has no header")
	}
	img = file.Draw()
	if img == nil || img.Bounds().Empty() {
		return nil, fmt.Errorf("EMF produced an empty image")
	}
	return img, nil
}

func validateEMF(data []byte) error {
	if len(data) < emfHeaderSize+20 || len(data) > maxEMFBytes {
		return fmt.Errorf("EMF size %d is outside the supported range", len(data))
	}
	if binary.LittleEndian.Uint32(data[0:4]) != 1 || binary.LittleEndian.Uint32(data[4:8]) != emfHeaderSize {
		return fmt.Errorf("EMF header record is invalid")
	}
	if binary.LittleEndian.Uint32(data[40:44]) != emfSignature {
		return fmt.Errorf("EMF signature is invalid")
	}
	if binary.LittleEndian.Uint32(data[48:52]) != uint32(len(data)) {
		return fmt.Errorf("EMF byte count does not match its data")
	}

	left := int64(int32(binary.LittleEndian.Uint32(data[8:12])))
	top := int64(int32(binary.LittleEndian.Uint32(data[12:16])))
	right := int64(int32(binary.LittleEndian.Uint32(data[16:20])))
	bottom := int64(int32(binary.LittleEndian.Uint32(data[20:24])))
	width, height := right-left, bottom-top
	rasterWidth, rasterHeight := width+1, height+1
	if width <= 0 || height <= 0 || rasterWidth > maxEMFDimension || rasterHeight > maxEMFDimension || rasterWidth*rasterHeight > maxEMFPixelCount {
		return fmt.Errorf("EMF bounds %dx%d exceed the supported image size", width, height)
	}
	for _, coordinate := range []int64{left, top, right, bottom} {
		if coordinate < -1_000_000 || coordinate > 1_000_000 {
			return fmt.Errorf("EMF bounds exceed the supported coordinate range")
		}
	}

	wantRecords := binary.LittleEndian.Uint32(data[52:56])
	if wantRecords < 2 || wantRecords > maxEMFRecords {
		return fmt.Errorf("EMF record count %d exceeds the supported range", wantRecords)
	}
	var records uint32
	for offset := 0; offset < len(data); {
		if len(data)-offset < 8 {
			return fmt.Errorf("truncated EMF record header at byte %d", offset)
		}
		typ := binary.LittleEndian.Uint32(data[offset : offset+4])
		size := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		if size < 8 || size%4 != 0 || uint64(size) > uint64(len(data)-offset) {
			return fmt.Errorf("invalid EMF record size %d at byte %d", size, offset)
		}
		minimum, supported := supportedEMFRecordSizes[typ]
		if !supported || size < minimum {
			return fmt.Errorf("unsupported or truncated EMF record %d", typ)
		}
		if offset == 0 && (typ != 1 || size != emfHeaderSize) {
			return fmt.Errorf("EMF header is not the first complete record")
		}
		if offset != 0 && typ == 1 {
			return fmt.Errorf("EMF contains a second header record")
		}
		if err := validateEMFRecord(data[offset:offset+int(size)], typ); err != nil {
			return fmt.Errorf("invalid EMF record %d at byte %d: %w", typ, offset, err)
		}
		records++
		if records > maxEMFRecords {
			return fmt.Errorf("EMF contains too many records")
		}
		atEnd := offset+int(size) == len(data)
		if typ == emfEndRecord && !atEnd {
			return fmt.Errorf("EMF EOF record is not last")
		}
		if atEnd && typ != emfEndRecord {
			return fmt.Errorf("EMF has no final EOF record")
		}
		offset += int(size)
	}
	if records != wantRecords {
		return fmt.Errorf("EMF record count is %d, header declares %d", records, wantRecords)
	}
	return nil
}

func validateEMFRecord(record []byte, typ uint32) error {
	switch typ {
	case emfBezierTo16, emfPolylineTo16:
		count := binary.LittleEndian.Uint32(record[24:28])
		if uint64(count) > uint64((len(record)-28)/4) {
			return fmt.Errorf("point count %d exceeds record data", count)
		}
		if typ == emfBezierTo16 && (count == 0 || count%3 != 0) {
			return fmt.Errorf("Bezier point count %d is invalid", count)
		}
	case emfExtCreatePen:
		penStyle := binary.LittleEndian.Uint32(record[28:32])
		styleCount := binary.LittleEndian.Uint32(record[48:52])
		if penStyle == 7 && uint64(styleCount) > uint64((len(record)-52)/4) {
			return fmt.Errorf("pen style count %d exceeds record data", styleCount)
		}
		offBMI := binary.LittleEndian.Uint32(record[12:16])
		cbBMI := binary.LittleEndian.Uint32(record[16:20])
		offBits := binary.LittleEndian.Uint32(record[20:24])
		cbBits := binary.LittleEndian.Uint32(record[24:28])
		if cbBits > uint32(len(record)-52) {
			return fmt.Errorf("pen bitmap data exceeds record size")
		}
		if cbBMI > 0 && offBMI >= 52 {
			styleBytes := uint64(0)
			if penStyle == 7 {
				styleBytes = uint64(styleCount) * 4
			}
			if uint64(offBMI) < 52+styleBytes || uint64(offBMI) > uint64(len(record)) || uint64(offBMI)+uint64(cbBits) > uint64(len(record)) {
				return fmt.Errorf("pen bitmap offsets exceed record data")
			}
			if offBits > uint32(len(record)) || uint64(offBits)+uint64(cbBits) > uint64(len(record)) {
				return fmt.Errorf("pen bitmap bits exceed record data")
			}
		}
	}
	return nil
}
