package fontfallback

import (
	"strings"
	"testing"
)

func TestWindowsEmojiPreference(t *testing.T) {
	s := PreferredTypefaceForOS("windows")
	if !strings.HasPrefix(s, "Inter, Microsoft YaHei, ") {
		t.Fatalf("wrong Windows CJK preference %s", s)
	}
	if strings.Index(s, "Glance Color Emoji") < 0 || strings.Index(s, "Glance Color Emoji") > strings.Index(s, "Segoe UI Emoji") {
		t.Fatalf("fallback order=%s", s)
	}
	if strings.Contains(PreferredTypefaceForOS("darwin"), "Glance Color Emoji") {
		t.Fatal("macOS fallback changed")
	}
}
