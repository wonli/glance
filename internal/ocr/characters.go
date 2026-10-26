package ocr

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"math"
	"sort"
	"strings"
	"unicode"
)

const CharacterGeometryVersion = 3

var ErrCharacterLocalizationUnavailable = errors.New("本机文字定位不可用")

type detectedCharacter struct {
	Text   string     `json:"text"`
	Bounds [4]float64 `json:"bounds"`
}
type detectedTextLine struct {
	Bounds     [4]float64          `json:"bounds"`
	Characters []detectedCharacter `json:"characters"`
}

// LocalizeCharacters supplements recognition with observed image geometry. It
// never divides a paragraph into assumed equal-width character cells.
func LocalizeCharacters(ctx context.Context, source image.Image, layout Layout) (Layout, error) {
	return localizeCharacters(ctx, source, layout, recognizeCharacterImage)
}

func localizeCharacters(ctx context.Context, source image.Image, layout Layout, recognize func([]byte) ([]detectedTextLine, error)) (Layout, error) {
	if source == nil || layout.Width <= 0 || layout.Height <= 0 {
		return layout, errors.New("ocr: invalid localization image")
	}
	layout.Characters = nil
	layout.CharacterVersion = 0
	width, height := source.Bounds().Dx(), source.Bounds().Dy()
	succeeded := false
	var lastError error
	for _, block := range layout.Blocks {
		if err := ctx.Err(); err != nil {
			return layout, err
		}
		if strings.TrimSpace(block.Text) == "" || block.Label == "image" || strings.Contains(block.Label, "footer") || strings.HasPrefix(block.Label, "header") || block.Label == "number" {
			continue
		}
		b := block.Bounds
		crop := image.Rect(int(math.Floor(b[0]*float64(width)/float64(layout.Width)))-2, int(math.Floor(b[1]*float64(height)/float64(layout.Height)))-2, int(math.Ceil(b[2]*float64(width)/float64(layout.Width)))+2, int(math.Ceil(b[3]*float64(height)/float64(layout.Height)))+2).Intersect(image.Rect(0, 0, width, height))
		if crop.Empty() {
			continue
		}
		pixels := image.NewRGBA(image.Rectangle{Max: crop.Size()})
		draw.Draw(pixels, pixels.Bounds(), source, source.Bounds().Min.Add(crop.Min), draw.Src)
		targetRunes := []rune(block.Text)
		count := 0
		for _, r := range targetRunes {
			if !unicode.IsSpace(r) {
				count++
			}
		}
		vertical := strings.Contains(block.Label, "vertical") || (count <= 30 && crop.Dy() > crop.Dx()*2 && !strings.Contains(block.Text, "\n"))
		recognitionImage := pixels
		var cells []verticalCharacterCell
		if vertical {
			recognitionImage, cells = unfoldVerticalCharacters(pixels, count)
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, recognitionImage); err != nil {
			return layout, err
		}
		lines, err := recognize(encoded.Bytes())
		if err != nil {
			lastError = err
			continue
		}
		succeeded = true
		lines = foldVerticalCharacterLines(lines, cells, recognitionImage.Bounds().Size(), pixels)
		sort.SliceStable(lines, func(i, j int) bool {
			a, b := lines[i].Bounds, lines[j].Bounds
			if vertical {
				if math.Abs(a[0]-b[0]) > math.Min(a[2]-a[0], b[2]-b[0])/2 {
					return a[0] > b[0]
				}
				return a[1] < b[1]
			}
			if math.Abs(a[1]-b[1]) > math.Min(a[3]-a[1], b[3]-b[1])/2 {
				return a[1] < b[1]
			}
			return a[0] < b[0]
		})
		var observed []detectedCharacter
		for _, line := range lines {
			observed = append(observed, line.Characters...)
		}
		for _, pair := range alignCharacterGeometry(block.Text, observed) {
			r := observed[pair.observed].Bounds
			bounds := [4]float64{
				(float64(crop.Min.X) + r[0]*float64(crop.Dx())) * float64(layout.Width) / float64(width),
				(float64(crop.Min.Y) + r[1]*float64(crop.Dy())) * float64(layout.Height) / float64(height),
				(float64(crop.Min.X) + r[2]*float64(crop.Dx())) * float64(layout.Width) / float64(width),
				(float64(crop.Min.Y) + r[3]*float64(crop.Dy())) * float64(layout.Height) / float64(height),
			}
			if bounds[2] <= bounds[0] || bounds[3] <= bounds[1] {
				continue
			}
			layout.Characters = append(layout.Characters, CharacterBox{BlockID: block.ID, Offset: pair.offset, Text: string(targetRunes[pair.offset]), Bounds: bounds})
		}
	}
	if lastError != nil {
		return layout, lastError
	}
	if err := ctx.Err(); err != nil {
		return layout, err
	}
	if !succeeded || len(layout.Characters) == 0 {
		return layout, errors.New("ocr: no reliable character geometry was localized")
	}
	layout.CharacterVersion = CharacterGeometryVersion
	return layout, nil
}

type characterGeometryPair struct {
	offset, observed int
	equal            bool
}

