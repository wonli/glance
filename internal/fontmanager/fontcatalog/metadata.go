package fontcatalog

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"unicode/utf16"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

const MaxFontBytes int64 = 64 << 20
const MaxSystemFontBytes int64 = 512 << 20

func Inspect(ctx context.Context, path string, source Source) ([]Face, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	limit := MaxSystemFontBytes
	if source != System {
		limit = MaxFontBytes
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("font exceeds %d bytes", limit)
	}
	loaders, err := ot.NewLoaders(file)
	if err != nil {
		return nil, err
	}
	if len(loaders) > 65536 {
		return nil, fmt.Errorf("too many font faces")
	}
	hash := sha256.New()
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		n, e := file.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > limit {
				return nil, fmt.Errorf("font exceeds %d bytes", limit)
			}
			hash.Write(buffer[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	var out []Face
	for i, ld := range loaders {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		md, _ := font.Describe(ld, nil)
		nameData, _ := ld.RawTable(ot.MustNewTag("name"))
		if md.Family == "" {
			continue
		}
		out = append(out, Face{ID: ResourceID(digest + "#" + strconv.Itoa(i)), Family: md.Family, Aliases: localizedNameTable(nameData), Path: path, Index: uint32(i), Weight: int(md.Aspect.Weight), Italic: md.Aspect.Style == font.StyleItalic, Source: source, Size: info.Size(), ModTime: info.ModTime().UnixNano(), Digest: digest})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("font has no named faces")
	}
	return out, nil
}

// Read the localized family records as well as the parser's preferred name.
// Bounds are checked independently because cache input may be untrusted.
func localizedNameTable(table []byte) []string {
	if len(table) < 6 {
		return nil
	}
	count := int(binary.BigEndian.Uint16(table[2:]))
	start := int(binary.BigEndian.Uint16(table[4:]))
	var out []string
	for i := 0; i < count; i++ {
		p := 6 + i*12
		if p+12 > len(table) {
			break
		}
		id := binary.BigEndian.Uint16(table[p+6:])
		if id != 1 && id != 16 && id != 21 {
			continue
		}
		platform := binary.BigEndian.Uint16(table[p:])
		length := int(binary.BigEndian.Uint16(table[p+8:]))
		off := start + int(binary.BigEndian.Uint16(table[p+10:]))
		if off < 0 || off > len(table)-length {
			continue
		}
		raw := table[off : off+length]
		var value string
		if platform == 0 || platform == 3 {
			u := make([]uint16, len(raw)/2)
			for j := range u {
				u[j] = binary.BigEndian.Uint16(raw[j*2:])
			}
			value = string(utf16.Decode(u))
		} else {
			continue
		}
		if value != "" {
			exists := false
			for _, s := range out {
				if s == value {
					exists = true
				}
			}
			if !exists {
				out = append(out, value)
			}
		}
	}
	return out
}
