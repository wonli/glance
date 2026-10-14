package xls

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

const (
	maxSSTStrings = 100_000
	maxSSTBytes   = 16 << 20
)

type SstInfo struct {
	Total uint32
	Count uint32
}

type sstCursor struct {
	segments [][]byte
	segment  int
	offset   int
}

func (w *WorkBook) parseSST(segments [][]byte) error {
	if len(segments) == 0 || len(segments[0]) < 8 {
		return fmt.Errorf("SST header is truncated")
	}
	total := binary.LittleEndian.Uint32(segments[0][:4])
	unique := binary.LittleEndian.Uint32(segments[0][4:8])
	if total < unique {
		return fmt.Errorf("SST total reference count %d is smaller than unique string count %d", total, unique)
	}
	if unique > maxSSTStrings {
		return fmt.Errorf("SST has %d unique strings, exceeding the %d-string parser budget", unique, maxSSTStrings)
	}
	w.sst = make([]string, int(unique))
	cursor := sstCursor{segments: segments, segment: 0, offset: 8}
	for index := range w.sst {
		cursor.skipExhaustedSegments()
		header, err := cursor.readFixed(3)
		if err != nil {
			return fmt.Errorf("SST string %d header: %w", index+1, err)
		}
		charCount := int(binary.LittleEndian.Uint16(header[:2]))
		flags := header[2]
		var richRunCount uint16
		var phoneticBytes uint32
		if flags&0x08 != 0 {
			value, err := cursor.readFixed(2)
			if err != nil {
				return fmt.Errorf("SST string %d rich-run count: %w", index+1, err)
			}
			richRunCount = binary.LittleEndian.Uint16(value)
		}
		if flags&0x04 != 0 {
			value, err := cursor.readFixed(4)
			if err != nil {
				return fmt.Errorf("SST string %d phonetic length: %w", index+1, err)
			}
			phoneticBytes = binary.LittleEndian.Uint32(value)
			if uint64(phoneticBytes) > maxSSTBytes {
				return fmt.Errorf("SST string %d phonetic data length %d exceeds parser budget", index+1, phoneticBytes)
			}
		}
		text, err := cursor.readCharacters(charCount, flags&0x01 != 0)
		if err != nil {
			return fmt.Errorf("SST string %d characters: %w", index+1, err)
		}
		runBytes := uint64(richRunCount) * 4
		if err := cursor.skip(runBytes); err != nil {
			return fmt.Errorf("SST string %d rich runs: %w", index+1, err)
		}
		if err := cursor.skip(uint64(phoneticBytes)); err != nil {
			return fmt.Errorf("SST string %d phonetic data: %w", index+1, err)
		}
		w.sst[index] = text
	}
	return nil
}

func (c *sstCursor) skipExhaustedSegments() {
	for c.segment < len(c.segments) && c.offset == len(c.segments[c.segment]) {
		c.segment++
		c.offset = 0
	}
}

func (c *sstCursor) readFixed(count int) ([]byte, error) {
	c.skipExhaustedSegments()
	if count < 0 || c.segment >= len(c.segments) || count > len(c.segments[c.segment])-c.offset {
		return nil, io.ErrUnexpectedEOF
	}
	value := c.segments[c.segment][c.offset : c.offset+count]
	c.offset += count
	return value, nil
}

func (c *sstCursor) readCharacters(count int, highByte bool) (string, error) {
	var result strings.Builder
	remaining := count
	for remaining > 0 {
		c.skipExhaustedSegments()
		if c.segment >= len(c.segments) {
			return "", io.ErrUnexpectedEOF
		}
		if c.offset == 0 && c.segment > 0 {
			// A CONTINUE record splits character data only after an option byte
			// that can change the compression mode for the remaining text.
			option, err := c.readFixed(1)
			if err != nil {
				return "", err
			}
			highByte = option[0]&1 != 0
		}
		payload := c.segments[c.segment]
		available := len(payload) - c.offset
		width := 1
		if highByte {
			width = 2
		}
		characters := available / width
		if characters == 0 {
			if highByte && available != 0 {
				return "", fmt.Errorf("UTF-16 character crosses a BIFF record boundary")
			}
			c.segment++
			c.offset = 0
			continue
		}
		if characters > remaining {
			characters = remaining
		}
		end := c.offset + characters*width
		part := payload[c.offset:end]
		if highByte {
			units := make([]uint16, characters)
			for index := range units {
				units[index] = binary.LittleEndian.Uint16(part[index*2:])
			}
			result.WriteString(string(utf16.Decode(units)))
		} else {
			for _, character := range part {
				result.WriteRune(rune(character))
			}
		}
		c.offset = end
		remaining -= characters
	}
	return result.String(), nil
}

func (c *sstCursor) skip(count uint64) error {
	for count > 0 {
		c.skipExhaustedSegments()
		if c.segment >= len(c.segments) {
			return io.ErrUnexpectedEOF
		}
		available := uint64(len(c.segments[c.segment]) - c.offset)
		step := count
		if step > available {
			step = available
		}
		c.offset += int(step)
		count -= step
	}
	return nil
}