// Align local geometry to the authoritative cloud text. Replacement runs of up
// to three characters require exact neighbours in a strongly matching block.
// Repeated word/line-sized boxes are rejected instead of called glyphs.
func alignCharacterGeometry(target string, observed []detectedCharacter) []characterGeometryPair {
	var a, b []rune
	var offsets, indices []int
	for offset, r := range []rune(target) {
		if !unicode.IsSpace(r) {
			a = append(a, r)
			offsets = append(offsets, offset)
		}
	}
	duplicates := make(map[[4]float64]int)
	for _, c := range observed {
		duplicates[c.Bounds]++
	}
	for i, c := range observed {
		rs := []rune(c.Text)
		if len(rs) == 1 && !unicode.IsSpace(rs[0]) {
			b = append(b, rs[0])
			indices = append(indices, i)
		}
	}
	rows, cols := len(a)+1, len(b)+1
	if len(a) == 0 || len(b) == 0 || rows > 4_000_000/cols {
		return nil
	}
	trace := make([]byte, rows*cols)
	prev, cur := make([]int, cols), make([]int, cols)
	for j := 1; j < cols; j++ {
		prev[j] = j
		trace[j] = 2
	}
	for i := 1; i < rows; i++ {
		cur[0] = i
		trace[i*cols] = 1
		for j := 1; j < cols; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			best, dir := prev[j-1]+cost, byte(0)
			if prev[j]+1 < best {
				best, dir = prev[j]+1, 1
			}
			if cur[j-1]+1 < best {
				best, dir = cur[j-1]+1, 2
			}
			cur[j] = best
			trace[i*cols+j] = dir
		}
		prev, cur = cur, prev
	}
	var pairs []characterGeometryPair
	matches := 0
	for i, j := len(a), len(b); i > 0 || j > 0; {
		switch trace[i*cols+j] {
		case 0:
			equal := a[i-1] == b[j-1]
			if equal {
				matches++
			}
			pairs = append(pairs, characterGeometryPair{offset: offsets[i-1], observed: indices[j-1], equal: equal})
			i--
			j--
		case 1:
			i--
		case 2:
			j--
		}
	}
	if matches*2 < max(len(a), len(b)) {
		return nil
	}
	for left, right := 0, len(pairs)-1; left < right; left, right = left+1, right-1 {
		pairs[left], pairs[right] = pairs[right], pairs[left]
	}
	var result []characterGeometryPair
	for i, pair := range pairs {
		box := observed[pair.observed].Bounds
		if duplicates[box] > 1 || box[0] < 0 || box[1] < 0 || box[2] > 1 || box[3] > 1 || box[2] <= box[0] || box[3] <= box[1] {
			continue
		}
		if !pair.equal && (matches*100 < max(len(a), len(b))*65 || !characterGeometryReplacementAnchored(pairs, i, observed, duplicates, len(a), len(b))) {
			continue
		}

		result = append(result, pair)
	}
	return result
}

// Use measured glyph rectangles for a short substitution run only when exact
// neighbours bound a one-to-one, contiguous mapping on the same observed line.
func characterGeometryReplacementAnchored(pairs []characterGeometryPair, index int, observed []detectedCharacter, duplicates map[[4]float64]int, targetCount, observedCount int) bool {
	start, end := index, index
	for start > 0 && !pairs[start-1].equal {
		start--
	}
	for end+1 < len(pairs) && !pairs[end+1].equal {
		end++
	}
	if end-start+1 > 3 {
		return false
	}
	left, right := start-1, end+1
	if left < 0 || right == len(pairs) {
		// A title edge has only one side to anchor. Require one substitution,
		// three exact neighbours, and no insertion/deletion anywhere in the block.
		if start != end || targetCount != observedCount || len(pairs) != targetCount || len(pairs) < 4 {
			return false
		}
		if left < 0 {
			left, right = 0, 3
		} else {
			left, right = len(pairs)-4, len(pairs)-1
		}
		for i := left; i <= right; i++ {
			if i != start && !pairs[i].equal {
				return false
			}
		}
	}
	horizontal, vertical := true, true
	for i := left; i <= right; i++ {
		p := pairs[i]
		box := observed[p.observed].Bounds
		if duplicates[box] > 1 || box[0] < 0 || box[1] < 0 || box[2] > 1 || box[3] > 1 || box[2] <= box[0] || box[3] <= box[1] {
			return false
		}
		if i == left {
			continue
		}
		prev := pairs[i-1]
		if prev.offset+1 != p.offset || prev.observed+1 != p.observed {
			return false
		}
		before := observed[prev.observed].Bounds
		yOverlap := math.Min(before[3], box[3]) - math.Max(before[1], box[1])
		xOverlap := math.Min(before[2], box[2]) - math.Max(before[0], box[0])
		horizontal = horizontal && 2*yOverlap >= math.Min(before[3]-before[1], box[3]-box[1]) && box[0] > before[0]
		vertical = vertical && 2*xOverlap >= math.Min(before[2]-before[0], box[2]-box[0]) && box[1] > before[1]
	}
	return horizontal || vertical
}
