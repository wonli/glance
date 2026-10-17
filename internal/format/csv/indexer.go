package csv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
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

	cacheMu  sync.Mutex
	rowCache map[int][]string
	rowFIFO  []int
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
	var offsets []int64
	if (fileSize-startOffset) >= 8*1024*1024 && runtime.GOMAXPROCS(0) >= 2 {
		offsets, err = scanOffsetsParallel(f, fileSize, startOffset)
	} else {
		offsets, err = scanOffsetsSequential(f, fileSize, startOffset)
	}
	if err != nil {
		return nil, err
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

// scanOffsetsParallel uses multi-core workers to count quotes and index row offsets in chunks.
func scanOffsetsParallel(f *os.File, fileSize, startOffset int64) ([]int64, error) {
	numWorkers := runtime.GOMAXPROCS(0)
	if numWorkers > 8 {
		numWorkers = 8
	}
	totalLen := fileSize - startOffset
	chunkSize := (totalLen + int64(numWorkers) - 1) / int64(numWorkers)
	if chunkSize < 2*1024*1024 {
		chunkSize = 2 * 1024 * 1024
	}
	numChunks := int((totalLen + chunkSize - 1) / chunkSize)

	type chunkMeta struct {
		start, end int64
		quoteCount int
		offsets    []int64
		err        error
	}
	chunks := make([]chunkMeta, numChunks)
	for i := 0; i < numChunks; i++ {
		cStart := startOffset + int64(i)*chunkSize
		cEnd := cStart + chunkSize
		if cEnd > fileSize {
			cEnd = fileSize
		}
		chunks[i].start = cStart
		chunks[i].end = cEnd
	}

	// Phase 1: parallel quote counting with bytes.Count
	var wg sync.WaitGroup
	wg.Add(numChunks)
	for i := 0; i < numChunks; i++ {
		go func(idx int) {
			defer wg.Done()
			c := &chunks[idx]
			buf := make([]byte, 2*1024*1024)
			curr := c.start
			for curr < c.end {
				toRead := int(c.end - curr)
				if toRead > len(buf) {
					toRead = len(buf)
				}
				n, err := f.ReadAt(buf[:toRead], curr)
				if n > 0 {
					c.quoteCount += bytes.Count(buf[:n], []byte{'"'})
					curr += int64(n)
				}
				if err != nil {
					if errors.Is(err, io.EOF) || curr >= c.end {
						break
					}
					c.err = err
					return
				}
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < numChunks; i++ {
		if chunks[i].err != nil {
			return nil, chunks[i].err
		}
	}

	// Compute initial inQuote state for each chunk
	inQuote := false
	chunkInQuotes := make([]bool, numChunks)
	for i := 0; i < numChunks; i++ {
		chunkInQuotes[i] = inQuote
		if chunks[i].quoteCount%2 != 0 {
			inQuote = !inQuote
		}
	}

	// Phase 2: parallel row offset scanning
	wg.Add(numChunks)
	for i := 0; i < numChunks; i++ {
		go func(idx int) {
			defer wg.Done()
			c := &chunks[idx]
			initCap := int((c.end - c.start) / 80)
			if initCap < 16 {
				initCap = 16
			}
			c.offsets = make([]int64, 0, initCap)
			buf := make([]byte, 2*1024*1024)
			curr := c.start
			q := chunkInQuotes[idx]
			for curr < c.end {
				toRead := int(c.end - curr)
				if toRead > len(buf) {
					toRead = len(buf)
				}
				n, err := f.ReadAt(buf[:toRead], curr)
				if n > 0 {
					chunk := buf[:n]
					pos := 0
					for pos < n {
						nextIdx := bytes.IndexAny(chunk[pos:], "\"\n")
						if nextIdx < 0 {
							break
						}
						pos += nextIdx
						if chunk[pos] == '"' {
							q = !q
						} else if !q {
							nextRowOffset := curr + int64(pos) + 1
							if nextRowOffset < fileSize {
								c.offsets = append(c.offsets, nextRowOffset)
							}
						}
						pos++
					}
					curr += int64(n)
				}
				if err != nil {
					if errors.Is(err, io.EOF) || curr >= c.end {
						break
					}
					c.err = err
					return
				}
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < numChunks; i++ {
		if chunks[i].err != nil {
			return nil, chunks[i].err
		}
	}

	totalRows := 1
	for i := 0; i < numChunks; i++ {
		totalRows += len(chunks[i].offsets)
	}
	offsets := make([]int64, 0, totalRows)
	offsets = append(offsets, startOffset)
	for i := 0; i < numChunks; i++ {
		offsets = append(offsets, chunks[i].offsets...)
	}
	return offsets, nil
}

// scanOffsetsSequential scans row offsets in a single thread with a 4MB buffer.
func scanOffsetsSequential(f *os.File, fileSize, startOffset int64) ([]int64, error) {
	initCap := int((fileSize - startOffset) / 80)
	if initCap < 16 {
		initCap = 16
	} else if initCap > 3000000 {
		initCap = 3000000
	}
	offsets := make([]int64, 0, initCap)
	offsets = append(offsets, startOffset)

	bufSize := 4 * 1024 * 1024 // 4 MB buffer
	buf := make([]byte, bufSize)
	currentOffset := startOffset
	inQuote := false

	for {
		n, readErr := f.ReadAt(buf, currentOffset)
		if n > 0 {
			chunk := buf[:n]
			i := 0
			for i < n {
				idx := bytes.IndexAny(chunk[i:], "\"\n")
				if idx < 0 {
					break
				}
				i += idx
				if chunk[i] == '"' {
					inQuote = !inQuote
				} else if !inQuote {
					nextRowOffset := currentOffset + int64(i) + 1
					if nextRowOffset < fileSize {
						offsets = append(offsets, nextRowOffset)
					}
				}
				i++
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
	return offsets, nil
}

// readRowRange reads a contiguous range of rows from disk or LRU cache and parses them.
func (idx *fileIndex) readRowRange(f *os.File, startRow, count int) ([][]string, error) {
	if startRow < 0 || startRow >= idx.totalRows || count <= 0 {
		return nil, nil
	}
	endRow := startRow + count
	if endRow > idx.totalRows {
		endRow = idx.totalRows
	}

	result := make([][]string, endRow-startRow)
	allCached := true

	idx.cacheMu.Lock()
	if idx.rowCache != nil {
		for r := startRow; r < endRow; r++ {
			if rowData, ok := idx.rowCache[r]; ok {
				result[r-startRow] = rowData
			} else {
				allCached = false
				break
			}
		}
	} else {
		allCached = false
	}
	idx.cacheMu.Unlock()

	if allCached {
		return result, nil
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

	// Cache the newly read rows in our LRU cache (capacity: 512 rows)
	idx.cacheMu.Lock()
	if idx.rowCache == nil {
		idx.rowCache = make(map[int][]string, 512)
		idx.rowFIFO = make([]int, 0, 512)
	}
	for i, rec := range records {
		rowNum := startRow + i
		if _, exists := idx.rowCache[rowNum]; !exists {
			if len(idx.rowFIFO) >= 512 {
				evictRow := idx.rowFIFO[0]
				idx.rowFIFO = idx.rowFIFO[1:]
				delete(idx.rowCache, evictRow)
			}
			idx.rowCache[rowNum] = rec
			idx.rowFIFO = append(idx.rowFIFO, rowNum)
		}
	}
	idx.cacheMu.Unlock()

	return records, nil
}
