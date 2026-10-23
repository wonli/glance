// Package builtin exposes font resources distributed with fontmanager.
// Rendering toolkits consume these bytes in host adapters.
package builtin

import (
	"embed"
	"errors"
)

const EmojiFamily = "Glance Color Emoji"
const EmojiResourceID = "builtin:noto-color-emoji:v1"

//go:embed assets/NotoColorEmoji.ttf assets/OFL.txt assets/manifest.json
var assets embed.FS

// EmojiFont returns a caller-owned copy of the bundled emoji font bytes.
func EmojiFont() ([]byte, error) {
	data, err := assets.ReadFile("assets/NotoColorEmoji.ttf")
	if err != nil {
		return nil, errors.New("read bundled emoji font: " + err.Error())
	}
	return data, nil
}
