package doc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf16"
)

const compoundEnd = uint32(0xfffffffe)
const compoundFree = uint32(0xffffffff)

type compoundReader struct {
	data         []byte
	sectorSize   int
	fat, miniFAT []uint32
	miniStream   []byte
}

func openCompound(data []byte) (*compoundReader, error) {
	if len(data) < 512 || string(data[:8]) != string(compoundFileSignature[:]) {
		return nil, errors.New("invalid Word compound file")
	}
	if binary.LittleEndian.Uint16(data[28:]) != 0xfffe {
		return nil, errors.New("unsupported compound byte order")
	}
	shift := binary.LittleEndian.Uint16(data[30:])
	if shift != 9 && shift != 12 {
		return nil, errors.New("invalid compound sector size")
	}
	if binary.LittleEndian.Uint16(data[32:]) != 6 {
		return nil, errors.New("invalid compound mini-sector size")
	}
	c := &compoundReader{data: data, sectorSize: 1 << shift}
	total := len(data)/c.sectorSize - 1
	if total < 1 {
		return nil, errors.New("truncated compound file")
	}
	fatCount := binary.LittleEndian.Uint32(data[44:])
	if uint64(fatCount) > uint64(total) {
		return nil, errors.New("invalid compound FAT count")
	}
	ids := make([]uint32, 0, int(fatCount))
	for i := 0; i < 109 && len(ids) < int(fatCount); i++ {
		id := binary.LittleEndian.Uint32(data[76+4*i:])
		if id != compoundFree {
			ids = append(ids, id)
		}
	}
	next := binary.LittleEndian.Uint32(data[68:])
	seen := map[uint32]bool{}
	for len(ids) < int(fatCount) && next != compoundEnd {
		if seen[next] {
			return nil, errors.New("cyclic compound DIFAT")
		}
		seen[next] = true
		sec, err := c.sector(next)
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(sec)/4-1 && len(ids) < int(fatCount); i++ {
			id := binary.LittleEndian.Uint32(sec[4*i:])
			if id != compoundFree {
				ids = append(ids, id)
			}
		}
		next = binary.LittleEndian.Uint32(sec[len(sec)-4:])
	}
	if len(ids) != int(fatCount) {
		return nil, errors.New("truncated compound DIFAT")
	}
	for _, id := range ids {
		sec, err := c.sector(id)
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(sec); i += 4 {
			c.fat = append(c.fat, binary.LittleEndian.Uint32(sec[i:]))
		}
	}
	miniStart := binary.LittleEndian.Uint32(data[60:])
	miniCount := binary.LittleEndian.Uint32(data[64:])
	if uint64(miniCount) > uint64(total) {
		return nil, errors.New("invalid mini FAT count")
	}
	if miniCount > 0 {
		m, err := c.chain(miniStart, c.fat, data[c.sectorSize:], c.sectorSize, int(miniCount)*c.sectorSize)
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(m); i += 4 {
			c.miniFAT = append(c.miniFAT, binary.LittleEndian.Uint32(m[i:]))
		}
	}
	return c, nil
}

var compoundFileSignature = [8]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}

func (c *compoundReader) sector(id uint32) ([]byte, error) {
	start := (uint64(id) + 1) * uint64(c.sectorSize)
	end := start + uint64(c.sectorSize)
	if end > uint64(len(c.data)) {
		return nil, errors.New("compound sector outside file")
	}
	return c.data[int(start):int(end)], nil
}

// Every chain is bounded by its backing store and rejects repeated sectors.
func (c *compoundReader) chain(start uint32, table []uint32, store []byte, unit, size int) ([]byte, error) {
	if size == 0 {
		return nil, nil
	}
	if size > len(store) {
		return nil, errors.New("compound stream size exceeds backing store")
	}
	var out []byte
	seen := map[uint32]bool{}
	for id := start; id != compoundEnd; id = table[id] {
		if uint64(id) >= uint64(len(table)) || uint64(id)*uint64(unit)+uint64(unit) > uint64(len(store)) {
			return nil, errors.New("compound stream sector outside backing store")
		}
		if seen[id] {
			return nil, errors.New("cyclic compound stream")
		}
		seen[id] = true
		at := int(id) * unit
		remaining := unit
		if size >= 0 {
			remaining = min(remaining, size-len(out))
		}
		out = append(out, store[at:at+remaining]...)
		if size >= 0 && len(out) == size {
			return out, nil
		}
	}
	if size >= 0 && len(out) != size {
		return nil, errors.New("truncated compound stream")
	}
	return out, nil
}

