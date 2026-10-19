//go:build linux

package systemlocale

import (
	"testing"

	"golang.org/x/text/language"
)

func TestPreferredLanguagesUsesPlatformPriority(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
		want language.Tag
	}{
		{name: "LC_ALL wins", env: map[string]string{"LC_ALL": "zh_CN.UTF-8", "LC_MESSAGES": "en_US.UTF-8", "LANG": "en_GB.UTF-8"}, want: language.MustParse("zh-CN")},
		{name: "LC_MESSAGES wins without LC_ALL", env: map[string]string{"LC_MESSAGES": "en_US.UTF-8", "LANG": "zh_CN.UTF-8"}, want: language.MustParse("en-US")},
		{name: "LANG is final fallback", env: map[string]string{"LANG": "zh_CN.UTF-8"}, want: language.MustParse("zh-CN")},
	} {
		t.Run(test.name, func(t *testing.T) {
			tags := preferredLanguagesFromEnvironment(func(key string) (string, bool) {
				value, ok := test.env[key]
				return value, ok
			})
			if len(tags) != 1 || tags[0] != test.want {
				t.Fatalf("preferred languages = %v, want [%v]", tags, test.want)
			}
		})
	}
}

func TestPreferredLanguagesUnsupportedHighestPriorityDoesNotLeakLowerVariable(t *testing.T) {
	tags := preferredLanguagesFromEnvironment(func(key string) (string, bool) {
		switch key {
		case "LC_ALL":
			return "C", true
		case "LANG":
			return "zh_CN.UTF-8", true
		default:
			return "", false
		}
	})
	if len(tags) != 0 {
		t.Fatalf("LC_ALL=C must suppress lower-priority LANG, got %v", tags)
	}
}
