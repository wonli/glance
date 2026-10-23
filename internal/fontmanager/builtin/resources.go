// Package builtin exposes font resources distributed with fontmanager.
// Rendering toolkits consume these bytes in host adapters.
package builtin

import (
	"embed"
	"errors"
	"fmt"
	"sync"
)

const EmojiFamily = "Glance Color Emoji"
const EmojiResourceID = "builtin:noto-color-emoji:v1"

//go:embed assets/NotoColorEmoji.ttf assets/OFL.txt assets/manifest.json
var assets embed.FS

var emojiFont struct {
	sync.Once
	data []byte
	err  error
}

// EmojiFont returns a caller-owned copy of the bundled emoji font with its
// proportional space metric normalized for consistent inline shaping.
func EmojiFont() ([]byte, error) {
	emojiFont.Do(func() {
		emojiFont.data, emojiFont.err = assets.ReadFile("assets/NotoColorEmoji.ttf")
		if emojiFont.err != nil {
			emojiFont.err = errors.New("read bundled emoji font: " + emojiFont.err.Error())
			return
		}
		if emojiFont.data, emojiFont.err = normalizeEmojiSpaceAdvance(emojiFont.data); emojiFont.err != nil {
			emojiFont.err = fmt.Errorf("normalize bundled emoji spacing: %w", emojiFont.err)
		}
	})
	if emojiFont.err != nil {
		return nil, emojiFont.err
	}
	return append([]byte(nil), emojiFont.data...), nil
}