func readCompoundWordStreams(data []byte) ([]byte, []byte, error) {
	c, err := openCompound(data)
	if err != nil {
		return nil, nil, err
	}
	dirs, err := c.chain(binary.LittleEndian.Uint32(data[48:]), c.fat, data[c.sectorSize:], c.sectorSize, -1)
	if err != nil {
		return nil, nil, err
	}
	type entry struct {
		start uint32
		size  uint64
	}
	entries := map[string]entry{}
	if len(dirs) < 128 || dirs[66] != 5 {
		return nil, nil, errors.New("missing compound root directory")
	}
	streamEntry := func(d []byte) (entry, error) {
		size := binary.LittleEndian.Uint64(d[120:])
		if binary.LittleEndian.Uint16(data[26:]) == 3 {
			size &= 0xffffffff
		}
		if size > uint64(len(data)) {
			return entry{}, errors.New("compound directory stream size exceeds file")
		}
		return entry{binary.LittleEndian.Uint32(d[116:]), size}, nil
	}
	root, err := streamEntry(dirs[:128])
	if err != nil {
		return nil, nil, err
	}
	// Resolve only the root storage's sibling tree. Embedded OLE storages may
	// legitimately repeat stream names, including WordDocument and 0Table.
	pending := []uint32{binary.LittleEndian.Uint32(dirs[76:])}
	seen := map[uint32]bool{}
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if id == compoundFree {
			continue
		}
		if uint64(id) >= uint64(len(dirs)/128) || seen[id] {
			return nil, nil, errors.New("invalid compound directory tree")
		}
		seen[id] = true
		d := dirs[int(id)*128 : (int(id)+1)*128]
		if d[66] != 1 && d[66] != 2 {
			return nil, nil, errors.New("invalid compound directory entry")
		}
		pending = append(pending, binary.LittleEndian.Uint32(d[68:]), binary.LittleEndian.Uint32(d[72:]))
		if d[66] != 2 {
			continue
		}
		l := int(binary.LittleEndian.Uint16(d[64:]))
		if l < 2 || l > 64 || l%2 != 0 {
			return nil, nil, errors.New("invalid compound directory name")
		}
		chars := make([]uint16, l/2-1)
		for i := range chars {
			chars[i] = binary.LittleEndian.Uint16(d[2*i:])
		}
		name := string(utf16.Decode(chars))
		if _, duplicate := entries[name]; duplicate {
			return nil, nil, errors.New("duplicate compound root stream")
		}
		e, err := streamEntry(d)
		if err != nil {
			return nil, nil, err
		}
		entries[name] = e
	}
	if root.size > 0 {
		c.miniStream, err = c.chain(root.start, c.fat, data[c.sectorSize:], c.sectorSize, int(root.size))
		if err != nil {
			return nil, nil, err
		}
	}
	read := func(name string) ([]byte, error) {
		e, ok := entries[name]
		if !ok {
			return nil, fmt.Errorf("missing Word stream %s", name)
		}
		if e.size < 4096 {
			return c.chain(e.start, c.miniFAT, c.miniStream, 64, int(e.size))
		}
		return c.chain(e.start, c.fat, data[c.sectorSize:], c.sectorSize, int(e.size))
	}
	word, err := read("WordDocument")
	if err != nil {
		return nil, nil, err
	}
	if len(word) < 12 {
		return nil, nil, errors.New("truncated Word FIB")
	}
	if binary.LittleEndian.Uint16(word[2:]) < 0xc1 {
		return word, nil, nil
	}
	tableName := "0Table"
	if binary.LittleEndian.Uint16(word[10:])&0x200 != 0 {
		tableName = "1Table"
	}
	table, err := read(tableName)
	return word, table, err
}
