package csv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// fileIndex holds the line offset index and CSV format properties for a file on disk.
type fileIndex struct {
	offsets   []int64 // offsets[i] is the byte offset of row i (0-based)
	totalRows int
	totalCols int
	hasBOM    bool
	useCRLF   bool
	comma     rune
	fileSize  int64
}

// scanFileIndex scans a CSV file and builds an array of row start byte offsets.
// Handles RFC 4180 quotes, delimiters, BOM, and line endings.
func scanFileIndex(f *os.File) (*fileIndex, error) {
	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat csv file: %w", err)
	}
	fileSize := stat.Size()
	if fileSize == 0 {
		return &fileIndex{
			offsets:   []int64{0},
			totalRows: 0,
			totalCols: 0,
			comma:     ',',
			fileSize:  0,
		}, nil
	}

	// Read initial header chunk (up to 64KB) for BOM and delimiter detection
	headSize := int64(65536)
	if headSize > fileSize {
		headSize = fileSize
	}
	headBuf := make([]byte, headSize)
	if _, err := f.ReadAt(headBuf, 0); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read header: %w", err)
	}

	startOffset := int64(0)
	hasBOM := false
	if len(headBuf) >= 3 && headBuf[0] == 0xef && headBuf[1] == 0xbb && headBuf[2] == 0xbf {
		hasBOM = true
		startOffset = 3
	}

	useCRLF := bytes.Contains(headBuf, []byte("\r\n"))
	comma := detectDelimiter(headBuf[startOffset:])

	// Scan row offsets across the entire file
	// Initial capacity estimate: fileSize / 80
	initCap := int(fileSize / 80)
	if initCap < 16 {
		initCap = 16
	} else if initCap > 3000000 {
		initCap = 3000000
	}
	offsets := make([]int64, 0, initCap)
	offsets = append(offsets, startOffset)

	bufSize := 1024 * 1024 // 1 MB buffer
	buf := make([]byte, bufSize)
	currentOffset := startOffset
	inQuote := false

	for {
		n, readErr := f.ReadAt(buf, currentOffset)
		if n > 0 {
			chunk := buf[:n]
			for i := 0; i < n; i++ {
				b := chunk[i]
				if b == '"' {
					inQuote = !inQuote
				} else if b == '\n' && !inQuote {
					nextRowOffset := currentOffset + int64(i) + 1
					if nextRowOffset < fileSize {
						offsets = append(offsets, nextRowOffset)
					}
				}
			}
			currentOffset += int64(n)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) || currentOffset >= fileSize {
				break
			}
			return nil, fmt.Errorf("scan csv offsets: %w", readErr)
		}
	}

	totalRows := len(offsets)

	// Measure totalCols from the first row
	totalCols := 0
	if totalRows > 0 {
		firstRowEnd := fileSize
		if len(offsets) > 1 {
			firstRowEnd = offsets[1]
		}
		rowBytes := make([]byte, firstRowEnd-offsets[0])
		if _, err := f.ReadAt(rowBytes, offsets[0]); err == nil || errors.Is(err, io.EOF) {
			r := csv.NewReader(bytes.NewReader(rowBytes))
			r.Comma = comma
			r.LazyQuotes = true
			r.FieldsPerRecord = -1
			if rec, err := r.Read(); err == nil {
				totalCols = len(rec)
			}
		}
	}

	return &fileIndex{
		offsets:   offsets,
		totalRows: totalRows,
		totalCols: totalCols,
		hasBOM:    hasBOM,
		useCRLF:   useCRLF,
		comma:     comma,
		fileSize:  fileSize,
	}, nil
}

// readRowRange reads a contiguous range of rows from disk and parses them.
func (idx *fileIndex) readRowRange(f *os.File, startRow, count int) ([][]string, error) {
	if startRow < 0 || startRow >= idx.totalRows || count <= 0 {
		return nil, nil
	}
	endRow := startRow + count
	if endRow > idx.totalRows {
		endRow = idx.totalRows
	}

	startByte := idx.offsets[startRow]
	endByte := idx.fileSize
	if endRow < len(idx.offsets) {
		endByte = idx.offsets[endRow]
	}

	byteLen := endByte - startByte
	if byteLen <= 0 {
		return make([][]string, endRow-startRow), nil
	}

	buf := make([]byte, byteLen)
	if _, err := f.ReadAt(buf, startByte); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read csv row range: %w", err)
	}

	r := csv.NewReader(bytes.NewReader(buf))
	r.Comma = idx.comma
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	records, err := r.ReadAll()
	if err != nil && !errors.Is(err, io.EOF) {
		// Fallback: line by line if full chunk failed
		records = make([][]string, 0, endRow-startRow)
		lines := strings.Split(string(buf), "\n")
		for _, line := range lines {
			line = strings.TrimRight(line, "\r")
			if line == "" && len(records) >= (endRow-startRow) {
				continue
			}
			subR := csv.NewReader(strings.NewReader(line))
			subR.Comma = idx.comma
			subR.LazyQuotes = true
			subR.FieldsPerRecord = -1
			rec, _ := subR.Read()
			records = append(records, rec)
			if len(records) >= (endRow-startRow) {
				break
			}
		}
	}

	return records, nil
}
