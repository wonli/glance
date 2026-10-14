package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
)

type icnsChunk struct {
	tag  string
	data []byte
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: combine-icns OUTPUT.icns INPUT.icns... [--png TAG INPUT.png ...]")
	}
	output := args[0]
	chunks := make([]icnsChunk, 0, len(args))
	seen := make(map[string]bool)
	for i := 1; i < len(args); i++ {
		if args[i] == "--png" {
			if i+2 >= len(args) {
				return errors.New("--png requires a four-character chunk tag and a PNG path")
			}
			tag, path := args[i+1], args[i+2]
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read PNG %q: %w", path, err)
			}
			if len(tag) != 4 || !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
				return fmt.Errorf("invalid PNG chunk %q from %q", tag, path)
			}
			if err := addChunk(&chunks, seen, icnsChunk{tag: tag, data: makeChunk(tag, data)}); err != nil {
				return err
			}
			i += 2
			continue
		}
		data, err := os.ReadFile(args[i])
		if err != nil {
			return fmt.Errorf("read ICNS %q: %w", args[i], err)
		}
		parsed, err := chunksFromICNS(data)
		if err != nil {
			return fmt.Errorf("read ICNS chunks from %q: %w", args[i], err)
		}
		for _, chunk := range parsed {
			if err := addChunk(&chunks, seen, chunk); err != nil {
				return err
			}
		}
	}
	if len(chunks) == 0 {
		return errors.New("no ICNS image chunks were supplied")
	}
	sort.SliceStable(chunks, func(i, j int) bool {
		return icnsChunkRank(chunks[i].tag) < icnsChunkRank(chunks[j].tag)
	})
	data, err := encodeICNS(chunks)
	if err != nil {
		return err
	}
	if err := os.WriteFile(output, data, 0o644); err != nil {
		return fmt.Errorf("write ICNS %q: %w", output, err)
	}
	return nil
}

func chunksFromICNS(data []byte) ([]icnsChunk, error) {
	if len(data) < 8 || string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:8])) != len(data) {
		return nil, errors.New("invalid ICNS header")
	}
	chunks := make([]icnsChunk, 0, 2)
	for offset := 8; offset < len(data); {
		if len(data)-offset < 8 {
			return nil, errors.New("truncated ICNS chunk header")
		}
		tag := string(data[offset : offset+4])
		length := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		if length < 8 || length > len(data)-offset {
			return nil, fmt.Errorf("invalid %q chunk length %d", tag, length)
		}
		if tag != "TOC " {
			chunk := append([]byte(nil), data[offset:offset+length]...)
			chunks = append(chunks, icnsChunk{tag: tag, data: chunk})
		}
		offset += length
	}
	return chunks, nil
}

func addChunk(chunks *[]icnsChunk, seen map[string]bool, chunk icnsChunk) error {
	if seen[chunk.tag] {
		return fmt.Errorf("duplicate ICNS chunk type %q", chunk.tag)
	}
	seen[chunk.tag] = true
	*chunks = append(*chunks, chunk)
	return nil
}

func makeChunk(tag string, payload []byte) []byte {
	data := make([]byte, 8+len(payload))
	copy(data[:4], tag)
	binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
	copy(data[8:], payload)
	return data
}

func encodeICNS(chunks []icnsChunk) ([]byte, error) {
	var tocEntries bytes.Buffer
	var body bytes.Buffer
	for _, chunk := range chunks {
		if len(chunk.data) < 8 || len(chunk.data) > int(^uint32(0)) {
			return nil, fmt.Errorf("invalid %q chunk size %d", chunk.tag, len(chunk.data))
		}
		tocEntries.WriteString(chunk.tag)
		if err := binary.Write(&tocEntries, binary.BigEndian, uint32(len(chunk.data))); err != nil {
			return nil, err
		}
		body.Write(chunk.data)
	}
	toc := makeChunk("TOC ", tocEntries.Bytes())
	totalSize := 8 + len(toc) + body.Len()
	if totalSize > int(^uint32(0)) {
		return nil, errors.New("ICNS file is too large")
	}
	result := make([]byte, 8+len(toc)+body.Len())
	copy(result[:4], "icns")
	binary.BigEndian.PutUint32(result[4:8], uint32(totalSize))
	copy(result[8:], toc)
	copy(result[8+len(toc):], body.Bytes())
	return result, nil
}

func icnsChunkRank(tag string) int {
	ranks := map[string]int{
		"is32": 10, "s8mk": 11,
		"il32": 20, "l8mk": 21,
		"ih32": 30, "h8mk": 31,
		"icp4": 40, "icp5": 50, "icp6": 60,
		"ic07": 70, "ic08": 80, "ic09": 90, "ic10": 100,
	}
	if rank, ok := ranks[tag]; ok {
		return rank
	}
	return 1000
}
